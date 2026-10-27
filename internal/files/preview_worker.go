package files

import (
	"bytes"
	"context"
	"errors"
	"filevault/internal/preview"
	"filevault/internal/storage"
	"fmt"
	"io"
	"log"
	"time"
)

const (
	PreviewFailed  = "failed"
	PreviewSkipped = "skipped"
)

type PreviewRepository interface {
	NextPendingPreview(ctx context.Context) (PreviewJob, bool, error)
	SetPreviewDone(ctx context.Context, fileID int64, previewPath string) error
	SetPreviewStatus(ctx context.Context, fileID int64, status string) error
}

type PreviewStorage interface {
	Open(path string) (io.ReadSeekCloser, error)
	SavePreview(r io.Reader) (storage.Saved, error)
	Remove(path string) error
}

type PreviewWorker struct {
	repo     PreviewRepository
	storage  PreviewStorage
	wake     chan struct{}
	interval time.Duration
}

func NewPreviewWorker(repo PreviewRepository, storage PreviewStorage) *PreviewWorker {
	return &PreviewWorker{
		repo:     repo,
		storage:  storage,
		wake:     make(chan struct{}, 1),
		interval: time.Minute,
	}
}

func (worker *PreviewWorker) Wake() {
	select {
	case worker.wake <- struct{}{}:
	default:
	}
}

func (worker *PreviewWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()

	for {
		worker.drain(ctx)

		select {
		case <-ctx.Done():
			return
		case <-worker.wake:
		case <-ticker.C:
		}
	}
}

func (worker *PreviewWorker) drain(ctx context.Context) {
	for ctx.Err() == nil {
		found, err := worker.processNext(ctx)
		if err != nil {
			log.Printf("preview worker: %v", err)
			return
		}
		if !found {
			return
		}
	}
}

func (worker *PreviewWorker) processNext(ctx context.Context) (bool, error) {
	job, found, err := worker.repo.NextPendingPreview(ctx)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	if !preview.Supported(job.MimeType) {
		return true, worker.repo.SetPreviewStatus(ctx, job.ID, PreviewSkipped)
	}

	previewPath, err := worker.makePreview(job)
	if err != nil {
		log.Printf("preview %d: %v", job.ID, err)
		return true, worker.repo.SetPreviewStatus(ctx, job.ID, PreviewFailed)
	}

	if err := worker.repo.SetPreviewDone(ctx, job.ID, previewPath); err != nil {
		if removeErr := worker.storage.Remove(previewPath); removeErr != nil {
			return true, errors.Join(err, fmt.Errorf("remove preview %q: %w", previewPath, removeErr))
		}
		return true, err
	}

	return true, nil
}

func (worker *PreviewWorker) makePreview(job PreviewJob) (path string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v", recovered)
		}
	}()

	src, err := worker.storage.Open(job.StoragePath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	var buf bytes.Buffer
	if err := preview.Make(src, &buf); err != nil {
		return "", err
	}

	saved, err := worker.storage.SavePreview(&buf)
	if err != nil {
		return "", err
	}

	return saved.Path, nil
}
