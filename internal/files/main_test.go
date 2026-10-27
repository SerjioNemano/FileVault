package files

import (
	"context"
	"database/sql"
	"filevault/internal/db"
	"log"
	"os"
	"testing"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		conn, err := db.Connect(context.Background(), url)
		if err != nil {
			log.Fatalf("тестовая база недоступна: %v", err)
		}
		if err := db.Migrate(url); err != nil {
			log.Fatalf("миграции: %v", err)
		}
		testDB = conn
	}

	code := m.Run()
	if testDB != nil {
		testDB.Close()
	}
	os.Exit(code)
}

func requireDB(t *testing.T) *sql.DB {
	t.Helper()
	if testDB == nil {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	return testDB
}
