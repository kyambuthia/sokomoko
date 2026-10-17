package lifecycle

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServe_DrainsRequestsAndJoinsMaintenance(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	workerStopped := make(chan struct{})
	gate := &Readiness{Check: func(context.Context) error { return nil }}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	})}
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, server, listener, gate.Drain, func(ctx context.Context) {
			<-ctx.Done()
			close(workerStopped)
		}, time.Second)
	}()
	response := make(chan string, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("http://" + listener.Addr().String())
		if err != nil {
			response <- err.Error()
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		response <- string(body)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request never started")
	}
	cancel()
	select {
	case <-workerStopped:
	case <-time.After(3 * time.Second):
		t.Fatal("maintenance was not cancelled")
	}
	select {
	case err := <-done:
		t.Fatalf("server returned before request drained: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not finish")
	}
	if body := <-response; body != "finished" {
		t.Fatalf("response = %q", body)
	}
	if err := gate.PingContext(context.Background()); err == nil {
		t.Fatal("draining instance remains ready")
	}
}

func TestRun_BindFailureDoesNotStartMaintenance(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	err = Run(context.Background(), &http.Server{Addr: listener.Addr().String()}, nil, func(context.Context) {
		t.Error("maintenance started after bind failure")
	})
	if err == nil {
		t.Fatal("expected bind failure")
	}
}

func TestServe_ListenerFailureStillDrainsAcceptedRequests(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	draining := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "finished")
	})}
	done := make(chan error, 1)
	go func() {
		done <- serve(context.Background(), server, listener, func() { close(draining) }, nil, time.Second)
	}()
	response := make(chan string, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("http://" + listener.Addr().String())
		if err != nil {
			response <- err.Error()
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		response <- string(body)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request never started")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-draining:
	case <-time.After(3 * time.Second):
		t.Fatal("listener failure did not drain")
	}
	select {
	case err := <-done:
		t.Fatalf("returned before request drained: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("error=%v, want closed listener", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not finish")
	}
	if body := <-response; body != "finished" {
		t.Fatalf("response=%q", body)
	}
}

func TestServe_ShutdownDeadlineClosesActiveConnections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	handlerDone := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(handlerDone)
	})}
	done := make(chan error, 1)
	go func() { done <- serve(ctx, server, listener, nil, nil, 20*time.Millisecond) }()
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		if resp, err := client.Get("http://" + listener.Addr().String()); err == nil {
			_ = resp.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request never started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown exceeded deadline")
	}
	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("connection was not closed")
	}
}
