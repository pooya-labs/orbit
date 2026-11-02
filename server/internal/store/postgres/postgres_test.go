package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"

)

func TestPostgresMigrationAndReadModels(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	services, err := repository.ListServices(ctx)
	if err != nil {
		t.Fatalf("list services: %v", err)
	}
	seededServices := 0
	for _, service := range services {
		if service.Source == "demo_seed" {
			seededServices++
		}
	}
	if seededServices < 5 {
		t.Fatalf("got %d demo_seed services among %d total, want at least 5 seeded services", seededServices, len(services))
	}

	incidents, err := repository.ListIncidents(ctx, "")
	if err != nil {
		t.Fatalf("list incidents: %v", err)
	}
	if len(incidents) < 2 {
		t.Fatalf("got %d incidents, want at least 2 seeded incidents", len(incidents))
	}

	filtered, err := repository.ListIncidents(ctx, "investigating")
	if err != nil {
		t.Fatalf("list filtered incidents: %v", err)
	}
	for _, incident := range filtered {
		if incident.Status != "investigating" {
			t.Fatalf("incident %q has unexpected status %q", incident.ID, incident.Status)
		}
	}

	if err := repository.Migrate(ctx); err != nil {
		t.Fatalf("second migration run should be idempotent: %v", err)
	}

	pagePrefix := fmt.Sprintf("zz-orbit-page-test-%d", time.Now().UnixNano())
	pageNames := []string{pagePrefix + "-a", pagePrefix + "-b"}
	_, err = repository.UpsertTelemetryServices(ctx, []store.TelemetryService{
		{Name: pageNames[0], Sources: []string{"prometheus"}},
		{Name: pageNames[1], Sources: []string{"loki"}},
	})
	if err != nil {
		t.Fatalf("seed telemetry pagination test services: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx,
			"DELETE FROM telemetry_services WHERE name = ANY($1::text[])",
			pageNames,
		)
	})

	firstPage, err := repository.ListTelemetryServicesPage(ctx, 1, &store.TelemetryServiceCursor{
		Name: pagePrefix,
		ID:   "00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("read first telemetry catalog page: %v", err)
	}
	if len(firstPage.Items) != 1 || firstPage.Items[0].Name != pageNames[0] || !firstPage.HasMore || firstPage.Next == nil {
		t.Fatalf("unexpected first telemetry catalog page: %+v", firstPage)
	}

	secondPage, err := repository.ListTelemetryServicesPage(ctx, 1, firstPage.Next)
	if err != nil {
		t.Fatalf("read second telemetry catalog page: %v", err)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].Name != pageNames[1] || secondPage.HasMore || secondPage.Next != nil {
		t.Fatalf("unexpected second telemetry catalog page: %+v", secondPage)
	}

	name := fmt.Sprintf("catalog-test-%d", time.Now().UnixNano())
	first, err := repository.UpsertTelemetryServices(ctx, []store.TelemetryService{{
		Name: name,
		Sources: []string{"prometheus", "loki"},
		ResourceAttributes: map[string]any{"deployment.environment.name": "test"},
		ResourceAttributeSources: map[string][]string{"deployment.environment.name": {"prometheus", "tempo"}},
	}})
	if err != nil {
		t.Fatalf("upsert telemetry service: %v", err)
	}
	if len(first) != 1 || first[0].ID == "" || first[0].FirstSeenAt.IsZero() || len(first[0].Sources) != 2 {
		t.Fatalf("unexpected persisted telemetry service: %+v", first)
	}

	time.Sleep(10 * time.Millisecond)
	second, err := repository.UpsertTelemetryServices(ctx, []store.TelemetryService{{
		Name: name,
		Sources: []string{"tempo"},
		ResourceAttributes: map[string]any{"cloud.region": "test-region"},
		ResourceAttributeSources: map[string][]string{"deployment.environment.name": {"loki"}, "cloud.region": {"tempo"}},
	}})
	if err != nil {
		t.Fatalf("upsert existing telemetry service: %v", err)
	}
	if len(second) != 1 || second[0].ID != first[0].ID || !second[0].FirstSeenAt.Equal(first[0].FirstSeenAt) {
		t.Fatalf("upsert should preserve ID and first-seen timestamp: first=%+v second=%+v", first[0], second)
	}

	catalog, err := repository.ListTelemetryServices(ctx, 250)
	if err != nil {
		t.Fatalf("list persisted telemetry services: %v", err)
	}
	var found *store.TelemetryService
	for index := range catalog {
		if catalog[index].Name == name {
			found = &catalog[index]
			break
		}
	}
	if found == nil || len(found.Sources) != 3 {
		t.Fatalf("expected persisted catalog to retain all three observed sources; got %+v", found)
	}
	if found.ResourceAttributes["deployment.environment.name"] != "test" || found.ResourceAttributes["cloud.region"] != "test-region" {
		t.Fatalf("expected resource attributes to merge across observations: %+v", found.ResourceAttributes)
	}
	if got := found.ResourceAttributeSources["deployment.environment.name"]; len(got) != 3 || got[0] != "loki" || got[1] != "prometheus" || got[2] != "tempo" {
		t.Fatalf("expected attribute provenance to merge across observations, got %v", got)
	}
	if got := found.ResourceAttributeSources["cloud.region"]; len(got) != 1 || got[0] != "tempo" {
		t.Fatalf("unexpected cloud.region provenance: %v", got)
	}
	if _, err := repository.pool.Exec(ctx, "DELETE FROM telemetry_services WHERE name = $1", name); err != nil {
		t.Fatalf("clean up catalog test service: %v", err)
	}
}


