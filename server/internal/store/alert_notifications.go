package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrAlertNotificationChannelNotFound = errors.New("alert notification channel not found")
	ErrAlertNotificationChannelConflict = errors.New("alert notification channel name already exists")
	ErrAlertNotificationInvalidInput = errors.New("invalid alert notification input")
	ErrAlertNotificationNoChange = errors.New("alert notification channel already has requested state")
)

type AlertNotificationChannel struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name string `json:"name"`
	Adapter string `json:"adapter"`
	Enabled bool `json:"enabled"`
	SecretConfigured bool `json:"secret_configured"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	SecretRef string `json:"-"`
}

type CreateAlertNotificationChannelInput struct {
	WorkspaceID string
	Name string
	SecretRef string
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type SetAlertNotificationChannelEnabledInput struct {
	WorkspaceID string
	ChannelID string
	Enabled bool
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type AlertNotificationAttempt struct {
	ID string `json:"id"`
	DeliveryID string `json:"delivery_id"`
	AttemptNumber int `json:"attempt_number"`
	Outcome string `json:"outcome"`
	HTTPStatus *int `json:"http_status,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	AttemptedAt time.Time `json:"attempted_at"`
}

type AlertNotificationDelivery struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	AlertInstanceID string `json:"alert_instance_id"`
	AlertInstanceEventID string `json:"event_id"`
	ChannelID string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	EventType string `json:"event_type"`
	Status string `json:"status"`
	Attempts int `json:"attempts"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	RuleName string `json:"rule_name"`
	ServiceName string `json:"service_name"`
	Metric string `json:"metric"`
	Operator string `json:"operator"`
	Threshold float64 `json:"threshold"`
	Value float64 `json:"value"`
	Severity string `json:"severity"`
	InstanceStatus string `json:"instance_status"`
	OccurredAt time.Time `json:"occurred_at"`

	SecretRef string `json:"-"`
}

type RecordAlertNotificationAttemptInput struct {
	DeliveryID string
	AttemptNumber int
	Outcome string
	HTTPStatus *int
	ErrorCode string
	NextAttemptAt time.Time
}

type AlertNotificationChannelRepository interface {
	CreateWorkspaceAlertNotificationChannel(context.Context, CreateAlertNotificationChannelInput) (AlertNotificationChannel, error)
	ListWorkspaceAlertNotificationChannels(context.Context, string, string, string, int) ([]AlertNotificationChannel, error)
	SetWorkspaceAlertNotificationChannelEnabled(context.Context, SetAlertNotificationChannelEnabledInput) (AlertNotificationChannel, error)
	ListWorkspaceAlertNotificationDeliveries(context.Context, string, string, string, string, int) ([]AlertNotificationDelivery, error)
	ListWorkspaceAlertNotificationAttempts(context.Context, string, string, string, string, int) ([]AlertNotificationAttempt, error)
}

type AlertNotificationDeliveryRepository interface {
	ClaimAlertNotificationDeliveries(context.Context, int, int) ([]AlertNotificationDelivery, error)
	RecordAlertNotificationAttempt(context.Context, RecordAlertNotificationAttemptInput) error
}
