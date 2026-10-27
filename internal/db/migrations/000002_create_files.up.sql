CREATE TABLE files (
    id                  BIGSERIAL   PRIMARY KEY,
    dir_id              BIGINT      NOT NULL,
    original_name       TEXT        NOT NULL,
    description         TEXT        NOT NULL DEFAULT '',
    storage_path        TEXT        NOT NULL UNIQUE,
    preview_path        TEXT        UNIQUE,
    mime_type           TEXT        NOT NULL,
    date_source         TEXT        NOT NULL CHECK (date_source IN ('exif','modtime','upload')),
    sha256              TEXT        NOT NULL,
    size_bytes          BIGINT      NOT NULL CHECK (size_bytes >= 0),
    original_created_at TIMESTAMP   NOT NULL, -- без таймзоны
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    FOREIGN KEY (dir_id) REFERENCES dirs(id) ON DELETE RESTRICT
);

CREATE TRIGGER files_set_updated_at
    BEFORE UPDATE ON files
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX files_dir_id_idx ON files (dir_id);