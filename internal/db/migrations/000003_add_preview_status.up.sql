ALTER TABLE files
    ADD COLUMN preview_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (preview_status IN ('pending', 'done', 'failed', 'skipped'));

CREATE INDEX files_preview_pending_idx ON files (id) WHERE preview_status = 'pending';
