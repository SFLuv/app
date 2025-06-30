package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type SFLuvDB struct {
	db *gorm.DB
}

func (s *SFLuvDB) GetGormDB() *gorm.DB {
	if s.db == nil {
		fmt.Println("Database connection is not initialized.")
		return nil
	}
	return s.db
}

func (s *SFLuvDB) GetDB() *sql.DB {
	db, err := s.db.DB()
	if err != nil {
		return nil
	}
	return db
}

func DBPath(name string) string {
	dbFolderPath := os.Getenv("DB_FOLDER_PATH")
	dbPath := fmt.Sprintf("%s/%s.db", dbFolderPath, name)

	if !exists(dbFolderPath) {
		fmt.Printf("no %s db folder found... creating %s\n", name, dbFolderPath)
		os.Mkdir(dbFolderPath, os.ModePerm)
	}

	if !exists(dbPath) {
		fmt.Printf("no %s db file found... creating %s\n", name, dbPath)
		os.Create(dbPath)
	}

	fmt.Printf("connecting to %s db...\n", name)
	return dbPath
}

func InitDB(name string) *SFLuvDB {
	db_type := os.Getenv("DB")
	if db_type == "" {
		db_type = "sqlite"
	}
	if db_type == "sqlite" {
		dbPath := DBPath(name)
		db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s", dbPath)), &gorm.Config{})
		if err != nil {
			fmt.Println(err)
			return nil
		}
		return &SFLuvDB{
			db: db,
		}
	} else if db_type == "postgres" {
		// Postgres connection logic here
		dsn := "host=localhost dbname=sfluv_development port=5432 sslmode=disable"
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			fmt.Println(err)
			return nil
		}
		return &SFLuvDB{
			db: db,
		}
	} else {
		return nil
	}
}

func exists(path string) bool {
	exists := true
	_, err := os.Stat(path)
	if err != nil {
		fmt.Println(err)
		exists = false
	}

	if os.IsExist(err) {
		exists = true
	}

	return exists
}
