package files

import (
	"bytes"
	"errors"
	"filevault/internal/storage"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestHandlerGetDirs(t *testing.T) {
	tests := []struct {
		name            string
		fakeDirRepo     *fakeDirRepo
		want            string
		wantCode        int
		wantContentType string
	}{
		{
			name:            "Одна директория",
			fakeDirRepo:     &fakeDirRepo{list: []DirInfo{{ID: 2, Name: "Семья", IsRoot: false, FileCount: 347}}},
			want:            `[{"id":2,"name":"Семья","is_root":false,"file_count":347}]`,
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
		},
		{
			name: "Несколько директорий",
			fakeDirRepo: &fakeDirRepo{list: []DirInfo{
				{ID: 1, Name: "Корень", IsRoot: true, FileCount: 12},
				{ID: 2, Name: "Отпуск", IsRoot: false, FileCount: 128},
				{ID: 3, Name: "Семья", IsRoot: false, FileCount: 347},
			}},
			want:            `[{"id":1,"name":"Корень","is_root":true,"file_count":12},{"id":2,"name":"Отпуск","is_root":false,"file_count":128},{"id":3,"name":"Семья","is_root":false,"file_count":347}]`,
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
		},
		{
			name:            "Пустой список",
			fakeDirRepo:     &fakeDirRepo{list: []DirInfo{}},
			want:            `[]`,
			wantContentType: "application/json",
			wantCode:        http.StatusOK,
		},
		{
			name:            "Репозиторий вернул ошибку",
			fakeDirRepo:     &fakeDirRepo{err: errors.New("")},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeFileRepo := &fakeFileRepo{list: []FileInfo{}}

			h := NewHandler(NewService(tt.fakeDirRepo, fakeFileRepo, nil, nil), 0)
			req := httptest.NewRequest(http.MethodGet, "/api/dirs", nil)
			rec := httptest.NewRecorder()

			h.GetDirs(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, tt.wantCode)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			got := strings.TrimSpace(rec.Body.String())
			if got != tt.want {
				t.Errorf("тело = %s, ожидалось %s", got, tt.want)
			}
		})
	}
}

func TestHandlerGetDirFiles(t *testing.T) {
	fakeTime, _ := time.Parse(time.RFC3339, "2020-08-19T14:32:10Z")

	tests := []struct {
		name            string
		dirId           string
		fakeFileRepo    *fakeFileRepo
		fakeDirRepo     *fakeDirRepo
		wantBody        string
		wantContentType string
		wantCode        int
		wantDirID       int64
	}{
		{
			name:        "Один файл",
			dirId:       "1",
			fakeDirRepo: &fakeDirRepo{exist: true},
			fakeFileRepo: &fakeFileRepo{
				list: []FileInfo{{
					ID:              123,
					OriginalName:    "IMG_0042.jpg",
					Description:     "Закат в Турции",
					Size:            9437184,
					MimeType:        "image/jpeg",
					OriginalCreated: fakeTime,
					HasPreview:      true,
					DateSource:      "exif",
				}},
			},
			wantBody:        `[{"id":123,"original_name":"IMG_0042.jpg","description":"Закат в Турции","size_bytes":9437184,"mime_type":"image/jpeg","original_created_at":"2020-08-19T14:32:10Z","has_preview":true,"date_source":"exif"}]`,
			wantCode:        http.StatusOK,
			wantDirID:       1,
			wantContentType: "application/json",
		},
		{
			name:        "Несколько файлов",
			dirId:       "1",
			fakeDirRepo: &fakeDirRepo{exist: true},
			fakeFileRepo: &fakeFileRepo{
				list: []FileInfo{
					{
						ID:              123,
						OriginalName:    "IMG_0042.jpg",
						Description:     "Закат в Турции",
						Size:            9437184,
						MimeType:        "image/jpeg",
						OriginalCreated: fakeTime,
						HasPreview:      true,
						DateSource:      "exif",
					},
					{
						ID:              124,
						OriginalName:    "IMG_0043.jpg",
						Description:     "Закат в Турции2",
						Size:            9437185,
						MimeType:        "image/jpeg",
						OriginalCreated: fakeTime,
						HasPreview:      false,
						DateSource:      "exif",
					},
				},
			},
			wantBody:        `[{"id":123,"original_name":"IMG_0042.jpg","description":"Закат в Турции","size_bytes":9437184,"mime_type":"image/jpeg","original_created_at":"2020-08-19T14:32:10Z","has_preview":true,"date_source":"exif"},{"id":124,"original_name":"IMG_0043.jpg","description":"Закат в Турции2","size_bytes":9437185,"mime_type":"image/jpeg","original_created_at":"2020-08-19T14:32:10Z","has_preview":false,"date_source":"exif"}]`,
			wantDirID:       1,
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
		},
		{
			name:        "Пустой список",
			dirId:       "1",
			fakeDirRepo: &fakeDirRepo{exist: true},
			fakeFileRepo: &fakeFileRepo{
				list: []FileInfo{},
			},
			wantBody:        `[]`,
			wantDirID:       1,
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
		},
		{
			name:            "строка в урл",
			dirId:           "eee",
			fakeDirRepo:     &fakeDirRepo{exist: true},
			fakeFileRepo:    &fakeFileRepo{},
			wantBody:        "invalid id",
			wantDirID:       0,
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "отрицательный ид",
			dirId:           "-1",
			fakeDirRepo:     &fakeDirRepo{exist: true},
			fakeFileRepo:    &fakeFileRepo{},
			wantBody:        "invalid id",
			wantCode:        http.StatusBadRequest,
			wantDirID:       0,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "Директория не найдена",
			dirId:           "2",
			fakeDirRepo:     &fakeDirRepo{exist: false},
			fakeFileRepo:    &fakeFileRepo{err: ErrDirNotFound},
			wantBody:        "dir is not found",
			wantDirID:       0,
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "Репозиторий вернул ошибку",
			dirId:           "2",
			wantDirID:       2,
			fakeDirRepo:     &fakeDirRepo{exist: true},
			fakeFileRepo:    &fakeFileRepo{err: errors.New("")},
			wantBody:        "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(tt.fakeDirRepo, tt.fakeFileRepo, nil, nil), 0)
			r := chi.NewRouter()
			r.Get("/api/dirs/{id}/files", h.GetDirFiles)

			url := fmt.Sprintf("/api/dirs/%s/files", tt.dirId)
			req := httptest.NewRequest(
				http.MethodGet,
				url,
				nil,
			)

			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf(
					"статус = %d, ожидался %d",
					rec.Code,
					tt.wantCode,
				)
			}

			got := strings.TrimSpace(rec.Body.String())
			if tt.fakeFileRepo.gotDirID != tt.wantDirID {
				t.Errorf("dirId in repo = %d, ожидался %d", tt.fakeFileRepo.gotDirID, tt.wantDirID)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if got != tt.wantBody {
				t.Errorf(
					"тело = %s, ожидалось %s",
					got,
					tt.wantBody,
				)
			}
		})
	}
}

