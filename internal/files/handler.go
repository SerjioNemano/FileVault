package files

import (
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxJSONBodyBytes = 64 << 10

type Handler struct {
	service        *Service
	maxUploadBytes int64
}

func NewHandler(service *Service, maxUploadBytes int64) *Handler {
	return &Handler{
		service:        service,
		maxUploadBytes: maxUploadBytes,
	}
}

func (h *Handler) GetDirs(w http.ResponseWriter, r *http.Request) {
	dirList, err := h.service.GetDirs(r.Context())
	if err != nil {
		log.Printf("GetDirs: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dtoList := []DirInfoResponseDTO{}

	for _, model := range dirList {
		dtoList = append(dtoList, DirInfoToDTO(model))
	}

	body, err := json.Marshal(dtoList)
	if err != nil {
		log.Printf("GetDirs: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *Handler) CreateDir(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)

	var dto DirDataRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "invalid data", http.StatusBadRequest)
		return
	}

	dir, err := h.service.CreateDir(r.Context(), NewDirData{Name: dto.Name, Description: dto.Description})
	if err != nil {
		if errors.Is(err, ErrInvalidName) {
			http.Error(w, "invalid dir name", http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrDirAlreadyExists) {
			http.Error(w, "dir already exists", http.StatusConflict)
			return
		}
		log.Printf("CreateDir: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(DirInfoToDTO(dir))
	if err != nil {
		log.Printf("CreateDir: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(body)
}

func (h *Handler) RenameDir(w http.ResponseWriter, r *http.Request) {
	dirID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || dirID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)

	var dto DirRenameRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "invalid data", http.StatusBadRequest)
		return
	}

	dir, err := h.service.RenameDir(r.Context(), dirID, dto.Name)
	if err != nil {
		if errors.Is(err, ErrInvalidName) {
			http.Error(w, "invalid dir name", http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrDirNotFound) {
			http.Error(w, "dir is not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrDirAlreadyExists) {
			http.Error(w, "dir already exists", http.StatusConflict)
			return
		}
		log.Printf("RenameDir: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(DirInfoToDTO(dir))
	if err != nil {
		log.Printf("RenameDir: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *Handler) DeleteDir(w http.ResponseWriter, r *http.Request) {
	dirID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || dirID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteDir(r.Context(), dirID); err != nil {
		if errors.Is(err, ErrDirNotFound) {
			http.Error(w, "dir is not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrDirNotEmpty) {
			http.Error(w, "dir is not empty", http.StatusConflict)
			return
		}
		log.Printf("DeleteDir: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetDirFiles(w http.ResponseWriter, r *http.Request) {
	dirID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || dirID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	fileList, err := h.service.GetFileListInDir(r.Context(), dirID)
	if err != nil {
		if errors.Is(err, ErrDirNotFound) {
			http.Error(w, "dir is not found", http.StatusNotFound)
			return
		}
		log.Printf("GetFileListInDir: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dtoList := []FileInfoResponseDTO{}

	for _, model := range fileList {
		dtoList = append(dtoList, FileInfoToDTO(model))
	}

	body, err := json.Marshal(dtoList)
	if err != nil {
		log.Printf("GetDirFiles: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *Handler) GetDirCountFiles(w http.ResponseWriter, r *http.Request) {
	dirID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || dirID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	fileCountInDir, err := h.service.GetCountFilesInDir(r.Context(), dirID)
	if err != nil {
		if errors.Is(err, ErrDirNotFound) {
			http.Error(w, "dir is not found", http.StatusNotFound)
			return
		}
		log.Printf("GetDirCountFiles: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dtoCountFilesInDir := CountFilesInDirToDTO(fileCountInDir)

	body, err := json.Marshal(dtoCountFilesInDir)
	if err != nil {
		log.Printf("GetDirCountFiles: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *Handler) AddFileToDir(w http.ResponseWriter, r *http.Request) {
	dirID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || dirID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)

	file, header, err := r.FormFile("file")
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	defer file.Close()

	var dto FileDataRequestDTO
	if raw := r.FormValue("data"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &dto); err != nil {
			http.Error(w, "invalid data", http.StatusBadRequest)
			return
		}
	}

	var modTime *time.Time
	if t, err := time.Parse("2006-01-02T15:04:05", dto.Modtime); err == nil {
		modTime = &t
	}

	info, err := h.service.AddFile(
		r.Context(),
		file,
		NewFile{
			DirID:        dirID,
			OriginalName: header.Filename,
			Description:  dto.Description,
			Modtime:      modTime,
		},
	)
	if err != nil {
		if errors.Is(err, ErrDirNotFound) {
			http.Error(w, "dir is not found", http.StatusNotFound)
			return
		}
		log.Printf("AddFile: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(FileInfoToDTO(info))
	if err != nil {
		log.Printf("AddFile: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(body)
}

func (h *Handler) GetFileByID(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || fileID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	file, content, err := h.service.GetFile(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, ErrFileNotFound) {
			http.Error(w, "file is not found", http.StatusNotFound)
			return
		}
		log.Printf("GetFileByID: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	defer file.Close()

	w.Header().Set("Content-Type", content.MimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")

	disposition := "inline"
	if r.URL.Query().Get("download") == "1" || strings.HasPrefix(content.MimeType, "text/html") {
		disposition = "attachment"
	}

	w.Header().Set("Content-Disposition",
		mime.FormatMediaType(disposition, map[string]string{"filename": content.OriginalName}))

	http.ServeContent(w, r, "", time.Time{}, file)
}

func (h *Handler) GetFilePreview(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || fileID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	file, err := h.service.GetPreview(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, ErrFileNotFound) {
			http.Error(w, "file is not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrPreviewNotFound) {
			http.Error(w, "preview is not found", http.StatusNotFound)
			return
		}
		log.Printf("GetFilePreview: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	defer file.Close()

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=86400")

	http.ServeContent(w, r, "", time.Time{}, file)
}

func (h *Handler) RenameFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || fileID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)

	var dto FileRenameRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "invalid data", http.StatusBadRequest)
		return
	}

	info, err := h.service.RenameFile(r.Context(), fileID, dto.Name)
	if err != nil {
		if errors.Is(err, ErrFileNotFound) {
			http.Error(w, "file is not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrInvalidName) {
			http.Error(w, "invalid file name", http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrFileNameTaken) {
			http.Error(w, "file name already exists", http.StatusConflict)
			return
		}
		log.Printf("RenameFile: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(FileInfoToDTO(info))
	if err != nil {
		log.Printf("RenameFile: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *Handler) MoveFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || fileID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)

	var dto FileMoveRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, "invalid data", http.StatusBadRequest)
		return
	}
	if dto.DirID < 1 {
		http.Error(w, "invalid dir id", http.StatusBadRequest)
		return
	}

	info, err := h.service.MoveFile(r.Context(), fileID, dto.DirID)
	if err != nil {
		if errors.Is(err, ErrFileNotFound) {
			http.Error(w, "file is not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrDirNotFound) {
			http.Error(w, "dir is not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrSameDir) {
			http.Error(w, "file is already in this dir", http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrFileNameTaken) {
			http.Error(w, "file name already exists", http.StatusConflict)
			return
		}
		log.Printf("MoveFile: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(FileInfoToDTO(info))
	if err != nil {
		log.Printf("MoveFile: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || fileID < 1 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteFile(r.Context(), fileID); err != nil {
		if errors.Is(err, ErrFileNotFound) {
			http.Error(w, "file is not found", http.StatusNotFound)
			return
		}
		log.Printf("DeleteFile: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
