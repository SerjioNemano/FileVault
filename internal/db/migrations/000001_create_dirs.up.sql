CREATE TABLE dirs (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT    NOT NULL,
    is_root      BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- корень ровно один: гарантирует база, а не приложение
CREATE UNIQUE INDEX dirs_single_root ON dirs (is_root) WHERE is_root;

-- общая для всех таблиц: DEFAULT now() срабатывает только на INSERT,
-- при UPDATE время обновляет триггер
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER dirs_set_updated_at
    BEFORE UPDATE ON dirs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO dirs (name, is_root) VALUES ('Корень', TRUE);