func TestHandlerGetDirCountFiles(t *testing.T) {
	tests := []struct {
		name            string
		dirId           string
		fakeFileRepo    *fakeFileRepo
		fakeDirRepo     *fakeDirRepo
		want            string
		wantCode        int
		wantContentType string
		wantDirID       int64
	}{
		{
			name:            "строка в урл",
			dirId:           "1t",
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
			fakeFileRepo:    &fakeFileRepo{},
			fakeDirRepo:     &fakeDirRepo{},
		},
		{
			name:            "нулевой dir_id в урл",
			dirId:           "0",
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
			fakeFileRepo:    &fakeFileRepo{},
			fakeDirRepo:     &fakeDirRepo{},
		},
		{
			name:            "пустой dir_id в урл",
			dirId:           "",
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
			fakeFileRepo:    &fakeFileRepo{},
			fakeDirRepo:     &fakeDirRepo{},
		},
		{
			name:            "сервис вернул ошибку",
			dirId:           "2",
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantDirID:       2,
			wantContentType: "text/plain; charset=utf-8",
			fakeFileRepo:    &fakeFileRepo{err: errors.New("error")},
			fakeDirRepo:     &fakeDirRepo{exist: true},
		},
		{
			name:            "директория не найдена",
			dirId:           "3",
			want:            "dir is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
			fakeFileRepo:    &fakeFileRepo{},
			fakeDirRepo:     &fakeDirRepo{exist: false},
		},
		{
			name:            "нет файлов в директории",
			dirId:           "4",
			want:            `{"file_count":0}`,
			wantCode:        http.StatusOK,
			wantDirID:       4,
			wantContentType: "application/json",
			fakeFileRepo:    &fakeFileRepo{countFilesInDir: CountFilesInDir{FileCount: 0}},
			fakeDirRepo:     &fakeDirRepo{exist: true},
		},
		{
			name:            "есть файлы в директории",
			dirId:           "5",
			want:            `{"file_count":10}`,
			wantContentType: "application/json",
			wantDirID:       5,
			wantCode:        http.StatusOK,
			fakeFileRepo:    &fakeFileRepo{countFilesInDir: CountFilesInDir{FileCount: 10}},
			fakeDirRepo:     &fakeDirRepo{exist: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(tt.fakeDirRepo, tt.fakeFileRepo, nil, nil), 0)
			r := chi.NewRouter()

			r.Get("/api/dirs/{id}/count_files", h.GetDirCountFiles)

			url := fmt.Sprintf("/api/dirs/%s/count_files", tt.dirId)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf(
					"статус = %d, ожидался %d",
					rec.Code,
					tt.wantCode,
				)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			got := strings.TrimSpace(rec.Body.String())

			if tt.fakeFileRepo.gotDirID != tt.wantDirID {
				t.Errorf("dirId in repo = %d, ожидался %d", tt.fakeFileRepo.gotDirID, tt.wantDirID)
			}

			if got != tt.want {
				t.Errorf(
					"тело = %s, ожидалось %s",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestHandlerAddFileToDir(t *testing.T) {
	tests := []struct {
		name            string
		dirId           string
		fileName        string
		content         string
		data            string
		fakeDirRepo     *fakeDirRepo
		fakeFileRepo    *fakeFileRepo
		fakeStorage     *fakeFileStorage
		want            string
		wantCode        int
		wantContentType string
	}{
		{
			name:            "строка в урл",
			dirId:           "abc",
			fakeDirRepo:     &fakeDirRepo{},
			fakeFileRepo:    &fakeFileRepo{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "битый JSON в data",
			dirId:           "1",
			fileName:        "a.jpg",
			content:         "hello",
			data:            `{"modtime":`,
			fakeDirRepo:     &fakeDirRepo{},
			fakeFileRepo:    &fakeFileRepo{},
			want:            "invalid data",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "нет файла",
			dirId:           "1",
			data:            `{"modtime":"2023-08-28T14:32:10"}`,
			fakeDirRepo:     &fakeDirRepo{},
			fakeFileRepo:    &fakeFileRepo{},
			want:            "invalid form",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "файл больше лимита",
			dirId:           "1",
			fileName:        "big.jpg",
			content:         strings.Repeat("x", 2000),
			fakeDirRepo:     &fakeDirRepo{},
			fakeFileRepo:    &fakeFileRepo{},
			want:            "file too large",
			wantCode:        http.StatusRequestEntityTooLarge,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "директория не найдена",
			dirId:           "999",
			fileName:        "a.jpg",
			content:         "hello",
			fakeDirRepo:     &fakeDirRepo{exist: false},
			fakeFileRepo:    &fakeFileRepo{},
			want:            "dir is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "ошибка сохранения файла",
			dirId:           "999",
			fileName:        "a.jpg",
			content:         "hello",
			fakeDirRepo:     &fakeDirRepo{err: errors.New("boom")},
			fakeFileRepo:    &fakeFileRepo{},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "файл загружен",
			dirId:           "1",
			fileName:        "a.jpg",
			content:         "hello",
			data:            `{"modtime":"2023-08-28T14:32:10","description":"тест"}`,
			fakeDirRepo:     &fakeDirRepo{exist: true},
			fakeFileRepo:    &fakeFileRepo{returnID: 42},
			fakeStorage:     &fakeFileStorage{saved: storage.Saved{Path: "original/x.jpg", Size: 5}},
			want:            `{"id":42,"original_name":"a.jpg","description":"тест","size_bytes":5,"mime_type":"text/plain; charset=utf-8","original_created_at":"2023-08-28T14:32:10Z","has_preview":false,"date_source":"modtime"}`,
			wantCode:        http.StatusCreated,
			wantContentType: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(tt.fakeDirRepo, tt.fakeFileRepo, tt.fakeStorage, &fakePreviewQueue{}), 1024)
			r := chi.NewRouter()

			r.Post("/api/dirs/{id}/files", h.AddFileToDir)

			url := fmt.Sprintf("/api/dirs/%s/files", tt.dirId)
			req := newUploadRequest(t, url, tt.fileName, tt.content, tt.data)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf(
					"статус = %d, ожидался %d",
					rec.Code,
					tt.wantCode,
				)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf(
					"тело = %s, ожидалось %s",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestHandlerGetFileByID(t *testing.T) {
	tests := []struct {
		name             string
		fileId           string
		query            string
		rangeHeader      string
		fakeFileRepo     *fakeFileRepo
		fakeStorage      *fakeFileStorage
		want             string
		wantCode         int
		wantContentType  string
		wantFileID       int64
		wantPath         string
		wantDisposition  string
		wantCacheControl string
	}{
		{
			name:            "строка в урл",
			fileId:          "abc",
			fakeFileRepo:    &fakeFileRepo{},
			fakeStorage:     &fakeFileStorage{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "нуль в урл",
			fileId:          "0",
			fakeFileRepo:    &fakeFileRepo{},
			fakeStorage:     &fakeFileStorage{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "файл не найден",
			fileId:          "999",
			fakeFileRepo:    &fakeFileRepo{err: ErrFileNotFound},
			fakeStorage:     &fakeFileStorage{},
			want:            "file is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
			wantFileID:      999,
		},
		{
			name:            "ошибка базы",
			fileId:          "1",
			fakeFileRepo:    &fakeFileRepo{err: errors.New("boom")},
			fakeStorage:     &fakeFileStorage{},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
			wantFileID:      1,
		},
		{
			name:   "файл отдан",
			fileId: "7",
			fakeFileRepo: &fakeFileRepo{fileContent: FileContent{
				OriginalName: "a.jpg",
				StoragePath:  "original/x.jpg",
				MimeType:     "image/jpeg",
			}},
			fakeStorage:      &fakeFileStorage{openFile: nopReadSeekCloser{strings.NewReader("hello")}},
			want:             "hello",
			wantCode:         http.StatusOK,
			wantContentType:  "image/jpeg",
			wantFileID:       7,
			wantPath:         "original/x.jpg",
			wantDisposition:  "inline; filename=a.jpg",
			wantCacheControl: "private, max-age=31536000, immutable",
		},
		{
			name:   "скачивание по ?download=1, кириллица в имени",
			fileId: "8",
			query:  "?download=1",
			fakeFileRepo: &fakeFileRepo{fileContent: FileContent{
				OriginalName: "фото.jpg",
				StoragePath:  "original/y.jpg",
				MimeType:     "image/jpeg",
			}},
			fakeStorage:      &fakeFileStorage{openFile: nopReadSeekCloser{strings.NewReader("hello")}},
			want:             "hello",
			wantCode:         http.StatusOK,
			wantContentType:  "image/jpeg",
			wantFileID:       8,
			wantPath:         "original/y.jpg",
			wantDisposition:  "attachment; filename*=utf-8''%D1%84%D0%BE%D1%82%D0%BE.jpg",
			wantCacheControl: "private, max-age=31536000, immutable",
		},
		{
			name:   "text/html всегда attachment",
			fileId: "9",
			fakeFileRepo: &fakeFileRepo{fileContent: FileContent{
				OriginalName: "page.html",
				StoragePath:  "original/z.html",
				MimeType:     "text/html; charset=utf-8",
			}},
			fakeStorage:      &fakeFileStorage{openFile: nopReadSeekCloser{strings.NewReader("<script>")}},
			want:             "<script>",
			wantCode:         http.StatusOK,
			wantContentType:  "text/html; charset=utf-8",
			wantFileID:       9,
			wantPath:         "original/z.html",
			wantDisposition:  "attachment; filename=page.html",
			wantCacheControl: "private, max-age=31536000, immutable",
		},
		{
			name:        "Range отдаёт кусок",
			fileId:      "10",
			rangeHeader: "bytes=1-3",
			fakeFileRepo: &fakeFileRepo{fileContent: FileContent{
				OriginalName: "a.jpg",
				StoragePath:  "original/x.jpg",
				MimeType:     "image/jpeg",
			}},
			fakeStorage:      &fakeFileStorage{openFile: nopReadSeekCloser{strings.NewReader("hello")}},
			want:             "ell",
			wantCode:         http.StatusPartialContent,
			wantContentType:  "image/jpeg",
			wantFileID:       10,
			wantPath:         "original/x.jpg",
			wantDisposition:  "inline; filename=a.jpg",
			wantCacheControl: "private, max-age=31536000, immutable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(&fakeDirRepo{}, tt.fakeFileRepo, tt.fakeStorage, nil), 1024)
			r := chi.NewRouter()

			r.Get("/api/files/{id}/original", h.GetFileByID)

			url := fmt.Sprintf("/api/files/%s/original%s", tt.fileId, tt.query)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.rangeHeader != "" {
				req.Header.Set("Range", tt.rangeHeader)
			}
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if tt.fakeFileRepo.gotFileID != tt.wantFileID {
				t.Errorf("в репозиторий ушёл id %d, ожидался %d", tt.fakeFileRepo.gotFileID, tt.wantFileID)
			}

			if tt.fakeStorage.openPath != tt.wantPath {
				t.Errorf("Open(%q), ожидался Open(%q)", tt.fakeStorage.openPath, tt.wantPath)
			}

			if rec.Code != tt.wantCode {
				t.Fatalf(
					"статус = %d, ожидался %d",
					rec.Code,
					tt.wantCode,
				)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if cd := rec.Header().Get("Content-Disposition"); cd != tt.wantDisposition {
				t.Errorf("Content-Disposition = %q, ожидался %q", cd, tt.wantDisposition)
			}

			if cc := rec.Header().Get("Cache-Control"); cc != tt.wantCacheControl {
				t.Errorf("Cache-Control = %q, ожидался %q", cc, tt.wantCacheControl)
			}

			// На ошибках nosniff ставит http.Error, на успехе — сам хендлер.
			if xcto := rec.Header().Get("X-Content-Type-Options"); xcto != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, ожидался %q", xcto, "nosniff")
			}

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf(
					"тело = %s, ожидалось %s",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestHandlerGetFilePreview(t *testing.T) {
	tests := []struct {
		name             string
		fileId           string
		fakeFileRepo     *fakeFileRepo
		fakeStorage      *fakeFileStorage
		want             string
		wantCode         int
		wantContentType  string
		wantFileID       int64
		wantPath         string
		wantCacheControl string
	}{
		{
			name:            "строка в урл",
			fileId:          "abc",
			fakeFileRepo:    &fakeFileRepo{},
			fakeStorage:     &fakeFileStorage{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "файл не найден",
			fileId:          "999",
			fakeFileRepo:    &fakeFileRepo{err: ErrFileNotFound},
			fakeStorage:     &fakeFileStorage{},
			want:            "file is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
			wantFileID:      999,
		},
		{
			name:            "файл есть, превью нет",
			fileId:          "5",
			fakeFileRepo:    &fakeFileRepo{},
			fakeStorage:     &fakeFileStorage{},
			want:            "preview is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
			wantFileID:      5,
		},
		{
			name:            "ошибка базы",
			fileId:          "1",
			fakeFileRepo:    &fakeFileRepo{err: errors.New("boom")},
			fakeStorage:     &fakeFileStorage{},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
			wantFileID:      1,
		},
		{
			name:             "превью отдано",
			fileId:           "7",
			fakeFileRepo:     &fakeFileRepo{previewPath: "preview/p.jpg"},
			fakeStorage:      &fakeFileStorage{openFile: nopReadSeekCloser{strings.NewReader("jpeg")}},
			want:             "jpeg",
			wantCode:         http.StatusOK,
			wantContentType:  "image/jpeg",
			wantFileID:       7,
			wantPath:         "preview/p.jpg",
			wantCacheControl: "private, max-age=86400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(&fakeDirRepo{}, tt.fakeFileRepo, tt.fakeStorage, nil), 1024)
			r := chi.NewRouter()

			r.Get("/api/files/{id}/preview", h.GetFilePreview)

			url := fmt.Sprintf("/api/files/%s/preview", tt.fileId)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if tt.fakeFileRepo.gotFileID != tt.wantFileID {
				t.Errorf("в репозиторий ушёл id %d, ожидался %d", tt.fakeFileRepo.gotFileID, tt.wantFileID)
			}

			if tt.fakeStorage.openPath != tt.wantPath {
				t.Errorf("Open(%q), ожидался Open(%q)", tt.fakeStorage.openPath, tt.wantPath)
			}

			if rec.Code != tt.wantCode {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, tt.wantCode)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if cc := rec.Header().Get("Cache-Control"); cc != tt.wantCacheControl {
				t.Errorf("Cache-Control = %q, ожидался %q", cc, tt.wantCacheControl)
			}

			if xcto := rec.Header().Get("X-Content-Type-Options"); xcto != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, ожидался %q", xcto, "nosniff")
			}

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("тело = %s, ожидалось %s", got, tt.want)
			}
		})
	}
}

func newUploadRequest(t *testing.T, url, fileName, content, data string) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if fileName != "" {
		fw, err := mw.CreateFormFile("file", fileName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if data != "" {
		if err := mw.WriteField("data", data); err != nil {
			t.Fatal(err)
		}
	}

	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	return req
}

func TestHandlerDeleteFile(t *testing.T) {
	tests := []struct {
		name            string
		fileId          string
		fakeFileRepo    *fakeFileRepo
		want            string
		wantCode        int
		wantContentType string
		wantDeletedID   int64
	}{
		{
			name:            "строка в урл",
			fileId:          "abc",
			fakeFileRepo:    &fakeFileRepo{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "ноль в урл",
			fileId:          "0",
			fakeFileRepo:    &fakeFileRepo{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "файл не найден",
			fileId:          "999",
			fakeFileRepo:    &fakeFileRepo{err: ErrFileNotFound},
			want:            "file is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:   "ошибка базы при удалении",
			fileId: "3",
			fakeFileRepo: &fakeFileRepo{
				fileContent: FileContent{StoragePath: "original/a.jpg"},
				deleteErr:   errors.New("boom"),
			},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
			wantDeletedID:   3,
		},
		{
			name:   "файл удалён",
			fileId: "7",
			fakeFileRepo: &fakeFileRepo{
				fileContent: FileContent{StoragePath: "original/a.jpg"},
			},
			want:          "",
			wantCode:      http.StatusNoContent,
			wantDeletedID: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(&fakeDirRepo{}, tt.fakeFileRepo, &fakeFileStorage{}, nil), 1024)
			r := chi.NewRouter()

			r.Delete("/api/files/{id}", h.DeleteFile)

			url := fmt.Sprintf("/api/files/%s", tt.fileId)
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if tt.fakeFileRepo.deletedFileID != tt.wantDeletedID {
				t.Errorf("DELETE в репозиторий с id %d, ожидался %d", tt.fakeFileRepo.deletedFileID, tt.wantDeletedID)
			}

			if rec.Code != tt.wantCode {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, tt.wantCode)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("тело = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestHandlerCreateDir(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		fakeDirRepo     *fakeDirRepo
		want            string
		wantCode        int
		wantContentType string
		wantDirData     NewDirData // нулевое — репозиторий не звали
	}{
		{
			name:            "битый JSON",
			body:            `{"name":`,
			fakeDirRepo:     &fakeDirRepo{},
			want:            "invalid data",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "тело больше лимита",
			body:            `{"name":"Отпуск","description":"` + strings.Repeat("я", maxJSONBodyBytes) + `"}`,
			fakeDirRepo:     &fakeDirRepo{},
			want:            "request too large",
			wantCode:        http.StatusRequestEntityTooLarge,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "недопустимое имя",
			body:            `{"name":"a/b"}`,
			fakeDirRepo:     &fakeDirRepo{},
			want:            "invalid dir name",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "имя занято",
			body:            `{"name":"Отпуск"}`,
			fakeDirRepo:     &fakeDirRepo{err: ErrDirAlreadyExists},
			want:            "dir already exists",
			wantCode:        http.StatusConflict,
			wantContentType: "text/plain; charset=utf-8",
			wantDirData:     NewDirData{Name: "Отпуск"},
		},
		{
			name:            "ошибка базы",
			body:            `{"name":"Отпуск"}`,
			fakeDirRepo:     &fakeDirRepo{err: errors.New("boom")},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
			wantDirData:     NewDirData{Name: "Отпуск"},
		},
		{
			name:            "директория создана",
			body:            `{"name":"  Отпуск  ","description":"Море, 2019"}`,
			fakeDirRepo:     &fakeDirRepo{returnID: 5},
			want:            `{"id":5,"name":"Отпуск","is_root":false,"file_count":0}`,
			wantCode:        http.StatusCreated,
			wantContentType: "application/json",
			wantDirData:     NewDirData{Name: "Отпуск", Description: "Море, 2019"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(tt.fakeDirRepo, nil, nil, nil), 1024)
			r := chi.NewRouter()

			r.Post("/api/dirs", h.CreateDir)

			req := httptest.NewRequest(http.MethodPost, "/api/dirs", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if tt.fakeDirRepo.newDirData != tt.wantDirData {
				t.Errorf("в репозиторий ушло %+v, ожидалось %+v", tt.fakeDirRepo.newDirData, tt.wantDirData)
			}

			if rec.Code != tt.wantCode {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, tt.wantCode)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("тело = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestHandlerRenameFile(t *testing.T) {
	created := time.Date(2023, 8, 28, 14, 32, 10, 0, time.UTC)

	tests := []struct {
		name            string
		fileId          string
		body            string
		fakeFileRepo    *fakeFileRepo
		want            string
		wantCode        int
		wantContentType string
		wantRenamedID   int64
	}{
		{
			name:            "строка в урл",
			fileId:          "abc",
			body:            `{"name":"Море"}`,
			fakeFileRepo:    &fakeFileRepo{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "битый JSON",
			fileId:          "7",
			body:            `{"name":`,
			fakeFileRepo:    &fakeFileRepo{},
			want:            "invalid data",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "файл не найден",
			fileId:          "999",
			body:            `{"name":"Море"}`,
			fakeFileRepo:    &fakeFileRepo{err: ErrFileNotFound},
			want:            "file is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "недопустимое имя",
			fileId:          "7",
			body:            `{"name":"a:b"}`,
			fakeFileRepo:    &fakeFileRepo{fileContent: FileContent{OriginalName: "a.jpg"}},
			want:            "invalid file name",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "имя занято",
			fileId:          "7",
			body:            `{"name":"Море"}`,
			fakeFileRepo:    &fakeFileRepo{fileContent: FileContent{OriginalName: "a.jpg"}, nameTaken: true},
			want:            "file name already exists",
			wantCode:        http.StatusConflict,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:   "ошибка базы при UPDATE",
			fileId: "7",
			body:   `{"name":"Море"}`,
			fakeFileRepo: &fakeFileRepo{
				fileContent: FileContent{OriginalName: "a.jpg"},
				renameErr:   errors.New("boom"),
			},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
			wantRenamedID:   7,
		},
		{
			name:   "переименован",
			fileId: "5",
			body:   `{"name":"Море"}`,
			fakeFileRepo: &fakeFileRepo{
				fileContent: FileContent{OriginalName: "a.jpg"},
				renamed: FileInfo{
					ID: 5, OriginalName: "Море.jpg", Size: 256, MimeType: "image/jpeg",
					OriginalCreated: created, HasPreview: true, DateSource: "modtime",
				},
			},
			want: `{"id":5,"original_name":"Море.jpg","description":"","size_bytes":256,` +
				`"mime_type":"image/jpeg","original_created_at":"2023-08-28T14:32:10Z",` +
				`"has_preview":true,"date_source":"modtime"}`,
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
			wantRenamedID:   5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(&fakeDirRepo{}, tt.fakeFileRepo, nil, nil), 1024)
			r := chi.NewRouter()

			r.Post("/api/files/{id}/rename", h.RenameFile)

			url := fmt.Sprintf("/api/files/%s/rename", tt.fileId)
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if tt.fakeFileRepo.renamedFileID != tt.wantRenamedID {
				t.Errorf("UPDATE с id %d, ожидался %d", tt.fakeFileRepo.renamedFileID, tt.wantRenamedID)
			}

			if rec.Code != tt.wantCode {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, tt.wantCode)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("тело = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestHandlerRenameDir(t *testing.T) {
	tests := []struct {
		name            string
		dirId           string
		body            string
		fakeDirRepo     *fakeDirRepo
		want            string
		wantCode        int
		wantContentType string
		wantRenamedID   int64
	}{
		{
			name:            "строка в урл",
			dirId:           "abc",
			body:            `{"name":"Отпуск"}`,
			fakeDirRepo:     &fakeDirRepo{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "битый JSON",
			dirId:           "4",
			body:            `{"name":`,
			fakeDirRepo:     &fakeDirRepo{},
			want:            "invalid data",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "недопустимое имя",
			dirId:           "4",
			body:            `{"name":".."}`,
			fakeDirRepo:     &fakeDirRepo{},
			want:            "invalid dir name",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "папки нет или это корень",
			dirId:           "1",
			body:            `{"name":"Отпуск"}`,
			fakeDirRepo:     &fakeDirRepo{renameErr: ErrDirNotFound},
			want:            "dir is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
			wantRenamedID:   1,
		},
		{
			name:            "имя занято",
			dirId:           "4",
			body:            `{"name":"Отпуск"}`,
			fakeDirRepo:     &fakeDirRepo{renameErr: ErrDirAlreadyExists},
			want:            "dir already exists",
			wantCode:        http.StatusConflict,
			wantContentType: "text/plain; charset=utf-8",
			wantRenamedID:   4,
		},
		{
			name:            "ошибка базы",
			dirId:           "4",
			body:            `{"name":"Отпуск"}`,
			fakeDirRepo:     &fakeDirRepo{renameErr: errors.New("boom")},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
			wantRenamedID:   4,
		},
		{
			name:            "переименована",
			dirId:           "4",
			body:            `{"name":"Отпуск"}`,
			fakeDirRepo:     &fakeDirRepo{renamed: DirInfo{ID: 4, Name: "Отпуск", FileCount: 12}},
			want:            `{"id":4,"name":"Отпуск","is_root":false,"file_count":12}`,
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
			wantRenamedID:   4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(tt.fakeDirRepo, nil, nil, nil), 1024)
			r := chi.NewRouter()

			r.Post("/api/dirs/{id}/rename", h.RenameDir)

			url := fmt.Sprintf("/api/dirs/%s/rename", tt.dirId)
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if tt.fakeDirRepo.renamedDirID != tt.wantRenamedID {
				t.Errorf("в репозиторий ушёл id %d, ожидался %d", tt.fakeDirRepo.renamedDirID, tt.wantRenamedID)
			}

			if rec.Code != tt.wantCode {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, tt.wantCode)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("тело = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestHandlerDeleteDir(t *testing.T) {
	tests := []struct {
		name            string
		dirId           string
		fakeDirRepo     *fakeDirRepo
		want            string
		wantCode        int
		wantContentType string
		wantDeletedID   int64
	}{
		{
			name:            "строка в урл",
			dirId:           "abc",
			fakeDirRepo:     &fakeDirRepo{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "ноль в урл",
			dirId:           "0",
			fakeDirRepo:     &fakeDirRepo{},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "папки нет или это корень",
			dirId:           "1",
			fakeDirRepo:     &fakeDirRepo{deleteErr: ErrDirNotFound},
			want:            "dir is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
			wantDeletedID:   1,
		},
		{
			name:            "в папке есть файлы",
			dirId:           "4",
			fakeDirRepo:     &fakeDirRepo{deleteErr: ErrDirNotEmpty},
			want:            "dir is not empty",
			wantCode:        http.StatusConflict,
			wantContentType: "text/plain; charset=utf-8",
			wantDeletedID:   4,
		},
		{
			name:            "ошибка базы",
			dirId:           "4",
			fakeDirRepo:     &fakeDirRepo{deleteErr: errors.New("boom")},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
			wantDeletedID:   4,
		},
		{
			name:          "папка удалена",
			dirId:         "7",
			fakeDirRepo:   &fakeDirRepo{},
			want:          "",
			wantCode:      http.StatusNoContent,
			wantDeletedID: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(tt.fakeDirRepo, nil, nil, nil), 1024)
			r := chi.NewRouter()

			r.Delete("/api/dirs/{id}", h.DeleteDir)

			url := fmt.Sprintf("/api/dirs/%s", tt.dirId)
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if tt.fakeDirRepo.deletedDirID != tt.wantDeletedID {
				t.Errorf("в репозиторий ушёл id %d, ожидался %d", tt.fakeDirRepo.deletedDirID, tt.wantDeletedID)
			}

			if rec.Code != tt.wantCode {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, tt.wantCode)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("тело = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestHandlerMoveFile(t *testing.T) {
	content := FileContent{DirID: 3, OriginalName: "Море.jpg"}

	tests := []struct {
		name            string
		fileId          string
		body            string
		fakeFileRepo    *fakeFileRepo
		fakeDirRepo     *fakeDirRepo
		want            string
		wantCode        int
		wantContentType string
		wantMovedDirID  int64
	}{
		{
			name:            "строка в урл",
			fileId:          "abc",
			body:            `{"dir_id":5}`,
			fakeFileRepo:    &fakeFileRepo{},
			fakeDirRepo:     &fakeDirRepo{exist: true},
			want:            "invalid id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "битый JSON",
			fileId:          "7",
			body:            `{"dir_id":`,
			fakeFileRepo:    &fakeFileRepo{},
			fakeDirRepo:     &fakeDirRepo{exist: true},
			want:            "invalid data",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "нет dir_id",
			fileId:          "7",
			body:            `{}`,
			fakeFileRepo:    &fakeFileRepo{},
			fakeDirRepo:     &fakeDirRepo{exist: true},
			want:            "invalid dir id",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "файла нет",
			fileId:          "7",
			body:            `{"dir_id":5}`,
			fakeFileRepo:    &fakeFileRepo{err: ErrFileNotFound},
			fakeDirRepo:     &fakeDirRepo{exist: true},
			want:            "file is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "целевой папки нет",
			fileId:          "7",
			body:            `{"dir_id":5}`,
			fakeFileRepo:    &fakeFileRepo{fileContent: content},
			fakeDirRepo:     &fakeDirRepo{exist: false},
			want:            "dir is not found",
			wantCode:        http.StatusNotFound,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "та же папка",
			fileId:          "7",
			body:            `{"dir_id":3}`,
			fakeFileRepo:    &fakeFileRepo{fileContent: content},
			fakeDirRepo:     &fakeDirRepo{exist: true},
			want:            "file is already in this dir",
			wantCode:        http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "имя занято",
			fileId:          "7",
			body:            `{"dir_id":5}`,
			fakeFileRepo:    &fakeFileRepo{fileContent: content, nameTaken: true},
			fakeDirRepo:     &fakeDirRepo{exist: true},
			want:            "file name already exists",
			wantCode:        http.StatusConflict,
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "ошибка базы при UPDATE",
			fileId:          "7",
			body:            `{"dir_id":5}`,
			fakeFileRepo:    &fakeFileRepo{fileContent: content, moveErr: errors.New("boom")},
			fakeDirRepo:     &fakeDirRepo{exist: true},
			want:            "internal error",
			wantCode:        http.StatusInternalServerError,
			wantContentType: "text/plain; charset=utf-8",
			wantMovedDirID:  5,
		},
		{
			name:   "перенесён",
			fileId: "7",
			body:   `{"dir_id":5}`,
			fakeFileRepo: &fakeFileRepo{fileContent: content, moved: FileInfo{
				ID: 7, OriginalName: "Море.jpg", Size: 256, MimeType: "image/jpeg",
				OriginalCreated: time.Date(2023, 8, 28, 14, 32, 10, 0, time.UTC), DateSource: "modtime",
			}},
			fakeDirRepo: &fakeDirRepo{exist: true},
			want: `{"id":7,"original_name":"Море.jpg","description":"","size_bytes":256,` +
				`"mime_type":"image/jpeg","original_created_at":"2023-08-28T14:32:10Z",` +
				`"has_preview":false,"date_source":"modtime"}`,
			wantCode:        http.StatusOK,
			wantContentType: "application/json",
			wantMovedDirID:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(NewService(tt.fakeDirRepo, tt.fakeFileRepo, nil, nil), 1024)
			r := chi.NewRouter()

			r.Post("/api/files/{id}/move", h.MoveFile)

			url := fmt.Sprintf("/api/files/%s/move", tt.fileId)
			req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if tt.fakeFileRepo.movedDirID != tt.wantMovedDirID {
				t.Errorf("UPDATE в папку %d, ожидалась %d", tt.fakeFileRepo.movedDirID, tt.wantMovedDirID)
			}

			if rec.Code != tt.wantCode {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, tt.wantCode)
			}

			if ct := rec.Header().Get("Content-Type"); ct != tt.wantContentType {
				t.Errorf("Content-Type = %q, ожидался %q", ct, tt.wantContentType)
			}

			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("тело = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}
