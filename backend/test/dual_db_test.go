package test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

var DBS = []string{
	"test",
	"account",
	"bot",
}

func TestMain(m *testing.M) {
	root, err := utils.GetProjectRoot()
	if err != nil {
		log.Fatal("error getting project root")
	}

	testEnv := path.Join(root, "./.test.env")
	if !utils.Exists(testEnv) {
		log.Fatal("no test env present")
	}

	godotenv.Load(testEnv)

	dbType := os.Getenv("DB_TYPE")
	if dbType == "postgres" {
		err = postgresSetup()
	} else {
		err = sqliteSetup()
	}
	if err != nil {
		log.Fatalf("db setup error: %s", err)
	}

	code := m.Run()

	if dbType == "postgres" {
		err = postgresTeardown()
	} else {
		err = sqliteTeardown()
	}
	if err != nil {
		log.Fatalf("db teardown error: %s", err)
	}

	os.Exit(code)
}

func postgresSetup() error {
	connString := db.MakeDbConnString("postgres")
	fmt.Println(connString)
	pdb, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		return fmt.Errorf("error connecting to postgres db during setup: %s", err)
	}
	defer pdb.Close(context.Background())
	for _, db := range DBS {
		_, err = pdb.Exec(context.Background(), fmt.Sprintf("CREATE DATABASE %s", fmt.Sprintf("test_%s", db)))
		if err != nil {
			return fmt.Errorf("error creating %s test db: %s", db, err)
		}
	}
	return nil
}

func postgresTeardown() error {
	connString := db.MakeDbConnString("postgres")
	pdb, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		return fmt.Errorf("error connecting to postgres db during setup: %s", err)
	}
	defer pdb.Close(context.Background())
	for _, d := range DBS {
		_, err = pdb.Exec(context.Background(), fmt.Sprintf("DROP DATABASE %s", fmt.Sprintf("test_%s", d)))
		if err != nil {
			return fmt.Errorf("error dropping %s test db: %s", d, err)
		}
	}
	return nil
}

func sqliteSetup() error {
	for _, d := range DBS {
		_, err := db.InitDB(fmt.Sprintf("test_%s", d))
		if err != nil {
			return fmt.Errorf("error initializing %s db", err)
		}
	}
	return nil
}

func sqliteTeardown() error {
	projectRoot, err := utils.GetProjectRoot()
	if err != nil {
		projectRoot = "./"
	}
	dbFolderPath := os.Getenv("DB_PATH")
	if dbFolderPath == "" {
		dbFolderPath = "./test_data"
	}
	dbFolderPath = filepath.Join(projectRoot, dbFolderPath)
	err = os.RemoveAll(dbFolderPath)
	if err != nil {
		return fmt.Errorf("error removing test db folder: %s", err)
	}
	return nil
}

func TestDBConnection(t *testing.T) {
	conn, err := db.InitDB("test_test")
	if err != nil {
		t.Fatalf("error establishing db connection: %s", err)
	}
	conn.Close()
}

func TestCreateAccountTables(t *testing.T) {
	adb, err := db.InitDB("test_account")
	if err != nil {
		t.Fatal("failed to establish db connection")
	}
	defer adb.Close()

	accountDB := db.Account(adb)

	err = accountDB.CreateTables()
	if err != nil {
		t.Fatalf("error creating account tables: %s", err)
	}
}
