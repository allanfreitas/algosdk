package migrate

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newMigrator creates a configured migrate.Migrator instance using embedded migrations and the database pool.
func newMigrator(db *pgxpool.Pool, fsys fs.FS, migrationsPath string) *Migrator {
	return New(Config{
		Pool:            db,
		FS:              fsys,
		MigrationsPath:  migrationsPath,
		ValidateOnStart: false, // We'll handle validation manually in the CLI commands
		SkipChecksum:    false,
	})
}

// MigrateDatabase executes pending migrations.
func MigrateDatabase(ctx context.Context, db *pgxpool.Pool, fsys fs.FS, migrationsPath string, args []string) error {
	rf := newMigrator(db, fsys, migrationsPath)
	fmt.Println("rapidfly: Starting database migration...")

	fs := flag.NewFlagSet("db:migrate", flag.ContinueOnError)
	refreshDb := fs.Bool("refresh", false, "Refresh the database by dropping all tables")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("failed to parse arguments: %w", err)
	}

	if *refreshDb {
		if err := rf.Refresh(ctx); err != nil {
			return fmt.Errorf("rapidfly refresh error: %w", err)
		}
	}

	// Print info of migrations before starting
	info, err := rf.Info(ctx)
	if err == nil {
		pendingCount := 0
		for _, entry := range info {
			if entry.State == "Pending" {
				pendingCount++
			}
		}
		fmt.Printf("rapidfly: Found %d pending migration(s) to execute.\n", pendingCount)
	}

	err = rf.Migrate(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly migrate error: %w", err)
	}

	fmt.Println("rapidfly: Migration completed successfully!")
	return nil
}

// ValidateDatabase checks database schema history consistency.
func ValidateDatabase(ctx context.Context, db *pgxpool.Pool, fsys fs.FS, migrationsPath string) error {
	rf := newMigrator(db, fsys, migrationsPath)
	fmt.Println("rapidfly: Validating schema history consistency...")

	err := rf.Validate(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly validation failure: %w", err)
	}

	fmt.Println("rapidfly: Schema history is consistent.")
	return nil
}

// InfoDatabase displays schema migration status.
func InfoDatabase(ctx context.Context, db *pgxpool.Pool, fsys fs.FS, migrationsPath string) error {
	rf := newMigrator(db, fsys, migrationsPath)
	info, err := rf.Info(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly info error: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Version\tDescription\tScript\tType\tState\tInstalled On")
	fmt.Fprintln(w, "-------\t-----------\t------\t----\t-----\t------------")

	for _, entry := range info {
		version := entry.Version
		if version == "" {
			version = "-"
		}

		installedOn := "-"
		if entry.InstalledOn != nil {
			installedOn = entry.InstalledOn.Format("2006-01-02 15:04:05")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			version,
			entry.Description,
			entry.Script,
			entry.Type,
			entry.State,
			installedOn,
		)
	}
	w.Flush()
	return nil
}

// RepairDatabase repairs metadata table inconsistencies.
func RepairDatabase(ctx context.Context, db *pgxpool.Pool, fsys fs.FS, migrationsPath string) error {
	rf := newMigrator(db, fsys, migrationsPath)
	fmt.Println("rapidfly: Repairing metadata inconsistencies...")

	err := rf.Repair(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly repair error: %w", err)
	}

	fmt.Println("rapidfly: Metadata repair completed successfully.")
	return nil
}
