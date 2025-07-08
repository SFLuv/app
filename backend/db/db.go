package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jackc/pgx/v5"
)

func PgxDB(name string) (*pgx.Conn, error) {
	connString := MakeDbConnString(name)

	return pgx.Connect(context.Background(), connString)
}

func MakeDbConnString(name string) string {
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbPath := os.Getenv("DB_URL")

	if dbPath == "" {
		dbPath = "localhost:5432"
	}
	if dbUser == "" {
		dbUser = "postgres"
	}

	return fmt.Sprintf("postgres://%s:%s@%s/%s", dbUser, dbPassword, dbPath, name)
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
