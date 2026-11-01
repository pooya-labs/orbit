package postgres

import (
	"context"
	"os"
	"testing"
	"time"
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
}
