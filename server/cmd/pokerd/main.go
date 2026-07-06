// Command pokerd is the poker_app game server: a single lightweight Go binary
// serving the authoritative game engine over WebSockets. No external runtime,
// tiny memory footprint, goroutine-per-connection concurrency.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/auth"
	"github.com/chayan-bit/poker_app/server/internal/ws"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

func main() {
	addr := envOr("POKERD_ADDR", ":8080")

	reg := table.NewRegistry()
	store := auth.NewMemStore()
	authn := auth.NewAuthenticator(authSecret(), store)
	gw := &ws.Gateway{Reg: reg, Auth: authn.FromRequest}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/ws", gw.Handler())
	mux.Handle("/api/auth/guest", authn.GuestHandler())
	mux.Handle("/api/auth/upgrade", authn.UpgradeHandler())
	// TODO: REST lobby endpoints (list public tables, create private room).

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("pokerd listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Print("pokerd stopped")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// authSecret reads POKERD_AUTH_SECRET, or generates a random ephemeral secret
// (with a warning) so the server still starts in dev. An ephemeral secret
// means every restart invalidates existing tokens/cookies.
func authSecret() []byte {
	if v := os.Getenv("POKERD_AUTH_SECRET"); v != "" {
		return []byte(v)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatalf("failed to generate ephemeral auth secret: %v", err)
	}
	log.Print("WARNING: POKERD_AUTH_SECRET not set; using a random ephemeral secret. " +
		"All auth tokens will be invalidated on restart. Set POKERD_AUTH_SECRET in production.")
	return secret
}
