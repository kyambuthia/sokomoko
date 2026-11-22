package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/domain"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
)

type stubAdminReportService struct {
	app.AdminService
}

func (s *stubAdminReportService) Reports(_ context.Context) (adminsvc.SalesReport, error) {
	return adminsvc.SalesReport{
		RevenueTotal:   12500,
		OrderCount:     3,
		PendingCount:   1,
		ShippedCount:   1,
		DeliveredCount: 1,
	}, nil
}

func (s *stubAdminReportService) TrafficSummary(_ context.Context) (adminsvc.TrafficSummary, error) {
	return adminsvc.TrafficSummary{
		ActiveAPITokens: 4,
		Last24h: []adminsvc.TrafficBreakdown{
			{ClientKind: "agent_token", Surface: "api", RequestCount: 25, Error4xxCount: 2, RateLimitedCount: 1},
		},
		Last7d: []adminsvc.TrafficBreakdown{
			{ClientKind: "agent_token", Surface: "api", RequestCount: 25, Error4xxCount: 2, RateLimitedCount: 1},
			{ClientKind: "browser", Surface: "storefront", RequestCount: 100},
		},
	}, nil
}

func TestAdminReportsIncludesTrafficSummary(t *testing.T) {
	h := &Handler{app: &app.App{
		Admin: &stubAdminReportService{},
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/reports", nil)
	req = req.WithContext(auth.ContextWithUser(req.Context(), &db.User{ID: 1, Role: domain.RoleAdmin}))
	rec := httptest.NewRecorder()
	h.adminReports(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data salesReportDTO `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Data.OrderCount != 3 || payload.Data.Traffic.ActiveAPITokens != 4 {
		t.Fatalf("unexpected payload: %+v", payload.Data)
	}
	if len(payload.Data.Traffic.Last24h) != 1 || payload.Data.Traffic.Last24h[0].ClientKind != "agent_token" {
		t.Fatalf("unexpected last_24h: %+v", payload.Data.Traffic.Last24h)
	}
	if len(payload.Data.Traffic.Last7d) != 2 {
		t.Fatalf("unexpected last_7d: %+v", payload.Data.Traffic.Last7d)
	}
}
