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

func TestWorkspaceSLOCRUDIsScopedAndAudited(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL repository: %v", err)
	}
	t.Cleanup(repository.Close)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	issuer := "https://slo-tests.example/"
	owner := "slo-owner-" + suffix
	viewer := "slo-viewer-" + suffix
	workspaceA, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "SLO workspace " + suffix,
		Slug: "slo-a-" + suffix,
		OwnerIssuer: issuer,
		OwnerSubject: owner,
	})
	if err != nil {
		t.Fatalf("create workspace A: %v", err)
	}
	workspaceB, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Other SLO workspace " + suffix,
		Slug: "slo-b-" + suffix,
		OwnerIssuer: issuer,
		OwnerSubject: "other-owner-" + suffix,
	})
	if err != nil {
		t.Fatalf("create workspace B: %v", err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO workspace_memberships (workspace_id, user_issuer, user_subject, role)
		VALUES ($1::uuid, $2, $3, 'viewer')
	`, workspaceA.ID, issuer, viewer); err != nil {
		t.Fatalf("add read-only workspace member: %v", err)
	}

	createService := func(workspaceID, name, environment string) string {
		t.Helper()
		var id string
		err := repository.pool.QueryRow(ctx, `
			INSERT INTO services (
				workspace_id, name, description, environment, region, status, source
			) VALUES ($1::uuid, $2, 'SLO integration fixture', $3, 'test-region', 'operational', 'integration_test')
			RETURNING id::text
		`, workspaceID, name, environment).Scan(&id)
		if err != nil {
			t.Fatalf("create SLO service %s: %v", name, err)
		}
		return id
	}
	serviceA := createService(workspaceA.ID, "slo-checkout-"+suffix, "staging")
	serviceB := createService(workspaceB.ID, "slo-other-"+suffix, "production")

	created, err := repository.CreateWorkspaceSLO(ctx, store.CreateWorkspaceSLOInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceA,
		Name: "Checkout availability",
		Route: "/emit",
		Target: 0.99,
		RollingWindowDays: 7,
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-slo-create-" + suffix,
	})
	if err != nil {
		t.Fatalf("create SLO: %v", err)
	}
	if created.ID == "" || created.ServiceID != serviceA || created.ServiceName != "slo-checkout-"+suffix ||
		created.Environment != "staging" || created.Target != 0.99 || created.Route != "/emit" || !created.Enabled {
		t.Fatalf("unexpected created SLO: %+v", created)
	}

	listed, err := repository.ListWorkspaceSLOs(ctx, workspaceA.ID, issuer, viewer, serviceA, 50)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("list SLOs as workspace viewer = %+v err=%v", listed, err)
	}
	if _, err := repository.GetWorkspaceSLO(ctx, workspaceA.ID, created.ID, issuer, "not-a-member-"+suffix); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member SLO read error = %v, want membership not found", err)
	}
	if _, err := repository.CreateWorkspaceSLO(ctx, store.CreateWorkspaceSLOInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceA,
		Name: "Forbidden viewer SLO",
		Route: "/emit",
		Target: 0.99,
		RollingWindowDays: 7,
		ActorIssuer: issuer,
		ActorSubject: viewer,
	}); !errors.Is(err, store.ErrSLOForbidden) {
		t.Fatalf("viewer create error = %v, want ErrSLOForbidden", err)
	}
	if _, err := repository.CreateWorkspaceSLO(ctx, store.CreateWorkspaceSLOInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceB,
		Name: "Cross-workspace SLO",
		Route: "/emit",
		Target: 0.99,
		RollingWindowDays: 7,
		ActorIssuer: issuer,
		ActorSubject: owner,
	}); !errors.Is(err, store.ErrSLOServiceNotFound) {
		t.Fatalf("cross-workspace service error = %v, want ErrSLOServiceNotFound", err)
	}
	if _, err := repository.CreateWorkspaceSLO(ctx, store.CreateWorkspaceSLOInput{
		WorkspaceID: workspaceA.ID,
		ServiceID: serviceA,
		Name: "Checkout availability",
		Route: "/emit",
		Target: 0.995,
		RollingWindowDays: 7,
		ActorIssuer: issuer,
		ActorSubject: owner,
	}); !errors.Is(err, store.ErrSLONameConflict) {
		t.Fatalf("duplicate SLO name error = %v, want ErrSLONameConflict", err)
	}

	updated, err := repository.UpdateWorkspaceSLO(ctx, store.UpdateWorkspaceSLOInput{
		WorkspaceID: workspaceA.ID,
		SLOID: created.ID,
		Name: "Checkout API success rate",
		Route: "/emit",
		Target: 0.995,
		RollingWindowDays: 3,
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-slo-update-" + suffix,
	})
	if err != nil || updated.Name != "Checkout API success rate" || updated.Target != 0.995 || updated.RollingWindowDays != 3 {
		t.Fatalf("update SLO = %+v err=%v", updated, err)
	}

	disabled, err := repository.SetWorkspaceSLOEnabled(ctx, store.SetWorkspaceSLOEnabledInput{
		WorkspaceID: workspaceA.ID,
		SLOID: created.ID,
		Enabled: false,
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-slo-disable-" + suffix,
	})
	if err != nil || disabled.Enabled {
		t.Fatalf("disable SLO = %+v err=%v", disabled, err)
	}
	if _, err := repository.SetWorkspaceSLOEnabled(ctx, store.SetWorkspaceSLOEnabledInput{
		WorkspaceID: workspaceA.ID,
		SLOID: created.ID,
		Enabled: false,
		ActorIssuer: issuer,
		ActorSubject: owner,
	}); !errors.Is(err, store.ErrSLONoChange) {
		t.Fatalf("repeat disable error = %v, want ErrSLONoChange", err)
	}
	if _, err := repository.SetWorkspaceSLOEnabled(ctx, store.SetWorkspaceSLOEnabledInput{
		WorkspaceID: workspaceA.ID,
		SLOID: created.ID,
		Enabled: true,
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-slo-enable-" + suffix,
	}); err != nil {
		t.Fatalf("re-enable SLO: %v", err)
	}

	for _, test := range []struct {
		name string
		target float64
		route string
		window int
	}{
		{name: "target below supported minimum", target: 0.89, route: "/emit", window: 7},
		{name: "target is 100 percent", target: 1, route: "/emit", window: 7},
		{name: "route is not absolute", target: 0.99, route: "emit", window: 7},
		{name: "route has repeated slash", target: 0.99, route: "/api//emit", window: 7},
		{name: "window exceeds Prometheus retention", target: 0.99, route: "/emit", window: 30},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := repository.CreateWorkspaceSLO(ctx, store.CreateWorkspaceSLOInput{
				WorkspaceID: workspaceA.ID,
				ServiceID: serviceA,
				Name: "Invalid " + test.name,
				Route: test.route,
				Target: test.target,
				RollingWindowDays: test.window,
				ActorIssuer: issuer,
				ActorSubject: owner,
			})
			if !errors.Is(err, store.ErrSLOInvalidInput) {
				t.Fatalf("invalid SLO error = %v, want ErrSLOInvalidInput", err)
			}
		})
	}

	if err := repository.DeleteWorkspaceSLO(ctx, store.DeleteWorkspaceSLOInput{
		WorkspaceID: workspaceA.ID,
		SLOID: created.ID,
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-slo-delete-" + suffix,
	}); err != nil {
		t.Fatalf("delete SLO: %v", err)
	}
	if _, err := repository.GetWorkspaceSLO(ctx, workspaceA.ID, created.ID, issuer, owner); !errors.Is(err, store.ErrSLONotFound) {
		t.Fatalf("read deleted SLO error = %v, want ErrSLONotFound", err)
	}

	var auditCount int
	if err := repository.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM workspace_audit_events
		WHERE workspace_id = $1::uuid
		  AND resource_type = 'slo_definition'
		  AND action LIKE 'slo.%'
	`, workspaceA.ID).Scan(&auditCount); err != nil {
		t.Fatalf("count SLO audit events: %v", err)
	}
	if auditCount != 5 {
		t.Fatalf("SLO audit event count = %d, want 5", auditCount)
	}
}
