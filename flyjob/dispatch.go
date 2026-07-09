package flyjob

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
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

// DispatchTx enqueues a job inside an existing transaction (e.g. *sql.Tx or pgx.Tx).
// T can be any JSON-serializable struct.
func DispatchTx[T any](ctx context.Context, c *Client, tx any, queue, action string, payload T) error {
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

// execTx executes a query on either *sql.Tx or pgx.Tx (using reflection to decouple from pgx dependency).
func execTx(ctx context.Context, tx any, query string, args ...any) error {
	if tx == nil {
		return fmt.Errorf("rapidfly/job: transaction is nil")
	}

	// 1. Try standard database/sql.Tx ExecContext
	if execer, ok := tx.(interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	}); ok {
		_, err := execer.ExecContext(ctx, query, args...)
		return err
	}

	// 2. Try pgx.Tx Exec using reflection
	val := reflect.ValueOf(tx)
	method := val.MethodByName("Exec")
	if method.IsValid() {
		inputs := make([]reflect.Value, 2+len(args))
		inputs[0] = reflect.ValueOf(ctx)
		inputs[1] = reflect.ValueOf(query)
		for i, arg := range args {
			inputs[2+i] = reflect.ValueOf(arg)
		}
		results := method.Call(inputs)
		if len(results) == 2 {
			errVal := results[1]
			if !errVal.IsNil() {
				return errVal.Interface().(error)
			}
			return nil
		}
	}

	return fmt.Errorf("rapidfly/job: unsupported transaction type %T", tx)
}
