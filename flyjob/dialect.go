package flyjob

import (
	"context"
	"database/sql"
)

// Dialect defines database-specific SQL and schema execution behavior for flyjob.
type Dialect interface {
	// EnsureSchema creates the jobs and job_attempts tables and indexes.
	EnsureSchema(ctx context.Context, conn *sql.Conn, jobsTable, attemptsTable string) error

	// InsertJobSQL returns the INSERT query template.
	InsertJobSQL(jobsTable string) string

	// FetchPendingJobs locks and retrieves pending jobs for the worker pool.
	FetchPendingJobs(ctx context.Context, db *sql.DB, jobsTable string, queues []string, limit int) ([]Job, error)

	// MarkCompleted marks a job as completed inside a transaction.
	MarkCompleted(ctx context.Context, db *sql.DB, jobsTable, attemptsTable string, job Job) error

	// MarkFailed marks a job as failed and inserts a new attempt record inside a transaction.
	MarkFailed(ctx context.Context, db *sql.DB, jobsTable, attemptsTable string, job Job, errMsg string) error
}
