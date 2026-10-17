package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/config"
)

func runHealthcheck(cfg config.Config) error {
	host := "localhost"
	for _, part := range strings.Split(cfg.AllowedHostsRaw, ",") {
		if value := strings.TrimSpace(part); value != "" {
			host = value
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return checkReadiness(ctx, "http://"+net.JoinHostPort("127.0.0.1", cfg.Port)+"/readyz", host)
}

func checkReadiness(ctx context.Context, target, host string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Host = host
	// The local probe must never use a proxy supplied by the environment.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("readiness probe failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness returned status %d", resp.StatusCode)
	}
	return nil
}
