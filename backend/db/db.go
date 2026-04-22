package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func PgxDB(name string) (*pgxpool.Pool, error) {
	connString := MakeDbConnString(name)
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 8

	return pgxpool.NewWithConfig(context.Background(), config)
}

func MakeDbConnString(name string) string {
	dbUser := envOrDefault("DB_USER", "postgres")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbPath := firstNonEmptyEnv("DB_BASE_URL", "DB_URL")
	if dbPath == "" {
		dbPath = "localhost:5432"
	}

	return fmt.Sprintf("postgres://%s:%s@%s/%s", dbUser, dbPassword, dbPath, name)
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func envOrDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func getProjectRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to get caller information")
	}
	currentDir := filepath.Dir(filename)

	for {
		// Check for common project root indicators
		if _, err := os.Stat(filepath.Join(currentDir, "go.mod")); err == nil {
			return currentDir, nil
		}
		if _, err := os.Stat(filepath.Join(currentDir, ".git")); err == nil {
			return currentDir, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir { // Reached file system root
			return "", fmt.Errorf("project root not found")
		}
		currentDir = parentDir
	}
}
