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

func TestWorkspaceIncidentLifecycleIsTransactionalScopedAndAppendOnly(t *testing.T) {
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
	actorIssuer := "https://incident-tests.example/"
	actorSubject := "incident-owner-" + suffix
	workspaceA, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Incident lifecycle " + suffix,
		Slug: "incident-a-" + suffix,
		OwnerIssuer: actorIssuer,
		OwnerSubject: actorSubject,
	})
	if err != nil {
		t.Fatalf("create workspace A: %v", err)
	}
	workspaceB, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Incident other workspace " + suffix,
		Slug: "incident-b-" + suffix,
		OwnerIssuer: actorIssuer,
		OwnerSubject: "other-owner-" + suffix,
	})
	if err != nil {
		t.Fatalf("create workspace B: %v", err)
	}

	createService := func(workspaceID, serviceName string) string {
		t.Helper()
		var id string
		err := repository.pool.QueryRow(ctx, `
			INSERT INTO services (
				name, description, environment, region, status, source, workspace_id
			) VALUES ($1, 'incident lifecycle integration fixture', 'test', 'local', 'operational', 'integration_test', $2::uuid)
			RETURNING id::text
		`, serviceName, workspaceID).Scan(&id)
		if err != nil {
			t.Fatalf("create service %s: %v", serviceName, err)
		}
		return id
	}
	serviceA := createService(workspaceA.ID, "incident-service-a-"+suffix)
	serviceB := createService(workspaceB.ID, "incident-service-b-"+suffix)

	incident, err := repository.CreateWorkspaceIncident(ctx, store.CreateWorkspaceIncidentInput{
		WorkspaceID: workspaceA.ID,
		Title: "Elevated checkout error rate",
		Description: "New errors observed after the latest deployment.",
		ServiceID: serviceA,
		Severity: "high",
		ActorIssuer: actorIssuer,
		ActorSubject: actorSubject,
		RequestID: "req-incident-create-"+suffix,
	})
	if err != nil {
		t.Fatalf("create workspace incident: %v", err)
	}
	if incident.ID == "" || incident.Status != "open" || incident.Source != "api" || incident.ServiceName != "incident-service-a-"+suffix {
		t.Fatalf("unexpected created incident: %+v", incident)
	}

	if _, err := repository.CreateWorkspaceIncident(ctx, store.CreateWorkspaceIncidentInput{
		WorkspaceID: workspaceA.ID, Title: "Cross-workspace service", ServiceID: serviceB,
		Severity: "low", ActorIssuer: actorIssuer, ActorSubject: actorSubject,
	}); !errors.Is(err, store.ErrIncidentServiceNotFound) {
		t.Fatalf("cross-workspace service creation error = %v, want ErrIncidentServiceNotFound", err)
	}

	if _, err := repository.CreateWorkspaceIncident(ctx, store.CreateWorkspaceIncidentInput{
		WorkspaceID: workspaceA.ID, Title: "Unauthorized", ServiceID: serviceA,
		Severity: "low", ActorIssuer: actorIssuer, ActorSubject: "not-a-member-"+suffix,
	}); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member create error = %v, want ErrWorkspaceMembershipNotFound", err)
	}

	transition := func(to, comment string) store.Incident {
		t.Helper()
		updated, transitionErr := repository.TransitionWorkspaceIncident(ctx, store.TransitionWorkspaceIncidentInput{
			WorkspaceID: workspaceA.ID,
			IncidentID: incident.ID,
			ToStatus: to,
			Comment: comment,
			ActorIssuer: actorIssuer,
			ActorSubject: actorSubject,
			RequestID: "req-incident-"+to+"-"+suffix,
		})
		if transitionErr != nil {
			t.Fatalf("transition incident to %s: %v", to, transitionErr)
		}
		return updated
	}

	investigating := transition("investigating", "On-call engineer is investigating.")
	if investigating.Status != "investigating" || investigating.ResolvedAt != nil {
		t.Fatalf("unexpected investigating incident: %+v", investigating)
	}
	monitoring := transition("monitoring", "Mitigation applied; watching recovery.")
	if monitoring.Status != "monitoring" || monitoring.ResolvedAt != nil {
		t.Fatalf("unexpected monitoring incident: %+v", monitoring)
	}
	resolved := transition("resolved", "Error rate returned to baseline.")
	if resolved.Status != "resolved" || resolved.ResolvedAt == nil {
		t.Fatalf("resolved incident must have resolved_at: %+v", resolved)
	}
	reopened := transition("investigating", "The error rate regressed; reopening investigation.")
	if reopened.Status != "investigating" || reopened.ResolvedAt != nil {
		t.Fatalf("reopened incident must clear resolved_at: %+v", reopened)
	}
	if _, err := repository.TransitionWorkspaceIncident(ctx, store.TransitionWorkspaceIncidentInput{
		WorkspaceID: workspaceA.ID, IncidentID: incident.ID, ToStatus: "open",
		ActorIssuer: actorIssuer, ActorSubject: actorSubject,
	}); !errors.Is(err, store.ErrIncidentInvalidTransition) {
		t.Fatalf("invalid transition error = %v, want ErrIncidentInvalidTransition", err)
	}
	if _, err := repository.TransitionWorkspaceIncident(ctx, store.TransitionWorkspaceIncidentInput{
		WorkspaceID: workspaceA.ID, IncidentID: incident.ID, ToStatus: "investigating",
		ActorIssuer: actorIssuer, ActorSubject: actorSubject,
	}); !errors.Is(err, store.ErrIncidentNoChange) {
		t.Fatalf("same-status transition error = %v, want ErrIncidentNoChange", err)
	}

	viewerIssuer, viewerSubject := actorIssuer, "incident-viewer-"+suffix
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO workspace_memberships(workspace_id, user_issuer, user_subject, role)
		VALUES($1::uuid, $2, $3, 'viewer')
	`, workspaceA.ID, viewerIssuer, viewerSubject); err != nil {
		t.Fatalf("add viewer membership: %v", err)
	}
	if _, err := repository.TransitionWorkspaceIncident(ctx, store.TransitionWorkspaceIncidentInput{
		WorkspaceID: workspaceA.ID, IncidentID: incident.ID, ToStatus: "resolved",
		ActorIssuer: viewerIssuer, ActorSubject: viewerSubject,
	}); !errors.Is(err, store.ErrIncidentMutationForbidden) {
		t.Fatalf("viewer transition error = %v, want ErrIncidentMutationForbidden", err)
	}

	if _, err := repository.ListWorkspaceIncidentTimeline(ctx, workspaceA.ID, incident.ID, actorIssuer, "not-a-member-"+suffix, 50); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member timeline error = %v, want ErrWorkspaceMembershipNotFound", err)
	}
	if _, err := repository.ListWorkspaceIncidentTimeline(ctx, workspaceB.ID, incident.ID, actorIssuer, "other-owner-"+suffix, 50); !errors.Is(err, store.ErrIncidentNotFound) {
		t.Fatalf("cross-workspace timeline error = %v, want ErrIncidentNotFound", err)
	}

	events, err := repository.ListWorkspaceIncidentTimeline(ctx, workspaceA.ID, incident.ID, actorIssuer, actorSubject, 100)
	if err != nil {
		t.Fatalf("list incident timeline: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("timeline event count = %d, want 5: %+v", len(events), events)
	}
	wantTypes := []string{
		"incident.created",
		"incident.status_changed",
		"incident.status_changed",
		"incident.status_changed",
		"incident.reopened",
	}
	for i, want := range wantTypes {
		if events[i].EventType != want {
			t.Fatalf("timeline event %d type = %q, want %q", i, events[i].EventType, want)
		}
		if i > 0 && events[i].CreatedAt.Before(events[i-1].CreatedAt) {
			t.Fatalf("timeline is not chronological at event %d", i)
		}
	}
	if events[3].ToStatus != "resolved" || events[4].FromStatus != "resolved" || events[4].ToStatus != "investigating" {
		t.Fatalf("resolution/reopen transition not represented correctly: %+v", events[3:])
	}

	// Every mutation creates a workspace audit event in the same DB transaction.
	audit, err := repository.ListWorkspaceAuditEvents(ctx, workspaceA.ID, 100)
	if err != nil {
		t.Fatalf("list workspace audit events: %v", err)
	}
	incidentAuditCount := 0
	for _, event := range audit {
		if event.ResourceType == "incident" && event.ResourceID == incident.ID {
			incidentAuditCount++
		}
	}
	if incidentAuditCount != 5 {
		t.Fatalf("incident audit event count = %d, want 5", incidentAuditCount)
	}

	// Timeline rows are protected against updates and deletes by database triggers.
	if _, err := repository.pool.Exec(ctx, `
		UPDATE incident_timeline_events SET comment = 'tampered'
		WHERE incident_id = $1::uuid
	`, incident.ID); err == nil {
		t.Fatal("incident timeline UPDATE unexpectedly succeeded")
	}
	if _, err := repository.pool.Exec(ctx, `
		DELETE FROM incident_timeline_events WHERE incident_id = $1::uuid
	`, incident.ID); err == nil {
		t.Fatal("incident timeline DELETE unexpectedly succeeded")
	}
}
