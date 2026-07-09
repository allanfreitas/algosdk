package flymigrate

import (
	"context"
	"fmt"
)

// Validate checks the consistency of migration history against local migration files.
func (rf *Migrator) Validate(ctx context.Context) error {
	db, selfCreated, err := rf.getDB(ctx)
	if err != nil {
		return err
	}
	if selfCreated {
		defer db.Close()
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to acquire connection: %w", err)
	}
	defer conn.Close()

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
