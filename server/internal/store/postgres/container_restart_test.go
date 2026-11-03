package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writePostgresRestartMarker(controlDir, name string) error {
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		return fmt.Errorf("create PostgreSQL restart control directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(controlDir, name), []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o600); err != nil {
		return fmt.Errorf("write PostgreSQL restart marker %q: %w", name, err)
	}
	return nil
}

func waitForPostgresRestartMarker(controlDir, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	path := filepath.Join(controlDir, name)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect PostgreSQL restart marker %q: %w", name, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for PostgreSQL restart marker %q after %s", name, timeout)
}

// TestRepositoryRecoversAfterPostgresContainerRestart is coordinated by
// scripts/postgres-restart-smoke.sh. The test and the host runner share a
// bind-mounted control directory: the test signals when its pool is warm,
// the host stops the actual Compose PostgreSQL container, the test observes
// loss of connectivity, and the host starts the same container again.
//
// This deliberately tests the original Repository/pgxpool instance rather
// than constructing a replacement pool after the restart.
func TestRepositoryRecoversAfterPostgresContainerRestart(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("ORBIT_TEST_DATABASE_URL"))
	controlDir := strings.TrimSpace(os.Getenv("ORBIT_TEST_POSTGRES_RESTART_CONTROL_DIR"))
	if databaseURL == "" || controlDir == "" {
		t.Skip("PostgreSQL container restart integration requires ORBIT_TEST_DATABASE_URL and ORBIT_TEST_POSTGRES_RESTART_CONTROL_DIR")
	}

	openCtx, openCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer openCancel()
	repository, err := Open(openCtx, databaseURL)
	if err != nil {
		t.Fatalf("open repository before PostgreSQL container restart: %v", err)
	}
	t.Cleanup(repository.Close)

	originalPool := repository.pool
	var originalPostmasterStart time.Time
	if err := repository.pool.QueryRow(openCtx, "SELECT pg_postmaster_start_time()").Scan(&originalPostmasterStart); err != nil {
		t.Fatalf("read original PostgreSQL postmaster start time: %v", err)
	}
	if originalPostmasterStart.IsZero() {
		t.Fatal("PostgreSQL returned an empty original postmaster start time")
	}

	// Force and return an established connection before the host stops the
	// database container. This is not merely a first-dial retry test.
	warmCtx, warmCancel := context.WithTimeout(context.Background(), 5*time.Second)
	connection, err := repository.pool.Acquire(warmCtx)
	if err != nil {
		warmCancel()
		t.Fatalf("acquire PostgreSQL pool connection before restart: %v", err)
	}
	if err := connection.Ping(warmCtx); err != nil {
		connection.Release()
		warmCancel()
		t.Fatalf("warm existing PostgreSQL pool connection: %v", err)
	}
	connection.Release()
	warmCancel()

	if err := writePostgresRestartMarker(controlDir, "ready"); err != nil {
		t.Fatal(err)
	}
	if err := waitForPostgresRestartMarker(controlDir, "postgres-stopped", 30*time.Second); err != nil {
		t.Fatal(err)
	}

	// The Compose service is fully stopped before the host creates this
	// marker. Poll briefly to tolerate TCP teardown and observe a real outage.
	outageDeadline := time.Now().Add(20 * time.Second)
	var outageObserved bool
	var lastOutageError error
	for time.Now().Before(outageDeadline) {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
		lastOutageError = repository.Ping(pingCtx)
		pingCancel()
		if lastOutageError != nil {
			outageObserved = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !outageObserved {
		t.Fatalf("the existing repository pool never observed PostgreSQL being stopped: %v", lastOutageError)
	}
	if err := writePostgresRestartMarker(controlDir, "outage-detected"); err != nil {
		t.Fatal(err)
	}

	if err := waitForPostgresRestartMarker(controlDir, "postgres-restarted", 45*time.Second); err != nil {
		t.Fatal(err)
	}

	// Wait for the same pool to discard dead sessions, dial the restarted
	// process, and run a query. Checking postmaster_start_time proves that the
	// database server process really restarted, rather than only a proxy path.
	recoveryDeadline := time.Now().Add(40 * time.Second)
	var recoveredPostmasterStart time.Time
	var lastRecoveryError error
	for time.Now().Before(recoveryDeadline) {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
		lastRecoveryError = repository.pool.QueryRow(
			pingCtx,
			"SELECT pg_postmaster_start_time()",
		).Scan(&recoveredPostmasterStart)
		pingCancel()
		if lastRecoveryError == nil && recoveredPostmasterStart.After(originalPostmasterStart) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastRecoveryError != nil {
		t.Fatalf("the existing PostgreSQL pool did not recover after the container restart: %v", lastRecoveryError)
	}
	if !recoveredPostmasterStart.After(originalPostmasterStart) {
		t.Fatalf("PostgreSQL postmaster start time did not advance across restart: before=%s after=%s", originalPostmasterStart, recoveredPostmasterStart)
	}
	if repository.pool != originalPool {
		t.Fatal("repository replaced its pool during PostgreSQL container recovery")
	}
	if err := writePostgresRestartMarker(controlDir, "recovered"); err != nil {
		t.Fatal(err)
	}
	t.Logf("same pgxpool recovered after PostgreSQL container restart: postmaster start time %s -> %s", originalPostmasterStart.Format(time.RFC3339Nano), recoveredPostmasterStart.Format(time.RFC3339Nano))
}
