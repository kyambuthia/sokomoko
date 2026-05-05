package db

import (
	"context"
	"database/sql"
	_ "embed"
	"log"
	"os"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

type Store struct {
	DB *sql.DB
}

//go:embed schema.sql
var schemaSQL string

func InitDB() {
	store, err := OpenStoreFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if err := store.ApplySchema(); err != nil {
		log.Fatal(err)
	}
}

func OpenStoreFromEnv() (*Store, error) {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./db/t.db"
	}
	return OpenStore(dbPath)
}

func OpenStore(dbPath string) (*Store, error) {
	return openStore(dbPath)
}

func openStore(dbPath string) (*Store, error) {
	dbConn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err = dbConn.Ping(); err != nil {
		_ = dbConn.Close()
		return nil, err
	}

	if _, err = dbConn.Exec("PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;"); err != nil {
		_ = dbConn.Close()
		return nil, err
	}

	// Keep pool bounded but allow nested read queries used by rendering paths.
	dbConn.SetMaxOpenConns(10)
	dbConn.SetMaxIdleConns(10)
	dbConn.SetConnMaxLifetime(0)

	return &Store{DB: dbConn}, nil
}

func (s *Store) ApplySchema() error {
	if s == nil || s.DB == nil {
		return sql.ErrConnDone
	}

	if _, err := s.DB.Exec(schemaSQL); err != nil {
		return err
	}

	if err := s.migrateLegacySessionsCSRFToken(); err != nil {
		return err
	}

	return s.recordSchemaVersion()
}

func (s *Store) migrateLegacySessionsCSRFToken() error {
	if s == nil || s.DB == nil {
		return sql.ErrConnDone
	}

	rows, err := s.DB.Query("PRAGMA table_info(sessions)")
	if err != nil {
		return err
	}
	defer rows.Close()

	hasCSRFToken := false
	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == "csrf_token" {
			hasCSRFToken = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if hasCSRFToken {
		return nil
	}

	// Existing databases may have a sessions table created before CSRF tokens existed.
	// Default to empty; empty tokens will be rejected, forcing re-login.
	_, err = s.DB.Exec("ALTER TABLE sessions ADD COLUMN csrf_token TEXT NOT NULL DEFAULT ''")
	return err
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) PingContext(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return sql.ErrConnDone
	}
	return s.DB.PingContext(ctx)
}
