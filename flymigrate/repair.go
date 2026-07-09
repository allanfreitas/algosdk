package flymigrate

import (
	"context"
	"fmt"
	"strings"
)

// Repair repairs metadata inconsistencies.
func (rf *Migrator) Repair(ctx context.Context) error {
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

	if err := rf.acquireLock(ctx, conn); err != nil {
		return err
	}
	defer func() {
		_ = rf.releaseLock(ctx, conn)
	}()

	localMigs, err := rf.loadMigrations()
	if err != nil {
		return err
	}

	localMap := make(map[string]*Migration)
	localVersionMap := make(map[string]*Migration)
	for _, lm := range localMigs {
		localMap[lm.Script] = lm
		if lm.Type == "SQL" {
			localVersionMap[lm.Version] = lm
		}
	}

	deleteFailedQuery := fmt.Sprintf("DELETE FROM %s WHERE success = %s", rf.config.TableName, rf.sqlBool(false))
	tag, err := conn.ExecContext(ctx, deleteFailedQuery)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to delete failed records: %w", err)
	}
	rowsAffected, _ := tag.RowsAffected()
	if rowsAffected > 0 {
		fmt.Printf("rapidfly/migrate: removed %d failed migration record(s)\n", rowsAffected)
	}

	history, err := rf.fetchHistory(ctx, conn)
	if err != nil {
		return err
	}

	var updateQuery string
	dialectType := fmt.Sprintf("%T", rf.config.Dialect)
	if strings.Contains(dialectType, "PostgresDialect") {
		updateQuery = fmt.Sprintf("UPDATE %s SET checksum = $1 WHERE installed_rank = $2", rf.config.TableName)
	} else if strings.Contains(dialectType, "OracleDialect") {
		updateQuery = fmt.Sprintf("UPDATE %s SET checksum = :1 WHERE installed_rank = :2", rf.config.TableName)
	} else {
		updateQuery = fmt.Sprintf("UPDATE %s SET checksum = ? WHERE installed_rank = ?", rf.config.TableName)
	}

	for _, entry := range history {
		var localMig *Migration
		var exists bool

		if entry.Type == "SQL" && entry.Version != nil {
			localMig, exists = localVersionMap[*entry.Version]
		} else {
			localMig, exists = localMap[entry.Script]
		}

		if !exists {
			fmt.Printf("rapidfly/migrate: marked missing file: %s (rank %d)\n", entry.Script, entry.InstalledRank)
			continue
		}

		if entry.Checksum != localMig.Checksum {
			_, err = conn.ExecContext(ctx, updateQuery, localMig.Checksum, entry.InstalledRank)
			if err != nil {
				return fmt.Errorf("rapidfly/migrate: failed to update checksum for %s: %w", entry.Script, err)
			}
			fmt.Printf("rapidfly/migrate: updated checksum for %s (rank %d)\n", entry.Script, entry.InstalledRank)
		}
	}

	return nil
}

func (rf *Migrator) sqlBool(val bool) string {
	dialectType := fmt.Sprintf("%T", rf.config.Dialect)
	if strings.Contains(dialectType, "OracleDialect") {
		if val {
			return "1"
		}
		return "0"
	}
	if val {
		return "true"
	}
	return "false"
}
