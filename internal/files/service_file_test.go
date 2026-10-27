package files

import (
	"context"
	"errors"
	"filevault/internal/storage"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeFileRepo struct {
	list            []FileInfo
	newFileData     NewFileData
	countFilesInDir CountFilesInDir
	fileContent     FileContent
	previewPath     string
	err             error
	gotDirID        int64
	returnID        int64
	gotFileID       int64
	deleteErr       error
	deletedFileID   int64
	nameTaken       bool
	nameTakenErr    error
	checkedDirID    int64
	checkedName     string
	checkedExceptID int64
	renamed         FileInfo
	renameErr       error
	renamedFileID   int64
	renamedName     string
	moved           FileInfo
	moveErr         error
	movedFileID     int64
	movedDirID      int64
}

func (f *fakeFileRepo) GetFileListInDir(ctx context.Context, dirID int64) ([]FileInfo, error) {
	f.gotDirID = dirID
	return f.list, f.err
}

func (f *fakeFileRepo) InsertFile(ctx context.Context, model NewFileData) (int64, error) {
	f.newFileData = model
	return f.returnID, f.err
}

func (f *fakeFileRepo) GetCountFilesInDir(ctx context.Context, dirID int64) (CountFilesInDir, error) {
	f.gotDirID = dirID
	return f.countFilesInDir, f.err
}

func (f *fakeFileRepo) GetPreviewPath(ctx context.Context, fileID int64) (string, bool, error) {
	f.gotFileID = fileID
	return f.previewPath, f.previewPath != "", f.err
}

func (f *fakeFileRepo) GetFileContentInfo(ctx context.Context, fileID int64) (FileContent, error) {
	f.gotFileID = fileID
	return f.fileContent, f.err
}

func (f *fakeFileRepo) DeleteFile(ctx context.Context, fileID int64) error {
	f.deletedFileID = fileID
	return f.deleteErr
}

func (f *fakeFileRepo) FileNameTaken(ctx context.Context, dirID int64, name string, exceptFileID int64) (bool, error) {
	f.checkedDirID = dirID
	f.checkedName = name
	f.checkedExceptID = exceptFileID
	return f.nameTaken, f.nameTakenErr
}

func (f *fakeFileRepo) MoveFile(ctx context.Context, fileID, dirID int64) (FileInfo, error) {
	f.movedFileID = fileID
	f.movedDirID = dirID
	return f.moved, f.moveErr
}

func (f *fakeFileRepo) RenameFile(ctx context.Context, fileID int64, name string) (FileInfo, error) {
	f.renamedFileID = fileID
	f.renamedName = name
	return f.renamed, f.renameErr
}

type fakeFileStorage struct {
	saved      storage.Saved
	saveError  error
	saveCalled bool
	removePath string
	openFile   io.ReadSeekCloser
	openErr    error
	openPath   string

	trashErr      map[string]error // путь -> ошибка MoveToTrash
	trashedPaths  []string
	restoredPaths []string
	removedPaths  []string
}

func (f *fakeFileStorage) Save(r io.Reader, ext string) (storage.Saved, error) {
	f.saveCalled = true
	return f.saved, f.saveError
}

func (f *fakeFileStorage) Remove(path string) error {
	f.removePath = path
	f.removedPaths = append(f.removedPaths, path)
	return nil
}

func (f *fakeFileStorage) Open(path string) (io.ReadSeekCloser, error) {
	f.openPath = path
	return f.openFile, f.openErr
}

func (f *fakeFileStorage) MoveToTrash(path string) (string, error) {
	if err := f.trashErr[path]; err != nil {
		return "", err
	}
	f.trashedPaths = append(f.trashedPaths, path)
	return "trash/" + path, nil
}

func (f *fakeFileStorage) RestoreFromTrash(trashPath, path string) error {
	f.restoredPaths = append(f.restoredPaths, trashPath+" -> "+path)
	return nil
}

type fakePreviewQueue struct {
	woken bool
}

func (f *fakePreviewQueue) Wake() {
	f.woken = true
}

type nopReadSeekCloser struct {
	io.ReadSeeker
}

func (nopReadSeekCloser) Close() error { return nil }

func TestAddFile(t *testing.T) {
	fakeStorage := fakeFileStorage{
		saved: storage.Saved{Path: "original/x.jpg"},
	}

	fakeRepo := fakeFileRepo{
		err:      errors.New("boom"),
		returnID: 0,
	}

	queue := fakePreviewQueue{}

	_, err := NewService(&fakeDirRepo{exist: true}, &fakeRepo, &fakeStorage, &queue).AddFile(
		context.Background(),
		strings.NewReader("hello"),
		NewFile{
			DirID:        1,
			OriginalName: "x.jpg",
			Description:  "",
		},
	)
	if !errors.Is(err, fakeRepo.err) {
		t.Fatalf("ошибка = %v, wantErr = %v", err, fakeRepo.err)
	}

	if fakeStorage.removePath != "original/x.jpg" {
		t.Errorf("удалили %q, а сохраняли original/x.jpg", fakeStorage.removePath)
	}

	if queue.woken {
		t.Errorf("worker превью разбужен, хотя строки в базе нет")
	}
}

func TestAddFileDirNotFound(t *testing.T) {
	fakeStorage := fakeFileStorage{
		saved: storage.Saved{Path: "original/x.jpg"},
	}

	fakeRepo := fakeFileRepo{
		returnID: 0,
	}

	_, err := NewService(&fakeDirRepo{exist: false}, &fakeRepo, &fakeStorage, nil).AddFile(
		context.Background(),
		strings.NewReader("hello"),
		NewFile{
			DirID:        1,
			OriginalName: "x.jpg",
			Description:  "",
		},
	)
	if !errors.Is(err, ErrDirNotFound) {
		t.Fatalf("ошибка = %v, want ErrDirNotFound", err)
	}
	if fakeStorage.saveCalled {
		t.Errorf("Save вызван, хотя директории нет")
	}
}

func TestAddFileNoModtime(t *testing.T) {
	fakeStorage := fakeFileStorage{
		saved: storage.Saved{Path: "original/x.jpg"},
	}

	fakeRepo := fakeFileRepo{
		returnID: 0,
	}

	queue := fakePreviewQueue{}

	_, err := NewService(&fakeDirRepo{exist: true}, &fakeRepo, &fakeStorage, &queue).AddFile(
		context.Background(),
		strings.NewReader("hello"),
		NewFile{
			DirID:        1,
			OriginalName: "x.jpg",
			Description:  "",
		},
	)
	if err != nil {
		t.Fatalf("ошибка = %v", err)
	}
	if fakeRepo.newFileData.DateSource != "upload" {
		t.Errorf("DateSource = %q, want upload", fakeRepo.newFileData.DateSource)
	}
	if !queue.woken {
		t.Errorf("файл добавлен, а worker превью не разбужен")
	}
}

func TestServiceGetFileListInDir(t *testing.T) {
	fakeTime, _ := time.Parse(time.RFC3339, "2020-08-19T14:32:10Z")
	tests := []struct {
		name         string
		dirId        int64
		fakeFileRepo *fakeFileRepo
		fakeDirRepo  *fakeDirRepo
		want         []FileInfo
		wantErr      bool
	}{
		{
			name:        "несколько файлов",
			dirId:       1,
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
						OriginalName:    "IMG_0048.jpg",
						Description:     "Рассвет в Турции",
						Size:            9437184,
						MimeType:        "image/jpeg",
						OriginalCreated: fakeTime,
						HasPreview:      false,
						DateSource:      "exif",
					},
				},
			},
			want: []FileInfo{
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
					OriginalName:    "IMG_0048.jpg",
					Description:     "Рассвет в Турции",
					Size:            9437184,
					MimeType:        "image/jpeg",
					OriginalCreated: fakeTime,
					HasPreview:      false,
					DateSource:      "exif",
				},
			},
		},
		{
			name:        "один файл файлов",
			dirId:       1,
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
				},
			},
			want: []FileInfo{
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
			},
		},
		{
			name:         "пустой список",
			dirId:        1,
			fakeDirRepo:  &fakeDirRepo{exist: true},
			fakeFileRepo: &fakeFileRepo{list: []FileInfo{}},
			want:         []FileInfo{},
		},
		{
			name:         "репозиторий вернул ошибку",
			dirId:        1,
			fakeDirRepo:  &fakeDirRepo{exist: true},
			fakeFileRepo: &fakeFileRepo{err: errors.New("boom")},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(tt.fakeDirRepo, tt.fakeFileRepo, nil, nil).GetFileListInDir(context.Background(), tt.dirId)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ошибка = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got = %v, want = %v", got, tt.want)
			}
		})
	}
}

