package flymigrate

import (
	"context"
	"fmt"
)

// Validate checks the consistency of migration history against local migration files.
func (rf *Migrator) Validate(ctx context.Context) error {
	pool, selfCreated, err := rf.getPool(ctx)
	if err != nil {
		return err
	}
	if selfCreated {
		defer pool.Close()
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to acquire connection: %w", err)
	}
	defer conn.Release()

	if err := rf.ensureMetadataTable(ctx, conn); err != nil {
		return err
	}

	localMigs, err := rf.loadMigrations()
	if err != nil {
		return err
	}

	history, err := rf.fetchHistory(ctx, conn)
	if err != nil {
		return err
	}

	return rf.validateState(localMigs, history)
}
