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

func OpenStoreNoSeed(dbPath string) (*Store, error) {
	return OpenStore(dbPath)
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

	_, err := s.DB.Exec(schemaSQL)
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