func TestDeleteFile(t *testing.T) {
	boom := errors.New("boom")

	tests := []struct {
		name          string
		fileRepo      *fakeFileRepo
		storage       *fakeFileStorage
		wantErr       error
		wantDeletedID int64
		wantTrashed   []string
		wantRestored  []string
		wantRemoved   []string
	}{
		{
			name: "оригинал и превью",
			fileRepo: &fakeFileRepo{
				fileContent: FileContent{StoragePath: "original/a.jpg"},
				previewPath: "preview/b.jpg",
			},
			storage:       &fakeFileStorage{},
			wantDeletedID: 7,
			wantTrashed:   []string{"original/a.jpg", "preview/b.jpg"},
			wantRemoved:   []string{"trash/original/a.jpg", "trash/preview/b.jpg"},
		},
		{
			name: "превью нет",
			fileRepo: &fakeFileRepo{
				fileContent: FileContent{StoragePath: "original/a.jpg"},
			},
			storage:       &fakeFileStorage{},
			wantDeletedID: 7,
			wantTrashed:   []string{"original/a.jpg"},
			wantRemoved:   []string{"trash/original/a.jpg"},
		},
		{
			name:     "файла нет",
			fileRepo: &fakeFileRepo{err: ErrFileNotFound},
			storage:  &fakeFileStorage{},
			wantErr:  ErrFileNotFound,
		},
		{
			name: "DELETE упал — файлы вернулись",
			fileRepo: &fakeFileRepo{
				fileContent: FileContent{StoragePath: "original/a.jpg"},
				previewPath: "preview/b.jpg",
				deleteErr:   boom,
			},
			storage:       &fakeFileStorage{},
			wantErr:       boom,
			wantDeletedID: 7,
			wantTrashed:   []string{"original/a.jpg", "preview/b.jpg"},
			wantRestored: []string{
				"trash/original/a.jpg -> original/a.jpg",
				"trash/preview/b.jpg -> preview/b.jpg",
			},
		},
		{
			name: "оригинал не перенёсся — база не тронута",
			fileRepo: &fakeFileRepo{
				fileContent: FileContent{StoragePath: "original/a.jpg"},
				previewPath: "preview/b.jpg",
			},
			storage: &fakeFileStorage{
				trashErr: map[string]error{"original/a.jpg": boom},
			},
			wantErr: boom,
		},
		{
			name: "превью не перенеслось — файл всё равно удалён",
			fileRepo: &fakeFileRepo{
				fileContent: FileContent{StoragePath: "original/a.jpg"},
				previewPath: "preview/b.jpg",
			},
			storage: &fakeFileStorage{
				trashErr: map[string]error{"preview/b.jpg": boom},
			},
			wantDeletedID: 7,
			wantTrashed:   []string{"original/a.jpg"},
			wantRemoved:   []string{"trash/original/a.jpg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(&fakeDirRepo{}, tt.fileRepo, tt.storage, &fakePreviewQueue{})

			err := service.DeleteFile(context.Background(), 7)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}

			if tt.fileRepo.deletedFileID != tt.wantDeletedID {
				t.Errorf("DeleteFile в репозитории с id %d, want %d", tt.fileRepo.deletedFileID, tt.wantDeletedID)
			}
			if !slices.Equal(tt.storage.trashedPaths, tt.wantTrashed) {
				t.Errorf("в trash: %q, want %q", tt.storage.trashedPaths, tt.wantTrashed)
			}
			if !slices.Equal(tt.storage.restoredPaths, tt.wantRestored) {
				t.Errorf("возвращены: %q, want %q", tt.storage.restoredPaths, tt.wantRestored)
			}
			if !slices.Equal(tt.storage.removedPaths, tt.wantRemoved) {
				t.Errorf("удалены из trash: %q, want %q", tt.storage.removedPaths, tt.wantRemoved)
			}
		})
	}
}

