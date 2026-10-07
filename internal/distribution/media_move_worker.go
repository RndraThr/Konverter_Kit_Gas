package distribution

import (
	"context"
	"log"
	"time"

	"konkit/internal/media"
)

type MediaMoveRepository interface {
	ClaimMediaMove(context.Context, time.Time, time.Duration) (MediaMoveJob, bool, error)
	CompleteMediaMove(context.Context, string, int64) error
	FailMediaMove(context.Context, string, int64, string, time.Time) error
}

type MediaMoveWorkerOptions struct {
	PollInterval  time.Duration
	LeaseDuration time.Duration
	MaxBackoff    time.Duration
}

type MediaMoveWorker struct {
	repository MediaMoveRepository
	storage    media.MovableStorage
	options    MediaMoveWorkerOptions
	now        func() time.Time
}

func NewMediaMoveWorker(repository MediaMoveRepository, storage media.MovableStorage, options MediaMoveWorkerOptions) *MediaMoveWorker {
	if options.PollInterval <= 0 {
		options.PollInterval = 5 * time.Second
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = 5 * time.Minute
	}
	if options.MaxBackoff <= 0 {
		options.MaxBackoff = 15 * time.Minute
	}
	return &MediaMoveWorker{repository: repository, storage: storage, options: options, now: time.Now}
}

func (w *MediaMoveWorker) ProcessOne(ctx context.Context) (bool, error) {
	now := w.now()
	job, ok, err := w.repository.ClaimMediaMove(ctx, now, w.options.LeaseDuration)
	if err != nil || !ok {
		return false, err
	}
	if err := w.storage.Move(ctx, job.StorageKey, job.TargetPath); err != nil {
		nextAttempt := now.Add(mediaMoveBackoff(job.Attempts, w.options.MaxBackoff))
		if failErr := w.repository.FailMediaMove(ctx, job.MediaFileID, job.TargetGeneration, err.Error(), nextAttempt); failErr != nil {
			return true, failErr
		}
		return true, nil
	}
	if err := w.repository.CompleteMediaMove(ctx, job.MediaFileID, job.TargetGeneration); err != nil {
		return true, err
	}
	return true, nil
}

func (w *MediaMoveWorker) Run(ctx context.Context) {
	for {
		processed, err := w.ProcessOne(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("distribution media move worker: %v", err)
		}
		if processed && err == nil {
			continue
		}
		timer := time.NewTimer(w.options.PollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func mediaMoveBackoff(attempts int, maximum time.Duration) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Second
	for i := 1; i < attempts && delay < maximum; i++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}
