// Command pokerd is the poker_app game server: a single lightweight Go binary
// serving the authoritative game engine over WebSockets. No external runtime,
// tiny memory footprint, goroutine-per-connection concurrency.
package main

import (
	"context"
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
	authn := &auth.Authenticator{}
	gw := &ws.Gateway{Reg: reg, Auth: authn.FromRequest}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/ws", gw.Handler())
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
