package flyjob

import "time"

// Job represents a generic background job in the queue.
type Job struct {
	ID           int64
	Queue        string
	Action       string
	Payload      []byte // raw JSONB
	Status       string
	Attempts     int
	MaxAttempts  int
	ErrorMessage *string
	RunAt        time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// JobAttempt records a single execution attempt of a Job.
type JobAttempt struct {
	ID            int64
	JobID         int64
	AttemptNumber int
	Status        string
	ErrorMessage  *string
	RanAt         time.Time
}
