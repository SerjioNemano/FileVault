ALTER TABLE dirs
    ADD COLUMN description TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX dirs_name_lower_key ON dirs (lower(name));
