package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

const (
	fixtureMime       = "image/jpeg"
	fixtureDateSource = "exif"
	fixtureSize       = 1024
	fixtureHash       = "deadbeef"
)

var fixtureCreated = time.Date(2020, 8, 19, 14, 32, 10, 0, time.UTC)

func TestIntegrationGetFileListInDir(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		seed    func(t *testing.T, tx *sql.Tx) (dirID int64, want []FileInfo)
		wantErr error
	}{
		{
			name: "файлы своей директории, сосед не попадает",
			seed: func(t *testing.T, tx *sql.Tx) (int64, []FileInfo) {
				dirA := insertDir(ctx, t, tx, "Отпуск")
				dirB := insertDir(ctx, t, tx, "Семья")

				fileA := insertFile(ctx, t, tx, dirA, "IMG_0001.jpg", true)
				fileB := insertFile(ctx, t, tx, dirA, "IMG_0002.jpg", false)

				// Путь есть, статус не 'done' — как строки seed.sql до worker'а.
				// has_preview обязан смотреть на статус, а не на путь
				fileC := insertFile(ctx, t, tx, dirA, "IMG_0003.jpg", true)
				if _, err := tx.ExecContext(ctx, `UPDATE files SET preview_status = 'pending' WHERE id = $1`, fileC); err != nil {
					t.Fatal(err)
				}

				insertFile(ctx, t, tx, dirB, "IMG_9999.jpg", false) // сосед

				return dirA, []FileInfo{
					wantFile(fileA, "IMG_0001.jpg", true),
					wantFile(fileB, "IMG_0002.jpg", false),
					wantFile(fileC, "IMG_0003.jpg", false),
				}
			},
		},
		{
			name: "пустая директория — пустой список, не ошибка",
			seed: func(t *testing.T, tx *sql.Tx) (int64, []FileInfo) {
				dirA := insertDir(ctx, t, tx, "Отпуск")
				return dirA, []FileInfo{}
			},
		},
		{
			name: "несуществующая директория",
			seed: func(t *testing.T, tx *sql.Tx) (int64, []FileInfo) {
				unlessDirID := unlessIdInTable(ctx, t, tx, "dirs")
				return unlessDirID, nil
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			dirID, want := tt.seed(t, tx)

			got, err := NewPgRepository(tx).GetFileListInDir(ctx, dirID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ошибка = %v, ожидалась %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			if !slices.Equal(got, want) {
				t.Errorf("got = %v, want = %v", got, want)
			}
		})
	}
}

func TestIntegrationGetDirList(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	var rootID int64
	var rootDirName string
	if err := tx.QueryRowContext(ctx, `SELECT id, name FROM dirs WHERE is_root`).Scan(&rootID, &rootDirName); err != nil {
		t.Fatal(err)
	}

	// Имена уникальны без учёта регистра (bike и Bike — одно), поэтому
	// регистр проверяем на разных словах: в "C" заглавные идут раньше
	// строчных (Bob < bike, Борис < байк), в en_US — наоборот
	dirA := insertDir(ctx, t, tx, "Отпуск")
	dirB := insertDir(ctx, t, tx, "Борис")
	dirC := insertDir(ctx, t, tx, "bike")
	dirD := insertDir(ctx, t, tx, "Bob")
	dirE := insertDir(ctx, t, tx, "байк")

	insertFile(ctx, t, tx, dirA, "IMG_0001.jpg", false)
	insertFile(ctx, t, tx, dirA, "IMG_0002.jpg", false)
	insertFile(ctx, t, tx, dirB, "IMG_0003.jpg", false)

	want := []DirInfo{
		{ID: dirC, Name: "bike", FileCount: 0},
		{ID: dirD, Name: "Bob", FileCount: 0},
		{ID: dirE, Name: "байк", FileCount: 0},
		{ID: dirB, Name: "Борис", FileCount: 1},
		{ID: rootID, Name: rootDirName, IsRoot: true, FileCount: 0},
		{ID: dirA, Name: "Отпуск", FileCount: 2},
	}

	got, err := NewPgRepository(tx).GetDirList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("got = %v, want = %v", got, want)
	}
}

