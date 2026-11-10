package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
	"github.com/pooya16pg-spec/orbit/server/internal/ingest"
	"time"
)

func TestLocalWorkBootstrapIsExplicitIdempotentAndWorkspaceScoped(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	defer repository.Close()

	for attempt := 0; attempt < 2; attempt++ {
		if err := repository.BootstrapLocalWork(ctx); err != nil {
			t.Fatalf("Local Work bootstrap attempt %d: %v", attempt+1, err)
		}
	}

	workspaces, err := repository.ListWorkspacesForPrincipal(ctx, "urn:orbit:local-work", "local-owner")
	if err != nil {
		t.Fatalf("list Local Work principal workspaces: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].Slug != "orbit-local-work" {
		t.Fatalf("Local Work workspaces = %#v, want the single orbit-local-work workspace", workspaces)
	}
	workspaceID := workspaces[0].ID

	membership, err := repository.GetWorkspaceMembership(ctx, workspaceID, "urn:orbit:local-work", "local-owner")
	if err != nil || membership.Role != "owner" {
		t.Fatalf("Local Work membership = %#v, error = %v; want owner", membership, err)
	}
	services, err := repository.ListWorkspaceServices(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list Local Work services: %v", err)
	}
	if len(services) != 6 {
		t.Fatalf("Local Work service count = %d, want 6", len(services))
	}
	incidents, err := repository.ListWorkspaceIncidents(ctx, workspaceID, "")
	if err != nil {
		t.Fatalf("list Local Work incidents: %v", err)
	}
	if len(incidents) != 2 {
		t.Fatalf("Local Work incident count = %d, want 2", len(incidents))
	}

	workloadToken := "local-workload-token-0123456789abcdef"
	workloadVerifier, err := auth.NewStaticTokenVerifier(workloadToken, auth.Principal{
		Issuer: "urn:orbit:local-workload", Subject: "orbit-telemetry-demo", Roles: []string{"viewer"},
	})
	if err != nil {
		t.Fatalf("create local workload verifier: %v", err)
	}
	workloadAuthorizer, err := ingest.NewAuthorizer(workloadVerifier, repository)
	if err != nil {
		t.Fatalf("create local workload authorizer: %v", err)
	}
	for _, signal := range []string{"metrics", "logs", "traces"} {
		decision, err := workloadAuthorizer.Authorize(
			ctx, []string{"Bearer " + workloadToken}, workspaceID, signal,
		)
		if err != nil || decision.WorkspaceID != workspaceID || decision.Signal != signal ||
			decision.IdentityID == "" {
			t.Fatalf("Local Work %s token decision = %#v, error = %v", signal, decision, err)
		}
	}

	assertCount := func(table string, want int) {
		t.Helper()
		var got int
		query := "SELECT COUNT(*) FROM " + table + " WHERE workspace_id = $1::uuid"
		if err := repository.pool.QueryRow(ctx, query, workspaceID).Scan(&got); err != nil {
			t.Fatalf("count Local Work rows in %s: %v", table, err)
		}
		if got != want {
			t.Fatalf("Local Work %s count = %d, want %d after two bootstrap runs", table, got, want)
		}
	}
	assertCount("alert_rules", 1)
	assertCount("service_level_objectives", 1)
	assertCount("deployment_annotations", 2)
	assertCount("incident_timeline_events", 2)
	assertCount("workspace_audit_events", 3)
	assertCount("workload_workspace_grants", 3)
	assertCount("workload_identity_audit_events", 3)
}
