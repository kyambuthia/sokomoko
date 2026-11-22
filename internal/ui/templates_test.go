package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kyambuthia/sokomoko/internal/money"
)

func TestDemoBannerAndCredentials(t *testing.T) {
	type authPageData struct {
		Title                string
		CSRFToken            string
		Error                string
		Username             string
		Email                string
		Next                 string
		SetupRequired        bool
		PasswordResetEnabled bool
	}

	t.Run("demo mode active renders demo-banner on login, signup, and admin/partner login", func(t *testing.T) {
		tmpls, err := ParseTemplatesWithDemo(true)
		if err != nil {
			t.Fatalf("ParseTemplatesWithDemo(true): %v", err)
		}

		for _, tc := range []struct {
			name        string
			tmpl        func() string
			wantContain []string
		}{
			{
				name: "login",
				tmpl: func() string {
					var buf bytes.Buffer
					if err := tmpls.Login.ExecuteTemplate(&buf, "root_template", authPageData{}); err != nil {
						t.Fatalf("execute login: %v", err)
					}
					return buf.String()
				},
				wantContain: []string{`class="demo-banner"`, "demo-customer"},
			},
			{
				name: "signup",
				tmpl: func() string {
					var buf bytes.Buffer
					if err := tmpls.Signup.ExecuteTemplate(&buf, "root_template", authPageData{}); err != nil {
						t.Fatalf("execute signup: %v", err)
					}
					return buf.String()
				},
				wantContain: []string{`class="demo-banner"`},
			},
			{
				name: "partner login",
				tmpl: func() string {
					var buf bytes.Buffer
					if err := tmpls.AdminLogin.ExecuteTemplate(&buf, "root_template", authPageData{Title: "Partner Login"}); err != nil {
						t.Fatalf("execute partner login: %v", err)
					}
					return buf.String()
				},
				wantContain: []string{`class="demo-banner"`, "demo-partner"},
			},
			{
				name: "admin login",
				tmpl: func() string {
					var buf bytes.Buffer
					if err := tmpls.AdminLogin.ExecuteTemplate(&buf, "root_template", authPageData{Title: "Admin Login"}); err != nil {
						t.Fatalf("execute admin login: %v", err)
					}
					return buf.String()
				},
				wantContain: []string{`class="demo-banner"`},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out := tc.tmpl()
				for _, want := range tc.wantContain {
					if !strings.Contains(out, want) {
						t.Fatalf("expected %q in rendered %s template", want, tc.name)
					}
				}
			})
		}
	})

	t.Run("demo mode disabled hides demo-banner and demo credentials", func(t *testing.T) {
		tmpls, err := ParseTemplatesWithDemo(false)
		if err != nil {
			t.Fatalf("ParseTemplatesWithDemo(false): %v", err)
		}

		for _, tc := range []struct {
			name     string
			render   func() string
			forbidIn []string
		}{
			{
				name: "login",
				render: func() string {
					var buf bytes.Buffer
					if err := tmpls.Login.ExecuteTemplate(&buf, "root_template", authPageData{}); err != nil {
						t.Fatalf("execute login: %v", err)
					}
					return buf.String()
				},
				forbidIn: []string{`class="demo-banner"`, "demo-customer"},
			},
			{
				name: "signup",
				render: func() string {
					var buf bytes.Buffer
					if err := tmpls.Signup.ExecuteTemplate(&buf, "root_template", authPageData{}); err != nil {
						t.Fatalf("execute signup: %v", err)
					}
					return buf.String()
				},
				forbidIn: []string{`class="demo-banner"`},
			},
			{
				name: "partner login",
				render: func() string {
					var buf bytes.Buffer
					if err := tmpls.AdminLogin.ExecuteTemplate(&buf, "root_template", authPageData{Title: "Partner Login"}); err != nil {
						t.Fatalf("execute partner login: %v", err)
					}
					return buf.String()
				},
				forbidIn: []string{`class="demo-banner"`, "demo-partner"},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out := tc.render()
				for _, forbid := range tc.forbidIn {
					if strings.Contains(out, forbid) {
						t.Fatalf("unexpected %q in rendered %s template when demo mode is off", forbid, tc.name)
					}
				}
			})
		}
	})
}

func TestFontPreloadUsesAssetHelper(t *testing.T) {
	tmpls, err := ParseTemplatesWithDemo(false)
	if err != nil {
		t.Fatalf("ParseTemplatesWithDemo(false): %v", err)
	}

	var buf bytes.Buffer
	if err := tmpls.Login.ExecuteTemplate(&buf, "root_template", struct {
		Title                string
		CSRFToken            string
		Error                string
		Username             string
		Email                string
		Next                 string
		SetupRequired        bool
		PasswordResetEnabled bool
	}{}); err != nil {
		t.Fatalf("execute login: %v", err)
	}

	out := buf.String()
	want := "/static/fonts/jetbrains-mono-latin-wght.woff2?v="
	if !strings.Contains(out, want) {
		t.Fatalf("expected cache-busted font preload %q in rendered HTML", want)
	}
}