func TestIntegrationInsertFile(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	dirID := insertDir(ctx, t, tx, "Wedding")

	want := NewFileData{
		DirID:           dirID,
		OriginalName:    "IMG_0042.jpg",
		Description:     "Some Description",
		StoragePath:     "original/2026/09/26/2c/abc.jpg",
		MimeType:        "image/jpeg",
		DateSource:      "modtime",
		SHA256:          "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		Size:            256,
		OriginalCreated: time.Date(2023, 8, 28, 14, 32, 10, 0, time.UTC),
	}

	newFileID, err := NewPgRepository(tx).InsertFile(ctx, want)
	if err != nil {
		t.Fatalf("InsertFile: %v", err)
	}

	var got NewFileData
	err = tx.QueryRowContext(
		ctx,
		`SELECT 
			dir_id, original_name, description, storage_path, mime_type,
			date_source, sha256, size_bytes, original_created_at
		FROM files
		WHERE id=$1
		`,
		newFileID,
	).Scan(
		&got.DirID,
		&got.OriginalName,
		&got.Description,
		&got.StoragePath,
		&got.MimeType,
		&got.DateSource,
		&got.SHA256,
		&got.Size,
		&got.OriginalCreated,
	)
	if err != nil {
		t.Fatalf("select inserted row: %v", err)
	}

	if got != want {
		t.Errorf("row mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestIntegrationDirExists(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name string
		seed func(t *testing.T, tx *sql.Tx) (dirID int64)
		want bool
	}{
		{
			name: "Директория существует",
			want: true,
			seed: func(t *testing.T, tx *sql.Tx) int64 {
				dirID := insertDir(ctx, t, tx, "Бугульма")

				return dirID
			},
		},
		{
			name: "Директория не существует",
			want: false,
			seed: func(t *testing.T, tx *sql.Tx) int64 {
				unlessDirID := unlessIdInTable(ctx, t, tx, "dirs")
				if err := tx.QueryRowContext(ctx, "SELECT coalesce(max(id), 0) + 1 FROM dirs").Scan(&unlessDirID); err != nil {
					t.Fatal(err)
				}
				return unlessDirID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			dirID := tt.seed(t, tx)

			got, err := NewPgRepository(tx).DirExists(ctx, dirID)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got = %v, want = %v", got, tt.want)
			}
		})
	}
}

func TestIntegrationGetCountFilesInDir(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name string
		seed func(t *testing.T, tx *sql.Tx) (int64, CountFilesInDir)
	}{
		{
			name: "Директория существует",
			seed: func(t *testing.T, tx *sql.Tx) (int64, CountFilesInDir) {
				dirID := insertDir(ctx, t, tx, "TEST_DIR")
				insertFile(ctx, t, tx, dirID, "test1", false)
				insertFile(ctx, t, tx, dirID, "test2", false)
				insertFile(ctx, t, tx, dirID, "test3", false)

				otherID := insertDir(ctx, t, tx, "TEST_DIR_2")
				insertFile(ctx, t, tx, otherID, "test4", false)

				return dirID, CountFilesInDir{FileCount: 3}
			},
		},
		{
			name: "Директория не существует",
			seed: func(t *testing.T, tx *sql.Tx) (int64, CountFilesInDir) {
				unlessDirID := unlessIdInTable(ctx, t, tx, "dirs")

				return unlessDirID, CountFilesInDir{}
			},
		},
		{
			name: "Директория существует 0 файлов",
			seed: func(t *testing.T, tx *sql.Tx) (int64, CountFilesInDir) {
				dirID := insertDir(ctx, t, tx, "TEST_DIR")

				return dirID, CountFilesInDir{FileCount: 0}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			dirID, countFilesInDir := tt.seed(t, tx)

			got, err := NewPgRepository(tx).GetCountFilesInDir(ctx, dirID)
			if err != nil {
				t.Fatal(err)
			}
			if got != countFilesInDir {
				t.Errorf("got = %v, want = %v", got, countFilesInDir)
			}
		})
	}
}

