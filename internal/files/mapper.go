package files

func DirInfoToDTO(model DirInfo) DirInfoResponseDTO {
	return DirInfoResponseDTO{
		ID:        model.ID,
		Name:      model.Name,
		IsRoot:    model.IsRoot,
		FileCount: model.FileCount,
	}
}

func FileInfoToDTO(model FileInfo) FileInfoResponseDTO {
	return FileInfoResponseDTO{
		ID:              model.ID,
		OriginalName:    model.OriginalName,
		Description:     model.Description,
		Size:            model.Size,
		MimeType:        model.MimeType,
		OriginalCreated: model.OriginalCreated,
		HasPreview:      model.HasPreview,
		DateSource:      model.DateSource,
	}
}

func CountFilesInDirToDTO(model CountFilesInDir) CountFilesInDirResponseDTO {
	return CountFilesInDirResponseDTO{
		FileCount: model.FileCount,
	}
}
