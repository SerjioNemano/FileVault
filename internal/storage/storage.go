package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Storage struct {
	root string
}

type Saved struct {
	Path string
	Hash string
	Size int64
}

func NewStorage(root string) (*Storage, error) {
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o755); err != nil {
		return nil, fmt.Errorf("create tmp dir: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "trash"), 0o755); err != nil {
		return nil, fmt.Errorf("create trash dir: %w", err)
	}

	return &Storage{root: root}, nil
}

func (s *Storage) Save(r io.Reader, ext string) (Saved, error) {
	return s.save("original", r, ext)
}

func (s *Storage) SavePreview(r io.Reader) (Saved, error) {
	return s.save("preview", r, ".jpg")
}

func (s *Storage) save(kind string, r io.Reader, ext string) (Saved, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return Saved{}, fmt.Errorf("random name: %w", err)
	}

	name := hex.EncodeToString(b) + ext

	tmpPath := filepath.Join(s.root, "tmp", name)
	f, err := os.Create(tmpPath)
	if err != nil {
		return Saved{}, fmt.Errorf("create %s: %w", tmpPath, err)
	}

	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return Saved{}, fmt.Errorf("write %s: %w", tmpPath, err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return Saved{}, fmt.Errorf("close %s: %w", tmpPath, err)
	}

	hash := hex.EncodeToString(h.Sum(nil))
	dir := makeStorageDirPath(kind, time.Now(), hash)

	dst := filepath.Join(s.root, dir, name)
	if err := os.MkdirAll(filepath.Join(s.root, dir), 0o755); err != nil {
		os.Remove(tmpPath)
		return Saved{}, fmt.Errorf("create %s dir: %w", kind, err)
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return Saved{}, fmt.Errorf("rename %s -> %s: %w", tmpPath, dst, err)
	}

	return Saved{
		Path: filepath.Join(dir, name),
		Size: size,
		Hash: hash,
	}, nil
}

func (s *Storage) Remove(path string) error {
	if !filepath.IsLocal(path) {
		return fmt.Errorf("remove %q: path escapes storage root", path)
	}

	fullPath := filepath.Join(s.root, path)

	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("remove %s: %w", fullPath, err)
	}

	return nil
}

func (s *Storage) MoveToTrash(path string) (string, error) {
	if !filepath.IsLocal(path) {
		return "", fmt.Errorf("move to trash %q: path escapes storage root", path)
	}

	trashPath := filepath.Join("trash", filepath.Base(path))

	fullPath := filepath.Join(s.root, path)
	fullTrashPath := filepath.Join(s.root, trashPath)

	if err := os.Rename(fullPath, fullTrashPath); err != nil {
		return "", fmt.Errorf("move to trash %s -> %s: %w", fullPath, fullTrashPath, err)
	}

	return trashPath, nil
}

func (s *Storage) RestoreFromTrash(trashPath, path string) error {
	if !filepath.IsLocal(trashPath) || !filepath.IsLocal(path) {
		return fmt.Errorf("restore %q -> %q: path escapes storage root", trashPath, path)
	}

	fullTrashPath := filepath.Join(s.root, trashPath)
	fullPath := filepath.Join(s.root, path)

	if err := os.Rename(fullTrashPath, fullPath); err != nil {
		return fmt.Errorf("restore %s -> %s: %w", fullTrashPath, fullPath, err)
	}

	return nil
}

func (s *Storage) Open(path string) (io.ReadSeekCloser, error) {
	if !filepath.IsLocal(path) {
		return nil, fmt.Errorf("open %q: path escapes storage root", path)
	}

	fullPath := filepath.Join(s.root, path)

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", fullPath, err)
	}

	return file, nil
}

func makeStorageDirPath(kind string, date time.Time, hash string) string {
	return filepath.Join(
		kind,
		date.Format("2006"),
		date.Format("01"),
		date.Format("02"),
		hash[:2],
	)
}
