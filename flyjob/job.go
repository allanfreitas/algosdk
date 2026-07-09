package flyjob

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Config configures the job package.
type Config struct {
	// DB is the required database connection.
	DB *sql.DB
	// Strategy dialect to use. Autodetected if nil.
	Dialect Dialect
	// JobsTable is the table name for jobs. Defaults to "jobs".
	JobsTable string
	// AttemptsTable is the table name for job attempts. Defaults to "job_attempts".
	AttemptsTable string
}

// Client is the main entry point for enqueueing jobs and creating workers.
type Client struct {
	cfg  Config
	repo *repository
}

// New creates a new Client with default table names applied.
func New(cfg Config) *Client {
	if cfg.JobsTable == "" {
		cfg.JobsTable = "jobs"
	}
	if cfg.AttemptsTable == "" {
		cfg.AttemptsTable = "job_attempts"
	}
	if cfg.Dialect == nil {
		cfg.Dialect = detectDialect(&cfg)
	}
	return &Client{
		cfg:  cfg,
		repo: newRepository(cfg),
	}
}

// detectDialect infers the database dialect to use based on the driver.
func detectDialect(config *Config) Dialect {
	var driverName string
	if config.DB != nil {
		driverName = fmt.Sprintf("%T", config.DB.Driver())
	}

	driverName = strings.ToLower(driverName)
	if strings.Contains(driverName, "mysql") {
		return &MySQLDialect{}
	}
	if strings.Contains(driverName, "oracle") || strings.Contains(driverName, "ora") || strings.Contains(driverName, "godror") {
		return &OracleDialect{}
	}
	// Default to PostgreSQL
	return &PostgresDialect{}
}

// EnsureSchema creates the jobs and job_attempts tables if they do not exist.
// It is safe to call on every application start.
func (c *Client) EnsureSchema(ctx context.Context) error {
	if c.cfg.DB == nil {
		return fmt.Errorf("rapidfly/job: DB must be configured to EnsureSchema")
	}
	conn, err := c.cfg.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to acquire connection: %w", err)
	}
	defer conn.Close()
	return c.repo.ensureSchema(ctx, conn)
}

// NewWorker creates a new WorkerPool bound to this client.
func (c *Client) NewWorker(cfg WorkerConfig) *WorkerPool {
	return &WorkerPool{client: c, cfg: cfg}
}
