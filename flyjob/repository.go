package flyjob

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct {
	pool          *pgxpool.Pool
	jobsTable     string
	attemptsTable string
}

func newRepository(cfg Config) *repository {
	return &repository{
		pool:          cfg.Pool,
		jobsTable:     cfg.JobsTable,
		attemptsTable: cfg.AttemptsTable,
	}
}

func (r *repository) validateTableNames() error {
	re := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	if !re.MatchString(r.jobsTable) {
		return fmt.Errorf("rapidfly/job: invalid jobs table name: %s", r.jobsTable)
	}
	if !re.MatchString(r.attemptsTable) {
		return fmt.Errorf("rapidfly/job: invalid attempts table name: %s", r.attemptsTable)
	}
	return nil
}

func (r *repository) ensureSchema(ctx context.Context, conn *pgx.Conn) error {
	if err := r.validateTableNames(); err != nil {
		return err
	}

	sql := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %[1]s (
    id            BIGSERIAL    PRIMARY KEY,
    queue         VARCHAR(100) NOT NULL DEFAULT 'default',
    action        VARCHAR(100) NOT NULL,
    payload       JSONB        NOT NULL DEFAULT '{}',
    status        VARCHAR(20)  NOT NULL DEFAULT 'pending',
    attempts      INT          NOT NULL DEFAULT 0,
    max_attempts  INT          NOT NULL DEFAULT 3,
    error_message TEXT,
    run_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_%[1]s_pending ON %[1]s(queue, status, run_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_%[1]s_action  ON %[1]s(action);

CREATE TABLE IF NOT EXISTS %[2]s (
    id             BIGSERIAL   PRIMARY KEY,
    job_id         BIGINT      NOT NULL REFERENCES %[1]s(id) ON DELETE CASCADE,
    attempt_number INT         NOT NULL,
    status         VARCHAR(20) NOT NULL,
    error_message  TEXT,
    ran_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_%[2]s_job_id ON %[2]s(job_id);
`, r.jobsTable, r.attemptsTable)

	_, err := conn.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to ensure schema: %w", err)
	}
	return nil
}

func (r *repository) insertJob(ctx context.Context, tx pgx.Tx, queue, action string, payload []byte) error {
	if err := r.validateTableNames(); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			queue, action, payload, status, attempts, max_attempts, 
			error_message, run_at, started_at, completed_at, created_at, updated_at
		) VALUES ($1, $2, $3, 'pending', 0, 3, NULL, NOW(), NULL, NULL, NOW(), NOW())
	`, r.jobsTable)

	var err error
	if tx != nil {
		_, err = tx.Exec(ctx, query, queue, action, payload)
	} else {
		_, err = r.pool.Exec(ctx, query, queue, action, payload)
	}
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to insert job: %w", err)
	}
	return nil
}

func (r *repository) fetchPendingJobs(ctx context.Context, queues []string, limit int) ([]Job, error) {
	if err := r.validateTableNames(); err != nil {
		return nil, err
	}

	if len(queues) == 0 {
		queues = []string{"default"}
	}

	query := fmt.Sprintf(`
		UPDATE %[1]s
		SET status = 'processing', started_at = NOW(), updated_at = NOW(), attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM %[1]s
			WHERE status = 'pending' AND queue = ANY($1) AND run_at <= NOW()
			ORDER BY id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, queue, action, payload, status, attempts, max_attempts, error_message, run_at, started_at, completed_at, created_at, updated_at;
	`, r.jobsTable)

	rows, err := r.pool.Query(ctx, query, queues, limit)
	if err != nil {
		return nil, fmt.Errorf("rapidfly/job: failed to fetch pending jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		err := rows.Scan(
			&j.ID, &j.Queue, &j.Action, &j.Payload, &j.Status, &j.Attempts, &j.MaxAttempts,
			&j.ErrorMessage, &j.RunAt, &j.StartedAt, &j.CompletedAt, &j.CreatedAt, &j.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("rapidfly/job: failed to scan job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (r *repository) markCompleted(ctx context.Context, job Job) error {
	if err := r.validateTableNames(); err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queryJob := fmt.Sprintf(`
		UPDATE %s
		SET status = 'completed', completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, r.jobsTable)
	_, err = tx.Exec(ctx, queryJob, job.ID)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to update job status to completed: %w", err)
	}

	queryAttempt := fmt.Sprintf(`
		INSERT INTO %s (job_id, attempt_number, status, error_message, ran_at)
		VALUES ($1, $2, 'completed', NULL, NOW())
	`, r.attemptsTable)
	_, err = tx.Exec(ctx, queryAttempt, job.ID, job.Attempts)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to insert job attempt audit: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *repository) markFailed(ctx context.Context, job Job, errMsg string) error {
	if err := r.validateTableNames(); err != nil {
		return err
	}

	status := "pending"
	runAt := time.Now().Add(time.Duration(job.Attempts) * 2 * time.Minute)
	if job.Attempts >= job.MaxAttempts {
		status = "failed"
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	queryJob := fmt.Sprintf(`
		UPDATE %s
		SET status = $1, error_message = $2, run_at = $3, updated_at = NOW()
		WHERE id = $4
	`, r.jobsTable)
	_, err = tx.Exec(ctx, queryJob, status, errMsg, runAt, job.ID)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to update job status to failed/pending: %w", err)
	}

	queryAttempt := fmt.Sprintf(`
		INSERT INTO %s (job_id, attempt_number, status, error_message, ran_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, r.attemptsTable)
	_, err = tx.Exec(ctx, queryAttempt, job.ID, job.Attempts, status, errMsg)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to insert job attempt audit: %w", err)
	}

	return tx.Commit(ctx)
}
