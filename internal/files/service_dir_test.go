package files

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

type fakeDirRepo struct {
	list         []DirInfo
	returnID     int64
	newDirData   NewDirData
	exist        bool
	err          error
	renamed      DirInfo
	renameErr    error
	renamedDirID int64
	renamedName  string
	deleteErr    error
	deletedDirID int64
	existsDirID  int64
}

func (f *fakeDirRepo) GetDirList(ctx context.Context) ([]DirInfo, error) {
	return f.list, f.err
}

func (f *fakeDirRepo) DirExists(ctx context.Context, dirID int64) (bool, error) {
	f.existsDirID = dirID
	return f.exist, f.err
}

func (f *fakeDirRepo) CreateDir(ctx context.Context, dirData NewDirData) (int64, error) {
	f.newDirData = dirData
	return f.returnID, f.err
}

func (f *fakeDirRepo) RenameDir(ctx context.Context, dirID int64, name string) (DirInfo, error) {
	f.renamedDirID = dirID
	f.renamedName = name
	return f.renamed, f.renameErr
}

func (f *fakeDirRepo) DeleteDir(ctx context.Context, dirID int64) error {
	f.deletedDirID = dirID
	return f.deleteErr
}

func TestServiceGetDirsError(t *testing.T) {
	wantErr := errors.New("база недоступна")
	service := NewService(&fakeDirRepo{err: wantErr}, nil, nil, nil)

	_, err := service.GetDirs(context.Background())
	if !errors.Is(err, wantErr) {
		t.Errorf("GetDirs() ошибка = %v, ожидалась %v", err, wantErr)
	}
}

