package routes

import (
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
)

func publicBaseURL(a *app.App, r *http.Request) string {
	base := a.PublicBase(r)
	if base == "" && r.Host != "" {
		scheme := "http"
		if r.TLS != nil || a.Production {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return strings.TrimRight(base, "/")
}

func writeTextPublic(w http.ResponseWriter, r *http.Request, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(body))
	}
}

// RobotsTxt serves crawler directives and advertises the XML sitemap.
func RobotsTxt(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		base := publicBaseURL(a, r)
		var b strings.Builder
		b.WriteString("User-agent: *\n")
		b.WriteString("Allow: /\n")
		b.WriteString("Allow: /llms.txt\n")
		b.WriteString("Allow: /api/v1/openapi.json\n")
		b.WriteString("Allow: /api/v1/catalog/\n")
		b.WriteString("Disallow: /account\n")
		b.WriteString("Disallow: /cart\n")
		b.WriteString("Disallow: /checkout\n")
		b.WriteString("Disallow: /login\n")
		b.WriteString("Disallow: /signup\n")
		b.WriteString("Disallow: /messages\n")
		b.WriteString("Disallow: /api/v1/account\n")
		b.WriteString("Disallow: /api/v1/cart\n")
		b.WriteString("Disallow: /api/v1/checkout\n")
		b.WriteString("Disallow: /api/v1/me\n")
		b.WriteString("Disallow: /api/v1/admin\n")
		b.WriteString("Disallow: /api/v1/partner\n")
		b.WriteString("Disallow: /api/v1/courier\n")
		if base != "" {
			b.WriteString("Sitemap: " + base + "/sitemap.xml\n")
		} else {
			b.WriteString("Sitemap: /sitemap.xml\n")
		}
		writeTextPublic(w, r, b.String())
	}
}

// LLMsTxt serves a concise machine-readable guide for AI agents and API clients.
func LLMsTxt(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		base := publicBaseURL(a, r)
		prefix := base
		var b strings.Builder
		b.WriteString("# Sokomoko\n\n")
		b.WriteString("> Server-rendered marketplace with a JSON API under `/api/v1`.\n\n")
		b.WriteString("## Machine Discovery\n\n")
		b.WriteString("- [OpenAPI 3.1 Spec](" + prefix + "/api/v1/openapi.json)\n")
		b.WriteString("- [Sitemap](" + prefix + "/sitemap.xml)\n\n")
		b.WriteString("## Authentication\n\n")
		b.WriteString("- Register (`POST /api/v1/auth/signup`) and sign in (`POST /api/v1/auth/login`) to obtain a session cookie and CSRF token.\n")
		b.WriteString("- Create a long-lived API token via `POST /api/v1/me/tokens` (or `/account` in the web UI).\n")
		b.WriteString("- Send `Authorization: Bearer smk_...` on `/api/v1/...` requests; `X-CSRF-Token` is not required with Bearer auth.\n\n")
		b.WriteString("## Public Catalog Endpoints\n\n")
		b.WriteString("- `GET /api/v1/catalog/categories`\n")
		b.WriteString("- `GET /api/v1/catalog/products?page=1&limit=20`\n")
		b.WriteString("- `GET /api/v1/catalog/products/{slug}`\n")
		b.WriteString("- `GET /api/v1/catalog/search?q=&category=`\n")
		b.WriteString("- `GET /api/v1/catalog/stores/{slug}`\n")
		b.WriteString("- `GET /api/v1/catalog/services`\n")
		b.WriteString("- `GET /api/v1/catalog/services/{slug}/availability?date=YYYY-MM-DD`\n\n")
		b.WriteString("## Customer Workflows (Bearer Auth)\n\n")
		b.WriteString("- Cart: `GET /api/v1/cart`, `POST /api/v1/cart/items`, `PATCH /api/v1/cart/items/{product_id}`, `DELETE /api/v1/cart/items/{product_id}`\n")
		b.WriteString("- Checkout: `POST /api/v1/checkout` -> `POST /api/v1/checkout/{token}/place-order` (`delivery_address`, `payment_method`)\n")
		b.WriteString("- Orders: `GET /api/v1/account/orders`, `GET /api/v1/account/orders/{id}`, `POST /api/v1/account/orders/{id}/cancel`\n")
		b.WriteString("- Bookings: `POST /api/v1/services/{slug}/bookings`, `GET /api/v1/account/service-bookings`, `POST /api/v1/account/service-bookings/{id}/cancel?version=`\n")
		writeTextPublic(w, r, b.String())
	}
}