func TestIntegrationGetFileContentInfo(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		seed    func(ctx context.Context, t *testing.T, tx *sql.Tx) (fileID int64, want FileContent)
		wantErr error
	}{
		{
			name: "файл существует",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, FileContent) {
				var dirID, fileID int64
				fileName := "test2"

				dirID = insertDir(ctx, t, tx, "TEST_DIR")
				insertFile(ctx, t, tx, dirID, "test1", false)
				fileID = insertFile(ctx, t, tx, dirID, fileName, false)
				insertFile(ctx, t, tx, dirID, "test3", false)

				otherID := insertDir(ctx, t, tx, "TEST_DIR_2")
				insertFile(ctx, t, tx, otherID, "test4", false)

				fileInfo := wantFile(fileID, fileName, false)
				return fileID, FileContent{
					DirID:        dirID,
					MimeType:     fileInfo.MimeType,
					OriginalName: fileInfo.OriginalName,
					StoragePath:  fmt.Sprintf("orig/%d/%s", dirID, fileName),
				}
			},
		},
		{
			name: "файл не существует",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, FileContent) {
				fileID := unlessIdInTable(ctx, t, tx, "files")
				return fileID, FileContent{}
			},
			wantErr: ErrFileNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			fileID, want := tt.seed(ctx, t, tx)

			got, err := NewPgRepository(tx).GetFileContentInfo(ctx, fileID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}

			if want != got {
				t.Errorf("got = %v, want = %v", got, want)
			}
		})
	}
}

func TestIntegrationGetPreviewPath(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		seed    func(t *testing.T, tx *sql.Tx) (fileID int64, wantPath string)
		wantOK  bool
		wantErr error
	}{
		{
			name: "превью готово",
			seed: func(t *testing.T, tx *sql.Tx) (int64, string) {
				dirID := insertDir(ctx, t, tx, "Отпуск")
				insertFile(ctx, t, tx, dirID, "сосед.jpg", true)
				fileID := insertFile(ctx, t, tx, dirID, "IMG_0001.jpg", true)
				return fileID, fmt.Sprintf("previews/%d/IMG_0001.jpg", dirID)
			},
			wantOK: true,
		},
		{
			name: "превью ещё нет",
			seed: func(t *testing.T, tx *sql.Tx) (int64, string) {
				dirID := insertDir(ctx, t, tx, "Отпуск")
				return insertFile(ctx, t, tx, dirID, "IMG_0001.jpg", false), ""
			},
		},
		{
			name: "путь есть, статус не done",
			seed: func(t *testing.T, tx *sql.Tx) (int64, string) {
				dirID := insertDir(ctx, t, tx, "Отпуск")
				fileID := insertFile(ctx, t, tx, dirID, "IMG_0001.jpg", true)
				if _, err := tx.ExecContext(ctx, `UPDATE files SET preview_status = 'pending' WHERE id = $1`, fileID); err != nil {
					t.Fatal(err)
				}
				return fileID, ""
			},
		},
		{
			name: "файла нет",
			seed: func(t *testing.T, tx *sql.Tx) (int64, string) {
				return unlessIdInTable(ctx, t, tx, "files"), ""
			},
			wantErr: ErrFileNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			fileID, wantPath := tt.seed(t, tx)

			gotPath, gotOK, err := NewPgRepository(tx).GetPreviewPath(ctx, fileID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}
			if gotPath != wantPath || gotOK != tt.wantOK {
				t.Errorf("got = (%q, %v), want = (%q, %v)", gotPath, gotOK, wantPath, tt.wantOK)
			}
		})
	}
}