func TestSharedRateLimitWindowUsesAtomicCountersAndExpiresOldBuckets(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	now := time.Now().UTC().Truncate(time.Minute)
	oldBucket := now.Add(-10 * time.Minute)
	oldKey := fmt.Sprintf("%064x", time.Now().UnixNano()+11)
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO api_rate_limit_windows (bucket_start, key_hash, request_count, expires_at)
		VALUES ($1::timestamptz, $2, 9, $1::timestamptz + INTERVAL '2 minutes')
	`, oldBucket, oldKey); err != nil {
		t.Fatalf("insert expired rate-limit bucket: %v", err)
	}

	keyA := fmt.Sprintf("%064x", time.Now().UnixNano()+12)
	keyB := fmt.Sprintf("%064x", time.Now().UnixNano()+13)
	first, err := repository.IncrementRateLimitWindow(ctx, keyA)
	if err != nil { t.Fatalf("first shared increment: %v", err) }
	second, err := repository.IncrementRateLimitWindow(ctx, keyA)
	if err != nil { t.Fatalf("second shared increment: %v", err) }
	other, err := repository.IncrementRateLimitWindow(ctx, keyB)
	if err != nil { t.Fatalf("independent shared increment: %v", err) }
	if first != 1 || second != 2 || other != 1 {
		t.Fatalf("unexpected shared counts: first=%d second=%d other=%d", first, second, other)
	}

	// Multiple concurrent API instances must not lose counter increments. Sum
	// across the two adjacent minute buckets so a test that crosses a minute
	// boundary remains deterministic.
	const concurrentRequests = 24
	concurrentKey := fmt.Sprintf("%064x", time.Now().UnixNano()+14)
	var workers sync.WaitGroup
	errors := make(chan error, concurrentRequests)
	for range concurrentRequests {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, incrementErr := repository.IncrementRateLimitWindow(ctx, concurrentKey)
			if incrementErr != nil {
				errors <- incrementErr
			}
		}()
	}
	workers.Wait()
	close(errors)
	for incrementErr := range errors {
		t.Fatalf("concurrent shared increment: %v", incrementErr)
	}
	var concurrentTotal int64
	if err := repository.pool.QueryRow(ctx,
		"SELECT COALESCE(SUM(request_count), 0) FROM api_rate_limit_windows WHERE key_hash = $1",
		concurrentKey,
	).Scan(&concurrentTotal); err != nil {
		t.Fatalf("read concurrent counter total: %v", err)
	}
	if concurrentTotal != concurrentRequests {
		t.Fatalf("concurrent counter total = %d, want %d", concurrentTotal, concurrentRequests)
	}

	var oldRows int
	if err := repository.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM api_rate_limit_windows WHERE key_hash = $1", oldKey,
	).Scan(&oldRows); err != nil { t.Fatalf("check expired bucket cleanup: %v", err) }
	if oldRows != 0 { t.Fatalf("expired bucket remains in database: %d rows", oldRows) }

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		_, _ = repository.pool.Exec(cleanupCtx,
			"DELETE FROM api_rate_limit_windows WHERE key_hash = ANY($1::char(64)[])",
			[]string{keyA, keyB, oldKey, concurrentKey},
		)
	})
}


func TestWorkspaceMembershipAndAppendOnlyAudit(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	issuer := "https://identity.example/"
	subject := "workspace-owner-" + suffix
	workspace, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Workspace " + suffix,
		Slug: "ws-" + suffix,
		OwnerIssuer: issuer,
		OwnerSubject: subject,
	})
	if err != nil {
		t.Fatalf("create workspace and owner atomically: %v", err)
	}
	if workspace.ID == "" || workspace.Slug != "ws-"+suffix || workspace.CreatedAt.IsZero() {
		t.Fatalf("unexpected workspace result: %+v", workspace)
	}

	membership, err := repository.GetWorkspaceMembership(ctx, workspace.ID, issuer, subject)
	if err != nil {
		t.Fatalf("read owner membership: %v", err)
	}
	if membership.Role != "owner" {
		t.Fatalf("new workspace owner role = %q, want owner", membership.Role)
	}
	workspaces, err := repository.ListWorkspacesForPrincipal(ctx, issuer, subject)
	if err != nil {
		t.Fatalf("list workspaces for owner: %v", err)
	}
	found := false
	for _, item := range workspaces {
		if item.ID == workspace.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created workspace %q is missing from owner membership listing", workspace.ID)
	}
	if _, err := repository.GetWorkspaceMembership(ctx, workspace.ID, issuer, "not-a-member"); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member lookup error = %v, want ErrWorkspaceMembershipNotFound", err)
	}

	event, err := repository.AppendWorkspaceAuditEvent(ctx, store.WorkspaceAuditEvent{
		WorkspaceID: workspace.ID,
		ActorIssuer: issuer,
		ActorSubject: subject,
		Action: "workspace.created",
		ResourceType: "workspace",
		ResourceID: workspace.ID,
		RequestID: "req-" + suffix,
		Details: map[string]any{"source": "integration_test"},
	})
	if err != nil {
		t.Fatalf("append audit event: %v", err)
	}
	if event.ID == "" || event.CreatedAt.IsZero() || event.ActorRef == "" || event.ActorRole != "owner" {
		t.Fatalf("audit event did not capture pseudonymous actor context: %+v", event)
	}
	if strings.EqualFold(event.ActorRef, subject) {
		t.Fatal("audit actor reference exposed the raw subject")
	}
	events, err := repository.ListWorkspaceAuditEvents(ctx, workspace.ID, 10)
	if err != nil {
		t.Fatalf("list workspace audit events: %v", err)
	}
	if len(events) != 2 || events[0].ID != event.ID || events[0].Details["source"] != "integration_test" ||
		events[0].ActorRole != "owner" || events[0].ActorRef != event.ActorRef ||
		events[1].Action != "workspace.created" || events[1].ActorRole != "owner" ||
		events[1].ActorRef != event.ActorRef {
		t.Fatalf("unexpected workspace audit timeline/actor context: %+v", events)
	}

	operatorSubject := "workspace-operator-" + suffix
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO workspace_memberships (workspace_id, user_issuer, user_subject, role)
		VALUES ($1::uuid, $2, $3, 'operator')
	`, workspace.ID, issuer, operatorSubject); err != nil {
		t.Fatalf("create operator membership: %v", err)
	}
	operatorEvent, err := repository.AppendWorkspaceAuditEvent(ctx, store.WorkspaceAuditEvent{
		WorkspaceID: workspace.ID,
		ActorIssuer: issuer,
		ActorSubject: operatorSubject,
		Action: "workspace.operator_action",
		ResourceType: "workspace",
		RequestID: "req-operator-" + suffix,
	})
	if err != nil {
		t.Fatalf("append operator audit event: %v", err)
	}
	if operatorEvent.ActorRole != "operator" || operatorEvent.ActorRef == "" || operatorEvent.ActorRef == event.ActorRef {
		t.Fatalf("operator audit actor context = %+v", operatorEvent)
	}
	if _, err := repository.pool.Exec(ctx, `
		UPDATE workspace_memberships SET role = 'viewer'
		WHERE workspace_id = $1::uuid AND user_issuer = $2 AND user_subject = $3
	`, workspace.ID, issuer, operatorSubject); err != nil {
		t.Fatalf("change operator membership role: %v", err)
	}
	events, err = repository.ListWorkspaceAuditEvents(ctx, workspace.ID, 10)
	if err != nil {
		t.Fatalf("read audit events after membership role change: %v", err)
	}
	var operatorEventFound bool
	for _, item := range events {
		if item.ID == operatorEvent.ID {
			operatorEventFound = true
			if item.ActorRole != "operator" || item.ActorRef != operatorEvent.ActorRef {
				t.Fatalf("audit actor context changed with current membership: %+v", item)
			}
		}
	}
	if !operatorEventFound {
		t.Fatalf("operator audit event %q missing from history", operatorEvent.ID)
	}
	if _, err := repository.AppendWorkspaceAuditEvent(ctx, store.WorkspaceAuditEvent{
		WorkspaceID: workspace.ID, ActorIssuer: issuer, ActorSubject: "not-a-member",
		Action: "workspace.changed", ResourceType: "workspace", ResourceID: workspace.ID,
	}); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member audit append error = %v, want ErrWorkspaceMembershipNotFound", err)
	}

	if _, err := repository.pool.Exec(ctx, "UPDATE workspace_audit_events SET action = 'workspace.changed' WHERE id = $1::uuid", event.ID); err == nil {
		t.Fatal("audit event update succeeded; expected append-only trigger to reject it")
	}
	if _, err := repository.pool.Exec(ctx, "DELETE FROM workspace_audit_events WHERE id = $1::uuid", event.ID); err == nil {
		t.Fatal("audit event delete succeeded; expected append-only trigger to reject it")
	}
	if _, err := repository.pool.Exec(ctx, "TRUNCATE workspace_audit_events"); err == nil {
		t.Fatal("audit event truncate succeeded; expected append-only trigger to reject it")
	}
	if _, err := repository.pool.Exec(ctx, `
		UPDATE workspace_audit_actors SET actor_subject = 'tampered'
		WHERE workspace_id = $1::uuid AND actor_ref = $2::uuid
	`, workspace.ID, event.ActorRef); err == nil {
		t.Fatal("audit actor reference update succeeded; expected immutable mapping trigger to reject it")
	}
	if _, err := repository.pool.Exec(ctx, `
		DELETE FROM workspace_audit_actors
		WHERE workspace_id = $1::uuid AND actor_ref = $2::uuid
	`, workspace.ID, event.ActorRef); err == nil {
		t.Fatal("audit actor reference delete succeeded; expected immutable mapping trigger to reject it")
	}
	if _, err := repository.pool.Exec(ctx, "TRUNCATE workspace_audit_actors"); err == nil {
		t.Fatal("audit actor reference truncate succeeded; expected immutable mapping trigger to reject it")
	}
	if _, err := repository.AppendWorkspaceAuditEvent(ctx, store.WorkspaceAuditEvent{
		WorkspaceID: workspace.ID, ActorIssuer: issuer, ActorSubject: subject,
		Action: "workspace.created", ResourceType: "workspace",
		Details: map[string]any{"oversized": strings.Repeat("x", 17*1024)},
	}); err == nil {
		t.Fatal("oversized audit details were accepted")
	}
}


