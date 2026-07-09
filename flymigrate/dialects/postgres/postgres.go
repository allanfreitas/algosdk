package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"hash/crc32"
	"strings"
	"time"
)

// PostgresDialect implements the Dialect interface for PostgreSQL.
type PostgresDialect struct{}

func (d *PostgresDialect) EnsureTable(ctx context.Context, conn *sql.Conn, tableName string) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			installed_rank BIGINT NOT NULL,
			version VARCHAR(50),
			description VARCHAR(255) NOT NULL,
			type VARCHAR(20) NOT NULL,
			script VARCHAR(1000) NOT NULL,
			checksum BIGINT,
			installed_by VARCHAR(100),
			installed_on TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			execution_time_ms BIGINT NOT NULL,
			success BOOLEAN NOT NULL,
			PRIMARY KEY (installed_rank)
		);
	`, tableName)

	_, err := conn.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("postgres/ensure_table: failed to create table: %w", err)
	}

	// Add indexes
	idxSuccess := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_rapidfly_success_%s ON %s(success)", tableName, tableName)
	idxVersion := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_rapidfly_version_%s ON %s(version)", tableName, tableName)
	_, _ = conn.ExecContext(ctx, idxSuccess)
	_, _ = conn.ExecContext(ctx, idxVersion)

	return nil
}

func (d *PostgresDialect) InsertMigrationSQL(tableName string) string {
	return fmt.Sprintf(`
		INSERT INTO %s (installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time_ms, success)
		VALUES (
			(SELECT COALESCE(MAX(installed_rank), 0) + 1 FROM %s),
			$1, $2, $3, $4, $5, $6, NOW(), $7, $8
		)
	`, tableName, tableName)
}

func (d *PostgresDialect) SelectHistorySQL(tableName string) string {
	return fmt.Sprintf(`
		SELECT installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time_ms, success
		FROM %s
		ORDER BY installed_rank ASC
	`, tableName)
}

func (d *PostgresDialect) AcquireLock(ctx context.Context, conn *sql.Conn, tableName string, timeout time.Duration) error {
	lockKey := int64(crc32.ChecksumIEEE([]byte(tableName)))

	if timeout <= 0 {
		_, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey)
		if err != nil {
			return fmt.Errorf("postgres/lock: failed to acquire advisory lock: %w", err)
		}
		return nil
	}

	start := time.Now()
	for {
		var acquired bool
		err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
		if err != nil {
			return fmt.Errorf("postgres/lock: failed to check advisory lock: %w", err)
		}
		if acquired {
			return nil
		}
		if time.Since(start) >= timeout {
			return fmt.Errorf("postgres/lock: lock acquisition timed out after %v", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (d *PostgresDialect) ReleaseLock(ctx context.Context, conn *sql.Conn, tableName string) error {
	lockKey := int64(crc32.ChecksumIEEE([]byte(tableName)))
	var released bool
	err := conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", lockKey).Scan(&released)
	if err != nil {
		return fmt.Errorf("postgres/lock: failed to release advisory lock: %w", err)
	}
	return nil
}

func (d *PostgresDialect) GetDBUser(ctx context.Context, conn *sql.Conn) string {
	var user string
	_ = conn.QueryRowContext(ctx, "SELECT CURRENT_USER").Scan(&user)
	if user == "" {
		return "rapidfly"
	}
	return user
}

func (d *PostgresDialect) Refresh(ctx context.Context, conn *sql.Conn) error {
	fmt.Println("rapidfly/migrate/postgres: Refreshing the database...")

	rows, err := conn.QueryContext(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	if err != nil {
		return fmt.Errorf("postgres/refresh: failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("postgres/refresh: failed to scan table: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("postgres/refresh: failed to read table list: %w", err)
	}

	if len(tables) == 0 {
		fmt.Println("rapidfly/migrate/postgres: No tables found to drop.")
		return nil
	}

	fmt.Printf("rapidfly/migrate/postgres: Found tables: %s\n", strings.Join(tables, ", "))

	for _, table := range tables {
		query := fmt.Sprintf(`DROP TABLE IF EXISTS "%s" CASCADE`, strings.ReplaceAll(table, `"`, `""`))
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("postgres/refresh: failed to drop table %s: %w", table, err)
		}
		fmt.Printf("Dropped table: %s\n", table)
	}

	fmt.Println("rapidfly/migrate/postgres: Database refreshed successfully!")
	return nil
}
