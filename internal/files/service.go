package files

import (
	"bytes"
	"context"
	"errors"
	"filevault/internal/storage"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const forbiddenNameChars = `/\<>:"|?*`

const maxNameBytes = 255 // байты

type DirRepository interface {
	GetDirList(ctx context.Context) ([]DirInfo, error)
	DirExists(ctx context.Context, dirID int64) (bool, error)
	CreateDir(ctx context.Context, dirData NewDirData) (int64, error)
	RenameDir(ctx context.Context, dirID int64, name string) (DirInfo, error)
	DeleteDir(ctx context.Context, dirID int64) error
}

type FileRepository interface {
	GetFileListInDir(ctx context.Context, dirID int64) ([]FileInfo, error)
	InsertFile(ctx context.Context, f NewFileData) (int64, error)
	GetCountFilesInDir(ctx context.Context, dirID int64) (CountFilesInDir, error)
	GetFileContentInfo(ctx context.Context, fileID int64) (FileContent, error)
	GetPreviewPath(ctx context.Context, fileID int64) (string, bool, error)
	DeleteFile(ctx context.Context, fileID int64) error
	FileNameTaken(ctx context.Context, dirID int64, name string, exceptFileID int64) (bool, error)
	RenameFile(ctx context.Context, fileID int64, name string) (FileInfo, error)
	MoveFile(ctx context.Context, fileID, dirID int64) (FileInfo, error)
}

type FileStorage interface {
	Save(r io.Reader, ext string) (storage.Saved, error)
	Remove(path string) error
	Open(path string) (io.ReadSeekCloser, error)
	MoveToTrash(path string) (string, error)
	RestoreFromTrash(trashPath, path string) error
}

type PreviewQueue interface {
	Wake()
}

type Service struct {
	dirRepo  DirRepository
	fileRepo FileRepository
	storage  FileStorage
	previews PreviewQueue
}

func NewService(dirRepo DirRepository, fileRepo FileRepository, storage FileStorage, previews PreviewQueue) *Service {
	return &Service{dirRepo: dirRepo, fileRepo: fileRepo, storage: storage, previews: previews}
}

func (service *Service) GetDirs(ctx context.Context) ([]DirInfo, error) {
	return service.dirRepo.GetDirList(ctx)
}

func (service *Service) CreateDir(ctx context.Context, data NewDirData) (DirInfo, error) {
	name := strings.TrimSpace(data.Name)
	if err := validateName(name); err != nil {
		return DirInfo{}, err
	}

	dirID, err := service.dirRepo.CreateDir(ctx, NewDirData{Name: name, Description: strings.TrimSpace(data.Description)})
	if err != nil {
		return DirInfo{}, err
	}

	return DirInfo{ID: dirID, Name: name}, nil
}

func (service *Service) RenameDir(ctx context.Context, dirID int64, name string) (DirInfo, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return DirInfo{}, err
	}

	return service.dirRepo.RenameDir(ctx, dirID, name)
}

func (service *Service) DeleteDir(ctx context.Context, dirID int64) error {
	return service.dirRepo.DeleteDir(ctx, dirID)
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." || len(name) > maxNameBytes {
		return ErrInvalidName
	}
	if strings.ContainsAny(name, forbiddenNameChars) || strings.ContainsFunc(name, unicode.IsControl) {
		return ErrInvalidName
	}

	return nil
}

func (service *Service) RenameFile(ctx context.Context, fileID int64, baseName string) (FileInfo, error) {
	content, err := service.fileRepo.GetFileContentInfo(ctx, fileID)
	if err != nil {
		return FileInfo{}, err
	}

	base := strings.TrimSpace(baseName)
	if base == "" {
		return FileInfo{}, ErrInvalidName
	}

	name := base + filepath.Ext(content.OriginalName)
	if err := validateName(name); err != nil {
		return FileInfo{}, err
	}

	taken, err := service.fileRepo.FileNameTaken(ctx, content.DirID, name, fileID)
	if err != nil {
		return FileInfo{}, err
	}
	if taken {
		return FileInfo{}, ErrFileNameTaken
	}

	return service.fileRepo.RenameFile(ctx, fileID, name)
}

func (service *Service) MoveFile(ctx context.Context, fileID, dirID int64) (FileInfo, error) {
	content, err := service.fileRepo.GetFileContentInfo(ctx, fileID)
	if err != nil {
		return FileInfo{}, err
	}
	if content.DirID == dirID {
		return FileInfo{}, ErrSameDir
	}

	exists, err := service.dirRepo.DirExists(ctx, dirID)
	if err != nil {
		return FileInfo{}, err
	}
	if !exists {
		return FileInfo{}, ErrDirNotFound
	}

	taken, err := service.fileRepo.FileNameTaken(ctx, dirID, content.OriginalName, fileID)
	if err != nil {
		return FileInfo{}, err
	}
	if taken {
		return FileInfo{}, ErrFileNameTaken
	}

	return service.fileRepo.MoveFile(ctx, fileID, dirID)
}

func (service *Service) GetFileListInDir(ctx context.Context, dirID int64) ([]FileInfo, error) {
	exists, err := service.dirRepo.DirExists(ctx, dirID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrDirNotFound
	}
	return service.fileRepo.GetFileListInDir(ctx, dirID)
}