func TestRenameFile(t *testing.T) {
	renamed := FileInfo{ID: 7, OriginalName: "Море 2019.jpg"}
	boom := errors.New("boom")

	tests := []struct {
		name            string
		repo            *fakeFileRepo
		baseName        string
		want            FileInfo
		wantErr         error
		wantCheckedName string
		wantRenamedName string
	}{
		{
			name:            "пробелы обрезаны, расширение прежнее",
			repo:            &fakeFileRepo{fileContent: FileContent{DirID: 3, OriginalName: "IMG_0001.jpg"}, renamed: renamed},
			baseName:        "  Море 2019 ",
			want:            renamed,
			wantCheckedName: "Море 2019.jpg",
			wantRenamedName: "Море 2019.jpg",
		},
		{
			name:            "точка в новом имени расширение не меняет",
			repo:            &fakeFileRepo{fileContent: FileContent{OriginalName: "IMG_0001.jpg"}},
			baseName:        "кадр.png",
			wantCheckedName: "кадр.png.jpg",
			wantRenamedName: "кадр.png.jpg",
		},
		{
			name:            "у файла не было расширения",
			repo:            &fakeFileRepo{fileContent: FileContent{OriginalName: "README"}},
			baseName:        "notes",
			wantCheckedName: "notes",
			wantRenamedName: "notes",
		},
		{
			name:            "255 байт вместе с расширением",
			repo:            &fakeFileRepo{fileContent: FileContent{OriginalName: "a.jpg"}},
			baseName:        strings.Repeat("x", 251),
			wantCheckedName: strings.Repeat("x", 251) + ".jpg",
			wantRenamedName: strings.Repeat("x", 251) + ".jpg",
		},
		{
			name:     "256 байт вместе с расширением",
			repo:     &fakeFileRepo{fileContent: FileContent{OriginalName: "a.jpg"}},
			baseName: strings.Repeat("x", 252),
			wantErr:  ErrInvalidName,
		},
		{
			name:     "файла нет",
			repo:     &fakeFileRepo{err: ErrFileNotFound},
			baseName: "Море",
			wantErr:  ErrFileNotFound,
		},
		{
			name:     "одни пробелы",
			repo:     &fakeFileRepo{fileContent: FileContent{OriginalName: "a.jpg"}},
			baseName: "   ",
			wantErr:  ErrInvalidName,
		},
		{
			name:     "запрещённый символ",
			repo:     &fakeFileRepo{fileContent: FileContent{OriginalName: "a.jpg"}},
			baseName: "a/b",
			wantErr:  ErrInvalidName,
		},
		{
			name:            "имя занято",
			repo:            &fakeFileRepo{fileContent: FileContent{OriginalName: "a.jpg"}, nameTaken: true},
			baseName:        "Море",
			wantErr:         ErrFileNameTaken,
			wantCheckedName: "Море.jpg",
		},
		{
			name:            "проверка занятости упала",
			repo:            &fakeFileRepo{fileContent: FileContent{OriginalName: "a.jpg"}, nameTakenErr: boom},
			baseName:        "Море",
			wantErr:         boom,
			wantCheckedName: "Море.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(&fakeDirRepo{}, tt.repo, nil, nil).RenameFile(context.Background(), 7, tt.baseName)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got = %+v, want = %+v", got, tt.want)
			}
			if tt.repo.checkedName != tt.wantCheckedName {
				t.Errorf("занятость проверена для %q, ожидалось %q", tt.repo.checkedName, tt.wantCheckedName)
			}
			if tt.wantCheckedName != "" && (tt.repo.checkedDirID != tt.repo.fileContent.DirID || tt.repo.checkedExceptID != 7) {
				t.Errorf("занятость проверена в папке %d без файла %d, ожидалось в %d без 7",
					tt.repo.checkedDirID, tt.repo.checkedExceptID, tt.repo.fileContent.DirID)
			}
			if tt.repo.renamedName != tt.wantRenamedName {
				t.Errorf("UPDATE с именем %q, ожидалось %q", tt.repo.renamedName, tt.wantRenamedName)
			}
			if tt.wantRenamedName != "" && tt.repo.renamedFileID != 7 {
				t.Errorf("UPDATE с id %d, ожидался 7", tt.repo.renamedFileID)
			}
		})
	}
}

