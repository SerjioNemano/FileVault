package files

import "errors"

var (
	ErrDirNotFound      = errors.New("dir not found")
	ErrFileNotFound     = errors.New("file not found")
	ErrPreviewNotFound  = errors.New("preview not found")
	ErrDirAlreadyExists = errors.New("dir already exists")
	ErrInvalidName      = errors.New("invalid name")
	ErrFileNameTaken    = errors.New("file name already exists")
	ErrDirNotEmpty      = errors.New("dir is not empty")
	ErrSameDir          = errors.New("file is already in this dir")
)
