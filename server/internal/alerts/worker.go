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

type Worker struct {
	repository store.AlertEvaluationRepository
	metrics MetricsSource
	logger *slog.Logger
	interval time.Duration
	now func() time.Time
}

func NewWorker(repository store.AlertEvaluationRepository, metrics MetricsSource, logger *slog.Logger, interval time.Duration) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Worker{
		repository: repository,
		metrics: metrics,
		logger: logger,
		interval: interval,
		now: time.Now,
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

func (w *Worker) evaluate(ctx context.Context) {
	release, acquired, err := w.repository.TryAcquireAlertEvaluationLock(ctx)
	if err != nil {
		w.logger.Error("could not acquire distributed alert evaluator lock", "error", err)
		return
	}
	if !acquired {
		return
	}
	defer release()

	rules, err := w.repository.ListEnabledWorkspaceAlertRules(ctx, maxRulesPerTick)
	if err != nil {
		w.logger.Error("could not list enabled alert rules", "error", err)
		return
	}
	if len(rules) == 0 {
		return
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
	for _, key := range keys {
		samples, err := w.metrics.WorkspaceRequestRates(ctx, key.workspaceID, "", key.windowSeconds)
		if err != nil {
			// Missing telemetry is not evidence of recovery; preserve existing alert state.
			w.logger.Warn("alert rule metrics query failed; existing state left unchanged",
				"workspace_id", key.workspaceID, "window_seconds", key.windowSeconds, "error", err)
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
				w.applySample(ctx, rule, rule.ServiceName, value, evaluatedAt)
				continue
			}
			for _, serviceName := range serviceNames {
				w.applySample(ctx, rule, serviceName, samples[serviceName], evaluatedAt)
			}
		}
	}
}

func (w *Worker) applySample(ctx context.Context, rule store.AlertRule, serviceName string, value float64, evaluatedAt time.Time) {
	instance, changed, err := w.repository.ApplyWorkspaceAlertSample(
		ctx, rule.WorkspaceID, rule.ID, serviceName, value, evaluatedAt,
	)
	if err != nil {
		w.logger.Error("could not persist alert evaluation",
			"workspace_id", rule.WorkspaceID, "alert_rule_id", rule.ID,
			"service_name", serviceName, "error", err)
		return
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
}
