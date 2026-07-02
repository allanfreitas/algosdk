package job

import (
	"context"
	"log"
	"time"
)

// HandlerFunc is the function signature for a job handler.
type HandlerFunc func(ctx context.Context, job Job) error

// WorkerConfig configures the worker pool.
type WorkerConfig struct {
	Queues       []string      // queues to consume; default ["default"]
	Concurrency  int           // number of goroutines; default 2
	PollInterval time.Duration // polling interval; default 5s
}

// WorkerPool manages background job processing.
type WorkerPool struct {
	client   *Client
	cfg      WorkerConfig
	handlers map[string]HandlerFunc
}

// Register binds a HandlerFunc to an action name.
func (w *WorkerPool) Register(action string, fn HandlerFunc) {
	if w.handlers == nil {
		w.handlers = make(map[string]HandlerFunc)
	}
	w.handlers[action] = fn
}

// Start launches the worker goroutines and the producer polling loop.
// It blocks until ctx is cancelled.
func (w *WorkerPool) Start(ctx context.Context) {
	if len(w.cfg.Queues) == 0 {
		w.cfg.Queues = []string{"default"}
	}
	if w.cfg.Concurrency <= 0 {
		w.cfg.Concurrency = 2
	}
	if w.cfg.PollInterval <= 0 {
		w.cfg.PollInterval = 5 * time.Second
	}

	log.Printf("rapidfly/job: Starting pool: concurrency=%d queues=%v poll=%s",
		w.cfg.Concurrency, w.cfg.Queues, w.cfg.PollInterval)

	jobChan := make(chan Job, w.cfg.Concurrency*2)

	for i := 0; i < w.cfg.Concurrency; i++ {
		go w.runWorker(ctx, i, jobChan)
	}

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	defer close(jobChan)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobs, err := w.client.repo.fetchPendingJobs(ctx, w.cfg.Queues, w.cfg.Concurrency)
			if err != nil {
				log.Printf("rapidfly/job: Error fetching jobs: %v", err)
				continue
			}
			for _, job := range jobs {
				select {
				case <-ctx.Done():
					return
				case jobChan <- job:
				default:
					log.Printf("rapidfly/job: Channel full, skipping job %d", job.ID)
				}
			}
		}
	}
}

func (w *WorkerPool) runWorker(ctx context.Context, id int, jobChan <-chan Job) {
	log.Printf("rapidfly/job: Worker %d started", id)
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobChan:
			if !ok {
				return
			}
			w.execute(ctx, job)
		}
	}
}

func (w *WorkerPool) execute(ctx context.Context, job Job) {
	handler, ok := w.handlers[job.Action]
	if !ok {
		log.Printf("rapidfly/job: No handler for action %q (job %d)", job.Action, job.ID)
		_ = w.client.repo.markFailed(ctx, job, "no handler registered for action: "+job.Action)
		return
	}
	if err := handler(ctx, job); err != nil {
		log.Printf("rapidfly/job: Job %d (%s) failed: %v", job.ID, job.Action, err)
		_ = w.client.repo.markFailed(ctx, job, err.Error())
	} else {
		_ = w.client.repo.markCompleted(ctx, job)
	}
}
