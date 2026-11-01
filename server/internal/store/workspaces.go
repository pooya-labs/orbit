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
