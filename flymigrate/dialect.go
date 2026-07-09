package flymigrate

import (
	"context"
	"database/sql"
	"time"
)

// Dialect defines the database-specific operations needed by the migrator.
type Dialect interface {
	// EnsureTable creates the migration history table and any necessary indexes.
	EnsureTable(ctx context.Context, conn *sql.Conn, tableName string) error

	// InsertMigrationSQL returns the SQL query (with placeholders) to insert a new history entry.
	InsertMigrationSQL(tableName string) string

	// SelectHistorySQL returns the SQL query to select all history entries ordered by installed_rank ASC.
	SelectHistorySQL(tableName string) string

	// AcquireLock acquires a session-level lock to prevent concurrent migrations.
	AcquireLock(ctx context.Context, conn *sql.Conn, tableName string, timeout time.Duration) error

	// ReleaseLock releases the acquired session-level lock.
	ReleaseLock(ctx context.Context, conn *sql.Conn, tableName string) error

	// GetDBUser retrieves the current database user name.
	GetDBUser(ctx context.Context, conn *sql.Conn) string

	// Refresh drops all user tables in the database schema.
	Refresh(ctx context.Context, conn *sql.Conn) error
}
