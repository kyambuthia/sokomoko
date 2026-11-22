package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TrafficRollupKey identifies an hourly rollup bucket by client kind and surface.
type TrafficRollupKey struct {
	BucketHour time.Time
	ClientKind string
	Surface    string
}

// TrafficRollupCounts holds aggregated request counters for one rollup key.
type TrafficRollupCounts struct {
	RequestCount     int64
	Error4xxCount    int64
	Error5xxCount    int64
	RateLimitedCount int64
}

// TrafficRollup combines a rollup key and its incremental counts.
type TrafficRollup struct {
	TrafficRollupKey
	TrafficRollupCounts
}

// TrafficBreakdownRow represents summed traffic metrics for a client kind and surface.
type TrafficBreakdownRow struct {
	ClientKind       string
	Surface          string
	RequestCount     int64
	Error4xxCount    int64
	Error5xxCount    int64
	RateLimitedCount int64
}

// TrafficSummary aggregates recent 24h and 7d traffic along with active API tokens.
type TrafficSummary struct {
	Last24h         []TrafficBreakdownRow
	Last7d          []TrafficBreakdownRow
	ActiveAPITokens int
}

// UpsertTrafficRollups atomically increments hourly traffic rollup counters.
func (s *Store) UpsertTrafficRollups(ctx context.Context, rollups []TrafficRollup) error {
	if len(rollups) == 0 {
		return nil
	}

	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	merged := make(map[TrafficRollupKey]TrafficRollupCounts, len(rollups))
	keys := make([]TrafficRollupKey, 0, len(rollups))
	for _, r := range rollups {
		clientKind := strings.TrimSpace(r.ClientKind)
		surface := strings.TrimSpace(r.Surface)
		if clientKind == "" || surface == "" {
			continue
		}
		if r.RequestCount == 0 && r.Error4xxCount == 0 && r.Error5xxCount == 0 && r.RateLimitedCount == 0 {
			continue
		}
		k := TrafficRollupKey{
			BucketHour: r.BucketHour.UTC().Truncate(time.Hour),
			ClientKind: clientKind,
			Surface:    surface,
		}
		prev, exists := merged[k]
		if !exists {
			keys = append(keys, k)
		}
		prev.RequestCount += r.RequestCount
		prev.Error4xxCount += r.Error4xxCount
		prev.Error5xxCount += r.Error5xxCount
		prev.RateLimitedCount += r.RateLimitedCount
		merged[k] = prev
	}
	if len(keys) == 0 {
		return nil
	}

	const batchSize = 200
	for start := 0; start < len(keys); start += batchSize {
		end := min(start+batchSize, len(keys))
		batch := keys[start:end]

		var b strings.Builder
		b.WriteString(`INSERT INTO traffic_rollups (bucket_hour, client_kind, surface, request_count, error_4xx_count, error_5xx_count, rate_limited_count) VALUES `)
		args := make([]any, 0, len(batch)*7)
		for i, k := range batch {
			if i > 0 {
				b.WriteString(", ")
			}
			base := i * 7
			fmt.Fprintf(&b, "($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
				base+1, base+2, base+3, base+4, base+5, base+6, base+7)
			c := merged[k]
			args = append(args, k.BucketHour, k.ClientKind, k.Surface,
				c.RequestCount, c.Error4xxCount, c.Error5xxCount, c.RateLimitedCount)
		}
		b.WriteString(` ON CONFLICT (bucket_hour, client_kind, surface) DO UPDATE SET
			request_count = traffic_rollups.request_count + EXCLUDED.request_count,
			error_4xx_count = traffic_rollups.error_4xx_count + EXCLUDED.error_4xx_count,
			error_5xx_count = traffic_rollups.error_5xx_count + EXCLUDED.error_5xx_count,
			rate_limited_count = traffic_rollups.rate_limited_count + EXCLUDED.rate_limited_count`)

		if _, err := s.DB.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

// GetTrafficSummary returns 24h and 7d traffic breakdowns plus the active API token count.
func (s *Store) GetTrafficSummary(ctx context.Context, now time.Time) (TrafficSummary, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	var summary TrafficSummary
	var err error
	summary.Last24h, err = s.queryTrafficBreakdown(ctx, now.Add(-24*time.Hour))
	if err != nil {
		return TrafficSummary{}, err
	}
	summary.Last7d, err = s.queryTrafficBreakdown(ctx, now.Add(-7*24*time.Hour))
	if err != nil {
		return TrafficSummary{}, err
	}
	err = s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_tokens WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at > $1)`,
		now,
	).Scan(&summary.ActiveAPITokens)
	if err != nil {
		return TrafficSummary{}, err
	}
	return summary, nil
}

func (s *Store) queryTrafficBreakdown(ctx context.Context, since time.Time) ([]TrafficBreakdownRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT client_kind, surface,
		       COALESCE(SUM(request_count), 0),
		       COALESCE(SUM(error_4xx_count), 0),
		       COALESCE(SUM(error_5xx_count), 0),
		       COALESCE(SUM(rate_limited_count), 0)
		FROM traffic_rollups
		WHERE bucket_hour >= $1
		GROUP BY client_kind, surface
		ORDER BY SUM(request_count) DESC, client_kind ASC, surface ASC`,
		since.UTC().Truncate(time.Hour),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TrafficBreakdownRow
	for rows.Next() {
		var row TrafficBreakdownRow
		if err := rows.Scan(
			&row.ClientKind,
			&row.Surface,
			&row.RequestCount,
			&row.Error4xxCount,
			&row.Error5xxCount,
			&row.RateLimitedCount,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
