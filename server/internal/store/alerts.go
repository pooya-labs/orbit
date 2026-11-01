package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrAlertRuleNotFound = errors.New("alert rule not found")
	ErrAlertRuleForbidden = errors.New("workspace role cannot manage alert rules")
	ErrAlertRuleInvalidInput = errors.New("invalid alert rule input")
	ErrAlertRuleNameConflict = errors.New("alert rule name already exists in workspace")
	ErrAlertRuleNoChange = errors.New("alert rule already has requested state")
)

type AlertRule struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name string `json:"name"`
	Description string `json:"description"`
	ServiceName string `json:"service_name,omitempty"`
	Metric string `json:"metric"`
	Operator string `json:"operator"`
	Threshold float64 `json:"threshold"`
	WindowSeconds int `json:"window_seconds"`
	PendingSeconds int `json:"pending_seconds"`
	Severity string `json:"severity"`
	Enabled bool `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateWorkspaceAlertRuleInput struct {
	WorkspaceID string
	Name string
	Description string
	ServiceName string
	Metric string
	Operator string
	Threshold float64
	WindowSeconds int
	PendingSeconds int
	Severity string
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type UpdateWorkspaceAlertRuleInput struct {
	WorkspaceID string
	RuleID string
	Name string
	Description string
	ServiceName string
	Metric string
	Operator string
	Threshold float64
	WindowSeconds int
	PendingSeconds int
	Severity string
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type SetWorkspaceAlertRuleEnabledInput struct {
	WorkspaceID string
	RuleID string
	Enabled bool
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type DeleteWorkspaceAlertRuleInput struct {
	WorkspaceID string
	RuleID string
	ActorIssuer string
	ActorSubject string
	RequestID string
}

// AlertRuleRepository manages workspace-scoped rule definitions. Evaluation
// and notification delivery are separate components and are not implied here.
type AlertRuleRepository interface {
	CreateWorkspaceAlertRule(context.Context, CreateWorkspaceAlertRuleInput) (AlertRule, error)
	ListWorkspaceAlertRules(context.Context, string, string, string, int) ([]AlertRule, error)
	UpdateWorkspaceAlertRule(context.Context, UpdateWorkspaceAlertRuleInput) (AlertRule, error)
	SetWorkspaceAlertRuleEnabled(context.Context, SetWorkspaceAlertRuleEnabledInput) (AlertRule, error)
	DeleteWorkspaceAlertRule(context.Context, DeleteWorkspaceAlertRuleInput) error
}


var ErrAlertRuleDisabled = errors.New("alert rule is disabled")

type AlertRuleState struct {
	RuleID string `json:"rule_id"`
	WorkspaceID string `json:"workspace_id"`
	State string `json:"state"`
	ActiveSince *time.Time `json:"active_since,omitempty"`
	LastValue *float64 `json:"last_value,omitempty"`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at,omitempty"`
	LastFiredAt *time.Time `json:"last_fired_at,omitempty"`
	LastResolvedAt *time.Time `json:"last_resolved_at,omitempty"`
}

type AlertRuleEvent struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	RuleID string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	EventType string `json:"event_type"`
	Severity string `json:"severity"`
	Metric string `json:"metric"`
	Operator string `json:"operator"`
	Threshold float64 `json:"threshold"`
	ObservedValue *float64 `json:"observed_value,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

type AlertRuleObservation struct {
	RuleID string
	Value *float64
	ObservedAt time.Time
}

// AlertRuleEvaluationRepository is used only by the internal scheduler.
// WithAlertEvaluationLease serializes scheduler ticks across API replicas.
type AlertRuleEvaluationRepository interface {
	WithAlertEvaluationLease(context.Context, func(context.Context) error) (bool, error)
	ListEnabledAlertRulesForEvaluation(context.Context, int) ([]AlertRule, error)
	ApplyAlertRuleObservation(context.Context, AlertRuleObservation) (AlertRuleState, error)
	GetWorkspaceAlertRuleState(context.Context, string, string, string, string) (AlertRuleState, error)
	ListWorkspaceAlertRuleEvents(context.Context, string, string, string, string, int) ([]AlertRuleEvent, error)
}
