package postgres

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

// TestAlertNotificationOutboxClaimsAreExclusiveAcrossRepositoryPools runs
// independent PostgreSQL connection pools as separate API replicas. The
// workers race to claim a full batch from the same durable notification
// outbox; every delivery must be owned by exactly one replica.
func TestAlertNotificationOutboxClaimsAreExclusiveAcrossRepositoryPools(t *testing.T) {
	repository, workspace, issuer, owner := openAlertEvaluationTestRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}

	const replicaCount = 4
	const deliveriesPerReplica = 6
	const deliveryCount = replicaCount * deliveriesPerReplica

	replicas := make([]*Repository, 0, replicaCount)
	replicas = append(replicas, repository)
	for replica := 1; replica < replicaCount; replica++ {
		pool, err := Open(ctx, databaseURL)
		if err != nil {
			t.Fatalf("open independent PostgreSQL pool for replica %d: %v", replica, err)
		}
		replicas = append(replicas, pool)
		t.Cleanup(pool.Close)
	}

	channel := createAlertNotificationChannelForTest(
		t, ctx, repository, workspace, issuer, owner,
		"Concurrent operations", "OPS_CONCURRENT",
	)
	rule, err := repository.CreateWorkspaceAlertRule(ctx, store.CreateWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID,
		Name: "Concurrent notification outbox " + fmt.Sprintf("%d", time.Now().UnixNano()),
		ServiceName: "",
		Metric: "request_rate",
		Operator: "gt",
		Threshold: 100,
		WindowSeconds: 60,
		PendingSeconds: 0,
		Severity: "high",
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-notification-concurrent-rule",
	})
	if err != nil {
		t.Fatalf("create all-services alert rule: %v", err)
	}

	for index := 0; index < deliveryCount; index++ {
		serviceName := fmt.Sprintf("notification-queue-service-%02d", index)
		instance, changed, applyErr := repository.ApplyWorkspaceAlertSample(
			ctx, workspace.ID, rule.ID, serviceName, 250, time.Now().UTC(),
		)
		if applyErr != nil || !changed || instance.Status != "firing" {
			t.Fatalf("create notification for service %s: instance=%+v changed=%v err=%v", serviceName, instance, changed, applyErr)
		}
	}

	type claimResult struct {
		replica int
		items   []store.AlertNotificationDelivery
		err     error
	}
	start := make(chan struct{})
	results := make(chan claimResult, replicaCount)
	var workers sync.WaitGroup
	for replica, pool := range replicas {
		workers.Add(1)
		go func(replica int, pool *Repository) {
			defer workers.Done()
			<-start
			items, claimErr := pool.ClaimAlertNotificationDeliveries(ctx, deliveriesPerReplica, 60)
			results <- claimResult{replica: replica, items: items, err: claimErr}
		}(replica, pool)
	}
	close(start)
	workers.Wait()
	close(results)

	claimedIDs := make(map[string]int, deliveryCount)
	totalClaimed := 0
	for result := range results {
		if result.err != nil {
			t.Errorf("replica %d could not claim deliveries: %v", result.replica, result.err)
			continue
		}
		if len(result.items) != deliveriesPerReplica {
			t.Errorf("replica %d claimed %d deliveries, want %d", result.replica, len(result.items), deliveriesPerReplica)
		}
		for _, delivery := range result.items {
			if delivery.ChannelID != channel.ID || delivery.Status != "processing" || delivery.Attempts != 1 {
				t.Errorf("replica %d received unexpected delivery state: %+v", result.replica, delivery)
			}
			if previousReplica, exists := claimedIDs[delivery.ID]; exists {
				t.Errorf("delivery %s was claimed by both replica %d and replica %d", delivery.ID, previousReplica, result.replica)
				continue
			}
			claimedIDs[delivery.ID] = result.replica
			totalClaimed++
		}
	}
	if totalClaimed != deliveryCount {
		t.Fatalf("total unique claims = %d, want %d", totalClaimed, deliveryCount)
	}

	// Every queued delivery was assigned once; a fifth claim must not find
	// work left behind or re-claim a row already leased by another pool.
	leftover, err := replicas[0].ClaimAlertNotificationDeliveries(ctx, 10, 60)
	if err != nil {
		t.Fatalf("check for unclaimed notification deliveries: %v", err)
	}
	if len(leftover) != 0 {
		t.Fatalf("leftover/reclaimed deliveries after four replica claims = %d, want 0", len(leftover))
	}

	t.Logf("four independent PostgreSQL pools claimed %d notification deliveries exactly once (%d each)", totalClaimed, deliveriesPerReplica)
}