func TestIntegrationNextPendingPreview(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	repo := NewPgRepository(tx)

	if _, found, err := repo.NextPendingPreview(ctx); err != nil || found {
		t.Fatalf("пустая очередь: found = %v, err = %v; ожидалось false, nil", found, err)
	}

	dirID := insertDir(ctx, t, tx, "Отпуск")
	insertFile(ctx, t, tx, dirID, "готово.jpg", true) // 'done' — не в очереди
	first := insertFile(ctx, t, tx, dirID, "первый.jpg", false)
	insertFile(ctx, t, tx, dirID, "второй.jpg", false)

	got, found, err := repo.NextPendingPreview(ctx)
	if err != nil || !found {
		t.Fatalf("found = %v, err = %v; ожидалось true, nil", found, err)
	}

	want := PreviewJob{
		ID:          first,
		StoragePath: fmt.Sprintf("orig/%d/первый.jpg", dirID),
		MimeType:    fixtureMime,
	}
	if got != want {
		t.Errorf("got = %+v, want = %+v", got, want)
	}
}

func TestIntegrationSetPreview(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	repo := NewPgRepository(tx)
	dirID := insertDir(ctx, t, tx, "Отпуск")
	target := insertFile(ctx, t, tx, dirID, "цель.jpg", false)
	neighbour := insertFile(ctx, t, tx, dirID, "сосед.jpg", false)

	readBack := func(fileID int64) (status string, path sql.NullString) {
		t.Helper()
		err := tx.QueryRowContext(ctx,
			`SELECT preview_status, preview_path FROM files WHERE id = $1`, fileID,
		).Scan(&status, &path)
		if err != nil {
			t.Fatal(err)
		}
		return status, path
	}

	if err := repo.SetPreviewDone(ctx, target, "preview/x.jpg"); err != nil {
		t.Fatal(err)
	}
	if status, path := readBack(target); status != "done" || path.String != "preview/x.jpg" {
		t.Errorf("после SetPreviewDone: (%q, %q), ожидалось (done, preview/x.jpg)", status, path.String)
	}

	if err := repo.SetPreviewStatus(ctx, target, PreviewFailed); err != nil {
		t.Fatal(err)
	}
	if status, path := readBack(target); status != PreviewFailed || path.Valid {
		t.Errorf("после SetPreviewStatus: (%q, %v), ожидалось (failed, NULL)", status, path)
	}

	if status, path := readBack(neighbour); status != "pending" || path.Valid {
		t.Errorf("сосед изменился: (%q, %v)", status, path)
	}
}

func TestIntegrationDeleteFile(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		seed    func(ctx context.Context, t *testing.T, tx *sql.Tx) (fileID, neighbourID int64)
		wantErr error
	}{
		{
			name: "файл существует",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64) {
				dirID := insertDir(ctx, t, tx, "TEST_DIR")
				fileID := insertFile(ctx, t, tx, dirID, "удалить.jpg", false)
				neighbourID := insertFile(ctx, t, tx, dirID, "оставить.jpg", false)
				return fileID, neighbourID
			},
		},
		{
			name: "файл не существует",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64) {
				dirID := insertDir(ctx, t, tx, "TEST_DIR")
				neighbourID := insertFile(ctx, t, tx, dirID, "оставить.jpg", false)
				return unlessIdInTable(ctx, t, tx, "files"), neighbourID
			},
			wantErr: ErrFileNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			fileID, neighbourID := tt.seed(ctx, t, tx)

			err = NewPgRepository(tx).DeleteFile(ctx, fileID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}

			exists := func(id int64) bool {
				t.Helper()
				var found bool
				err := tx.QueryRowContext(ctx,
					`SELECT EXISTS(SELECT 1 FROM files WHERE id = $1)`, id,
				).Scan(&found)
				if err != nil {
					t.Fatal(err)
				}
				return found
			}

			if exists(fileID) {
				t.Errorf("файл %d остался в базе", fileID)
			}
			if !exists(neighbourID) {
				t.Errorf("сосед %d удалён вместе с файлом", neighbourID)
			}
		})
	}
}

