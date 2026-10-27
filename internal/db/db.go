package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Connect(ctx context.Context, databaseURL string) (*sql.DB, error) {
	connect, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("fail to create pool: %w", err)
	}

	connect.SetMaxOpenConns(10)

	for i := range 5 {
		if err = connect.PingContext(ctx); err == nil {
			return connect, nil
		}
		if i == 4 {
			break
		}
		select {
		case <-ctx.Done():
			connect.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	connect.Close()
	return nil, fmt.Errorf("fail to connect db: %w", err)
}
