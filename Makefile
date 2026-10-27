DB_USER      := $(shell grep '^DB_USER=' .env | cut -d= -f2)
DB_PASSWORD  := $(shell grep '^DB_PASSWORD=' .env | cut -d= -f2)
DB_HOST_PORT := $(shell grep '^DB_HOST_PORT=' .env | cut -d= -f2)
TEST_DB_NAME ?= filevault_test

# База видна с хоста на localhost:DB_HOST_PORT.
# DATABASE_URL из .env для этого не годится: там имя контейнера,
# оно резолвится только внутри docker-сети.
TEST_DATABASE_URL := postgres://$(DB_USER):$(DB_PASSWORD)@localhost:$(DB_HOST_PORT)/$(TEST_DB_NAME)?sslmode=disable

.PHONY: run stop logs test test-int test-db-create

run:
	docker compose up -d

stop:
	docker compose down

logs:
	docker compose logs -f app

# Быстрые тесты: без базы, интеграционные пропускаются по t.Skip
test:
	go test ./...

# Полный прогон. -count=1 обязателен: go test кэширует результаты,
# а состояние внешней базы в ключ кэша не входит
test-int:
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test ./... -count=1 -v

# Один раз перед первым test-int. Таблицы создаст db.Migrate() из TestMain
test-db-create:
	docker compose exec filestorage_db psql -U $(DB_USER) -d postgres \
		-c "CREATE DATABASE $(TEST_DB_NAME)"