func (service *Service) GetCountFilesInDir(ctx context.Context, dirID int64) (CountFilesInDir, error) {
	exists, err := service.dirRepo.DirExists(ctx, dirID)
	if err != nil {
		return CountFilesInDir{}, err
	}
	if !exists {
		return CountFilesInDir{}, ErrDirNotFound
	}

	return service.fileRepo.GetCountFilesInDir(ctx, dirID)
}

func (service *Service) AddFile(ctx context.Context, reader io.Reader, data NewFile) (FileInfo, error) {
	exists, err := service.dirRepo.DirExists(ctx, data.DirID)
	if err != nil {
		return FileInfo{}, err
	}
	if !exists {
		return FileInfo{}, ErrDirNotFound
	}

	mime, reader, err := detectMime(reader)
	if err != nil {
		return FileInfo{}, err
	}

	savedFileData, err := service.storage.Save(reader, strings.ToLower(filepath.Ext(data.OriginalName)))
	if err != nil {
		return FileInfo{}, err
	}

	newFileData := NewFileData{
		DirID:        data.DirID,
		OriginalName: data.OriginalName,
		Description:  data.Description,
		StoragePath:  savedFileData.Path,
		MimeType:     mime,
		SHA256:       savedFileData.Hash,
		Size:         savedFileData.Size,
	}

	if data.Modtime != nil {
		newFileData.OriginalCreated = *data.Modtime
		newFileData.DateSource = "modtime"
	} else {
		newFileData.OriginalCreated = time.Now()
		newFileData.DateSource = "upload"
	}

	newFileID, err := service.fileRepo.InsertFile(ctx, newFileData)
	if err != nil {
		if removeErr := service.storage.Remove(savedFileData.Path); removeErr != nil {
			return FileInfo{}, errors.Join(
				fmt.Errorf("insert file: %w", err),
				fmt.Errorf("remove file %q: %w", savedFileData.Path, removeErr),
			)
		}
		return FileInfo{}, err
	}

	service.previews.Wake()

	fileInfo := FileInfo{
		ID:              newFileID,
		OriginalName:    newFileData.OriginalName,
		Description:     newFileData.Description,
		Size:            newFileData.Size,
		MimeType:        newFileData.MimeType,
		OriginalCreated: newFileData.OriginalCreated,
		HasPreview:      false,
		DateSource:      newFileData.DateSource,
	}

	return fileInfo, nil
}

func (service *Service) GetFile(ctx context.Context, fileID int64) (io.ReadSeekCloser, FileContent, error) {
	fileContent, err := service.fileRepo.GetFileContentInfo(ctx, fileID)
	if err != nil {
		return nil, FileContent{}, err
	}

	fileReader, err := service.storage.Open(fileContent.StoragePath)
	if err != nil {
		return nil, FileContent{}, err
	}

	return fileReader, fileContent, nil
}

func (service *Service) GetPreview(ctx context.Context, fileID int64) (io.ReadSeekCloser, error) {
	path, ok, err := service.fileRepo.GetPreviewPath(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPreviewNotFound
	}

	return service.storage.Open(path)
}

func (service *Service) DeleteFile(ctx context.Context, fileID int64) error {
	fileContent, err := service.fileRepo.GetFileContentInfo(ctx, fileID)
	if err != nil {
		return err
	}

	previewPath, hasPreview, err := service.fileRepo.GetPreviewPath(ctx, fileID)
	if err != nil {
		return err
	}

	type trashedFile struct {
		trashPath, path string
	}
	var trashed []trashedFile

	trashPath, err := service.storage.MoveToTrash(fileContent.StoragePath)
	if err != nil {
		return err
	}
	trashed = append(trashed, trashedFile{trashPath, fileContent.StoragePath})

	// Превью не держит удаление (принцип 7): не перенеслось — останется на диске сиротой.
	if hasPreview {
		previewTrashPath, err := service.storage.MoveToTrash(previewPath)
		if err != nil {
			log.Printf("DeleteFile %d: move preview to trash: %v", fileID, err)
		} else {
			trashed = append(trashed, trashedFile{previewTrashPath, previewPath})
		}
	}

	if err := service.fileRepo.DeleteFile(ctx, fileID); err != nil {
		var restoreErrs []error
		for _, file := range trashed {
			if restoreErr := service.storage.RestoreFromTrash(file.trashPath, file.path); restoreErr != nil {
				restoreErrs = append(restoreErrs, restoreErr)
			}
		}

		if len(restoreErrs) > 0 {
			return errors.Join(append([]error{fmt.Errorf("delete file: %w", err)}, restoreErrs...)...)
		}
		return err
	}

	for _, file := range trashed {
		if removeErr := service.storage.Remove(file.trashPath); removeErr != nil {
			log.Printf("DeleteFile %d: clean trash: %v", fileID, removeErr)
		}
	}

	return nil
}

func detectMime(r io.Reader) (string, io.Reader, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", nil, err
	}

	mime := http.DetectContentType(buf[:n])
	combined := io.MultiReader(bytes.NewReader(buf[:n]), r)

	return mime, combined, nil
}
