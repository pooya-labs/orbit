package alerts

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

const maxRulesPerTick = 1000

type MetricsSource interface {
	WorkspaceRequestRates(context.Context, string, string, int) (map[string]float64, error)
}

type evaluationGroup struct {
	workspaceID string
	windowSeconds int
}

type WorkerObserver interface {
	StartWorker(context.Context, string) (context.Context, func(string, int64, error))
}

type Worker struct {
	repository store.AlertEvaluationRepository
	metrics MetricsSource
	logger *slog.Logger
	interval time.Duration
	now func() time.Time
	observer WorkerObserver
}

func NewWorker(repository store.AlertEvaluationRepository, metrics MetricsSource, logger *slog.Logger, interval time.Duration, observers ...WorkerObserver) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	var observer WorkerObserver
	if len(observers) > 0 {
		observer = observers[0]
	}
	return &Worker{
		repository: repository,
		metrics: metrics,
		logger: logger,
		interval: interval,
		now: time.Now,
		observer: observer,
	}
}

func (w *Worker) Run(ctx context.Context) {
	if w == nil || w.repository == nil || w.metrics == nil {
		return
	}
	w.evaluate(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.evaluate(ctx)
		}
	}
}

func (w *Worker) evaluate(parent context.Context) {
	ctx := parent
	finish := func(string, int64, error) {}
	if w.observer != nil {
		ctx, finish = w.observer.StartWorker(parent, "alert_evaluator")
	}
	outcome, items, err := w.evaluateOnce(ctx)
	finish(outcome, items, err)
}

func (w *Worker) evaluateOnce(ctx context.Context) (string, int64, error) {
	release, acquired, err := w.repository.TryAcquireAlertEvaluationLock(ctx)
	if err != nil {
		w.logger.Error("could not acquire distributed alert evaluator lock", "error", err)
		return "error", 0, err
	}
	if !acquired {
		return "skipped", 0, nil
	}
	defer release()

	rules, err := w.repository.ListEnabledWorkspaceAlertRules(ctx, maxRulesPerTick)
	if err != nil {
		w.logger.Error("could not list enabled alert rules", "error", err)
		return "error", 0, err
	}
	if len(rules) == 0 {
		return "ok", 0, nil
	}

	groups := make(map[evaluationGroup][]store.AlertRule)
	for _, rule := range rules {
		key := evaluationGroup{workspaceID: rule.WorkspaceID, windowSeconds: rule.WindowSeconds}
		groups[key] = append(groups[key], rule)
	}
	keys := make([]evaluationGroup, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].workspaceID == keys[j].workspaceID {
			return keys[i].windowSeconds < keys[j].windowSeconds
		}
		return keys[i].workspaceID < keys[j].workspaceID
	})

	evaluatedAt := w.now().UTC()
	var items int64
	var firstErr error
	for _, key := range keys {
		samples, queryErr := w.metrics.WorkspaceRequestRates(ctx, key.workspaceID, "", key.windowSeconds)
		if queryErr != nil {
			// Missing telemetry is not evidence of recovery; preserve existing alert state.
			w.logger.Warn("alert rule metrics query failed; existing state left unchanged",
				"workspace_id", key.workspaceID, "window_seconds", key.windowSeconds, "error", queryErr)
			if firstErr == nil {
				firstErr = queryErr
			}
			continue
		}
		serviceNames := make([]string, 0, len(samples))
		for serviceName := range samples {
			serviceNames = append(serviceNames, serviceName)
		}
		sort.Strings(serviceNames)

		for _, rule := range groups[key] {
			if rule.ServiceName != "" {
				value, exists := samples[rule.ServiceName]
				if !exists {
					continue
				}
				items++
				if applyErr := w.applySample(ctx, rule, rule.ServiceName, value, evaluatedAt); applyErr != nil && firstErr == nil {
					firstErr = applyErr
				}
				continue
			}
			for _, serviceName := range serviceNames {
				items++
				if applyErr := w.applySample(ctx, rule, serviceName, samples[serviceName], evaluatedAt); applyErr != nil && firstErr == nil {
					firstErr = applyErr
				}
			}
		}
	}
	if firstErr != nil {
		return "error", items, firstErr
	}
	return "ok", items, nil
}

func (w *Worker) applySample(ctx context.Context, rule store.AlertRule, serviceName string, value float64, evaluatedAt time.Time) error {
	instance, changed, err := w.repository.ApplyWorkspaceAlertSample(
		ctx, rule.WorkspaceID, rule.ID, serviceName, value, evaluatedAt,
	)
	if err != nil {
		w.logger.Error("could not persist alert evaluation",
			"workspace_id", rule.WorkspaceID, "alert_rule_id", rule.ID,
			"service_name", serviceName, "error", err)
		return err
	}
	if changed {
		w.logger.Info("alert instance state changed",
			"workspace_id", instance.WorkspaceID,
			"alert_rule_id", instance.AlertRuleID,
			"alert_instance_id", instance.ID,
			"service_name", instance.ServiceName,
			"status", instance.Status,
			"value", instance.Value,
			"severity", instance.Severity)
	}
	return nil
}
