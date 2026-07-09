package flyjob

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type PostgresDialect struct{}

func (d *PostgresDialect) EnsureSchema(ctx context.Context, conn *sql.Conn, jobsTable, attemptsTable string) error {
	query := fmt.Sprintf(`
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
`, jobsTable, attemptsTable)

	_, err := conn.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("postgres/ensure_schema: %w", err)
	}
	return nil
}

func (d *PostgresDialect) InsertJobSQL(jobsTable string) string {
	return fmt.Sprintf(`
		INSERT INTO %s (
			queue, action, payload, status, attempts, max_attempts, 
			error_message, run_at, started_at, completed_at, created_at, updated_at
		) VALUES ($1, $2, $3, 'pending', 0, 3, NULL, NOW(), NULL, NULL, NOW(), NOW())
	`, jobsTable)
}

func (d *PostgresDialect) FetchPendingJobs(ctx context.Context, db *sql.DB, jobsTable string, queues []string, limit int) ([]Job, error) {
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
	`, jobsTable)

	rows, err := db.QueryContext(ctx, query, queues, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres/fetch_pending: %w", err)
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
			return nil, fmt.Errorf("postgres/fetch_pending/scan: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (d *PostgresDialect) MarkCompleted(ctx context.Context, db *sql.DB, jobsTable, attemptsTable string, job Job) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres/completed: %w", err)
	}
	defer tx.Rollback()

	queryJob := fmt.Sprintf(`
		UPDATE %s
		SET status = 'completed', completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, jobsTable)
	_, err = tx.ExecContext(ctx, queryJob, job.ID)
	if err != nil {
		return fmt.Errorf("postgres/completed/job: %w", err)
	}

	queryAttempt := fmt.Sprintf(`
		INSERT INTO %s (job_id, attempt_number, status, error_message, ran_at)
		VALUES ($1, $2, 'completed', NULL, NOW())
	`, attemptsTable)
	_, err = tx.ExecContext(ctx, queryAttempt, job.ID, job.Attempts)
	if err != nil {
		return fmt.Errorf("postgres/completed/attempt: %w", err)
	}

	return tx.Commit()
}

func (d *PostgresDialect) MarkFailed(ctx context.Context, db *sql.DB, jobsTable, attemptsTable string, job Job, errMsg string) error {
	status := "pending"
	runAt := time.Now().Add(time.Duration(job.Attempts) * 2 * time.Minute)
	if job.Attempts >= job.MaxAttempts {
		status = "failed"
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres/failed: %w", err)
	}
	defer tx.Rollback()

	queryJob := fmt.Sprintf(`
		UPDATE %s
		SET status = $1, error_message = $2, run_at = $3, updated_at = NOW()
		WHERE id = $4
	`, jobsTable)
	_, err = tx.ExecContext(ctx, queryJob, status, errMsg, runAt, job.ID)
	if err != nil {
		return fmt.Errorf("postgres/failed/job: %w", err)
	}

	queryAttempt := fmt.Sprintf(`
		INSERT INTO %s (job_id, attempt_number, status, error_message, ran_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, attemptsTable)
	_, err = tx.ExecContext(ctx, queryAttempt, job.ID, job.Attempts, status, errMsg)
	if err != nil {
		return fmt.Errorf("postgres/failed/attempt: %w", err)
	}

	return tx.Commit()
}
