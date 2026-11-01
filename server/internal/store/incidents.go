package store

import (
	"context"
	"errors"
)

var (
	ErrIncidentNotFound = errors.New("incident not found")
	ErrIncidentServiceNotFound = errors.New("workspace service not found")
	ErrIncidentMutationForbidden = errors.New("workspace role cannot mutate incidents")
	ErrIncidentInvalidTransition = errors.New("incident status transition is not allowed")
	ErrIncidentNoChange = errors.New("incident is already in the requested status")
)

type IncidentTimelineEvent struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	IncidentID string `json:"incident_id"`
	ActorIssuer string `json:"-"`
	ActorSubject string `json:"-"`
	EventType string `json:"event_type"`
	FromStatus string `json:"from_status,omitempty"`
	ToStatus string `json:"to_status,omitempty"`
	Comment string `json:"comment,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateWorkspaceIncidentInput struct {
	WorkspaceID string
	Title string
	Description string
	ServiceID string
	Severity string
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type TransitionWorkspaceIncidentInput struct {
	WorkspaceID string
	IncidentID string
	ToStatus string
	Comment string
	ActorIssuer string
	ActorSubject string
	RequestID string
}

// IncidentLifecycleRepository owns transactional, workspace-scoped incident
// mutations and the immutable event timeline that accompanies every change.
type IncidentLifecycleRepository interface {
	CreateWorkspaceIncident(context.Context, CreateWorkspaceIncidentInput) (Incident, error)
	TransitionWorkspaceIncident(context.Context, TransitionWorkspaceIncidentInput) (Incident, error)
	ListWorkspaceIncidentTimeline(context.Context, string, string, string, string, int) ([]IncidentTimelineEvent, error)
}