func TestIntegrationCreateDir(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		seed    func(ctx context.Context, t *testing.T, tx *sql.Tx)
		dirData NewDirData
		wantErr error
	}{
		{
			name:    "директория создана",
			seed:    func(ctx context.Context, t *testing.T, tx *sql.Tx) {},
			dirData: NewDirData{Name: "Отпуск", Description: "Море, 2019"},
		},
		{
			name:    "пустое описание",
			seed:    func(ctx context.Context, t *testing.T, tx *sql.Tx) {},
			dirData: NewDirData{Name: "Отпуск"},
		},
		{
			name: "имя занято в другом регистре",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) {
				insertDir(ctx, t, tx, "Отпуск")
			},
			dirData: NewDirData{Name: "оТПУСК"},
			wantErr: ErrDirAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			tt.seed(ctx, t, tx)

			dirID, err := NewPgRepository(tx).CreateDir(ctx, tt.dirData)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			var got NewDirData
			var isRoot bool
			err = tx.QueryRowContext(ctx,
				`SELECT name, description, is_root FROM dirs WHERE id = $1`, dirID,
			).Scan(&got.Name, &got.Description, &isRoot)
			if err != nil {
				t.Fatalf("строка %d не найдена: %v", dirID, err)
			}
			if got != tt.dirData {
				t.Errorf("got = %+v, want = %+v", got, tt.dirData)
			}
			if isRoot {
				t.Errorf("новая директория %d стала корнем", dirID)
			}
		})
	}
}

func TestIntegrationFileNameTaken(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		seed  func(ctx context.Context, t *testing.T, tx *sql.Tx) (dirID, exceptFileID int64)
		check string
		want  bool
	}{
		{
			name: "занято в папке, в другом регистре",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64) {
				dirID := insertDir(ctx, t, tx, "Отпуск")
				insertFile(ctx, t, tx, dirID, "Море.jpg", false)
				return dirID, insertFile(ctx, t, tx, dirID, "IMG_0001.jpg", false)
			},
			check: "мОРЕ.JPG",
			want:  true,
		},
		{
			name: "такое имя только в соседней папке",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64) {
				dirID := insertDir(ctx, t, tx, "Отпуск")
				otherID := insertDir(ctx, t, tx, "Семья")
				insertFile(ctx, t, tx, otherID, "Море.jpg", false)
				return dirID, insertFile(ctx, t, tx, dirID, "IMG_0001.jpg", false)
			},
			check: "Море.jpg",
			want:  false,
		},
		{
			name: "своё же имя — не занято",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64) {
				dirID := insertDir(ctx, t, tx, "Отпуск")
				return dirID, insertFile(ctx, t, tx, dirID, "Море.jpg", false)
			},
			check: "Море.jpg",
			want:  false,
		},
		{
			name: "перенос: имя занято в чужой для файла папке",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64) {
				fromID := insertDir(ctx, t, tx, "Отпуск")
				toID := insertDir(ctx, t, tx, "Семья")
				insertFile(ctx, t, tx, toID, "Море.jpg", false)
				return toID, insertFile(ctx, t, tx, fromID, "Море.jpg", false)
			},
			check: "Море.jpg",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			dirID, exceptFileID := tt.seed(ctx, t, tx)

			got, err := NewPgRepository(tx).FileNameTaken(ctx, dirID, tt.check, exceptFileID)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got = %v, want = %v", got, tt.want)
			}
		})
	}
}

