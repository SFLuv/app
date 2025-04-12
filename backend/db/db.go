package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

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

func InitDB(name string) *sql.DB {
	dbPath := DBPath(name)
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s", dbPath))
	if err != nil {
		fmt.Println(err)
		return nil
	}

	return db
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
