package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// hashOpaqueToken stores bearer tokens (session IDs, reset tokens) as SHA-256
// digests so a database leak does not expose usable credentials.
func hashOpaqueToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func nullIfEmpty(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullInt64(value int) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(value), Valid: true}
}

// int64Slice converts ids into a slice pgx can bind to an int8[] parameter.
func int64Slice(ids []int) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out
}

// escapeLike escapes LIKE/ILIKE wildcards in user input.
func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func rowsAffected(result sql.Result) (int64, error) {
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return affected, nil
}

// timeNow is a seam for tests that need to control the clock.
var timeNow = time.Now
