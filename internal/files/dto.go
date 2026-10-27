package files

import "time"

type DirInfoResponseDTO struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IsRoot    bool   `json:"is_root"`
	FileCount int    `json:"file_count"`
}

type FileInfoResponseDTO struct {
	ID              int64     `json:"id"`
	OriginalName    string    `json:"original_name"`
	Description     string    `json:"description"`
	Size            int64     `json:"size_bytes"`
	MimeType        string    `json:"mime_type"`
	OriginalCreated time.Time `json:"original_created_at"`
	HasPreview      bool      `json:"has_preview"`
	DateSource      string    `json:"date_source"`
}

type FileDataRequestDTO struct {
	Description string `json:"description"`
	Modtime     string `json:"modtime"`
}

type DirDataRequestDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type DirRenameRequestDTO struct {
	Name string `json:"name"`
}

type FileRenameRequestDTO struct {
	Name string `json:"name"`
}

type FileMoveRequestDTO struct {
	DirID int64 `json:"dir_id"`
}

type CountFilesInDirResponseDTO struct {
	FileCount int `json:"file_count"`
}
