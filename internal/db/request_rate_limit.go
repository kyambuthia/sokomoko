package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	requestRateLimitCleanupBatch = 1000
	requestRateLimitRetries      = 3
)

// EnforceRequestRateLimit atomically consumes one hit from a process-shared
// fixed window. Only a keyed digest of the caller's IP is persisted.
func (s *Store) EnforceRequestRateLimit(ctx context.Context, keyHash []byte, max int, window time.Duration) (bool, time.Duration, error) {
	if len(keyHash) != 32 || max <= 0 || max == int(^uint(0)>>1) || window <= 0 {
		return false, 0, errors.New("invalid request rate limit")
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	const upsert = `INSERT INTO http_post_rate_limits(key_hash,hits,reset_at)
VALUES($1,1,statement_timestamp()+$2::double precision*interval '1 second')
ON CONFLICT(key_hash) DO UPDATE SET
 hits=CASE WHEN http_post_rate_limits.reset_at<=statement_timestamp() THEN 1
           ELSE LEAST(http_post_rate_limits.hits+1,$3::bigint) END,
 reset_at=CASE WHEN http_post_rate_limits.reset_at<=statement_timestamp()
                THEN statement_timestamp()+$2::double precision*interval '1 second'
                ELSE http_post_rate_limits.reset_at END
WHERE http_post_rate_limits.reset_at<=statement_timestamp()
   OR http_post_rate_limits.hits<$3::bigint
RETURNING hits,GREATEST(1,CEIL(EXTRACT(EPOCH FROM (reset_at-statement_timestamp()))))::bigint`
	for attempt := 0; attempt < requestRateLimitRetries; attempt++ {
		var hits, retrySeconds int64
		err := s.DB.QueryRowContext(ctx, upsert, keyHash, window.Seconds(), int64(max)).Scan(&hits, &retrySeconds)
		if err == nil {
			return true, time.Duration(retrySeconds) * time.Second, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, 0, err
		}

		// The conditional upsert does not rewrite a row once it is limited.
		// Read the retry time separately so a concurrent expiry or cleanup can
		// simply retry with a fresh statement timestamp.
		var active bool
		err = s.DB.QueryRowContext(ctx, `SELECT reset_at>statement_timestamp(),
 GREATEST(1,CEIL(EXTRACT(EPOCH FROM (reset_at-statement_timestamp()))))::bigint
FROM http_post_rate_limits WHERE key_hash=$1`, keyHash).Scan(&active, &retrySeconds)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !active) {
			continue
		}
		if err != nil {
			return false, 0, err
		}
		return false, time.Duration(retrySeconds) * time.Second, nil
	}
	return false, 0, fmt.Errorf("request rate limit window changed repeatedly")
}

// CleanupRequestRateLimits removes expired per-IP keys in bounded batches.
func (s *Store) CleanupRequestRateLimits(ctx context.Context) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	for {
		result, err := s.DB.ExecContext(ctx, `WITH expired AS (
 SELECT key_hash FROM http_post_rate_limits
 WHERE reset_at<=statement_timestamp()
 ORDER BY reset_at
 LIMIT $1
 FOR UPDATE SKIP LOCKED
)
DELETE FROM http_post_rate_limits AS limits
USING expired
WHERE limits.key_hash=expired.key_hash`, requestRateLimitCleanupBatch)
		if err != nil {
			return err
		}
		removed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if removed < requestRateLimitCleanupBatch {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
