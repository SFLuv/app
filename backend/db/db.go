package db

import (
	"context"
	"fmt"
	"log"
	"os"

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
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")

	if dbHost == "" {
		dbHost = "localhost:5432"
	}
	if dbUser == "" {
		dbUser = "postgres"
	}

	log.Printf("DB_USER=%s", dbUser)
	log.Printf("DB_HOST=%s", dbHost)

	return fmt.Sprintf("postgres://%s:%s@%s/%s", dbUser, dbPassword, dbHost, name)
}
