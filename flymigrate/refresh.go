package flymigrate

import (
	"context"
	"fmt"
)

// Refresh drops all tables in the database schema.
func (rf *Migrator) Refresh(ctx context.Context) error {
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

	if err := rf.config.Dialect.Refresh(ctx, conn); err != nil {
		return err
	}

	return nil
}