func TestWorkspaceScopedDataReadsDoNotCrossWorkspace(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	defer repository.Close()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	create := func(label string) store.Workspace {
		workspace, createErr := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
			Name: "Scope " + label + " " + suffix,
			Slug: "scope-" + label + "-" + suffix,
			OwnerIssuer: "https://scope-test.example/",
			OwnerSubject: label + "-" + suffix,
		})
		if createErr != nil {
			t.Fatalf("create %s workspace: %v", label, createErr)
		}
		return workspace
	}
	workspaceA := create("alpha")
	workspaceB := create("bravo")

	var serviceAID string
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO services (workspace_id, name, environment, region, status, source)
		VALUES ($1::uuid, $2, 'test', 'test-region', 'operational', 'scope_test')
		RETURNING id::text
	`, workspaceA.ID, "scope-service-"+suffix).Scan(&serviceAID); err != nil {
		t.Fatalf("insert workspace A service: %v", err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO incidents (workspace_id, title, service_id, severity, status, source)
		VALUES ($1::uuid, $2, $3::uuid, 'low', 'open', 'scope_test')
	`, workspaceA.ID, "scope-incident-"+suffix, serviceAID); err != nil {
		t.Fatalf("insert workspace A incident: %v", err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO telemetry_services (workspace_id, name)
		VALUES ($1::uuid, $2)
	`, workspaceA.ID, "scope-telemetry-"+suffix); err != nil {
		t.Fatalf("insert workspace A telemetry service: %v", err)
	}

	servicesA, err := repository.ListWorkspaceServices(ctx, workspaceA.ID)
	if err != nil || len(servicesA) != 1 || servicesA[0].Name != "scope-service-"+suffix {
		t.Fatalf("workspace A services = %+v, err=%v", servicesA, err)
	}
	servicesB, err := repository.ListWorkspaceServices(ctx, workspaceB.ID)
	if err != nil || len(servicesB) != 0 {
		t.Fatalf("workspace B must not see workspace A services: %+v, err=%v", servicesB, err)
	}
	incidentsA, err := repository.ListWorkspaceIncidents(ctx, workspaceA.ID, "open")
	if err != nil || len(incidentsA) != 1 || incidentsA[0].ServiceID != serviceAID {
		t.Fatalf("workspace A incidents = %+v, err=%v", incidentsA, err)
	}
	incidentsB, err := repository.ListWorkspaceIncidents(ctx, workspaceB.ID, "")
	if err != nil || len(incidentsB) != 0 {
		t.Fatalf("workspace B must not see workspace A incidents: %+v, err=%v", incidentsB, err)
	}
	telemetryA, err := repository.ListWorkspaceTelemetryServices(ctx, workspaceA.ID, 50)
	if err != nil || len(telemetryA) != 1 || telemetryA[0].Name != "scope-telemetry-"+suffix {
		t.Fatalf("workspace A telemetry = %+v, err=%v", telemetryA, err)
	}
	telemetryB, err := repository.ListWorkspaceTelemetryServices(ctx, workspaceB.ID, 50)
	if err != nil || len(telemetryB) != 0 {
		t.Fatalf("workspace B must not see workspace A telemetry: %+v, err=%v", telemetryB, err)
	}

	_, err = repository.pool.Exec(ctx, `
		INSERT INTO incidents (workspace_id, title, service_id, severity, status, source)
		VALUES ($1::uuid, $2, $3::uuid, 'low', 'open', 'scope_test')
	`, workspaceB.ID, "cross-workspace-link-"+suffix, serviceAID)
	if err == nil {
		t.Fatal("database must reject an incident linked to a service in another workspace")
	}
}


func TestWorkloadIdentityWorkspaceGrantAuthorization(t *testing.T) {
 databaseURL:=os.Getenv("ORBIT_TEST_DATABASE_URL")
 if databaseURL=="" { t.Skip("ORBIT_TEST_DATABASE_URL is not set") }
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 repository,err:=Open(ctx,databaseURL);if err!=nil{t.Fatalf("open repository: %v",err)};defer repository.Close()
 suffix:=fmt.Sprintf("%d",time.Now().UnixNano())
 actorIssuer:="https://identity.example/"
 actorSubject:="workload-admin-"+suffix
 workspaceA,err:=repository.CreateWorkspace(ctx,store.CreateWorkspaceInput{Name:"Workload A "+suffix,Slug:"workload-a-"+suffix,OwnerIssuer:actorIssuer,OwnerSubject:actorSubject})
 if err!=nil{t.Fatalf("create workspace A: %v",err)}
 workspaceB,err:=repository.CreateWorkspace(ctx,store.CreateWorkspaceInput{Name:"Workload B "+suffix,Slug:"workload-b-"+suffix,OwnerIssuer:actorIssuer,OwnerSubject:actorSubject})
 if err!=nil{t.Fatalf("create workspace B: %v",err)}
 identity,err:=repository.CreateWorkloadIdentity(ctx,store.CreateWorkloadIdentityInput{
  Issuer:"https://workloads.example/",Subject:"collector-"+suffix,DisplayName:"Test Collector",
  ActorIssuer:actorIssuer,ActorSubject:actorSubject,
 })
 if err!=nil{t.Fatalf("create workload identity: %v",err)}
 grant:=store.WorkloadGrantInput{IdentityID:identity.ID,WorkspaceID:workspaceA.ID,Signal:"metrics",ActorIssuer:actorIssuer,ActorSubject:actorSubject}
 if err:=repository.SetWorkloadWorkspaceGrant(ctx,grant,true);err!=nil{t.Fatalf("grant metrics: %v",err)}
 authorized,err:=repository.AuthorizeWorkloadTelemetry(ctx,identity.Issuer,identity.Subject,workspaceA.ID,"metrics")
 if err!=nil||authorized.ID!=identity.ID{t.Fatalf("authorized workload = %+v, err=%v",authorized,err)}
 listed,err:=repository.ListWorkloadIdentities(ctx,100)
 if err!=nil{t.Fatalf("list workload identities: %v",err)}
 foundIdentity:=false
 for _,item:=range listed { if item.ID==identity.ID && item.Issuer==identity.Issuer && item.Subject==identity.Subject { foundIdentity=true } }
 if !foundIdentity { t.Fatalf("workload identity %s was not listed",identity.ID) }
 grants,err:=repository.ListWorkspaceWorkloadGrants(ctx,workspaceA.ID)
 if err!=nil{t.Fatalf("list workspace workload grants: %v",err)}
 if len(grants)!=1 || grants[0].IdentityID!=identity.ID || grants[0].Signal!="metrics" { t.Fatalf("workspace A grants = %+v, want only metrics",grants) }
 grantsB,err:=repository.ListWorkspaceWorkloadGrants(ctx,workspaceB.ID)
 if err!=nil{t.Fatalf("list workspace B grants: %v",err)}
 if len(grantsB)!=0 { t.Fatalf("workspace B grants = %+v, want empty",grantsB) }
 if _,err:=repository.AuthorizeWorkloadTelemetry(ctx,identity.Issuer,identity.Subject,workspaceA.ID,"logs");!errors.Is(err,store.ErrWorkloadAuthorizationDenied){t.Fatalf("logs should be denied, got %v",err)}
 if _,err:=repository.AuthorizeWorkloadTelemetry(ctx,identity.Issuer,identity.Subject,workspaceB.ID,"metrics");!errors.Is(err,store.ErrWorkloadAuthorizationDenied){t.Fatalf("cross-workspace access should be denied, got %v",err)}
 if err:=repository.SetWorkloadWorkspaceGrant(ctx,grant,false);err!=nil{t.Fatalf("revoke metrics grant: %v",err)}
 if _,err:=repository.AuthorizeWorkloadTelemetry(ctx,identity.Issuer,identity.Subject,workspaceA.ID,"metrics");!errors.Is(err,store.ErrWorkloadAuthorizationDenied){t.Fatalf("revoked grant should be denied, got %v",err)}
 if err:=repository.SetWorkloadIdentityStatus(ctx,identity.ID,actorIssuer,actorSubject,"revoked");err!=nil{t.Fatalf("revoke workload identity: %v",err)}
 if _,err:=repository.AuthorizeWorkloadTelemetry(ctx,identity.Issuer,identity.Subject,workspaceA.ID,"metrics");!errors.Is(err,store.ErrWorkloadAuthorizationDenied){t.Fatalf("revoked identity should be denied, got %v",err)}
 if err:=repository.SetWorkloadIdentityStatus(ctx,identity.ID,actorIssuer,actorSubject,"active");!errors.Is(err,store.ErrWorkloadAuthorizationDenied){t.Fatalf("revoked identity must not be re-enabled, got %v",err)}
 var denialCount int
 if err:=repository.pool.QueryRow(ctx,`
  SELECT COUNT(*) FROM workload_identity_audit_events
  WHERE actor_issuer=$1 AND actor_subject=$2
    AND action='workload.authorization.denied'
    AND workspace_id IN ($3::uuid,$4::uuid)
 `,identity.Issuer,identity.Subject,workspaceA.ID,workspaceB.ID).Scan(&denialCount);err!=nil{
  t.Fatalf("count workload authorization denial audit events: %v",err)
 }
 if denialCount != 4 { t.Fatalf("denial audit events = %d, want 4",denialCount) }
}


func TestConcurrentRepositoryOpenSerializesMigrations(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const workers = 3
	errorsByWorker := make(chan error, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			repository, err := Open(ctx, databaseURL)
			if err == nil {
				repository.Close()
			}
			errorsByWorker <- err
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent repository Open() failed: %v", err)
		}
	}
}
