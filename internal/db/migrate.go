package db

import (
	"database/sql"
	"fmt"
)

const CurrentSchemaVersion = 1

// CurrentSchemaVersionNote documents the current state of schema management.
// The application now records the latest applied schema version, but
// incremental database migrations are not implemented yet.
const CurrentSchemaVersionNote = "incremental migrations are not implemented yet"

func (s *Store) recordSchemaVersion() error {
	if s == nil || s.DB == nil {
		return sql.ErrConnDone
	}

	if _, err := s.DB.Exec(
		"INSERT OR IGNORE INTO schema_version (id, version) VALUES (1, ?)",
		CurrentSchemaVersion,
	); err != nil {
		return fmt.Errorf("insert schema version: %w", err)
	}

	if _, err := s.DB.Exec(
		"UPDATE schema_version SET version = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1",
		CurrentSchemaVersion,
	); err != nil {
		return fmt.Errorf("update schema version: %w", err)
	}

	return nil
}
