package migrate

import (
	"fmt"
	"hash/crc32"
	"math/big"
	"regexp"
	"strings"
)

// ParseFilename parses a migration file name according to RapidFly spec rules.
func ParseFilename(filename string) (*Migration, error) {
	if !strings.HasSuffix(filename, ".sql") {
		return nil, fmt.Errorf("rapidfly/migrate: extension must be .sql: %s", filename)
	}

	// 1. Repeatable Migrations: R__{DESCRIPTION}.sql
	if strings.HasPrefix(filename, "R__") {
		descPart := strings.TrimSuffix(filename[3:], ".sql")
		if descPart == "" {
			return nil, fmt.Errorf("rapidfly/migrate: description cannot be empty: %s", filename)
		}
		// Validate description is English (US) alphanumeric/underscore
		if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(descPart) {
			return nil, fmt.Errorf("rapidfly/migrate: description must be in English (US): %s", filename)
		}
		return &Migration{
			Version:     "",
			Description: descPart,
			Type:        "REPEATABLE",
			Script:      filename,
		}, nil
	}

	// 2. Versioned Migrations: V{VERSION}__{DESCRIPTION}.sql
	if strings.HasPrefix(filename, "V") {
		body := strings.TrimSuffix(filename[1:], ".sql")
		parts := strings.SplitN(body, "__", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("rapidfly/migrate: double underscore (__) must separate version and description: %s", filename)
		}
		versionStr := parts[0]
		descPart := parts[1]

		// Validate version is numeric
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(versionStr) {
			return nil, fmt.Errorf("rapidfly/migrate: version must be numeric: %s", filename)
		}

		// Validate description uses PascalCase and English (US)
		if !regexp.MustCompile(`^[A-Z][a-zA-Z0-9]*$`).MatchString(descPart) {
			return nil, fmt.Errorf("rapidfly/migrate: description must be in English (US) and use PascalCase: %s", filename)
		}

		return &Migration{
			Version:     versionStr,
			Description: descPart,
			Type:        "SQL",
			Script:      filename,
		}, nil
	}

	return nil, fmt.Errorf("rapidfly/migrate: filename must start with V or R__: %s", filename)
}

// CalculateChecksum computes the CRC32 checksum for a file's content, normalizing line endings.
func CalculateChecksum(content []byte) int64 {
	// Normalize CRLF (Windows) to LF (Linux) to ensure consistent checksums across platforms
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	return int64(crc32.ChecksumIEEE([]byte(normalized)))
}

// compareVersions compares two version strings numerically using big.Int to prevent overflow.
func compareVersions(v1, v2 string) int {
	b1, ok1 := new(big.Int).SetString(v1, 10)
	b2, ok2 := new(big.Int).SetString(v2, 10)
	if ok1 && ok2 {
		return b1.Cmp(b2)
	}
	// Fallback to string comparison if not numeric
	return strings.Compare(v1, v2)
}

// validateTableName ensures the table name is safe to prevent SQL injection.
func (rf *Migrator) validateTableName() error {
	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(rf.config.TableName) {
		return fmt.Errorf("rapidfly/migrate: invalid table name: %s", rf.config.TableName)
	}
	return nil
}
