package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ensureMetadataTable creates the history table if it does not exist.
func (rf *Migrator) ensureMetadataTable(ctx context.Context, conn *pgxpool.Conn) error {
	if err := rf.validateTableName(); err != nil {
		return err
	}

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
	`, rf.config.TableName)

	_, err := conn.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("rapidfly/migrate: failed to create metadata table: %w", err)
	}

	// Add indexes
	idxSuccess := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_rapidfly_success_%s ON %s(success)", rf.config.TableName, rf.config.TableName)
	idxVersion := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_rapidfly_version_%s ON %s(version)", rf.config.TableName, rf.config.TableName)
	_, _ = conn.Exec(ctx, idxSuccess)
	_, _ = conn.Exec(ctx, idxVersion)

	return nil
}
