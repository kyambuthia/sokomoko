package db

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"
)

const (
	authAbuseQueryTimeout = 150 * time.Millisecond
	authAbuseCleanupBatch = 1000
)

// CheckAuthAbuseLock returns whether a keyed authentication abuse record is
// currently locked and, when locked, its remaining duration.
func (s *Store) CheckAuthAbuseLock(ctx context.Context, keyHash []byte) (bool, time.Duration, int64, error) {
	if len(keyHash) != 32 {
		return false, 0, 0, errors.New("invalid authentication abuse key")
	}
	ctx, cancel := context.WithTimeout(ctx, authAbuseQueryTimeout)
	defer cancel()

	var generation, retrySeconds int64
	var locked bool
	err := s.DB.QueryRowContext(ctx, `SELECT success_generation,
 COALESCE(locked_until>statement_timestamp(),false),
 CASE WHEN locked_until>statement_timestamp()
   THEN GREATEST(1,CEIL(EXTRACT(EPOCH FROM (locked_until-statement_timestamp()))))::bigint
   ELSE 0
 END
FROM auth_abuse_limits WHERE key_hash=$1`, keyHash).Scan(&generation, &locked, &retrySeconds)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, 0, nil
	}
	if err != nil {
		return false, 0, 0, err
	}
	if !locked {
		return false, 0, generation, nil
	}
	return true, time.Duration(retrySeconds) * time.Second, generation, nil
}

// RecordAuthAbuseFailure atomically increments an authentication failure and
// sets capped exponential backoff in the database. Inactive keys restart at
// one failure after the configured maximum backoff interval.
func (s *Store) RecordAuthAbuseFailure(ctx context.Context, keyHash []byte, generation int64, maxFailures int, backoffBase, backoffMax time.Duration) error {
	if len(keyHash) != 32 || maxFailures <= 0 || uint64(maxFailures) > math.MaxInt32 || backoffBase <= 0 || backoffMax < backoffBase {
		return errors.New("invalid authentication abuse limit")
	}

	exponentCap := authAbuseExponentCap(backoffBase, backoffMax)
	failureCap := int64(maxFailures) + exponentCap
	ctx, cancel := context.WithTimeout(ctx, authAbuseQueryTimeout)
	defer cancel()

	_, err := s.DB.ExecContext(ctx, `INSERT INTO auth_abuse_limits(key_hash,success_generation,failure_count,locked_until,expires_at)
VALUES($1,0,1,
 CASE WHEN $3::bigint<=1 THEN statement_timestamp()+$4::double precision*interval '1 second' END,
 statement_timestamp()+$5::double precision*interval '1 second')
ON CONFLICT(key_hash) DO UPDATE SET
 failure_count=CASE
   WHEN auth_abuse_limits.expires_at<=statement_timestamp()
     AND (auth_abuse_limits.locked_until IS NULL OR auth_abuse_limits.locked_until<=statement_timestamp()) THEN 1
   ELSE LEAST(auth_abuse_limits.failure_count+1,$6::bigint)
 END,
 locked_until=CASE
   WHEN auth_abuse_limits.expires_at<=statement_timestamp()
     AND (auth_abuse_limits.locked_until IS NULL OR auth_abuse_limits.locked_until<=statement_timestamp()) THEN
     CASE WHEN $3::bigint<=1
       THEN statement_timestamp()+$4::double precision*interval '1 second'
       ELSE NULL
     END
   WHEN LEAST(auth_abuse_limits.failure_count+1,$6::bigint)>=$3::bigint THEN
     statement_timestamp()+LEAST(
       $4::double precision*power(2::double precision,
         LEAST(GREATEST(auth_abuse_limits.failure_count+1-$3::bigint,0),$7::bigint)::double precision),
       $5::double precision
     )*interval '1 second'
   ELSE auth_abuse_limits.locked_until
 END,
 expires_at=statement_timestamp()+$5::double precision*interval '1 second'
WHERE $2::bigint<0 OR auth_abuse_limits.success_generation=$2::bigint`,
		keyHash,
		generation,
		int64(maxFailures),
		backoffBase.Seconds(),
		backoffMax.Seconds(),
		failureCap,
		exponentCap,
	)
	return err
}

// ClearAuthAbuseFailures resets a key and advances its generation after successful authentication.
func (s *Store) ClearAuthAbuseFailures(ctx context.Context, keyHash []byte, backoffMax time.Duration) error {
	if len(keyHash) != 32 || backoffMax <= 0 {
		return errors.New("invalid authentication abuse key")
	}
	ctx, cancel := context.WithTimeout(ctx, authAbuseQueryTimeout)
	defer cancel()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO auth_abuse_limits(key_hash,success_generation,failure_count,locked_until,expires_at)
VALUES($1,1,0,NULL,statement_timestamp()+$2::double precision*interval '1 second')
ON CONFLICT(key_hash) DO UPDATE SET
 success_generation=auth_abuse_limits.success_generation+1,
 failure_count=0,
 locked_until=NULL,
 expires_at=statement_timestamp()+$2::double precision*interval '1 second'`, keyHash, backoffMax.Seconds())
	return err
}

// CleanupAuthAbuseLimits prunes inactive authentication keys in small batches.
func (s *Store) CleanupAuthAbuseLimits(ctx context.Context) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	for {
		result, err := s.DB.ExecContext(ctx, `WITH expired AS (
 SELECT key_hash FROM auth_abuse_limits
 WHERE expires_at<=statement_timestamp()
   AND (locked_until IS NULL OR locked_until<=statement_timestamp())
 ORDER BY expires_at
 LIMIT $1
 FOR UPDATE SKIP LOCKED
)
DELETE FROM auth_abuse_limits AS limits
USING expired
WHERE limits.key_hash=expired.key_hash`, authAbuseCleanupBatch)
		if err != nil {
			return err
		}
		removed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if removed < authAbuseCleanupBatch {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func authAbuseExponentCap(backoffBase, backoffMax time.Duration) int64 {
	var steps int64
	for value := backoffBase; value < backoffMax; steps++ {
		if value > backoffMax/2 {
			value = backoffMax
		} else {
			value *= 2
		}
	}
	return steps
}
