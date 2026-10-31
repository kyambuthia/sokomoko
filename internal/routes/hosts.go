package routes

import (
	"net"
	"net/http"
	"strings"
)

// Workspace host prefixes. The host router dispatches on these, so a sibling
// host can be derived from the current one without extra configuration.
const (
	adminHostPrefix   = "admin."
	partnerHostPrefix = "partner."
	courierHostPrefix = "courier."
)

// siblingURL returns an absolute URL for path on the host that shares the
// current request's domain but carries prefix ("" for the storefront).
// For admin.shop.example:8080 and prefix "partner." it yields
// partner.shop.example:8080.
func siblingURL(r *http.Request, prefix, path string) string {
	return siblingURLWithProxyTrust(r, prefix, path, false)
}

func siblingURLWithProxyTrust(r *http.Request, prefix, path string, trustProxyHeaders bool) string {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, port = r.Host, ""
	}
	host = strings.ToLower(host)
	for _, p := range []string{adminHostPrefix, partnerHostPrefix, courierHostPrefix} {
		if strings.HasPrefix(host, p) {
			host = strings.TrimPrefix(host, p)
			break
		}
	}
	host = prefix + host
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	scheme := "http"
	if r.TLS != nil || (trustProxyHeaders && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")) {
		scheme = "https"
	}
	return scheme + "://" + host + path
}

// redirectToSibling sends GET and HEAD requests to path on a sibling host.
func redirectToSibling(prefix, path string) http.HandlerFunc {
	return redirectToSiblingWithProxyTrust(prefix, path, false)
}

func redirectToSiblingWithProxyTrust(prefix, path string, trustProxyHeaders bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		http.Redirect(w, r, siblingURLWithProxyTrust(r, prefix, path, trustProxyHeaders), http.StatusFound)
	}
}
