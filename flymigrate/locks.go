package flymigrate

import (
	"context"
	"database/sql"
)

// acquireLock acquires a session-level advisory lock.
func (rf *Migrator) acquireLock(ctx context.Context, conn *sql.Conn) error {
	return rf.config.Dialect.AcquireLock(ctx, conn, rf.config.TableName, rf.config.LockTimeout)
}

// releaseLock releases the session-level advisory lock.
func (rf *Migrator) releaseLock(ctx context.Context, conn *sql.Conn) error {
	return rf.config.Dialect.ReleaseLock(ctx, conn, rf.config.TableName)
}
