package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDeploymentAnnotationNotFound = errors.New("deployment annotation not found")
	ErrDeploymentAnnotationServiceNotFound = errors.New("workspace service not found")
	ErrDeploymentAnnotationForbidden = errors.New("workspace role cannot manage deployment annotations")
	ErrDeploymentAnnotationInvalidInput = errors.New("invalid deployment annotation input")
	ErrDeploymentAnnotationComparisonInvalid = errors.New("deployment annotations cannot be compared")
)

type DeploymentAnnotation struct {
	ID string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	ServiceID string `json:"service_id"`
	ServiceName string `json:"service_name"`
	Environment string `json:"environment"`
	Version string `json:"version"`
	CommitSHA string `json:"commit_sha,omitempty"`
	ChangeReference string `json:"change_reference,omitempty"`
	ReleaseNotes string `json:"release_notes,omitempty"`
	DeployedAt time.Time `json:"deployed_at"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateWorkspaceDeploymentAnnotationInput struct {
	WorkspaceID string
	ServiceID string
	Version string
	CommitSHA string
	ChangeReference string
	ReleaseNotes string
	DeployedAt time.Time
	ActorIssuer string
	ActorSubject string
	RequestID string
}

type DeploymentAnnotationRepository interface {
	CreateWorkspaceDeploymentAnnotation(context.Context, CreateWorkspaceDeploymentAnnotationInput) (DeploymentAnnotation, error)
	ListWorkspaceDeploymentAnnotations(context.Context, string, string, string, string, int) ([]DeploymentAnnotation, error)
	GetWorkspaceDeploymentAnnotation(context.Context, string, string, string, string) (DeploymentAnnotation, error)
}
