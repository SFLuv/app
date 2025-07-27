package db

import (
	"context"
	"fmt"
	"os"

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
