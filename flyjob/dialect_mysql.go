package flyjob

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type MySQLDialect struct{}

func (d *MySQLDialect) EnsureSchema(ctx context.Context, conn *sql.Conn, jobsTable, attemptsTable string) error {
	query := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %[1]s (
    id            BIGINT       AUTO_INCREMENT PRIMARY KEY,
    queue         VARCHAR(100) NOT NULL DEFAULT 'default',
    action        VARCHAR(100) NOT NULL,
    payload       JSON         NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'pending',
    attempts      INT          NOT NULL DEFAULT 0,
    max_attempts  INT          NOT NULL DEFAULT 3,
    error_message TEXT,
    run_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at    TIMESTAMP    NULL,
    completed_at  TIMESTAMP    NULL,
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
`, jobsTable)

	_, err := conn.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("mysql/ensure_schema/jobs: %w", err)
	}

	var count int
	_ = conn.QueryRowContext(ctx, `
		SELECT COUNT(1) 
		FROM information_schema.statistics 
		WHERE table_schema = DATABASE() 
		  AND table_name = ? 
		  AND index_name = ?
	`, jobsTable, "idx_"+jobsTable+"_pending").Scan(&count)
	if count == 0 {
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("CREATE INDEX idx_%s_pending ON %s(queue, status, run_at)", jobsTable, jobsTable))
	}

	count = 0
	_ = conn.QueryRowContext(ctx, `
		SELECT COUNT(1) 
		FROM information_schema.statistics 
		WHERE table_schema = DATABASE() 
		  AND table_name = ? 
		  AND index_name = ?
	`, jobsTable, "idx_"+jobsTable+"_action").Scan(&count)
	if count == 0 {
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("CREATE INDEX idx_%s_action ON %s(action)", jobsTable, jobsTable))
	}

	queryAttempts := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %[2]s (
    id             BIGINT      AUTO_INCREMENT PRIMARY KEY,
    job_id         BIGINT      NOT NULL,
    attempt_number INT         NOT NULL,
    status         VARCHAR(20) NOT NULL,
    error_message  TEXT,
    ran_at         TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES %[1]s(id) ON DELETE CASCADE
);
`, jobsTable, attemptsTable)
	_, err = conn.ExecContext(ctx, queryAttempts)
	if err != nil {
		return fmt.Errorf("mysql/ensure_schema/attempts: %w", err)
	}

	count = 0
	_ = conn.QueryRowContext(ctx, `
		SELECT COUNT(1) 
		FROM information_schema.statistics 
		WHERE table_schema = DATABASE() 
		  AND table_name = ? 
		  AND index_name = ?
	`, attemptsTable, "idx_"+attemptsTable+"_job_id").Scan(&count)
	if count == 0 {
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("CREATE INDEX idx_%s_job_id ON %s(job_id)", attemptsTable, attemptsTable))
	}

	return nil
}

func (d *MySQLDialect) InsertJobSQL(jobsTable string) string {
	return fmt.Sprintf(`
		INSERT INTO %s (
			queue, action, payload, status, attempts, max_attempts, 
			error_message, run_at, started_at, completed_at, created_at, updated_at
		) VALUES (?, ?, ?, 'pending', 0, 3, NULL, NOW(), NULL, NULL, NOW(), NOW())
	`, jobsTable)
}

func (d *MySQLDialect) FetchPendingJobs(ctx context.Context, db *sql.DB, jobsTable string, queues []string, limit int) ([]Job, error) {
	if len(queues) == 0 {
		queues = []string{"default"}
	}

	placeholders := make([]string, len(queues))
	args := make([]any, len(queues)+1)
	for i, q := range queues {
		placeholders[i] = "?"
		args[i] = q
	}
	args[len(queues)] = limit

	queryUpdate := fmt.Sprintf(`
		UPDATE %[1]s
		SET status = 'processing', started_at = NOW(), updated_at = NOW(), attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM (
				SELECT id FROM %[1]s
				WHERE status = 'pending' AND queue IN (%[2]s) AND run_at <= NOW()
				ORDER BY id ASC
				LIMIT ?
				FOR UPDATE SKIP LOCKED
			) AS tmp
		);
	`, jobsTable, strings.Join(placeholders, ", "))

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mysql/fetch_pending: failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, queryUpdate, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql/fetch_pending/update: %w", err)
	}

	querySelect := fmt.Sprintf(`
		SELECT id, queue, action, payload, status, attempts, max_attempts, error_message, run_at, started_at, completed_at, created_at, updated_at
		FROM %[1]s
		WHERE status = 'processing' AND queue IN (%[2]s)
		ORDER BY updated_at DESC
		LIMIT ?;
	`, jobsTable, strings.Join(placeholders, ", "))

	rows, err := tx.QueryContext(ctx, querySelect, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql/fetch_pending/select: %w", err)
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
			return nil, fmt.Errorf("mysql/fetch_pending/scan: %w", err)
		}
		jobs = append(jobs, j)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("mysql/fetch_pending/commit: %w", err)
	}

	return jobs, nil
}

func (d *MySQLDialect) MarkCompleted(ctx context.Context, db *sql.DB, jobsTable, attemptsTable string, job Job) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mysql/completed: %w", err)
	}
	defer tx.Rollback()

	queryJob := fmt.Sprintf(`
		UPDATE %s
		SET status = 'completed', completed_at = NOW(), updated_at = NOW()
		WHERE id = ?
	`, jobsTable)
	_, err = tx.ExecContext(ctx, queryJob, job.ID)
	if err != nil {
		return fmt.Errorf("mysql/completed/job: %w", err)
	}

	queryAttempt := fmt.Sprintf(`
		INSERT INTO %s (job_id, attempt_number, status, error_message, ran_at)
		VALUES (?, ?, 'completed', NULL, NOW())
	`, attemptsTable)
	_, err = tx.ExecContext(ctx, queryAttempt, job.ID, job.Attempts)
	if err != nil {
		return fmt.Errorf("mysql/completed/attempt: %w", err)
	}

	return tx.Commit()
}

func (d *MySQLDialect) MarkFailed(ctx context.Context, db *sql.DB, jobsTable, attemptsTable string, job Job, errMsg string) error {
	status := "pending"
	runAt := time.Now().Add(time.Duration(job.Attempts) * 2 * time.Minute)
	if job.Attempts >= job.MaxAttempts {
		status = "failed"
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mysql/failed: %w", err)
	}
	defer tx.Rollback()

	queryJob := fmt.Sprintf(`
		UPDATE %s
		SET status = ?, error_message = ?, run_at = ?, updated_at = NOW()
		WHERE id = ?
	`, jobsTable)
	_, err = tx.ExecContext(ctx, queryJob, status, errMsg, runAt, job.ID)
	if err != nil {
		return fmt.Errorf("mysql/failed/job: %w", err)
	}

	queryAttempt := fmt.Sprintf(`
		INSERT INTO %s (job_id, attempt_number, status, error_message, ran_at)
		VALUES (?, ?, ?, ?, NOW())
	`, attemptsTable)
	_, err = tx.ExecContext(ctx, queryAttempt, job.ID, job.Attempts, status, errMsg)
	if err != nil {
		return fmt.Errorf("mysql/failed/attempt: %w", err)
	}

	return tx.Commit()
}
