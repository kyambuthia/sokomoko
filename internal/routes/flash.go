package routes

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
)

const flashCookieName = "flash"

// Flash is a one-time message shown on the next page view. It replaces
// passing messages through query strings, which let anyone craft links that
// display arbitrary text on the site.
type Flash struct {
	Kind    string `json:"k"` // "success" or "danger"
	Message string `json:"m"`
}

func setFlash(w http.ResponseWriter, kind, message string) {
	payload, err := json.Marshal(Flash{Kind: kind, Message: message})
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    base64.RawURLEncoding.EncodeToString(payload),
		Path:     "/",
		MaxAge:   60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// popFlash reads and clears the pending flash message, if any.
func popFlash(w http.ResponseWriter, r *http.Request) Flash {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil || cookie.Value == "" {
		return Flash{}
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})

	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return Flash{}
	}
	var flash Flash
	if json.Unmarshal(raw, &flash) != nil || len(flash.Message) > 500 {
		return Flash{}
	}
	if flash.Kind != "success" && flash.Kind != "danger" {
		flash.Kind = "info"
	}
	return flash
}

func redirectWithFlash(w http.ResponseWriter, r *http.Request, target, kind, message string) {
	setFlash(w, kind, message)
	http.Redirect(w, r, target, http.StatusFound)
}

func urlQueryEscape(s string) string { return url.QueryEscape(s) }
