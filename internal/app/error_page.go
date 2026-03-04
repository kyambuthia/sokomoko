package app

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

func requestIDFromResponse(w http.ResponseWriter, r *http.Request) string {
	requestID := strings.TrimSpace(w.Header().Get("X-Request-Id"))
	if requestID == "" && r != nil {
		requestID = strings.TrimSpace(r.Header.Get("X-Request-Id"))
	}
	if requestID == "" {
		requestID = "unavailable"
	}
	return requestID
}

func RenderErrorPage(w http.ResponseWriter, r *http.Request, status int, title string, message string) {
	if status < 400 {
		status = http.StatusInternalServerError
	}

	requestID := requestIDFromResponse(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
</head>
<body>
  <main>
    <h1>%s</h1>
    <p>%s</p>
    <p><a href="/">Return to home</a></p>
    <p><small>Request ID: %s</small></p>
  </main>
</body>
</html>`,
		html.EscapeString(title),
		html.EscapeString(title),
		html.EscapeString(message),
		html.EscapeString(requestID),
	)
}
