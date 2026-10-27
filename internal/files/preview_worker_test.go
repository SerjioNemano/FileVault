package files

import (
	"bytes"
	"context"
	"errors"
	"filevault/internal/storage"
	"image"
	"image/jpeg"
	"io"
	"testing"
)

type fakePreviewRepo struct {
	job     PreviewJob
	found   bool
	nextErr error
	doneErr error

	doneID   int64
	donePath string
	statusID int64
	status   string
}

func (f *fakePreviewRepo) NextPendingPreview(ctx context.Context) (PreviewJob, bool, error) {
	return f.job, f.found, f.nextErr
}

func (f *fakePreviewRepo) SetPreviewDone(ctx context.Context, fileID int64, previewPath string) error {
	f.doneID, f.donePath = fileID, previewPath
	return f.doneErr
}

func (f *fakePreviewRepo) SetPreviewStatus(ctx context.Context, fileID int64, status string) error {
	f.statusID, f.status = fileID, status
	return nil
}

type fakePreviewStorage struct {
	original []byte
	openErr  error

	openPath   string
	saved      []byte
	removePath string
}

func (f *fakePreviewStorage) Open(path string) (io.ReadSeekCloser, error) {
	f.openPath = path
	if f.openErr != nil {
		return nil, f.openErr
	}
	return nopReadSeekCloser{bytes.NewReader(f.original)}, nil
}

func (f *fakePreviewStorage) SavePreview(r io.Reader) (storage.Saved, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return storage.Saved{}, err
	}
	f.saved = data
	return storage.Saved{Path: "preview/p.jpg"}, nil
}

func (f *fakePreviewStorage) Remove(path string) error {
	f.removePath = path
	return nil
}

func TestPreviewWorkerProcessNext(t *testing.T) {
	jpegJob := PreviewJob{ID: 7, StoragePath: "original/x.jpg", MimeType: "image/jpeg"}
	boom := errors.New("boom")

	tests := []struct {
		name    string
		repo    *fakePreviewRepo
		storage *fakePreviewStorage

		wantFound    bool
		wantErr      error
		wantStatus   string // что записано через SetPreviewStatus
		wantDonePath string // что записано через SetPreviewDone
		wantOpenPath string
		wantSaved    bool
		wantRemove   string
	}{
		{
			name:    "очередь пуста",
			repo:    &fakePreviewRepo{found: false},
			storage: &fakePreviewStorage{},
		},
		{
			name:    "база не ответила",
			repo:    &fakePreviewRepo{nextErr: boom},
			storage: &fakePreviewStorage{},
			wantErr: boom,
		},
		{
			name:       "видео — skipped, оригинал не открываем",
			repo:       &fakePreviewRepo{found: true, job: PreviewJob{ID: 7, StoragePath: "original/v.mp4", MimeType: "video/mp4"}},
			storage:    &fakePreviewStorage{},
			wantFound:  true,
			wantStatus: PreviewSkipped,
		},
		{
			name:         "оригинала нет на диске — failed",
			repo:         &fakePreviewRepo{found: true, job: jpegJob},
			storage:      &fakePreviewStorage{openErr: errors.New("no such file")},
			wantFound:    true,
			wantStatus:   PreviewFailed,
			wantOpenPath: "original/x.jpg",
		},
		{
			name:         "битая картинка — failed",
			repo:         &fakePreviewRepo{found: true, job: jpegJob},
			storage:      &fakePreviewStorage{original: []byte("это не jpeg")},
			wantFound:    true,
			wantStatus:   PreviewFailed,
			wantOpenPath: "original/x.jpg",
		},
		{
			name:         "превью сделано",
			repo:         &fakePreviewRepo{found: true, job: jpegJob},
			storage:      &fakePreviewStorage{original: testJPEG(t, 640, 480)},
			wantFound:    true,
			wantDonePath: "preview/p.jpg",
			wantOpenPath: "original/x.jpg",
			wantSaved:    true,
		},
		{
			name:         "превью сделано, но база не записала — файл превью удаляем",
			repo:         &fakePreviewRepo{found: true, job: jpegJob, doneErr: boom},
			storage:      &fakePreviewStorage{original: testJPEG(t, 640, 480)},
			wantFound:    true,
			wantErr:      boom,
			wantDonePath: "preview/p.jpg",
			wantOpenPath: "original/x.jpg",
			wantSaved:    true,
			wantRemove:   "preview/p.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := NewPreviewWorker(tt.repo, tt.storage)

			found, err := worker.processNext(context.Background())

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}
			if found != tt.wantFound {
				t.Errorf("found = %v, ожидалось %v", found, tt.wantFound)
			}
			if tt.repo.status != tt.wantStatus {
				t.Errorf("статус = %q, ожидался %q", tt.repo.status, tt.wantStatus)
			}
			if tt.wantStatus != "" && tt.repo.statusID != tt.repo.job.ID {
				t.Errorf("статус записан для id %d, ожидался %d", tt.repo.statusID, tt.repo.job.ID)
			}
			if tt.repo.donePath != tt.wantDonePath {
				t.Errorf("SetPreviewDone(%q), ожидался %q", tt.repo.donePath, tt.wantDonePath)
			}
			if tt.wantDonePath != "" && tt.repo.doneID != tt.repo.job.ID {
				t.Errorf("SetPreviewDone для id %d, ожидался %d", tt.repo.doneID, tt.repo.job.ID)
			}
			if tt.storage.openPath != tt.wantOpenPath {
				t.Errorf("Open(%q), ожидался Open(%q)", tt.storage.openPath, tt.wantOpenPath)
			}
			if gotSaved := tt.storage.saved != nil; gotSaved != tt.wantSaved {
				t.Errorf("превью сохранено = %v, ожидалось %v", gotSaved, tt.wantSaved)
			}
			if tt.wantSaved {
				if _, err := jpeg.Decode(bytes.NewReader(tt.storage.saved)); err != nil {
					t.Errorf("сохранённое превью — не JPEG: %v", err)
				}
			}
			if tt.storage.removePath != tt.wantRemove {
				t.Errorf("Remove(%q), ожидался Remove(%q)", tt.storage.removePath, tt.wantRemove)
			}
		})
	}
}

func TestPreviewWorkerWakeDoesNotBlock(t *testing.T) {
	worker := NewPreviewWorker(&fakePreviewRepo{}, &fakePreviewStorage{})

	worker.Wake()
	worker.Wake()

	if len(worker.wake) != 1 {
		t.Errorf("в канале %d сигналов, ожидался 1", len(worker.wake))
	}
}

func testJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