func TestIntegrationMoveFile(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		seed    func(ctx context.Context, t *testing.T, tx *sql.Tx) (fileID, toDirID, neighbourID int64)
		wantErr error
	}{
		{
			name: "перенесён",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64, int64) {
				fromID := insertDir(ctx, t, tx, "Отпуск")
				toID := insertDir(ctx, t, tx, "Семья")
				fileID := insertFile(ctx, t, tx, fromID, "IMG_0001.jpg", true)
				neighbourID := insertFile(ctx, t, tx, fromID, "IMG_0002.jpg", false)
				return fileID, toID, neighbourID
			},
		},
		{
			name: "файла нет",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64, int64) {
				fromID := insertDir(ctx, t, tx, "Отпуск")
				toID := insertDir(ctx, t, tx, "Семья")
				neighbourID := insertFile(ctx, t, tx, fromID, "IMG_0002.jpg", false)
				return unlessIdInTable(ctx, t, tx, "files"), toID, neighbourID
			},
			wantErr: ErrFileNotFound,
		},
		{
			name: "целевой папки нет",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64, int64) {
				fromID := insertDir(ctx, t, tx, "Отпуск")
				fileID := insertFile(ctx, t, tx, fromID, "IMG_0001.jpg", true)
				neighbourID := insertFile(ctx, t, tx, fromID, "IMG_0002.jpg", false)
				return fileID, unlessIdInTable(ctx, t, tx, "dirs"), neighbourID
			},
			wantErr: ErrDirNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			fileID, toDirID, neighbourID := tt.seed(ctx, t, tx)

			dirOf := func(id int64) int64 {
				t.Helper()
				var dirID int64
				if err := tx.QueryRowContext(ctx, `SELECT dir_id FROM files WHERE id = $1`, id).Scan(&dirID); err != nil {
					t.Fatal(err)
				}
				return dirID
			}
			neighbourDir := dirOf(neighbourID)

			if _, err := tx.ExecContext(ctx, `SAVEPOINT move`); err != nil {
				t.Fatal(err)
			}

			got, err := NewPgRepository(tx).MoveFile(ctx, fileID, toDirID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}

			if err != nil {
				if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT move`); err != nil {
					t.Fatal(err)
				}
			} else {
				if want := wantFile(fileID, "IMG_0001.jpg", true); got != want {
					t.Errorf("got = %+v, want = %+v", got, want)
				}
				if dir := dirOf(fileID); dir != toDirID {
					t.Errorf("файл в папке %d, ожидалась %d", dir, toDirID)
				}
			}

			if dir := dirOf(neighbourID); dir != neighbourDir {
				t.Errorf("сосед переехал из %d в %d", neighbourDir, dir)
			}
		})
	}
}

func TestIntegrationRenameFile(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		seed    func(ctx context.Context, t *testing.T, tx *sql.Tx) (fileID, neighbourID int64)
		wantErr error
	}{
		{
			name: "файл существует",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64) {
				dirID := insertDir(ctx, t, tx, "Отпуск")
				fileID := insertFile(ctx, t, tx, dirID, "IMG_0001.jpg", true)
				neighbourID := insertFile(ctx, t, tx, dirID, "IMG_0002.jpg", false)
				return fileID, neighbourID
			},
		},
		{
			name: "файл не существует",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, int64) {
				dirID := insertDir(ctx, t, tx, "Отпуск")
				neighbourID := insertFile(ctx, t, tx, dirID, "IMG_0002.jpg", false)
				return unlessIdInTable(ctx, t, tx, "files"), neighbourID
			},
			wantErr: ErrFileNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			fileID, neighbourID := tt.seed(ctx, t, tx)

			got, err := NewPgRepository(tx).RenameFile(ctx, fileID, "Море.jpg")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				if want := wantFile(fileID, "Море.jpg", true); got != want {
					t.Errorf("got = %+v, want = %+v", got, want)
				}
			}

			var neighbourName string
			err = tx.QueryRowContext(ctx,
				`SELECT original_name FROM files WHERE id = $1`, neighbourID,
			).Scan(&neighbourName)
			if err != nil {
				t.Fatal(err)
			}
			if neighbourName != "IMG_0002.jpg" {
				t.Errorf("сосед переименован в %q", neighbourName)
			}
		})
	}
}

func TestIntegrationRenameDir(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		seed    func(ctx context.Context, t *testing.T, tx *sql.Tx) (dirID int64, want DirInfo)
		newName string
		wantErr error
	}{
		{
			name: "переименована, счётчик своих файлов",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, DirInfo) {
				dirID := insertDir(ctx, t, tx, "Отпск")
				otherID := insertDir(ctx, t, tx, "Семья")
				insertFile(ctx, t, tx, dirID, "IMG_0001.jpg", false)
				insertFile(ctx, t, tx, dirID, "IMG_0002.jpg", false)
				insertFile(ctx, t, tx, otherID, "IMG_0003.jpg", false)
				return dirID, DirInfo{ID: dirID, Name: "Отпуск", FileCount: 2}
			},
			newName: "Отпуск",
		},
		{
			name: "сменить только регистр можно",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, DirInfo) {
				dirID := insertDir(ctx, t, tx, "bike")
				return dirID, DirInfo{ID: dirID, Name: "Bike"}
			},
			newName: "Bike",
		},
		{
			name: "имя занято другой папкой в другом регистре",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, DirInfo) {
				insertDir(ctx, t, tx, "Отпуск")
				return insertDir(ctx, t, tx, "Семья"), DirInfo{}
			},
			newName: "оТПУСК",
			wantErr: ErrDirAlreadyExists,
		},
		{
			name: "корень",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, DirInfo) {
				var rootID int64
				if err := tx.QueryRowContext(ctx, `SELECT id FROM dirs WHERE is_root`).Scan(&rootID); err != nil {
					t.Fatal(err)
				}
				return rootID, DirInfo{}
			},
			newName: "Отпуск",
			wantErr: ErrDirNotFound,
		},
		{
			name: "папки нет",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) (int64, DirInfo) {
				return unlessIdInTable(ctx, t, tx, "dirs"), DirInfo{}
			},
			newName: "Отпуск",
			wantErr: ErrDirNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			dirID, want := tt.seed(ctx, t, tx)

			namesBefore := otherDirNames(ctx, t, tx, dirID)

			if _, err := tx.ExecContext(ctx, `SAVEPOINT rename`); err != nil {
				t.Fatal(err)
			}

			got, err := NewPgRepository(tx).RenameDir(ctx, dirID, tt.newName)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}
			if got != want {
				t.Errorf("got = %+v, want = %+v", got, want)
			}

			if err != nil {
				if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT rename`); err != nil {
					t.Fatal(err)
				}
			}

			namesAfter := otherDirNames(ctx, t, tx, dirID)
			if !slices.Equal(namesBefore, namesAfter) {
				t.Errorf("другие папки изменились: было %v, стало %v", namesBefore, namesAfter)
			}
		})
	}
}

