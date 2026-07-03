package flymigrate

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config represents the configuration for the RapidFly migrator.
type Config struct {
	Driver          string
	DSN             string
	MigrationsPath  string
	TableName       string
	ValidateOnStart bool
	LockTimeout     time.Duration

	// Optional embed.FS or custom filesystem
	FS fs.FS
	// Optional existing database connection pool to reuse
	Pool *pgxpool.Pool
	// Optional flag to skip checksum validation
	SkipChecksum bool
}

// Migrator is the main engine for executing migrations.
type Migrator struct {
	config Config
}

// Migration represents a parsed migration file.
type Migration struct {
	Version     string // e.g. "001", empty for repeatable
	Description string // e.g. "CreateUsersTable"
	Type        string // "SQL" or "REPEATABLE"
	Script      string // filename, e.g. "V001__CreateUsersTable.sql"
	Content     string
	Checksum    int64
}

// HistoryEntry represents a row in the metadata schema history table.
type HistoryEntry struct {
	InstalledRank   int64
	Version         *string // Nullable for repeatable migrations
	Description     string
	Type            string
	Script          string
	Checksum        int64
	InstalledBy     string
	InstalledOn     time.Time
	ExecutionTimeMs int64
	Success         bool
}

// StatusEntry represents a consolidated migration status.
type StatusEntry struct {
	Version     string
	Description string
	Script      string
	Type        string
	State       string // "Success", "Failed", "Pending", "Missing", "ChecksumMismatch"
	InstalledOn *time.Time
}

// New creates a new Migrator instance.
func New(config Config) *Migrator {
	if config.TableName == "" {
		config.TableName = "rapidfly_schema_history"
	}
	if config.MigrationsPath == "" {
		config.MigrationsPath = "database/migrations"
	}
	return &Migrator{
		config: config,
	}
}

// fetchHistory retrieves all migration history from the database.
func (rf *Migrator) fetchHistory(ctx context.Context, conn *pgxpool.Conn) ([]HistoryEntry, error) {
	if err := rf.validateTableName(); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time_ms, success
		FROM %s
		ORDER BY installed_rank ASC
	`, rf.config.TableName)

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("rapidfly/migrate: failed to fetch history: %w", err)
	}
	defer rows.Close()

	var history []HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		var versionVal *string
		err := rows.Scan(
			&entry.InstalledRank,
			&versionVal,
			&entry.Description,
			&entry.Type,
			&entry.Script,
			&entry.Checksum,
			&entry.InstalledBy,
			&entry.InstalledOn,
			&entry.ExecutionTimeMs,
			&entry.Success,
		)
		if err != nil {
			return nil, fmt.Errorf("rapidfly/migrate: failed to scan history entry: %w", err)
		}
		entry.Version = versionVal
		history = append(history, entry)
	}
	return history, nil
}

// getDBUser gets the current connected PostgreSQL user.
func (rf *Migrator) getDBUser(ctx context.Context, conn *pgxpool.Conn) string {
	var user string
	_ = conn.QueryRow(ctx, "SELECT CURRENT_USER").Scan(&user)
	if user == "" {
		return "rapidfly"
	}
	return user
}

// recordMigration inserts a record of a migration execution.
func (rf *Migrator) recordMigration(ctx context.Context, conn *pgxpool.Conn, m *Migration, executionTimeMs int64, success bool, installedBy string) error {
	var versionVal *string
	if m.Version != "" {
		versionVal = &m.Version
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time_ms, success)
		VALUES (
			(SELECT COALESCE(MAX(installed_rank), 0) + 1 FROM %s),
			$1, $2, $3, $4, $5, $6, NOW(), $7, $8
		)
	`, rf.config.TableName, rf.config.TableName)

	_, err := conn.Exec(ctx, query, versionVal, m.Description, m.Type, m.Script, m.Checksum, installedBy, executionTimeMs, success)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to record migration history: %w", err)
	}
	return nil
}

// validateState performs validation logic on loaded migrations and DB history.
func (rf *Migrator) validateState(localMigs []*Migration, history []HistoryEntry) error {
	// 1. Check for failed migrations
	for _, entry := range history {
		if !entry.Success {
			return fmt.Errorf("rapidfly/migrate: failed migration detected")
		}
	}

	// Maps for easy lookup
	localMap := make(map[string]*Migration)
	localVersionMap := make(map[string]*Migration)
	for _, lm := range localMigs {
		localMap[lm.Script] = lm
		if lm.Type == "SQL" {
			localVersionMap[lm.Version] = lm
		}
	}

	// 2. Validate applied versioned/repeatable migrations
	for _, entry := range history {
		if entry.Type == "SQL" && entry.Version != nil {
			localMig, exists := localVersionMap[*entry.Version]
			if !exists {
				// State is Missing, which is not an automatic hard failure under "Validation Rules"
				// unless specified. We skip.
				continue
			}

			// Validate checksum
			if !rf.config.SkipChecksum {
				if entry.Checksum != localMig.Checksum {
					return fmt.Errorf("rapidfly/migrate: checksum mismatch for %s", localMig.Script)
				}
			}
		}
	}

	return nil
}

// getLatestRepeatableEntry returns the latest history entry for a repeatable script.
func getLatestRepeatableEntry(history []HistoryEntry, script string) *HistoryEntry {
	var latest *HistoryEntry
	for i := range history {
		entry := &history[i]
		if entry.Type == "REPEATABLE" && entry.Script == script {
			if latest == nil || entry.InstalledRank > latest.InstalledRank {
				latest = entry
			}
		}
	}
	return latest
}

// getPool returns the connection pool.
func (rf *Migrator) getPool(ctx context.Context) (*pgxpool.Pool, bool, error) {
	if rf.config.Pool != nil {
		return rf.config.Pool, false, nil
	}
	if rf.config.DSN == "" {
		return nil, false, fmt.Errorf("rapidfly/migrate: DSN or Pool must be configured")
	}
	poolConfig, err := pgxpool.ParseConfig(rf.config.DSN)
	if err != nil {
		return nil, false, fmt.Errorf("rapidfly/migrate: failed to parse DSN: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, false, fmt.Errorf("rapidfly/migrate: failed to connect to database: %w", err)
	}
	return pool, true, nil
}

// executeMigration runs a single migration SQL in a transaction and records results.
func (rf *Migrator) executeMigration(ctx context.Context, conn *pgxpool.Conn, m *Migration, installedBy string) error {
	start := time.Now()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to start transaction: %w", err)
	}

	_, err = tx.Exec(ctx, m.Content)
	elapsedMs := time.Since(start).Milliseconds()

	if err != nil {
		_ = tx.Rollback(ctx)
		_ = rf.recordMigration(ctx, conn, m, elapsedMs, false, installedBy)
		return fmt.Errorf("rapidfly/migrate: failed migration %s: %w", m.Script, err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to commit transaction: %w", err)
	}

	err = rf.recordMigration(ctx, conn, m, elapsedMs, true, installedBy)
	if err != nil {
		return err
	}

	return nil
}
