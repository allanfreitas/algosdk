package flymigrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/allanfreitas/algosdk/flymigrate/dialects/mysql"
	"github.com/allanfreitas/algosdk/flymigrate/dialects/oracle"
	"github.com/allanfreitas/algosdk/flymigrate/dialects/postgres"
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
	// Optional existing database connection to reuse
	DB *sql.DB
	// Strategy dialect to use (Postgres, MySQL, Oracle, etc.)
	Dialect Dialect
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
	if config.Dialect == nil {
		config.Dialect = detectDialect(&config)
	}
	return &Migrator{
		config: config,
	}
}

// detectDialect infers the database dialect to use based on the driver.
func detectDialect(config *Config) Dialect {
	var driverName string
	if config.DB != nil {
		driverName = fmt.Sprintf("%T", config.DB.Driver())
	} else {
		driverName = config.Driver
	}

	driverName = strings.ToLower(driverName)
	if strings.Contains(driverName, "mysql") {
		return &mysql.MySQLDialect{}
	}
	if strings.Contains(driverName, "oracle") || strings.Contains(driverName, "ora") || strings.Contains(driverName, "godror") {
		return &oracle.OracleDialect{}
	}
	// Default to PostgreSQL
	return &postgres.PostgresDialect{}
}

// fetchHistory retrieves all migration history from the database.
func (rf *Migrator) fetchHistory(ctx context.Context, conn *sql.Conn) ([]HistoryEntry, error) {
	if err := rf.validateTableName(); err != nil {
		return nil, err
	}

	query := rf.config.Dialect.SelectHistorySQL(rf.config.TableName)

	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("rapidfly/migrate: failed to fetch history: %w", err)
	}
	defer rows.Close()

	var history []HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		var versionVal *string
		var successVal interface{}
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
			&successVal,
		)
		if err != nil {
			return nil, fmt.Errorf("rapidfly/migrate: failed to scan history entry: %w", err)
		}

		// Convert successVal dynamically (Postgres boolean vs MySQL tinyint vs Oracle number)
		switch v := successVal.(type) {
		case bool:
			entry.Success = v
		case int64:
			entry.Success = v != 0
		case int32:
			entry.Success = v != 0
		case int:
			entry.Success = v != 0
		case uint8:
			entry.Success = v != 0
		default:
			entry.Success = fmt.Sprintf("%v", v) == "true" || fmt.Sprintf("%v", v) == "1"
		}

		entry.Version = versionVal
		history = append(history, entry)
	}
	return history, nil
}

// getDBUser gets the current connected database user.
func (rf *Migrator) getDBUser(ctx context.Context, conn *sql.Conn) string {
	return rf.config.Dialect.GetDBUser(ctx, conn)
}

// recordMigration inserts a record of a migration execution.
func (rf *Migrator) recordMigration(ctx context.Context, conn *sql.Conn, m *Migration, executionTimeMs int64, success bool, installedBy string) error {
	var versionVal *string
	if m.Version != "" {
		versionVal = &m.Version
	}

	query := rf.config.Dialect.InsertMigrationSQL(rf.config.TableName)

	var successArg interface{} = success
	if _, ok := rf.config.Dialect.(*oracle.OracleDialect); ok {
		if success {
			successArg = 1
		} else {
			successArg = 0
		}
	}

	_, err := conn.ExecContext(ctx, query, versionVal, m.Description, m.Type, m.Script, m.Checksum, installedBy, executionTimeMs, successArg)
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

// getDB returns the database connection pool.
func (rf *Migrator) getDB(ctx context.Context) (*sql.DB, bool, error) {
	if rf.config.DB != nil {
		return rf.config.DB, false, nil
	}
	if rf.config.DSN == "" {
		return nil, false, fmt.Errorf("rapidfly/migrate: DSN or DB must be configured")
	}
	if rf.config.Driver == "" {
		return nil, false, fmt.Errorf("rapidfly/migrate: Driver must be configured when using DSN")
	}
	db, err := sql.Open(rf.config.Driver, rf.config.DSN)
	if err != nil {
		return nil, false, fmt.Errorf("rapidfly/migrate: failed to connect to database: %w", err)
	}
	return db, true, nil
}

// executeMigration runs a single migration SQL in a transaction and records results.
func (rf *Migrator) executeMigration(ctx context.Context, conn *sql.Conn, m *Migration, installedBy string) error {
	start := time.Now()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to start transaction: %w", err)
	}

	_, err = tx.ExecContext(ctx, m.Content)
	elapsedMs := time.Since(start).Milliseconds()

	if err != nil {
		_ = tx.Rollback()
		_ = rf.recordMigration(ctx, conn, m, elapsedMs, false, installedBy)
		return fmt.Errorf("rapidfly/migrate: failed migration %s: %w", m.Script, err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to commit transaction: %w", err)
	}

	err = rf.recordMigration(ctx, conn, m, elapsedMs, true, installedBy)
	if err != nil {
		return err
	}

	return nil
}
