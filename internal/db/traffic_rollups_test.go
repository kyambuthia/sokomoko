package db

import (
	"database/sql"
	"testing"
	"time"
)

func TestUpsertAndGetTrafficSummary(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	now := time.Date(2026, 11, 22, 14, 30, 0, 0, time.UTC)

	userID := mustCreateUser(t, s, RoleUser)
	if _, err := s.CreateAPIToken(ctx, userID, "active", "smk_act1", "smk_act1_secret", "agent", sql.NullTime{}); err != nil {
		t.Fatalf("CreateAPIToken active: %v", err)
	}
	revokedTok, err := s.CreateAPIToken(ctx, userID, "revoked", "smk_rev1", "smk_rev1_secret", "agent", sql.NullTime{})
	if err != nil {
		t.Fatalf("CreateAPIToken revoked: %v", err)
	}
	if _, err := s.RevokeAPIToken(ctx, userID, revokedTok.ID); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if _, err := s.CreateAPIToken(ctx, userID, "expired", "smk_exp1", "smk_exp1_secret", "agent", sql.NullTime{
		Time:  now.Add(-time.Hour),
		Valid: true,
	}); err != nil {
		t.Fatalf("CreateAPIToken expired: %v", err)
	}

	err = s.UpsertTrafficRollups(ctx, []TrafficRollup{
		{
			TrafficRollupKey: TrafficRollupKey{
				BucketHour: now.Add(-2 * time.Hour),
				ClientKind: "agent_token",
				Surface:    "api",
			},
			TrafficRollupCounts: TrafficRollupCounts{
				RequestCount:     10,
				Error4xxCount:    2,
				Error5xxCount:    1,
				RateLimitedCount: 1,
			},
		},
		{
			TrafficRollupKey: TrafficRollupKey{
				BucketHour: now.Add(-2*time.Hour + 15*time.Minute), // same hour bucket
				ClientKind: "agent_token",
				Surface:    "api",
			},
			TrafficRollupCounts: TrafficRollupCounts{
				RequestCount: 5,
			},
		},
		{
			TrafficRollupKey: TrafficRollupKey{
				BucketHour: now.Add(-48 * time.Hour),
				ClientKind: "browser",
				Surface:    "storefront",
			},
			TrafficRollupCounts: TrafficRollupCounts{
				RequestCount:  20,
				Error4xxCount: 1,
			},
		},
		{
			TrafficRollupKey: TrafficRollupKey{
				BucketHour: now.Add(-10 * 24 * time.Hour), // older than 7d
				ClientKind: "crawler",
				Surface:    "storefront",
			},
			TrafficRollupCounts: TrafficRollupCounts{
				RequestCount: 99,
			},
		},
	})
	if err != nil {
		t.Fatalf("UpsertTrafficRollups: %v", err)
	}

	// Upsert again to verify ON CONFLICT accumulation.
	err = s.UpsertTrafficRollups(ctx, []TrafficRollup{
		{
			TrafficRollupKey: TrafficRollupKey{
				BucketHour: now.Add(-2 * time.Hour),
				ClientKind: "agent_token",
				Surface:    "api",
			},
			TrafficRollupCounts: TrafficRollupCounts{
				RequestCount: 3,
			},
		},
	})
	if err != nil {
		t.Fatalf("UpsertTrafficRollups second: %v", err)
	}

	summary, err := s.GetTrafficSummary(ctx, now)
	if err != nil {
		t.Fatalf("GetTrafficSummary: %v", err)
	}
	if summary.ActiveAPITokens != 1 {
		t.Fatalf("ActiveAPITokens = %d, want 1", summary.ActiveAPITokens)
	}
	if len(summary.Last24h) != 1 {
		t.Fatalf("Last24h len = %d, want 1 (%+v)", len(summary.Last24h), summary.Last24h)
	}
	got24 := summary.Last24h[0]
	if got24.ClientKind != "agent_token" || got24.Surface != "api" || got24.RequestCount != 18 || got24.Error4xxCount != 2 || got24.Error5xxCount != 1 || got24.RateLimitedCount != 1 {
		t.Fatalf("Last24h[0] = %+v", got24)
	}
	if len(summary.Last7d) != 2 {
		t.Fatalf("Last7d len = %d, want 2 (%+v)", len(summary.Last7d), summary.Last7d)
	}
}