func TestAccountAPITokensTemplateRendersTokensAndNewToken(t *testing.T) {
	tmpls, err := ParseTemplatesWithDemo(false)
	if err != nil {
		t.Fatalf("ParseTemplatesWithDemo(false): %v", err)
	}

	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	exp := now.Add(30 * 24 * time.Hour)
	rev := now.Add(-1 * time.Hour)

	type tokenView struct {
		ID          int64
		Name        string
		TokenPrefix string
		ClientType  string
		LastUsedAt  *time.Time
		ExpiresAt   *time.Time
		RevokedAt   *time.Time
		CreatedAt   time.Time
		IsRevoked   bool
		IsExpired   bool
	}
	type viewer struct {
		SignedIn bool
		Username string
		Role     string
	}
	type flash struct {
		Kind    string
		Message string
	}
	type pager struct {
		Page       int
		TotalPages int
		HasPrev    bool
		HasNext    bool
		PrevURL    string
		NextURL    string
	}

	data := struct {
		Title           string
		Flash           flash
		Orders          []any
		Bookings        []any
		APITokens       []tokenView
		NewAPIToken     string
		Pager           pager
		Viewer          viewer
		ProfileUsername string
		ProfileEmail    string
		CSRFToken       string
	}{
		Title:       "Your Account",
		NewAPIToken: "smk_once_secret_token_1234",
		Viewer:      viewer{SignedIn: true, Username: "alice", Role: "user"},
		CSRFToken:   "csrf-123",
		APITokens: []tokenView{
			{
				ID:          1,
				Name:        "shopping-agent",
				TokenPrefix: "smk_abc12345",
				ClientType:  "agent",
				LastUsedAt:  &now,
				ExpiresAt:   &exp,
				CreatedAt:   now,
			},
			{
				ID:          2,
				Name:        "old-cli",
				TokenPrefix: "smk_old99999",
				ClientType:  "cli",
				RevokedAt:   &rev,
				CreatedAt:   now,
				IsRevoked:   true,
			},
		},
	}

	var buf bytes.Buffer
	if err := tmpls.Account.ExecuteTemplate(&buf, "root_template", data); err != nil {
		t.Fatalf("execute account template: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`id="api-tokens-title"`,
		"API Tokens",
		"smk_once_secret_token_1234",
		"shopping-agent",
		"smk_abc12345",
		"old-cli",
		"smk_old99999",
		"/account/tokens",
		"/account/tokens/1/revoke",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in rendered account template", want)
		}
	}
}

func TestAdminReportsTemplateRendersTrafficAndAgents(t *testing.T) {
	tmpls, err := ParseTemplatesWithDemo(false)
	if err != nil {
		t.Fatalf("ParseTemplatesWithDemo(false): %v", err)
	}

	type trafficRow struct {
		ClientKind       string
		Surface          string
		RequestCount     int64
		Error4xxCount    int64
		Error5xxCount    int64
		RateLimitedCount int64
	}
	type trafficSummary struct {
		Last24h         []trafficRow
		Last7d          []trafficRow
		ActiveAPITokens int
	}

	data := struct {
		Title          string
		CSRFToken      string
		Role           string
		ProductCount   int
		SessionCount   int
		AdminCount     int
		StaffCount     int
		UserCount      int
		RevenueTotal   money.Cents
		OrderCount     int
		PendingCount   int
		ShippedCount   int
		DeliveredCount int
		CancelledCount int
		Traffic        trafficSummary
	}{
		Title:     "Sales Reports",
		CSRFToken: "csrf-admin",
		Role:      "admin",
		Traffic: trafficSummary{
			ActiveAPITokens: 7,
			Last24h: []trafficRow{
				{ClientKind: "agent_token", Surface: "api", RequestCount: 42, Error4xxCount: 3, Error5xxCount: 0, RateLimitedCount: 1},
			},
			Last7d: []trafficRow{
				{ClientKind: "browser", Surface: "storefront", RequestCount: 310, Error4xxCount: 4, Error5xxCount: 1, RateLimitedCount: 0},
			},
		},
	}

	var buf bytes.Buffer
	if err := tmpls.Admin.ExecuteTemplate(&buf, "root_template", data); err != nil {
		t.Fatalf("execute admin template: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`id="traffic-agents-title"`,
		"Active API tokens",
		"agent_token",
		"storefront",
		"310",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in rendered admin reports template", want)
		}
	}
}
