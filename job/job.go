package job

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config configures the job package.
type Config struct {
	// Pool is the required database connection pool.
	Pool *pgxpool.Pool
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
	return &Client{
		cfg:  cfg,
		repo: newRepository(cfg),
	}
}

// EnsureSchema creates the jobs and job_attempts tables if they do not exist.
// It is safe to call on every application start.
func (c *Client) EnsureSchema(ctx context.Context) error {
	conn, err := c.cfg.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to acquire connection: %w", err)
	}
	defer conn.Release()
	return c.repo.ensureSchema(ctx, conn.Conn())
}

// NewWorker creates a new WorkerPool bound to this client.
func (c *Client) NewWorker(cfg WorkerConfig) *WorkerPool {
	return &WorkerPool{client: c, cfg: cfg}
}
