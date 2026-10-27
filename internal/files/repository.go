package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	pgUniqueViolation     = "23505" // код ошибки PostgreSQL: нарушено ограничение уникальности
	pgForeignKeyViolation = "23503" // код ошибки PostgreSQL: нарушен внешний ключ (files.dir_id, ON DELETE RESTRICT)
)

type DBTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type PgRepository struct {
	db DBTX
}

func NewPgRepository(db DBTX) *PgRepository {
	return &PgRepository{db: db}
}

func (repo *PgRepository) GetDirList(ctx context.Context) ([]DirInfo, error) {
	list := []DirInfo{}

	const query = `
		SELECT d.id, d.name, d.is_root, count(f.id) as file_count
		FROM dirs AS d
		LEFT JOIN files AS f ON f.dir_id = d.id
		GROUP BY d.id
		ORDER BY d.name
	`

	rows, err := repo.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query dirs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		dirInfo := DirInfo{}
		if err := rows.Scan(&dirInfo.ID, &dirInfo.Name, &dirInfo.IsRoot, &dirInfo.FileCount); err != nil {
			return nil, fmt.Errorf("scan dir: %w", err)
		}

		list = append(list, dirInfo)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return list, nil
}

func (repo *PgRepository) DirExists(ctx context.Context, dirID int64) (bool, error) {
	var exists bool
	err := repo.db.QueryRowContext(
		ctx,
		`SELECT EXISTS(SELECT 1 FROM dirs WHERE id = $1)`,
		dirID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check dir: %w", err)
	}

	return exists, nil
}

func (repo *PgRepository) GetCountFilesInDir(ctx context.Context, dirID int64) (CountFilesInDir, error) {
	countFiles := CountFilesInDir{}
	const query = `SELECT count(id) FROM files where dir_id = $1`
	err := repo.db.QueryRowContext(ctx, query, dirID).Scan(&countFiles.FileCount)
	if err != nil {
		return countFiles, fmt.Errorf("get count files: %w", err)
	}

	return countFiles, nil
}

func (repo *PgRepository) GetFileListInDir(ctx context.Context, dirID int64) ([]FileInfo, error) {
	list := []FileInfo{}

	const query = `
		SELECT
			id, original_name, description, size_bytes, mime_type, original_created_at,
			preview_status = 'done' AS has_preview, date_source
		FROM files
		WHERE dir_id=$1
		ORDER BY id
	`

	rows, err := repo.db.QueryContext(ctx, query, dirID)
	if err != nil {
		return list, fmt.Errorf("query files: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		fileInfo := FileInfo{}

		if err := rows.Scan(
			&fileInfo.ID,
			&fileInfo.OriginalName,
			&fileInfo.Description,
			&fileInfo.Size,
			&fileInfo.MimeType,
			&fileInfo.OriginalCreated,
			&fileInfo.HasPreview,
			&fileInfo.DateSource,
		); err != nil {
			return list, fmt.Errorf("scan file: %w", err)
		}
		list = append(list, fileInfo)
	}

	if err := rows.Err(); err != nil {
		return list, fmt.Errorf("rows: %w", err)
	}

	return list, nil
}

func (repo *PgRepository) InsertFile(ctx context.Context, fileData NewFileData) (int64, error) {
	const query = `
		INSERT INTO files (
			dir_id, original_name, description, storage_path, mime_type,
			date_source, sha256, size_bytes, original_created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
		RETURNING id
	`

	var id int64
	err := repo.db.QueryRowContext(
		ctx,
		query,
		fileData.DirID,
		fileData.OriginalName,
		fileData.Description,
		fileData.StoragePath,
		fileData.MimeType,
		fileData.DateSource,
		fileData.SHA256,
		fileData.Size,
		fileData.OriginalCreated,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert file: %w", err)
	}

	return id, nil
}

func (repo *PgRepository) GetFileContentInfo(ctx context.Context, fileID int64) (FileContent, error) {
	const query = `SELECT dir_id, original_name, storage_path, mime_type FROM files WHERE id = $1`

	var fileContent FileContent

	if err := repo.db.QueryRowContext(ctx, query, fileID).Scan(
		&fileContent.DirID,
		&fileContent.OriginalName,
		&fileContent.StoragePath,
		&fileContent.MimeType,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fileContent, ErrFileNotFound
		}

		return fileContent, fmt.Errorf("get file content: %w", err)
	}

	return fileContent, nil
}

func (repo *PgRepository) GetPreviewPath(ctx context.Context, fileID int64) (string, bool, error) {
	const query = `SELECT preview_status = 'done', preview_path FROM files WHERE id = $1`

	var done bool
	var path sql.NullString

	if err := repo.db.QueryRowContext(ctx, query, fileID).Scan(&done, &path); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, ErrFileNotFound
		}

		return "", false, fmt.Errorf("get preview path: %w", err)
	}

	if !done || !path.Valid {
		return "", false, nil
	}

	return path.String, true, nil
}

func (repo *PgRepository) NextPendingPreview(ctx context.Context) (PreviewJob, bool, error) {
	const query = `
		SELECT id, storage_path, mime_type
		FROM files
		WHERE preview_status = 'pending'
		ORDER BY id
		LIMIT 1
	`

	var job PreviewJob

	if err := repo.db.QueryRowContext(ctx, query).Scan(&job.ID, &job.StoragePath, &job.MimeType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PreviewJob{}, false, nil
		}

		return PreviewJob{}, false, fmt.Errorf("next pending preview: %w", err)
	}

	return job, true, nil
}

