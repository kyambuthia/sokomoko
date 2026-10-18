package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func defaultClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

// doJSON sends body as JSON (when non-nil) and decodes a JSON response into
// out. It returns the HTTP status so providers can map their error formats.
func doJSON(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrProviderFailure, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("%w: read response: %v", ErrProviderFailure, err)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("%w: decode response (status %d): %v", ErrProviderFailure, resp.StatusCode, err)
		}
	}
	return resp.StatusCode, nil
}
