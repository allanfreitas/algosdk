package flymigrate

import (
	"context"
	"fmt"
	"hash/crc32"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// getLockKey computes the 64-bit advisory lock key from the table name.
func (rf *Migrator) getLockKey() int64 {
	return int64(crc32.ChecksumIEEE([]byte(rf.config.TableName)))
}

// acquireLock acquires a session-level advisory lock.
func (rf *Migrator) acquireLock(ctx context.Context, conn *pgxpool.Conn) error {
	lockKey := rf.getLockKey()

	if rf.config.LockTimeout <= 0 {
		_, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey)
		if err != nil {
			return fmt.Errorf("rapidfly/migrate: failed to acquire advisory lock: %w", err)
		}
		return nil
	}

	start := time.Now()
	for {
		var acquired bool
		err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
		if err != nil {
			return fmt.Errorf("rapidfly/migrate: failed to check advisory lock: %w", err)
		}
		if acquired {
			return nil
		}
		if time.Since(start) >= rf.config.LockTimeout {
			return fmt.Errorf("rapidfly/migrate: lock acquisition timed out after %v", rf.config.LockTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// releaseLock releases the session-level advisory lock.
func (rf *Migrator) releaseLock(ctx context.Context, conn *pgxpool.Conn) error {
	lockKey := rf.getLockKey()
	var released bool
	err := conn.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", lockKey).Scan(&released)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to release advisory lock: %w", err)
	}
	return nil
}
