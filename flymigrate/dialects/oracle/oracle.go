package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// OracleDialect implements the Dialect interface for Oracle.
type OracleDialect struct{}

func oracleIndexName(prefix, tableName string) string {
	name := prefix + "_" + tableName
	if len(name) > 30 {
		name = name[:30]
	}
	return strings.ToUpper(name)
}

func (d *OracleDialect) EnsureTable(ctx context.Context, conn *sql.Conn, tableName string) error {
	upperTable := strings.ToUpper(tableName)
	idxSuccess := oracleIndexName("idx_rf_succ", tableName)
	idxVersion := oracleIndexName("idx_rf_vers", tableName)

	// PL/SQL block to safely check and create the history table and indexes
	plsql := fmt.Sprintf(`
		DECLARE
			tbl_count NUMBER;
			idx_count NUMBER;
		BEGIN
			SELECT COUNT(*) INTO tbl_count FROM user_tables WHERE table_name = '%s';
			IF tbl_count = 0 THEN
				EXECUTE IMMEDIATE 'CREATE TABLE "%s" (
					installed_rank NUMBER(19) NOT NULL,
					version VARCHAR2(50),
					description VARCHAR2(255) NOT NULL,
					type VARCHAR2(20) NOT NULL,
					script VARCHAR2(1000) NOT NULL,
					checksum NUMBER(19),
					installed_by VARCHAR2(100),
					installed_on TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
					execution_time_ms NUMBER(19) NOT NULL,
					success NUMBER(1) NOT NULL,
					CONSTRAINT pk_%s PRIMARY KEY (installed_rank)
				)';
			END IF;

			SELECT COUNT(*) INTO idx_count FROM user_indexes WHERE index_name = '%s';
			IF idx_count = 0 THEN
				EXECUTE IMMEDIATE 'CREATE INDEX "%s" ON "%s"(success)';
			END IF;

			SELECT COUNT(*) INTO idx_count FROM user_indexes WHERE index_name = '%s';
			IF idx_count = 0 THEN
				EXECUTE IMMEDIATE 'CREATE INDEX "%s" ON "%s"(version)';
			END IF;
		END;
	`, upperTable, upperTable, upperTable, idxSuccess, idxSuccess, upperTable, idxVersion, idxVersion, upperTable)

	_, err := conn.ExecContext(ctx, plsql)
	if err != nil {
		return fmt.Errorf("oracle/ensure_table: failed to initialize table: %w", err)
	}

	return nil
}

func (d *OracleDialect) InsertMigrationSQL(tableName string) string {
	upperTable := strings.ToUpper(tableName)
	// Oracle uses standard Positional parameters (e.g. :1, :2...) or bind variables.
	// Many Go oracle drivers support :1, :2, etc. or ? depending on configuration.
	// Using :1, :2, :3, :4, :5, :6, :7, :8 is standard for Oracle SQL.
	return fmt.Sprintf(`
		INSERT INTO "%s" (installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time_ms, success)
		VALUES (
			(SELECT COALESCE(MAX(installed_rank), 0) + 1 FROM "%s"),
			:1, :2, :3, :4, :5, :6, CURRENT_TIMESTAMP, :7, :8
		)
	`, upperTable, upperTable)
}

func (d *OracleDialect) SelectHistorySQL(tableName string) string {
	upperTable := strings.ToUpper(tableName)
	return fmt.Sprintf(`
		SELECT installed_rank, version, description, type, script, checksum, installed_by, installed_on, execution_time_ms, 
		       CASE WHEN success = 1 THEN 1 ELSE 0 END as success
		FROM "%s"
		ORDER BY installed_rank ASC
	`, upperTable)
}

func (d *OracleDialect) AcquireLock(ctx context.Context, conn *sql.Conn, tableName string, timeout time.Duration) error {
	timeoutSec := int(timeout.Seconds())
	if timeoutSec <= 0 {
		timeoutSec = 10
	}

	// Oracle DBMS_LOCK logic:
	// We allocate a unique handle for our lock name and then request it.
	// lockmode = 6 (X_MODE - Exclusive Mode)
	// release_on_commit = FALSE (persists lock beyond the current transaction)
	plsql := `
		DECLARE
			lockhandle VARCHAR2(128);
			res NUMBER;
		BEGIN
			DBMS_LOCK.ALLOCATE_UNIQUE(:1, lockhandle);
			res := DBMS_LOCK.REQUEST(lockhandle, 6, :2, FALSE);
			IF res <> 0 AND res <> 4 THEN
				raise_application_error(-20000, 'DBMS_LOCK.REQUEST failed with code: ' || res);
			END IF;
		END;
	`
	lockName := strings.ToUpper(tableName) + "_LOCK"
	_, err := conn.ExecContext(ctx, plsql, lockName, timeoutSec)
	if err != nil {
		return fmt.Errorf("oracle/lock: failed to acquire lock using DBMS_LOCK: %w", err)
	}
	return nil
}

func (d *OracleDialect) ReleaseLock(ctx context.Context, conn *sql.Conn, tableName string) error {
	plsql := `
		DECLARE
			lockhandle VARCHAR2(128);
			res NUMBER;
		BEGIN
			DBMS_LOCK.ALLOCATE_UNIQUE(:1, lockhandle);
			res := DBMS_LOCK.RELEASE(lockhandle);
		END;
	`
	lockName := strings.ToUpper(tableName) + "_LOCK"
	_, err := conn.ExecContext(ctx, plsql, lockName)
	if err != nil {
		return fmt.Errorf("oracle/lock: failed to release lock using DBMS_LOCK: %w", err)
	}
	return nil
}

func (d *OracleDialect) GetDBUser(ctx context.Context, conn *sql.Conn) string {
	var user string
	_ = conn.QueryRowContext(ctx, "SELECT USER FROM DUAL").Scan(&user)
	if user == "" {
		return "rapidfly"
	}
	return user
}

func (d *OracleDialect) Refresh(ctx context.Context, conn *sql.Conn) error {
	fmt.Println("rapidfly/migrate/oracle: Refreshing the database...")

	rows, err := conn.QueryContext(ctx, "SELECT table_name FROM user_tables")
	if err != nil {
		return fmt.Errorf("oracle/refresh: failed to query user tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("oracle/refresh: failed to scan table name: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("oracle/refresh: failed to read tables: %w", err)
	}

	if len(tables) == 0 {
		fmt.Println("rapidfly/migrate/oracle: No tables found to drop.")
		return nil
	}

	fmt.Printf("rapidfly/migrate/oracle: Found tables: %s\n", strings.Join(tables, ", "))

	for _, table := range tables {
		query := fmt.Sprintf(`DROP TABLE "%s" CASCADE CONSTRAINTS`, strings.ReplaceAll(table, `"`, `""`))
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("oracle/refresh: failed to drop table %s: %w", table, err)
		}
		fmt.Printf("Dropped table: %s\n", table)
	}

	fmt.Println("rapidfly/migrate/oracle: Database refreshed successfully!")
	return nil
}
