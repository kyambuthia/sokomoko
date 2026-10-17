// Package lifecycle owns process draining and background work. It has no
// dependency on routes, feature services, or the concrete database store.
package lifecycle

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const DrainTimeout = 30 * time.Second

// Readiness prevents a draining instance from accepting new traffic.
type Readiness struct {
	Check    func(context.Context) error
	draining atomic.Bool
}

func (r *Readiness) Drain() { r.draining.Store(true) }

func (r *Readiness) PingContext(ctx context.Context) error {
	if r.draining.Load() {
		return errors.New("server is draining")
	}
	return r.Check(ctx)
}

// Run binds the listener before starting maintenance. Maintenance must honor
// cancellation; it is joined before returning so its dependencies can be closed.
func Run(ctx context.Context, server *http.Server, drain func(), maintenance func(context.Context)) error {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	return serve(ctx, server, listener, drain, maintenance, DrainTimeout)
}

func serve(ctx context.Context, server *http.Server, listener net.Listener, drain func(), maintenance func(context.Context), timeout time.Duration) error {
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if maintenance != nil {
			maintenance(workerCtx)
		}
	}()
	defer func() { stopWorker(); <-workerDone }()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	select {
	case err := <-serveDone:
		if drain != nil {
			drain()
		}
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		if drain != nil {
			drain()
		}
		stopWorker()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			// Shutdown leaves active connections open when its deadline expires.
			// Force-close them before the caller closes the database pool.
			err = errors.Join(err, server.Close())
		}
		serveErr := <-serveDone
		if !errors.Is(serveErr, http.ErrServerClosed) {
			err = errors.Join(err, serveErr)
		}
		return err
	}
}
