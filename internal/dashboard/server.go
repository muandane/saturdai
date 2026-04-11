package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Server is an HTTP server Runnable for the embedded dashboard.
type Server struct {
	Log    logr.Logger
	Client client.Client
	Addr   string
	srv    *http.Server
}

// NewMux builds the root handler (UI + API).
func NewMux(c client.Client) http.Handler {
	fileServer := http.FileServer(http.FS(webAssets))

	mux := http.NewServeMux()
	mux.Handle("GET /dashboard/", http.StripPrefix("/dashboard/", fileServer))
	mux.Handle("GET /dashboard", http.RedirectHandler("/dashboard/", http.StatusMovedPermanently))
	mux.Handle("/api/dashboard/", apiHandler(c))
	return mux
}

// Start implements manager.Runnable.
func (s *Server) Start(ctx context.Context) error {
	if s.Client == nil {
		return fmt.Errorf("dashboard server: nil client")
	}
	mux := NewMux(s.Client)
	s.srv = &http.Server{
		Addr:              s.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.Log.Info("starting dashboard HTTP server", "addr", s.Addr)
		errCh <- s.srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			s.Log.Error(err, "dashboard server shutdown")
			return err
		}
		return nil
	}
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
func (s *Server) NeedLeaderElection() bool {
	return false
}
