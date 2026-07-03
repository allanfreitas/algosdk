package flyconfig

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// LoadEnv loads variables from a local .env file into the system environment if it exists.
func LoadEnv(filePath string) {
	if filePath == "" {
		filePath = ".env.local"
	}

	file, err := os.Open(filePath)
	if err != nil {
		return // .env.local file is optional (e.g. environment variables can be set directly)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Trim quotes
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			val = val[1 : len(val)-1]
		}

		// Set environment variable if not already set (retains command-line overrides)
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("error reading env file %s: %v", filePath, err)
	}
}

func GetEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
