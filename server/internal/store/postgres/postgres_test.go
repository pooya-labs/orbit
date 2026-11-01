package postgres

import (
	"context"
	"fmt"
	"os"
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
	if len(services) < 5 {
		t.Fatalf("got %d services, want at least 5 seeded services", len(services))
	}
	for _, service := range services {
		if service.Source != "demo_seed" {
			t.Fatalf("service %q source = %q, want demo_seed", service.Name, service.Source)
		}
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
		VALUES ($1, $2, 9, $1 + INTERVAL '2 minutes')
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
			[]string{keyA, keyB, oldKey},
		)
	})
}
