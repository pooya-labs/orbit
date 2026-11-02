package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSLONotFound = errors.New("service level objective not found")
	ErrSLOForbidden = errors.New("workspace role cannot manage service level objectives")
	ErrSLOInvalidInput = errors.New("invalid service level objective input")
	ErrSLONameConflict = errors.New("SLO name already exists for this service")
	ErrSLONoChange = errors.New("SLO already has requested state")
	ErrSLODisabled = errors.New("service level objective is disabled")
)

type ServiceLevelObjective struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	ServiceID string `json:"service_id"`
	ServiceName string `json:"service_name"`
	Environment string `json:"environment"`
	Name string `json:"name"`
	Route string `json:"route"`
	Target float64 `json:"target"`
	RollingWindowDays int `json:"rolling_window_days"`
	Enabled bool `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateWorkspaceSLOInput struct {
	WorkspaceID string
	ServiceID string
	Name string
	Route string
	Target float64
	RollingWindowDays int
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type UpdateWorkspaceSLOInput struct {
	WorkspaceID string
	SLOID string
	Name string
	Route string
	Target float64
	RollingWindowDays int
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type SetWorkspaceSLOEnabledInput struct {
	WorkspaceID string
	SLOID string
	Enabled bool
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type DeleteWorkspaceSLOInput struct {
	WorkspaceID string
	SLOID string
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type WorkspaceSLORepository interface {
	CreateWorkspaceSLO(context.Context, CreateWorkspaceSLOInput) (ServiceLevelObjective, error)
	ListWorkspaceSLOs(context.Context, string, string, string, string, int) ([]ServiceLevelObjective, error)
	GetWorkspaceSLO(context.Context, string, string, string, string) (ServiceLevelObjective, error)
	UpdateWorkspaceSLO(context.Context, UpdateWorkspaceSLOInput) (ServiceLevelObjective, error)
	SetWorkspaceSLOEnabled(context.Context, SetWorkspaceSLOEnabledInput) (ServiceLevelObjective, error)
	DeleteWorkspaceSLO(context.Context, DeleteWorkspaceSLOInput) error
}
