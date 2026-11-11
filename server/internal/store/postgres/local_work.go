package postgres

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
)

//go:embed local_work_bootstrap.sql
var localWorkBootstrapSQL string

// BootstrapLocalWork creates the single-user workspace and clearly marked
// starter records. It is called only when ORBIT_LOCAL_WORK=true, after schema
// migrations have completed; normal and production database opens never seed
// this local identity or workspace.
func (r *Repository) BootstrapLocalWork(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return errors.New("Local Work bootstrap requires an open PostgreSQL repository")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Local Work bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, localWorkBootstrapSQL); err != nil {
		return fmt.Errorf("seed Local Work workspace: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Local Work bootstrap: %w", err)
	}
	return nil
}
