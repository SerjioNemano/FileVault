package files

import "time"

type DirInfo struct {
	ID        int64
	Name      string
	IsRoot    bool
	FileCount int
}

type FileInfo struct {
	ID              int64
	OriginalName    string
	Description     string
	Size            int64
	MimeType        string
	OriginalCreated time.Time
	HasPreview      bool
	DateSource      string
}

type NewFileData struct {
	DirID           int64
	OriginalName    string
	Description     string
	StoragePath     string
	MimeType        string
	DateSource      string
	SHA256          string
	Size            int64
	OriginalCreated time.Time
}

type NewDirData struct {
	Name        string
	Description string
}

type NewFile struct {
	DirID        int64
	OriginalName string
	Description  string
	Modtime      *time.Time // nil — даты нет
}

type CountFilesInDir struct {
	FileCount int
}

type FileContent struct {
	DirID        int64
	OriginalName string
	StoragePath  string
	MimeType     string
}

type PreviewJob struct {
	ID          int64
	StoragePath string
	MimeType    string
}
