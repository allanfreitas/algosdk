package flyjob

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
)

type repository struct {
	db            *sql.DB
	dialect       Dialect
	jobsTable     string
	attemptsTable string
}

func newRepository(cfg Config) *repository {
	return &repository{
		db:            cfg.DB,
		dialect:       cfg.Dialect,
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

func (r *repository) ensureSchema(ctx context.Context, conn *sql.Conn) error {
	if err := r.validateTableNames(); err != nil {
		return err
	}
	return r.dialect.EnsureSchema(ctx, conn, r.jobsTable, r.attemptsTable)
}

func (r *repository) insertJob(ctx context.Context, tx any, queue, action string, payload []byte) error {
	if err := r.validateTableNames(); err != nil {
		return err
	}

	query := r.dialect.InsertJobSQL(r.jobsTable)

	var err error
	if tx != nil {
		err = execTx(ctx, tx, query, queue, action, payload)
	} else {
		if r.db == nil {
			return fmt.Errorf("rapidfly/job: DB must be configured to insert job")
		}
		_, err = r.db.ExecContext(ctx, query, queue, action, payload)
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
	if r.db == nil {
		return nil, fmt.Errorf("rapidfly/job: DB must be configured to fetch pending jobs")
	}
	return r.dialect.FetchPendingJobs(ctx, r.db, r.jobsTable, queues, limit)
}

func (r *repository) markCompleted(ctx context.Context, job Job) error {
	if err := r.validateTableNames(); err != nil {
		return err
	}
	if r.db == nil {
		return fmt.Errorf("rapidfly/job: DB must be configured to mark job completed")
	}
	return r.dialect.MarkCompleted(ctx, r.db, r.jobsTable, r.attemptsTable, job)
}

func (r *repository) markFailed(ctx context.Context, job Job, errMsg string) error {
	if err := r.validateTableNames(); err != nil {
		return err
	}
	if r.db == nil {
		return fmt.Errorf("rapidfly/job: DB must be configured to mark job failed")
	}
	return r.dialect.MarkFailed(ctx, r.db, r.jobsTable, r.attemptsTable, job, errMsg)
}
