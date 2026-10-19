package db

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func PgxDB(name string) (*pgxpool.Pool, error) {
	connString, err := DbConnString(name)
	if err != nil {
		return nil, err
	}
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 8

	return pgxpool.NewWithConfig(context.Background(), config)
}

// DbConnString builds the connection string for one named database.
//
// The backend opens several databases on one server (app, bot, ponder), so it
// takes the server and credentials once and swaps in each name. Two ways to
// give them:
//
//   - DATABASE_URL: a full postgres:// URL, as Railway and most hosts hand out.
//     Its database name is replaced with name; its query (sslmode etc.) is kept.
//   - DB_USER / DB_PASSWORD / DB_URL (host:port): the original form, unchanged.
//
// PONDER_DATABASE_URL, when set, is used verbatim (database name included) for
// the ponder database, so the indexer's database can live on a different server
// from app and bot.
//
// DB_SSLMODE, when set, overrides sslmode in either form. Unset, the existing
// behaviour stands: pgx's default, which uses TLS when the server offers it.
//
// A malformed DATABASE_URL or PONDER_DATABASE_URL is an error, never a silent
// fall-through to the DB_USER/DB_URL defaults (localhost): a typo in the URL
// must stop the boot, not connect somewhere unintended.
func DbConnString(name string) (string, error) {
	if name == ponderDBName() {
		if raw := strings.TrimSpace(os.Getenv("PONDER_DATABASE_URL")); raw != "" {
			// Its own database name wins: the indexer's database is often not
			// called "ponder" (a Railway Postgres defaults to "railway").
			connString, err := withDatabaseName(raw, "")
			if err != nil {
				return "", fmt.Errorf("invalid PONDER_DATABASE_URL: %w", err)
			}
			return connString, nil
		}
	}
	if raw := strings.TrimSpace(os.Getenv("DATABASE_URL")); raw != "" {
		connString, err := withDatabaseName(raw, name)
		if err != nil {
			return "", fmt.Errorf("invalid DATABASE_URL: %w", err)
		}
		return connString, nil
	}

	connString := legacyConnString(name)
	if sslmode := strings.TrimSpace(os.Getenv("DB_SSLMODE")); sslmode != "" {
		connString += "?sslmode=" + url.QueryEscape(sslmode)
	}
	return connString, nil
}

// MakeDbConnString is DbConnString for callers that cannot handle an error
// (tests and scripts building a server-level connection). It panics on a
// malformed URL rather than returning a connection string pointing elsewhere.
func MakeDbConnString(name string) string {
	connString, err := DbConnString(name)
	if err != nil {
		panic(err)
	}
	return connString
}

// withDatabaseName points a postgres URL at the named database. An empty name
// keeps the URL's own, which must then be present.
func withDatabaseName(raw, name string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("unsupported database URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("database URL has no host")
	}
	if name != "" {
		parsed.Path = "/" + name
		parsed.RawPath = ""
	} else if strings.Trim(parsed.Path, "/") == "" {
		return "", fmt.Errorf("database URL has no database name")
	}
	if sslmode := strings.TrimSpace(os.Getenv("DB_SSLMODE")); sslmode != "" {
		query := parsed.Query()
		query.Set("sslmode", sslmode)
		parsed.RawQuery = query.Encode()
	}
	return parsed.String(), nil
}

func ponderDBName() string {
	return envOrDefault("PONDER_DB_NAME", "ponder")
}

func legacyConnString(name string) string {
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
