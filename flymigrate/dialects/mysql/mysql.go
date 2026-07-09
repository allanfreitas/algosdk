package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// MySQLDialect implements the Dialect interface for MySQL.
type MySQLDialect struct{}

func (d *MySQLDialect) EnsureTable(ctx context.Context, conn *sql.Conn, tableName string) error {
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
		return fmt.Errorf("mysql/ensure_table: failed to create table: %w", err)
	}

	// Safely create indexes check if statistical metadata indicates they already exist
	var count int
	_ = conn.QueryRowContext(ctx, `
		SELECT COUNT(1) 
		FROM information_schema.statistics 
		WHERE table_schema = DATABASE() 
		  AND table_name = ? 
		  AND index_name = ?
	`, tableName, "idx_rapidfly_success_"+tableName).Scan(&count)
	if count == 0 {
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("CREATE INDEX %s ON %s(success)", "idx_rapidfly_success_"+tableName, tableName))
	}

	count = 0
	_ = conn.QueryRowContext(ctx, `
		SELECT COUNT(1) 
		FROM information_schema.statistics 
		WHERE table_schema = DATABASE() 
		  AND table_name = ? 
		  AND index_name = ?
	`, tableName, "idx_rapidfly_version_"+tableName).Scan(&count)
	if count == 0 {
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("CREATE INDEX %s ON %s(version)", "idx_rapidfly_version_"+tableName, tableName))
	}

	return nil
}

func (d *MySQLDialect) InsertMigrationSQL(tableName string) string {
	return fmt.Sprintf(`
		INSERT INTO %s (installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time_ms, success)
		VALUES (
			(SELECT COALESCE(MAX(installed_rank), 0) + 1 FROM (SELECT installed_rank FROM %s) AS tmp),
			?, ?, ?, ?, ?, ?, NOW(), ?, ?
		)
	`, tableName, tableName)
}

func (d *MySQLDialect) SelectHistorySQL(tableName string) string {
	return fmt.Sprintf(`
		SELECT installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time_ms, success
		FROM %s
		ORDER BY installed_rank ASC
	`, tableName)
}

func (d *MySQLDialect) AcquireLock(ctx context.Context, conn *sql.Conn, tableName string, timeout time.Duration) error {
	timeoutSec := int(timeout.Seconds())
	if timeoutSec <= 0 {
		timeoutSec = 10
	}

	var acquired int
	err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", tableName+"_lock", timeoutSec).Scan(&acquired)
	if err != nil {
		return fmt.Errorf("mysql/lock: failed to acquire lock: %w", err)
	}
	if acquired != 1 {
		return fmt.Errorf("mysql/lock: failed to acquire lock (acquired status %d)", acquired)
	}
	return nil
}

func (d *MySQLDialect) ReleaseLock(ctx context.Context, conn *sql.Conn, tableName string) error {
	var released interface{}
	err := conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", tableName+"_lock").Scan(&released)
	if err != nil {
		return fmt.Errorf("mysql/lock: failed to release lock: %w", err)
	}
	return nil
}

func (d *MySQLDialect) GetDBUser(ctx context.Context, conn *sql.Conn) string {
	var user string
	_ = conn.QueryRowContext(ctx, "SELECT USER()").Scan(&user)
	if user == "" {
		return "rapidfly"
	}
	return user
}

func (d *MySQLDialect) Refresh(ctx context.Context, conn *sql.Conn) error {
	fmt.Println("rapidfly/migrate/mysql: Refreshing the database...")

	rows, err := conn.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return fmt.Errorf("mysql/refresh: failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("mysql/refresh: failed to scan table: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("mysql/refresh: failed to read table list: %w", err)
	}

	if len(tables) == 0 {
		fmt.Println("rapidfly/migrate/mysql: No tables found to drop.")
		return nil
	}

	fmt.Printf("rapidfly/migrate/mysql: Found tables: %s\n", strings.Join(tables, ", "))

	_, _ = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0")
	defer func() {
		_, _ = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1")
	}()

	for _, table := range tables {
		query := fmt.Sprintf("DROP TABLE IF EXISTS `%s` CASCADE", strings.ReplaceAll(table, "`", "``"))
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("mysql/refresh: failed to drop table %s: %w", table, err)
		}
		fmt.Printf("Dropped table: %s\n", table)
	}

	fmt.Println("rapidfly/migrate/mysql: Database refreshed successfully!")
	return nil
}
