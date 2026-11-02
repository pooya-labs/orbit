package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrWorkspaceNotFound = errors.New("workspace not found")
	ErrWorkspaceMembershipNotFound = errors.New("workspace membership not found")
)

type Workspace struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	CreatedByIssuer string `json:"-"`
	CreatedBySubject string `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WorkspaceMembership struct {
	WorkspaceID string `json:"workspace_id"`
	UserIssuer string `json:"-"`
	UserSubject string `json:"-"`
	Role string `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WorkspaceAuditEvent struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	ActorRef string `json:"actor_ref"`
	ActorRole string `json:"actor_role"`
	ActorIssuer string `json:"-"`
	ActorSubject string `json:"-"`
	Action string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID string `json:"resource_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Details map[string]any `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateWorkspaceInput struct {
	Name string
	Slug string
	OwnerIssuer string
	OwnerSubject string
}

type WorkspaceRepository interface {
	CreateWorkspace(ctx context.Context, input CreateWorkspaceInput) (Workspace, error)
	ListWorkspacesForPrincipal(ctx context.Context, issuer, subject string) ([]Workspace, error)
	GetWorkspaceMembership(ctx context.Context, workspaceID, issuer, subject string) (WorkspaceMembership, error)
	AppendWorkspaceAuditEvent(ctx context.Context, event WorkspaceAuditEvent) (WorkspaceAuditEvent, error)
	ListWorkspaceAuditEvents(ctx context.Context, workspaceID string, limit int) ([]WorkspaceAuditEvent, error)
}


var (
	ErrWorkloadIdentityNotFound = errors.New("workload identity not found")
	ErrWorkloadAuthorizationDenied = errors.New("workload telemetry authorization denied")
)

type WorkloadIdentity struct {
	ID string `json:"id"`
	Issuer string `json:"-"`
	Subject string `json:"-"`
	DisplayName string `json:"display_name"`
	Status string `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type WorkloadIdentityGrant struct {
	IdentityID string
	Issuer string
	Subject string
	DisplayName string
	Status string
	ExpiresAt *time.Time
	WorkspaceID string
	Signal string
	GrantedAt time.Time
}

type CreateWorkloadIdentityInput struct {
	Issuer string
	Subject string
	DisplayName string
	ExpiresAt *time.Time
	ActorIssuer string
	ActorSubject string
}

type WorkloadGrantInput struct {
	IdentityID string
	WorkspaceID string
	Signal string
	ActorIssuer string
	ActorSubject string
}

// WorkloadIdentityRepository manages machine identities independently from
// human workspace memberships. Implementations must fail closed on DB errors.
type WorkloadIdentityRepository interface {
	CreateWorkloadIdentity(ctx context.Context, input CreateWorkloadIdentityInput) (WorkloadIdentity, error)
	SetWorkloadWorkspaceGrant(ctx context.Context, input WorkloadGrantInput, granted bool) error
	AuthorizeWorkloadTelemetry(ctx context.Context, issuer, subject, workspaceID, signal string) (WorkloadIdentity, error)
	SetWorkloadIdentityStatus(ctx context.Context, identityID, actorIssuer, actorSubject, status string) error
	ListWorkloadIdentities(ctx context.Context, limit int) ([]WorkloadIdentity, error)
	ListWorkspaceWorkloadGrants(ctx context.Context, workspaceID string) ([]WorkloadIdentityGrant, error)
}

type WorkloadResourceAttributeRepository interface {
	RecordWorkloadResourceAttributes(ctx context.Context, workspaceID string, observations []WorkloadResourceObservation) error
}

type WorkspaceDataRepository interface {
	ListWorkspaceServices(ctx context.Context, workspaceID string) ([]Service, error)
	ListWorkspaceIncidents(ctx context.Context, workspaceID, status string) ([]Incident, error)
	ListWorkspaceTelemetryServices(ctx context.Context, workspaceID string, limit int) ([]TelemetryService, error)
}
