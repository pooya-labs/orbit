package store

import (
	"context"
	"time"
)

type AlertInstance struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	AlertRuleID string `json:"alert_rule_id"`
	RuleName string `json:"rule_name"`
	ServiceName string `json:"service_name"`
	Metric string `json:"metric"`
	Operator string `json:"operator"`
	Threshold float64 `json:"threshold"`
	WindowSeconds int `json:"window_seconds"`
	PendingSeconds int `json:"pending_seconds"`
	Severity string `json:"severity"`
	Status string `json:"status"`
	Value float64 `json:"value"`
	PendingSince time.Time `json:"pending_since"`
	FiredAt *time.Time `json:"fired_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	LastEvaluatedAt time.Time `json:"last_evaluated_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AlertInstanceEvent struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	AlertInstanceID string `json:"alert_instance_id"`
	AlertRuleID string `json:"alert_rule_id"`
	EventType string `json:"event_type"`
	Details map[string]any `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

// AlertEvaluationRepository provides the durable state machine used by the
// background evaluator. Implementations coordinate workers across replicas.
type AlertEvaluationRepository interface {
	TryAcquireAlertEvaluationLock(context.Context) (release func(), acquired bool, err error)
	ListEnabledWorkspaceAlertRules(context.Context, int) ([]AlertRule, error)
	ApplyWorkspaceAlertSample(context.Context, string, string, string, float64, time.Time) (AlertInstance, bool, error)
	ListWorkspaceAlertInstances(context.Context, string, string, string, string, int) ([]AlertInstance, error)
}
