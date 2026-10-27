DROP INDEX IF EXISTS files_preview_pending_idx;
ALTER TABLE files DROP COLUMN IF EXISTS preview_status;
