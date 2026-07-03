package flyjob

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Dispatch enqueues a job outside of an existing transaction (acquires its own).
// T can be any JSON-serializable struct.
func Dispatch[T any](ctx context.Context, c *Client, queue, action string, payload T) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to marshal payload: %w", err)
	}
	return c.repo.insertJob(ctx, nil, queue, action, payloadBytes)
}

// DispatchTx enqueues a job inside an existing pgx transaction.
// T can be any JSON-serializable struct.
func DispatchTx[T any](ctx context.Context, c *Client, tx pgx.Tx, queue, action string, payload T) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("rapidfly/job: failed to marshal payload: %w", err)
	}
	return c.repo.insertJob(ctx, tx, queue, action, payloadBytes)
}

// DecodePayload is a convenience generic helper for handlers to unmarshal job.Payload.
func DecodePayload[T any](job Job) (T, error) {
	var v T
	if err := json.Unmarshal(job.Payload, &v); err != nil {
		return v, fmt.Errorf("rapidfly/job: failed to decode payload: %w", err)
	}
	return v, nil
}
