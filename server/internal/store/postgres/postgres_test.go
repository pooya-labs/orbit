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

	name := fmt.Sprintf("catalog-test-%d", time.Now().UnixNano())
	first, err := repository.UpsertTelemetryServices(ctx, []store.TelemetryService{{
		Name: name,
		Sources: []string{"prometheus", "loki"},
		ResourceAttributes: map[string]any{"deployment.environment.name": "test"},
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
	if _, err := repository.pool.Exec(ctx, "DELETE FROM telemetry_services WHERE name = $1", name); err != nil {
		t.Fatalf("clean up catalog test service: %v", err)
	}
}
