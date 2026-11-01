package alerts

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

const (
	defaultEvaluationInterval = 30 * time.Second
	maxConcurrentRuleEvaluations = 8
	maxRulesPerEvaluation = 500
)

type RuleRepository interface {
	WithAlertEvaluationLease(context.Context, func(context.Context) error) (bool, error)
	ListEnabledAlertRulesForEvaluation(context.Context, int) ([]store.AlertRule, error)
	ApplyAlertRuleObservation(context.Context, store.AlertRuleObservation) (store.AlertRuleState, error)
}

type RequestRateSource interface {
	WorkspaceRequestRate(context.Context, string, string, int) (float64, bool, error)
}

type Evaluator struct {
	repository RuleRepository
	metrics RequestRateSource
	logger *slog.Logger
	interval time.Duration
	now func() time.Time
}

func NewEvaluator(repository RuleRepository, metrics RequestRateSource, logger *slog.Logger, interval time.Duration) *Evaluator {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = defaultEvaluationInterval
	}
	return &Evaluator{
		repository: repository,
		metrics: metrics,
		logger: logger,
		interval: interval,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// Run evaluates once at startup and then on the configured interval. The
// repository's PostgreSQL advisory lease ensures only one replica runs a tick.
func (e *Evaluator) Run(ctx context.Context) {
	if e == nil || e.repository == nil || e.metrics == nil {
		return
	}
	if err := e.EvaluateOnce(ctx); err != nil && ctx.Err() == nil {
		e.logger.Error("alert evaluation cycle failed", "error", err)
	}
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.EvaluateOnce(ctx); err != nil && ctx.Err() == nil {
				e.logger.Error("alert evaluation cycle failed", "error", err)
			}
		}
	}
}

func (e *Evaluator) EvaluateOnce(ctx context.Context) error {
	if e == nil || e.repository == nil || e.metrics == nil {
		return errors.New("alert evaluator dependencies are not configured")
	}
	acquired, err := e.repository.WithAlertEvaluationLease(ctx, func(leaseCtx context.Context) error {
		rules, err := e.repository.ListEnabledAlertRulesForEvaluation(leaseCtx, maxRulesPerEvaluation)
		if err != nil {
			return err
		}
		return e.evaluateRules(leaseCtx, rules)
	})
	if err != nil {
		return err
	}
	if !acquired {
		e.logger.Debug("alert evaluation skipped because another replica holds the lease")
	}
	return nil
}

func (e *Evaluator) evaluateRules(ctx context.Context, rules []store.AlertRule) error {
	if len(rules) == 0 {
		return nil
	}
	semaphore := make(chan struct{}, maxConcurrentRuleEvaluations)
	var workers sync.WaitGroup
	var firstErr error
	var errorOnce sync.Once
	for _, rule := range rules {
		rule := rule
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-semaphore }()
			if err := e.evaluateRule(ctx, rule); err != nil {
				e.logger.Error(
					"alert rule evaluation failed",
					"rule_id", rule.ID,
					"workspace_id", rule.WorkspaceID,
					"error", err,
				)
				errorOnce.Do(func() { firstErr = err })
			}
		}()
	}
	workers.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return firstErr
}

func (e *Evaluator) evaluateRule(ctx context.Context, rule store.AlertRule) error {
	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	value, available, err := e.metrics.WorkspaceRequestRate(
		queryCtx, rule.WorkspaceID, rule.ServiceName, rule.WindowSeconds,
	)
	cancel()
	if err != nil {
		return err
	}
	var observation *float64
	if available {
		observation = &value
	}
	_, err = e.repository.ApplyAlertRuleObservation(ctx, store.AlertRuleObservation{
		RuleID: rule.ID,
		Value: observation,
		ObservedAt: e.now().UTC(),
	})
	if errors.Is(err, store.ErrAlertRuleDisabled) || errors.Is(err, store.ErrAlertRuleNotFound) {
		// The rule may have been disabled or deleted after the evaluator listed it.
		return nil
	}
	return err
}