func TestIntegrationDeleteDir(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		seed    func(ctx context.Context, t *testing.T, tx *sql.Tx) (dirID int64)
		wantErr error
	}{
		{
			name: "пустая папка удалена",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) int64 {
				return insertDir(ctx, t, tx, "Пустая")
			},
		},
		{
			name: "в папке есть файлы",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) int64 {
				dirID := insertDir(ctx, t, tx, "Отпуск")
				insertFile(ctx, t, tx, dirID, "IMG_0001.jpg", false)
				return dirID
			},
			wantErr: ErrDirNotEmpty,
		},
		{
			name: "корень",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) int64 {
				var rootID int64
				if err := tx.QueryRowContext(ctx, `SELECT id FROM dirs WHERE is_root`).Scan(&rootID); err != nil {
					t.Fatal(err)
				}
				return rootID
			},
			wantErr: ErrDirNotFound,
		},
		{
			name: "папки нет",
			seed: func(ctx context.Context, t *testing.T, tx *sql.Tx) int64 {
				return unlessIdInTable(ctx, t, tx, "dirs")
			},
			wantErr: ErrDirNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			exists := func(id int64) bool {
				t.Helper()
				var found bool
				err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dirs WHERE id = $1)`, id).Scan(&found)
				if err != nil {
					t.Fatal(err)
				}
				return found
			}

			neighbourID := insertDir(ctx, t, tx, "Сосед")
			dirID := tt.seed(ctx, t, tx)
			existedBefore := exists(dirID)

			var filesBefore int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM files`).Scan(&filesBefore); err != nil {
				t.Fatal(err)
			}

			if _, err := tx.ExecContext(ctx, `SAVEPOINT delete_dir`); err != nil {
				t.Fatal(err)
			}

			err = NewPgRepository(tx).DeleteDir(ctx, dirID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}

			if err != nil {
				if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT delete_dir`); err != nil {
					t.Fatal(err)
				}
			}

			if got, want := exists(dirID), existedBefore && tt.wantErr != nil; got != want {
				t.Errorf("папка %d: существует = %v, ожидалось %v", dirID, got, want)
			}
			if !exists(neighbourID) {
				t.Errorf("сосед %d удалён", neighbourID)
			}

			var filesAfter int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM files`).Scan(&filesAfter); err != nil {
				t.Fatal(err)
			}
			if filesAfter != filesBefore {
				t.Errorf("файлов было %d, стало %d", filesBefore, filesAfter)
			}
		})
	}
}

