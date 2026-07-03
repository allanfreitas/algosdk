package flymigrate

import (
	"context"
	"fmt"
)

// Repair repairs metadata inconsistencies.
func (rf *Migrator) Repair(ctx context.Context) error {
	pool, selfCreated, err := rf.getPool(ctx)
	if err != nil {
		return err
	}
	if selfCreated {
		defer pool.Close()
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to acquire connection: %w", err)
	}
	defer conn.Release()

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

	deleteFailedQuery := fmt.Sprintf("DELETE FROM %s WHERE success = false", rf.config.TableName)
	tag, err := conn.Exec(ctx, deleteFailedQuery)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to delete failed records: %w", err)
	}
	if tag.RowsAffected() > 0 {
		fmt.Printf("rapidfly/migrate: removed %d failed migration record(s)\n", tag.RowsAffected())
	}

	history, err := rf.fetchHistory(ctx, conn)
	if err != nil {
		return err
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
			updateQuery := fmt.Sprintf("UPDATE %s SET checksum = $1 WHERE installed_rank = $2", rf.config.TableName)
			_, err = conn.Exec(ctx, updateQuery, localMig.Checksum, entry.InstalledRank)
			if err != nil {
				return fmt.Errorf("rapidfly/migrate: failed to update checksum for %s: %w", entry.Script, err)
			}
			fmt.Printf("rapidfly/migrate: updated checksum for %s (rank %d)\n", entry.Script, entry.InstalledRank)
		}
	}

	return nil
}
