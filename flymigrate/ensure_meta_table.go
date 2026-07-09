package flymigrate

import (
	"context"
	"database/sql"
)

// ensureMetadataTable creates the history table if it does not exist.
func (rf *Migrator) ensureMetadataTable(ctx context.Context, conn *sql.Conn) error {
	if err := rf.validateTableName(); err != nil {
		return err
	}

	return rf.config.Dialect.EnsureTable(ctx, conn, rf.config.TableName)
}