func otherDirNames(ctx context.Context, t *testing.T, tx *sql.Tx, exceptID int64) []string {
	t.Helper()

	rows, err := tx.QueryContext(ctx, `SELECT name FROM dirs WHERE id <> $1 ORDER BY id`, exceptID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	return names
}

func unlessIdInTable(ctx context.Context, t *testing.T, tx *sql.Tx, tableName string) int64 {
	t.Helper()

	query := fmt.Sprintf("SELECT coalesce(max(id), 0) + 1 FROM %s", tableName)
	var unlessDirID int64
	if err := tx.QueryRowContext(ctx, query).Scan(&unlessDirID); err != nil {
		t.Fatal(err)
	}

	return unlessDirID
}

func insertDir(ctx context.Context, t *testing.T, tx *sql.Tx, name string) int64 {
	t.Helper()

	const query = `INSERT INTO dirs (name, is_root) VALUES ($1, FALSE) RETURNING id`
	var id int64

	err := tx.QueryRowContext(ctx, query, name).Scan(&id)
	if err != nil {
		t.Fatalf("insertDir(%q): %v", name, err)
	}
	return id
}

func insertFile(ctx context.Context, t *testing.T, tx *sql.Tx, dirID int64, name string, hasPreview bool) int64 {
	t.Helper()

	const query = `
		INSERT INTO files
			(dir_id, original_name, storage_path, preview_path, preview_status,
			mime_type, date_source, sha256, size_bytes, original_created_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`

	var preview any // nil уедет в базу как NULL
	previewStatus := "pending"
	if hasPreview {
		preview = fmt.Sprintf("previews/%d/%s", dirID, name)
		previewStatus = "done"
	}

	var id int64
	err := tx.QueryRowContext(ctx,
		query,
		dirID,
		name,
		fmt.Sprintf("orig/%d/%s", dirID, name),
		preview,
		previewStatus,
		fixtureMime,
		fixtureDateSource,
		fixtureHash,
		fixtureSize,
		fixtureCreated,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insertFile(%q): %v", name, err)
	}

	return id
}

func wantFile(id int64, name string, hasPreview bool) FileInfo {
	return FileInfo{
		ID:              id,
		OriginalName:    name,
		Description:     "",
		Size:            fixtureSize,
		MimeType:        fixtureMime,
		OriginalCreated: fixtureCreated,
		HasPreview:      hasPreview,
		DateSource:      fixtureDateSource,
	}
}