func TestServiceGetDirs(t *testing.T) {
	tests := []struct {
		name    string
		repo    *fakeDirRepo
		want    []DirInfo
		wantErr bool
	}{
		{
			name: "несколько директорий",
			repo: &fakeDirRepo{
				list: []DirInfo{
					{ID: 4, Name: "Байк", IsRoot: false, FileCount: 42},
					{ID: 3, Name: "Отпуск", IsRoot: false, FileCount: 128},
					{ID: 2, Name: "Семья", IsRoot: false, FileCount: 347},
				},
			},
			want: []DirInfo{
				{ID: 4, Name: "Байк", IsRoot: false, FileCount: 42},
				{ID: 3, Name: "Отпуск", IsRoot: false, FileCount: 128},
				{ID: 2, Name: "Семья", IsRoot: false, FileCount: 347},
			},
		},
		{
			name: "пустой список",
			repo: &fakeDirRepo{list: []DirInfo{}},
			want: []DirInfo{},
		},
		{
			name:    "репозиторий вернул ошибку",
			repo:    &fakeDirRepo{err: errors.New("boom")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(tt.repo, nil, nil, nil).GetDirs(context.Background())
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

func TestCreateDir(t *testing.T) {
	tests := []struct {
		name         string
		dirData      NewDirData
		repo         *fakeDirRepo
		want         DirInfo
		wantErr      error
		wantRepoData NewDirData
	}{
		{
			name:         "пробелы по краям обрезаются",
			dirData:      NewDirData{Name: "  Отпуск \t", Description: "\nМоре  "},
			repo:         &fakeDirRepo{returnID: 5},
			want:         DirInfo{ID: 5, Name: "Отпуск"},
			wantRepoData: NewDirData{Name: "Отпуск", Description: "Море"},
		},
		{
			name:         "имя ровно 255 байт",
			dirData:      NewDirData{Name: strings.Repeat("я", 127) + "a"},
			repo:         &fakeDirRepo{returnID: 6},
			want:         DirInfo{ID: 6, Name: strings.Repeat("я", 127) + "a"},
			wantRepoData: NewDirData{Name: strings.Repeat("я", 127) + "a"},
		},
		{
			name:         "имя занято",
			dirData:      NewDirData{Name: "Отпуск"},
			repo:         &fakeDirRepo{err: ErrDirAlreadyExists},
			wantErr:      ErrDirAlreadyExists,
			wantRepoData: NewDirData{Name: "Отпуск"},
		},
		{
			name: "пустое имя", dirData: NewDirData{Name: ""},
			repo:    &fakeDirRepo{},
			wantErr: ErrInvalidName,
		},
		{
			name: "одни пробелы", dirData: NewDirData{Name: "   "},
			repo:    &fakeDirRepo{},
			wantErr: ErrInvalidName,
		},
		{
			name: "точка", dirData: NewDirData{Name: "."},
			repo:    &fakeDirRepo{},
			wantErr: ErrInvalidName,
		},
		{
			name: "две точки", dirData: NewDirData{Name: ".."},
			repo:    &fakeDirRepo{},
			wantErr: ErrInvalidName,
		},
		{
			name: "256 байт", dirData: NewDirData{Name: strings.Repeat("я", 128)},
			repo:    &fakeDirRepo{},
			wantErr: ErrInvalidName,
		},
		{
			name: "табуляция внутри", dirData: NewDirData{Name: "a\tb"},
			repo:    &fakeDirRepo{},
			wantErr: ErrInvalidName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(tt.repo, nil, nil, nil).CreateDir(context.Background(), tt.dirData)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got = %+v, want = %+v", got, tt.want)
			}
			if tt.repo.newDirData != tt.wantRepoData {
				t.Errorf("в репозиторий ушло %+v, ожидалось %+v", tt.repo.newDirData, tt.wantRepoData)
			}
		})
	}
}

func TestCreateDirForbiddenChars(t *testing.T) {
	for _, char := range []string{"/", `\`, "<", ">", ":", `"`, "|", "?", "*", "\x00", "\x7f"} {
		t.Run(fmt.Sprintf("%q", char), func(t *testing.T) {
			repo := &fakeDirRepo{}
			_, err := NewService(repo, nil, nil, nil).CreateDir(context.Background(), NewDirData{Name: "a" + char + "b"})
			if !errors.Is(err, ErrInvalidName) {
				t.Fatalf("err = %v, ожидалась %v", err, ErrInvalidName)
			}
			if repo.newDirData != (NewDirData{}) {
				t.Errorf("репозиторий позван с %+v", repo.newDirData)
			}
		})
	}
}

func TestRenameDir(t *testing.T) {
	renamed := DirInfo{ID: 4, Name: "Отпуск", FileCount: 12}

	tests := []struct {
		name            string
		dirName         string
		repo            *fakeDirRepo
		want            DirInfo
		wantErr         error
		wantRenamedName string
	}{
		{
			name:            "пробелы по краям обрезаются",
			dirName:         "  Отпуск \t",
			repo:            &fakeDirRepo{renamed: renamed},
			want:            renamed,
			wantRenamedName: "Отпуск",
		},
		{
			name:            "имя занято",
			dirName:         "Отпуск",
			repo:            &fakeDirRepo{renameErr: ErrDirAlreadyExists},
			wantErr:         ErrDirAlreadyExists,
			wantRenamedName: "Отпуск",
		},
		{
			name:            "папки нет или это корень",
			dirName:         "Отпуск",
			repo:            &fakeDirRepo{renameErr: ErrDirNotFound},
			wantErr:         ErrDirNotFound,
			wantRenamedName: "Отпуск",
		},
		{name: "одни пробелы", dirName: "   ", repo: &fakeDirRepo{}, wantErr: ErrInvalidName},
		{name: "запрещённый символ", dirName: "a|b", repo: &fakeDirRepo{}, wantErr: ErrInvalidName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(tt.repo, nil, nil, nil).RenameDir(context.Background(), 4, tt.dirName)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, ожидалась %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got = %+v, want = %+v", got, tt.want)
			}
			if tt.repo.renamedName != tt.wantRenamedName {
				t.Errorf("в репозиторий ушло имя %q, ожидалось %q", tt.repo.renamedName, tt.wantRenamedName)
			}
			if tt.wantRenamedName != "" && tt.repo.renamedDirID != 4 {
				t.Errorf("в репозиторий ушёл id %d, ожидался 4", tt.repo.renamedDirID)
			}
		})
	}
}