func TestMoveFile(t *testing.T) {
	moved := FileInfo{ID: 7, OriginalName: "Море.jpg"}
	boom := errors.New("boom")
	content := FileContent{DirID: 3, OriginalName: "Море.jpg"}

	tests := []struct {
		name            string
		fileRepo        *fakeFileRepo
		dirRepo         *fakeDirRepo
		targetDirID     int64
		want            FileInfo
		wantErr         error
		wantExistsDirID int64
		wantChecked     bool
		wantMoved       bool
	}{
		{
			name:            "перенесён",
			fileRepo:        &fakeFileRepo{fileContent: content, moved: moved},
			dirRepo:         &fakeDirRepo{exist: true},
			targetDirID:     5,
			want:            moved,
			wantExistsDirID: 5,
			wantChecked:     true,
			wantMoved:       true,
		},
		{
			name:        "файла нет",
			fileRepo:    &fakeFileRepo{err: ErrFileNotFound},
			dirRepo:     &fakeDirRepo{exist: true},
			targetDirID: 5,
			wantErr:     ErrFileNotFound,
		},
		{
			name:        "та же папка",
			fileRepo:    &fakeFileRepo{fileContent: content},
			dirRepo:     &fakeDirRepo{exist: true},
			targetDirID: 3,
			wantErr:     ErrSameDir,
		},
		{
			name:            "целевой папки нет",
			fileRepo:        &fakeFileRepo{fileContent: content},
			dirRepo:         &fakeDirRepo{exist: false},
			targetDirID:     5,
			wantErr:         ErrDirNotFound,
			wantExistsDirID: 5,
		},
		{
			name:            "проверка папки упала",
			fileRepo:        &fakeFileRepo{fileContent: content},
			dirRepo:         &fakeDirRepo{err: boom},
			targetDirID:     5,
			wantErr:         boom,
			wantExistsDirID: 5,
		},
		{
			name:            "имя занято в целевой папке",
			fileRepo:        &fakeFileRepo{fileContent: content, nameTaken: true},
			dirRepo:         &fakeDirRepo{exist: true},
			targetDirID:     5,
			wantErr:         ErrFileNameTaken,
			wantExistsDirID: 5,
			wantChecked:     true,
		},
		{
			name:            "проверка имени упала",
			fileRepo:        &fakeFileRepo{fileContent: content, nameTakenErr: boom},
			dirRepo:         &fakeDirRepo{exist: true},
			targetDirID:     5,
			wantErr:         boom,
			wantExistsDirID: 5,
			wantChecked:     true,
		},
		{
			name:            "UPDATE упал",
			fileRepo:        &fakeFileRepo{fileContent: content, moveErr: boom},
			dirRepo:         &fakeDirRepo{exist: true},
			targetDirID:     5,
			wantErr:         boom,
			wantExistsDirID: 5,
			wantChecked:     true,
			wantMoved:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(tt.dirRepo, tt.fileRepo, nil, nil).MoveFile(context.Background(), 7, tt.targetDirID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got = %+v, want = %+v", got, tt.want)
			}
			if tt.dirRepo.existsDirID != tt.wantExistsDirID {
				t.Errorf("проверена папка %d, ожидалась %d", tt.dirRepo.existsDirID, tt.wantExistsDirID)
			}

			checked := tt.fileRepo.checkedName != ""
			if checked != tt.wantChecked {
				t.Errorf("занятость имени проверена = %v, ожидалось %v", checked, tt.wantChecked)
			}
			// имя ищется в целевой папке, сам файл не в счёт
			if checked && (tt.fileRepo.checkedDirID != tt.targetDirID ||
				tt.fileRepo.checkedName != content.OriginalName || tt.fileRepo.checkedExceptID != 7) {
				t.Errorf("занятость проверена: папка %d, имя %q, кроме %d; ожидалось %d, %q, кроме 7",
					tt.fileRepo.checkedDirID, tt.fileRepo.checkedName, tt.fileRepo.checkedExceptID,
					tt.targetDirID, content.OriginalName)
			}

			movedCalled := tt.fileRepo.movedFileID != 0
			if movedCalled != tt.wantMoved {
				t.Errorf("UPDATE вызван = %v, ожидалось %v", movedCalled, tt.wantMoved)
			}
			if movedCalled && (tt.fileRepo.movedFileID != 7 || tt.fileRepo.movedDirID != tt.targetDirID) {
				t.Errorf("UPDATE файла %d в папку %d, ожидалось 7 в %d",
					tt.fileRepo.movedFileID, tt.fileRepo.movedDirID, tt.targetDirID)
			}
		})
	}
}
