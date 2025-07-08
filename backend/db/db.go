package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	_ "github.com/mattn/go-sqlite3"
)

func DBPath(name string) string {
	projectRoot, err := getProjectRoot()
	if err != nil {
		fmt.Println("err getting project root")
		projectRoot = "./"
	}
	dbFolderPath := os.Getenv("DB_FOLDER_PATH")
	dbFolderPath = filepath.Join(projectRoot, dbFolderPath)
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
