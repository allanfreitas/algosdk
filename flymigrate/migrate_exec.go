package flymigrate

import (
	"context"
	"fmt"
)

// Migrate executes pending and changed migrations under an advisory lock.
func (rf *Migrator) Migrate(ctx context.Context) error {
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

	if rf.config.ValidateOnStart {
		localMigs, err := rf.loadMigrations()
		if err != nil {
			return err
		}
		history, err := rf.fetchHistory(ctx, conn)
		if err != nil {
			return err
		}
		if err := rf.validateState(localMigs, history); err != nil {
			return err
		}
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

	history, err := rf.fetchHistory(ctx, conn)
	if err != nil {
		return err
	}

	if err := rf.validateState(localMigs, history); err != nil {
		return err
	}

	installedBy := rf.getDBUser(ctx, conn)

	appliedVersions := make(map[string]bool)
	for _, entry := range history {
		if entry.Type == "SQL" && entry.Version != nil && entry.Success {
			appliedVersions[*entry.Version] = true
		}
	}

	var versionedMigs []*Migration
	var repeatableMigs []*Migration
	for _, lm := range localMigs {
		switch lm.Type {
		case "SQL":
			versionedMigs = append(versionedMigs, lm)
		case "REPEATABLE":
			repeatableMigs = append(repeatableMigs, lm)
		}
	}

	for _, vm := range versionedMigs {
		if appliedVersions[vm.Version] {
			continue
		}
		if err := rf.executeMigration(ctx, conn, vm, installedBy); err != nil {
			return err
		}
	}

	for _, rm := range repeatableMigs {
		latest := getLatestRepeatableEntry(history, rm.Script)
		if latest == nil || !latest.Success || latest.Checksum != rm.Checksum {
			if err := rf.executeMigration(ctx, conn, rm, installedBy); err != nil {
				return err
			}
		}
	}

	return nil
}
