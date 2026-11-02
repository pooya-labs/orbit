package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

func TestWorkspaceDeploymentAnnotationsAreScopedAuditedAndAppendOnly(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL repository: %v", err)
	}
	t.Cleanup(repository.Close)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	issuer := "https://deployment-tests.example/"
	actor := "deployment-owner-" + suffix
	workspaceA, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Deployment workspace " + suffix,
		Slug: "deploy-a-" + suffix,
		OwnerIssuer: issuer,
		OwnerSubject: actor,
	})
	if err != nil {
		t.Fatalf("create workspace A: %v", err)
	}
	workspaceB, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Other deployment workspace " + suffix,
		Slug: "deploy-b-" + suffix,
		OwnerIssuer: issuer,
		OwnerSubject: "other-owner-" + suffix,
	})
	if err != nil {
		t.Fatalf("create workspace B: %v", err)
	}

	createService := func(workspaceID, name, environment string) string {
		t.Helper()
		var serviceID string
		err := repository.pool.QueryRow(ctx, `
			INSERT INTO services (
				workspace_id, name, description, environment, region,
				status, source
			) VALUES ($1::uuid, $2, 'deployment annotation test fixture', $3,
			          'test-region', 'operational', 'integration_test')
			RETURNING id::text
		`, workspaceID, name, environment).Scan(&serviceID)
		if err != nil {
			t.Fatalf("create service %s: %v", name, err)
		}
		return serviceID
	}
	serviceA := createService(workspaceA.ID, "deployment-service-a-"+suffix, "staging")
	serviceB := createService(workspaceB.ID, "deployment-service-b-"+suffix, "production")

	firstAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Second)
	first, err := repository.CreateWorkspaceDeploymentAnnotation(ctx, store.CreateWorkspaceDeploymentAnnotationInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceA,
		Version: "v1.2.0",
		CommitSHA: "abcdef1234567890",
		ChangeReference: "ORBIT-123",
		ReleaseNotes: "First staging rollout.",
		DeployedAt: firstAt,
		ActorIssuer: issuer,
		ActorSubject: actor,
		RequestID: "req-deployment-first-" + suffix,
	})
	if err != nil {
		t.Fatalf("create first deployment annotation: %v", err)
	}
	if first.ID == "" || first.ServiceID != serviceA || first.Environment != "staging" ||
		first.Version != "v1.2.0" || first.CommitSHA != "abcdef1234567890" || !first.DeployedAt.Equal(firstAt) {
		t.Fatalf("unexpected first deployment annotation: %+v", first)
	}

	second, err := repository.CreateWorkspaceDeploymentAnnotation(ctx, store.CreateWorkspaceDeploymentAnnotationInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceA,
		Version: "v1.3.0",
		CommitSHA: "1234567890abcdef",
		ChangeReference: "https://example.com/change/124",
		ReleaseNotes: "Second staging rollout.",
		DeployedAt: firstAt.Add(5 * time.Minute),
		ActorIssuer: issuer,
		ActorSubject: actor,
		RequestID: "req-deployment-second-" + suffix,
	})
	if err != nil {
		t.Fatalf("create second deployment annotation: %v", err)
	}

	items, err := repository.ListWorkspaceDeploymentAnnotations(ctx, workspaceA.ID, issuer, actor, serviceA, 50)
	if err != nil || len(items) != 2 || items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("list deployment annotations = %+v err=%v", items, err)
	}
	if _, err := repository.GetWorkspaceDeploymentAnnotation(ctx, workspaceA.ID, first.ID, issuer, "not-a-member-"+suffix); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member read error = %v, want membership not found", err)
	}
	if _, err := repository.CreateWorkspaceDeploymentAnnotation(ctx, store.CreateWorkspaceDeploymentAnnotationInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceB,
		Version: "v9.9.9",
		ActorIssuer: issuer,
		ActorSubject: actor,
	}); !errors.Is(err, store.ErrDeploymentAnnotationServiceNotFound) {
		t.Fatalf("cross-workspace service annotation error = %v, want service not found", err)
	}
	if _, err := repository.CreateWorkspaceDeploymentAnnotation(ctx, store.CreateWorkspaceDeploymentAnnotationInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceA,
		Version: "v9.9.9",
		ActorIssuer: issuer,
		ActorSubject: "not-a-member-" + suffix,
	}); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member create error = %v, want membership not found", err)
	}
	if _, err := repository.CreateWorkspaceDeploymentAnnotation(ctx, store.CreateWorkspaceDeploymentAnnotationInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceA,
		Version: "invalid version with spaces",
		ActorIssuer: issuer,
		ActorSubject: actor,
	}); !errors.Is(err, store.ErrDeploymentAnnotationInvalidInput) {
		t.Fatalf("invalid version error = %v, want invalid input", err)
	}
	if _, err := repository.CreateWorkspaceDeploymentAnnotation(ctx, store.CreateWorkspaceDeploymentAnnotationInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceA,
		Version: "v1.4.0",
		DeployedAt: time.Now().UTC().Add(10 * time.Minute),
		ActorIssuer: issuer,
		ActorSubject: actor,
	}); !errors.Is(err, store.ErrDeploymentAnnotationInvalidInput) {
		t.Fatalf("future deployment time error = %v, want invalid input", err)
	}

	var auditCount int
	if err := repository.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM workspace_audit_events
		WHERE workspace_id = $1::uuid
		  AND resource_type = 'deployment_annotation'
		  AND action = 'deployment.annotation_created'
	`, workspaceA.ID).Scan(&auditCount); err != nil {
		t.Fatalf("count deployment annotation audit events: %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("deployment annotation audit count = %d, want 2", auditCount)
	}

	if _, err := repository.pool.Exec(ctx, `
		UPDATE deployment_annotations SET version = 'tampered'
		WHERE id = $1::uuid
	`, first.ID); err == nil {
		t.Fatal("deployment annotation table allowed UPDATE")
	}
	if _, err := repository.pool.Exec(ctx, `
		DELETE FROM deployment_annotations WHERE id = $1::uuid
	`, first.ID); err == nil {
		t.Fatal("deployment annotation table allowed DELETE")
	}
	if _, err := repository.pool.Exec(ctx, "TRUNCATE deployment_annotations"); err == nil {
		t.Fatal("deployment annotation table allowed TRUNCATE")
	}
}
