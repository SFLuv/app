package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
	_ "github.com/mattn/go-sqlite3"
)

func PgxDB(name string) (*pgx.Conn, error) {
	dbType := os.Getenv("DB_TYPE")
	if dbType != "postgres" {
		return nil, errors.New("must be using postgres db type to create pgx connection")
	}
	connString := MakeDbConnString(name)

	return pgx.Connect(context.Background(), connString)
}

func InitDB(name string) (*sql.DB, error) {
	dbType := os.Getenv("DB_TYPE")

	connStr := MakeDbConnString(name)

	var driver string
	if dbType == "postgres" {
		driver = "pgx"
	} else {
		driver = "sqlite3"
	}

	conn, err := sql.Open(driver, connStr)
	if err != nil {
		return nil, err
	}

	err = conn.Ping()
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func MakeDbConnString(name string) string {
	dbType := os.Getenv("DB_TYPE")

	switch dbType {
	case "postgres":
		{
			dbUser := os.Getenv("DB_USER")
			dbPassword := os.Getenv("DB_PASSWORD")
			dbPath := os.Getenv("DB_PATH")

			if dbPath == "" {
				dbPath = "localhost:5432"
			}
			if dbUser == "" {
				dbUser = "test"
			}
			if dbPassword == "" {
				dbPassword = "test"
			}

			return fmt.Sprintf("postgres://%s:%s@%s/%s", dbUser, dbPassword, dbPath, name)
		}
	default:
		{
			return fmt.Sprintf("file:%s", sqlitePath(name))
		}
	}
}

func sqlitePath(name string) string {
	projectRoot, err := utils.GetProjectRoot()
	if err != nil {
		fmt.Println("error getting project root")
		projectRoot = "./"
	}
	dbFolderPath := os.Getenv("DB_PATH")
	if dbFolderPath == "" {
		dbFolderPath = "./test_data"
	}
	dbFolderPath = filepath.Join(projectRoot, dbFolderPath)
	dbPath := fmt.Sprintf("%s/%s.db", dbFolderPath, name)

	if !utils.Exists(dbFolderPath) {
		fmt.Printf("no %s db folder found... creating %s\n", name, dbFolderPath)
		os.Mkdir(dbFolderPath, os.ModePerm)
	}

	if !utils.Exists(dbPath) {
		fmt.Printf("no %s db file found... creating %s\n", name, dbPath)
		os.Create(dbPath)
	}

	fmt.Printf("connecting to %s db...\n", name)
	return dbPath
}
