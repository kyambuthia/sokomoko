package ui

import (
	"bytes"
	"strings"
	"testing"
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
