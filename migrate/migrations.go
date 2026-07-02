package migrate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// loadMigrations loads all SQL migration files from the filesystem.
func (rf *Migrator) loadMigrations() ([]*Migration, error) {
	var entries []fs.DirEntry
	var err error

	path := rf.config.MigrationsPath
	fsys := rf.config.FS

	if fsys != nil {
		entries, err = fs.ReadDir(fsys, path)
	} else {
		entries, err = os.ReadDir(path)
	}

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("rapidfly/migrate: failed to read migrations directory: %w", err)
	}

	var migrations []*Migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		mig, err := ParseFilename(name)
		if err != nil {
			return nil, err
		}

		var content []byte
		if fsys != nil {
			content, err = fs.ReadFile(fsys, filepath.Join(path, name))
		} else {
			content, err = os.ReadFile(filepath.Join(path, name))
		}
		if err != nil {
			return nil, fmt.Errorf("rapidfly/migrate: failed to read migration file %s: %w", name, err)
		}

		mig.Content = string(content)
		mig.Checksum = CalculateChecksum(content)
		migrations = append(migrations, mig)
	}

	versions := make(map[string]*Migration)
	for _, m := range migrations {
		if m.Type == "SQL" {
			if existing, exists := versions[m.Version]; exists {
				return nil, fmt.Errorf("rapidfly/migrate: duplicate migration version %s (found in %s and %s)", m.Version, existing.Script, m.Script)
			}
			versions[m.Version] = m
		}
	}

	sort.Slice(migrations, func(i, j int) bool {
		if migrations[i].Type == "SQL" && migrations[j].Type == "SQL" {
			return compareVersions(migrations[i].Version, migrations[j].Version) < 0
		}
		if migrations[i].Type == "SQL" && migrations[j].Type != "SQL" {
			return true
		}
		if migrations[i].Type != "SQL" && migrations[j].Type == "SQL" {
			return false
		}
		return migrations[i].Script < migrations[j].Script
	})

	return migrations, nil
}