func (repo *PgRepository) SetPreviewDone(ctx context.Context, fileID int64, previewPath string) error {
	const query = `UPDATE files SET preview_path = $2, preview_status = 'done' WHERE id = $1`

	if _, err := repo.db.ExecContext(ctx, query, fileID, previewPath); err != nil {
		return fmt.Errorf("set preview done: %w", err)
	}

	return nil
}

func (repo *PgRepository) SetPreviewStatus(ctx context.Context, fileID int64, status string) error {
	const query = `UPDATE files SET preview_path = NULL, preview_status = $2 WHERE id = $1`

	if _, err := repo.db.ExecContext(ctx, query, fileID, status); err != nil {
		return fmt.Errorf("set preview status: %w", err)
	}

	return nil
}

func (repo *PgRepository) DeleteFile(ctx context.Context, fileID int64) error {
	const query = `DELETE FROM files WHERE id = $1`

	res, err := repo.db.ExecContext(ctx, query, fileID)
	if err != nil {
		return fmt.Errorf("delete file: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete file: rows affected: %w", err)
	}
	if n == 0 {
		return ErrFileNotFound
	}

	return nil
}

func (repo *PgRepository) CreateDir(ctx context.Context, dirData NewDirData) (int64, error) {
	const query = `INSERT INTO dirs (name, description) VALUES ($1, $2) RETURNING id`

	var dirID int64

	err := repo.db.QueryRowContext(ctx, query, dirData.Name, dirData.Description).Scan(&dirID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return 0, ErrDirAlreadyExists
		}

		return 0, fmt.Errorf("insert dir: %w", err)
	}

	return dirID, nil
}

func (repo *PgRepository) FileNameTaken(ctx context.Context, dirID int64, name string, exceptFileID int64) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM files
			WHERE dir_id = $1
				AND lower(original_name) = lower($2)
				AND id <> $3
		)
	`

	var taken bool
	if err := repo.db.QueryRowContext(ctx, query, dirID, name, exceptFileID).Scan(&taken); err != nil {
		return false, fmt.Errorf("file name taken: %w", err)
	}

	return taken, nil
}

func (repo *PgRepository) RenameFile(ctx context.Context, fileID int64, name string) (FileInfo, error) {
	const query = `
		UPDATE files SET original_name = $2 WHERE id = $1
		RETURNING
			id, original_name, description, size_bytes, mime_type, original_created_at,
			preview_status = 'done' AS has_preview, date_source
	`

	var fileInfo FileInfo
	if err := repo.db.QueryRowContext(ctx, query, fileID, name).Scan(
		&fileInfo.ID,
		&fileInfo.OriginalName,
		&fileInfo.Description,
		&fileInfo.Size,
		&fileInfo.MimeType,
		&fileInfo.OriginalCreated,
		&fileInfo.HasPreview,
		&fileInfo.DateSource,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FileInfo{}, ErrFileNotFound
		}

		return FileInfo{}, fmt.Errorf("rename file: %w", err)
	}

	return fileInfo, nil
}

func (repo *PgRepository) RenameDir(ctx context.Context, dirID int64, name string) (DirInfo, error) {
	const query = `
		UPDATE dirs SET name = $2 WHERE id = $1 AND NOT is_root
		RETURNING id, name, is_root, (SELECT count(id) FROM files WHERE dir_id = dirs.id)
	`

	var dirInfo DirInfo
	err := repo.db.QueryRowContext(ctx, query, dirID, name).Scan(
		&dirInfo.ID,
		&dirInfo.Name,
		&dirInfo.IsRoot,
		&dirInfo.FileCount,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DirInfo{}, ErrDirNotFound
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return DirInfo{}, ErrDirAlreadyExists
		}

		return DirInfo{}, fmt.Errorf("rename dir: %w", err)
	}

	return dirInfo, nil
}

func (repo *PgRepository) DeleteDir(ctx context.Context, dirID int64) error {
	const query = `DELETE FROM dirs WHERE id = $1 AND NOT is_root`

	res, err := repo.db.ExecContext(ctx, query, dirID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return ErrDirNotEmpty
		}

		return fmt.Errorf("delete dir: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete dir: rows affected: %w", err)
	}
	if n == 0 {
		return ErrDirNotFound
	}

	return nil
}

func (repo *PgRepository) MoveFile(ctx context.Context, fileID, dirID int64) (FileInfo, error) {
	const query = `
		UPDATE files SET dir_id = $2 WHERE id = $1
		RETURNING
			id, original_name, description, size_bytes, mime_type, original_created_at,
			preview_status = 'done' AS has_preview, date_source
	`

	var fileInfo FileInfo
	if err := repo.db.QueryRowContext(ctx, query, fileID, dirID).Scan(
		&fileInfo.ID,
		&fileInfo.OriginalName,
		&fileInfo.Description,
		&fileInfo.Size,
		&fileInfo.MimeType,
		&fileInfo.OriginalCreated,
		&fileInfo.HasPreview,
		&fileInfo.DateSource,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FileInfo{}, ErrFileNotFound
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			return FileInfo{}, ErrDirNotFound
		}

		return FileInfo{}, fmt.Errorf("move file: %w", err)
	}

	return fileInfo, nil
}
