package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port           string
	DatabaseURL    string
	StorageDir     string
	MaxUploadBytes int64
}

func Load() (*Config, error) {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is not set")
	}

	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		return nil, errors.New("STORAGE_DIR is not set")
	}

	maxUploadBytesStr := os.Getenv("MAX_UPLOAD_BYTES")
	if maxUploadBytesStr == "" {
		return nil, errors.New("MAX_UPLOAD_BYTES is not set")
	}

	maxUploadBytes, err := strconv.ParseInt(maxUploadBytesStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("MAX_UPLOAD_BYTES: %w", err)
	}
	if maxUploadBytes <= 0 {
		return nil, fmt.Errorf("MAX_UPLOAD_BYTES must be positive, got %d", maxUploadBytes)
	}

	return &Config{
		Port:           port,
		DatabaseURL:    databaseURL,
		StorageDir:     storageDir,
		MaxUploadBytes: maxUploadBytes,
	}, nil
}
