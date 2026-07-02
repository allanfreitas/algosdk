package migrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Refresh drops all tables in the public schema.
func (rf *Migrator) Refresh(ctx context.Context) error {
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

	if err := rf.refreshDatabase(ctx, conn); err != nil {
		return err
	}

	return nil
}

func (rf *Migrator) refreshDatabase(ctx context.Context, conn *pgxpool.Conn) error {
	fmt.Println("rapidfly/migrate: Refreshing the database...")

	rows, err := conn.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	if err != nil {
		return fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("failed to scan table: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read table list: %w", err)
	}

	if len(tables) == 0 {
		fmt.Println("rapidfly/migrate: No tables found to drop.")
		return nil
	}

	fmt.Printf("rapidfly/migrate: Found tables: %s\n", strings.Join(tables, ", "))

	for _, table := range tables {
		query := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", pgx.Identifier{table}.Sanitize())
		if _, err := conn.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
		fmt.Printf("Dropped table: %s\n", table)
	}

	fmt.Println("rapidfly/migrate: Database refreshed successfully!")
	return nil
}