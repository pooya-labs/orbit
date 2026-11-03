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
		t.Skip("PostgreSQL restart integration requires ORBIT_TEST_DATABASE_URL and ORBIT_TEST_POSTGRES_RESTART_CONTROL_DIR")
	}

	openCtx, openCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer openCancel()
	repository, err := Open(openCtx, databaseURL)
	if err != nil {
		t.Fatalf("open repository before PostgreSQL restart tests: %v", err)
	}
	t.Cleanup(repository.Close)

	originalPool := repository.pool
	var currentPostmasterStart time.Time
	if err := repository.pool.QueryRow(openCtx, "SELECT pg_postmaster_start_time()").Scan(&currentPostmasterStart); err != nil {
		t.Fatalf("read original PostgreSQL postmaster start time: %v", err)
	}
	if currentPostmasterStart.IsZero() {
		t.Fatal("PostgreSQL returned an empty original postmaster start time")
	}

	// Run a clean stop/start first, then an ungraceful SIGKILL/restart. Both
	// cycles use the same Repository and pgxpool instance.
	for _, phase := range []string{"clean", "crash"} {
		t.Run(phase, func(t *testing.T) {
			// Force and return an established connection before the host stops
			// the database container. This is not a first-dial retry test.
			warmCtx, warmCancel := context.WithTimeout(context.Background(), 5*time.Second)
			connection, err := repository.pool.Acquire(warmCtx)
			if err != nil {
				warmCancel()
				t.Fatalf("acquire PostgreSQL pool connection before %s restart: %v", phase, err)
			}
			if err := connection.Ping(warmCtx); err != nil {
				connection.Release()
				warmCancel()
				t.Fatalf("warm existing PostgreSQL pool connection before %s restart: %v", phase, err)
			}
			connection.Release()
			warmCancel()

			if err := writePostgresRestartMarker(controlDir, "ready-"+phase); err != nil {
				t.Fatal(err)
			}
			if err := waitForPostgresRestartMarker(controlDir, "postgres-stopped-"+phase, 30*time.Second); err != nil {
				t.Fatal(err)
			}

			// The host signals only after Compose has stopped/killed the
			// database container. Verify the existing pool observes the
			// outage before permitting the host to start PostgreSQL again.
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
				t.Fatalf("existing repository pool never observed PostgreSQL %s stop: %v", phase, lastOutageError)
			}
			if err := writePostgresRestartMarker(controlDir, "outage-detected-"+phase); err != nil {
				t.Fatal(err)
			}

			if err := waitForPostgresRestartMarker(controlDir, "postgres-restarted-"+phase, 45*time.Second); err != nil {
				t.Fatal(err)
			}

			// Require the same pool to discard broken sessions, reconnect,
			// and query the restarted server. The postmaster timestamp must
			// advance in each cycle, not merely return a healthy old session.
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
				if lastRecoveryError == nil && recoveredPostmasterStart.After(currentPostmasterStart) {
					break
				}
				time.Sleep(150 * time.Millisecond)
			}
			if lastRecoveryError != nil {
				t.Fatalf("same PostgreSQL pool did not recover after %s restart: %v", phase, lastRecoveryError)
			}
			if !recoveredPostmasterStart.After(currentPostmasterStart) {
				t.Fatalf("PostgreSQL postmaster start time did not advance after %s restart: before=%s after=%s", phase, currentPostmasterStart, recoveredPostmasterStart)
			}
			if repository.pool != originalPool {
				t.Fatal("repository replaced its pool during PostgreSQL recovery")
			}
			t.Logf("same pgxpool recovered after PostgreSQL %s restart: postmaster start time %s -> %s", phase, currentPostmasterStart.Format(time.RFC3339Nano), recoveredPostmasterStart.Format(time.RFC3339Nano))
			currentPostmasterStart = recoveredPostmasterStart

			if err := writePostgresRestartMarker(controlDir, "recovered-"+phase); err != nil {
				t.Fatal(err)
			}
		})
	}
}
