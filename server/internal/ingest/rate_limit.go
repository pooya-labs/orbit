package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

func allowWorkloadRate(
	ctx context.Context,
	shared WorkloadRateLimitStore,
	fallback *workspaceRateLimiter,
	limit int,
	rawKey string,
	now time.Time,
) (bool, error) {
	if shared == nil {
		return fallback.Allow(rawKey, now), nil
	}
	digest := sha256.Sum256([]byte(rawKey))
	count, err := shared.IncrementRateLimitWindow(ctx, hex.EncodeToString(digest[:]))
	if err != nil {
		return false, fmt.Errorf("increment shared workload rate limit: %w", err)
	}
	return count <= int64(limit), nil
}
