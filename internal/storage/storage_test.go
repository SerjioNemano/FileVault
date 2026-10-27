package storage_test

import (
	"errors"
	"filevault/internal/storage"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSave(t *testing.T) {
	root := t.TempDir()

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	saved, err := s.Save(strings.NewReader("hello"), ".jpg")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	wantHash := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if saved.Hash != wantHash {
		t.Errorf("Hash = %s, want %s", saved.Hash, wantHash)
	}
	if saved.Size != 5 {
		t.Errorf("Size = %d, want 5", saved.Size)
	}
	got, err := os.ReadFile(filepath.Join(root, saved.Path))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}

	if !strings.HasPrefix(saved.Path, "original/") {
		t.Errorf("Path = %q, want prefix %q", saved.Path, "original/")
	}

	if got := filepath.Base(filepath.Dir(saved.Path)); got != wantHash[:2] {
		t.Errorf("hash subdir = %q, want %q", got, wantHash[:2])
	}

	tmpDirFiles, err := os.ReadDir(filepath.Join(root, "tmp"))
	if err != nil {
		t.Fatalf("read tmp dir: %v", err)
	}

	if len(tmpDirFiles) != 0 {
		t.Errorf("files in tmp dir is %d, want 0", len(tmpDirFiles))
	}
}

func TestSavePreview(t *testing.T) {
	root := t.TempDir()

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	saved, err := s.SavePreview(strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("SavePreview: %v", err)
	}

	if !strings.HasPrefix(saved.Path, "preview/") {
		t.Errorf("Path = %q, want prefix %q", saved.Path, "preview/")
	}
	if filepath.Ext(saved.Path) != ".jpg" {
		t.Errorf("Path = %q, want ext .jpg", saved.Path)
	}

	got, err := os.ReadFile(filepath.Join(root, saved.Path))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
}

func TestSaveCleansTmpOnError(t *testing.T) {
	root := t.TempDir()

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "original"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = s.Save(strings.NewReader("hello"), ".jpg")
	if err == nil {
		t.Fatal("Save: want error, got nil")
	}

	files, err := os.ReadDir(filepath.Join(root, "tmp"))
	if err != nil {
		t.Fatalf("read tmp dir: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("files in tmp dir is %d, want 0", len(files))
	}
}

func TestRemove(t *testing.T) {
	root := t.TempDir()

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	saved, err := s.Save(strings.NewReader("Hello"), ".jpg")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := s.Remove(saved.Path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err = os.Stat(filepath.Join(root, saved.Path))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("file still exists after Remove (Stat err = %v)", err)
	}
}

func TestRemoveRejectsOutsidePath(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// настоящий файл за пределами root
	outside := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outside, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.Remove("../outside.txt"); err == nil {
		t.Error("Remove outside root: want error, got nil")
	}

	if _, err := os.Stat(outside); err != nil {
		t.Errorf("file outside root was touched: %v", err)
	}
}

func TestMoveToTrash(t *testing.T) {
	root := t.TempDir()

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	saved, err := s.Save(strings.NewReader("hello"), ".jpg")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	trashPath, err := s.MoveToTrash(saved.Path)
	if err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}

	if !strings.HasPrefix(trashPath, "trash"+string(filepath.Separator)) {
		t.Errorf("trashPath = %q, want prefix trash/", trashPath)
	}

	if _, err := os.Stat(filepath.Join(root, saved.Path)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("old path: err = %v, want fs.ErrNotExist", err)
	}

	content, err := os.ReadFile(filepath.Join(root, trashPath))
	if err != nil {
		t.Fatalf("read trash file: %v", err)
	}
	if string(content) != "hello" {
		t.Errorf("trash content = %q, want %q", content, "hello")
	}
}

func TestMoveToTrashRejectsOutsidePath(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	// настоящий файл за пределами root
	outside := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outside, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.MoveToTrash("../outside.txt"); err == nil {
		t.Error("MoveToTrash outside root: want error, got nil")
	}

	if _, err := os.Stat(outside); err != nil {
		t.Errorf("file outside root was touched: %v", err)
	}
}

func TestRestoreFromTrash(t *testing.T) {
	root := t.TempDir()

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	saved, err := s.Save(strings.NewReader("hello"), ".jpg")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	trashPath, err := s.MoveToTrash(saved.Path)
	if err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}

	if err := s.RestoreFromTrash(trashPath, saved.Path); err != nil {
		t.Fatalf("RestoreFromTrash: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, trashPath)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("trash path: err = %v, want fs.ErrNotExist", err)
	}

	content, err := os.ReadFile(filepath.Join(root, saved.Path))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(content) != "hello" {
		t.Errorf("restored content = %q, want %q", content, "hello")
	}
}

func TestRestoreFromTrashRejectsOutsidePath(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	saved, err := s.Save(strings.NewReader("hello"), ".jpg")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	trashPath, err := s.MoveToTrash(saved.Path)
	if err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}

	if err := s.RestoreFromTrash(trashPath, "../escaped.jpg"); err == nil {
		t.Error("RestoreFromTrash outside root: want error, got nil")
	}

	if _, err := os.Stat(filepath.Join(parent, "escaped.jpg")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("file written outside root: err = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, trashPath)); err != nil {
		t.Errorf("trash file was touched: %v", err)
	}
}

func TestOpen(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name        string
		seed        func(s *storage.Storage) (io.ReadSeekCloser, error)
		wantError   error
		wantContent string
	}{
		{
			name: "файл существует",
			seed: func(s *storage.Storage) (io.ReadSeekCloser, error) {
				saved, err := s.Save(strings.NewReader("Hello"), ".jpg")
				if err != nil {
					t.Fatalf("Save: %v", err)
				}

				return s.Open(saved.Path)
			},
			wantContent: "Hello",
		},
		{
			name: "файл не существует",
			seed: func(s *storage.Storage) (io.ReadSeekCloser, error) {
				return s.Open("some/path/undef.jpg")
			},
			wantError: fs.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := storage.NewStorage(root)
			if err != nil {
				t.Fatalf("NewStorage: %v", err)
			}

			file, err := tt.seed(s)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Open: %v", err)
			}
			if err != nil {
				return
			}

			defer file.Close()

			got, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}

			if string(got) != tt.wantContent {
				t.Errorf("content = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

func TestOpenRejectsOutsidePath(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")

	s, err := storage.NewStorage(root)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	outside := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	file, err := s.Open("../outside.txt")
	if err == nil {
		file.Close()
		t.Error("Open outside root: want error, got nil")
	}
}
