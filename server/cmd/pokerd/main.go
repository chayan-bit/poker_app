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
	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/handsapi"
	"github.com/chayan-bit/poker_app/server/internal/history"
	"github.com/chayan-bit/poker_app/server/internal/lobby"
	"github.com/chayan-bit/poker_app/server/internal/postgres"
	"github.com/chayan-bit/poker_app/server/internal/social"
	"github.com/chayan-bit/poker_app/server/internal/table"
	"github.com/chayan-bit/poker_app/server/internal/tourney"
	"github.com/chayan-bit/poker_app/server/internal/ws"
)

func main() {
	addr := envOr("POKERD_ADDR", ":8080")

	authStore, econStore, histStore := buildStores()

	ledger := economy.NewLedger(econStore, time.Now)
	reg := table.NewRegistryWithDeps(table.Deps{Ledger: ledger, History: histStore})
	authn := auth.NewAuthenticator(authSecret(), authStore)
	gw := &ws.Gateway{
		Reg:            reg,
		Auth:           wsAuth(authn),
		AllowedOrigins: ws.OriginsFromEnv(os.Getenv("POKERD_ALLOWED_ORIGINS")),
		OnJoin:         social.Presence.SetTable,
		OnDisconnect:   social.Presence.SetOffline,
	}
	sngMgr := tourney.NewManager(ledger, reg)
	lob := lobby.New(reg, authn.FromRequest).WithSNG(sngMgr)
	hands := handsapi.New(histStore, authn.FromRequest)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/ws", gw.Handler())
	mux.Handle("/api/auth/guest", authn.GuestHandler())
	mux.Handle("/api/auth/upgrade", authn.UpgradeHandler())
	mux.Handle("/api/tables", lob.ListTables())
	mux.Handle("/api/rooms", lob.CreateRoom())
	mux.Handle("/api/rooms/join", lob.JoinRoom())
	mux.Handle("/api/quickseat", lob.Quickseat())
	mux.Handle("POST /api/sng", lob.CreateSNG())
	mux.Handle("GET /api/sng", lob.ListSNG())
	mux.Handle("POST /api/sng/register", lob.RegisterSNG())
	hands.Register(mux)
	social.New(social.NewMemFriendStore(), social.Presence, authn.FromRequest, nil).Register(mux)

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

// buildStores returns persistent stores when POKERD_DATABASE_URL is set,
// otherwise in-memory ones (dev mode: everything resets on restart).
func buildStores() (auth.Store, economy.Store, history.Store) {
	dsn := os.Getenv("POKERD_DATABASE_URL")
	if dsn == "" {
		log.Print("WARNING: POKERD_DATABASE_URL not set; using in-memory stores. " +
			"Accounts, balances, and hand histories reset on restart.")
		return auth.NewMemStore(), economy.NewMemoryStore(), history.NewMemStore()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := postgres.Connect(ctx, dsn)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("postgres migrate: %v", err)
	}
	log.Print("postgres connected, migrations applied")
	return postgres.NewAuthStore(db), postgres.NewEconomyStore(db), postgres.NewHistoryStore(db)
}

// wsAuth adapts the authenticator for WebSocket handshakes: browsers cannot
// set headers on WS connections, so a ?token= query parameter is accepted and
// promoted to a bearer header before normal verification.
func wsAuth(a *auth.Authenticator) func(*http.Request) (string, error) {
	return func(r *http.Request) (string, error) {
		if tok := r.URL.Query().Get("token"); tok != "" && r.Header.Get("Authorization") == "" {
			r.Header.Set("Authorization", "Bearer "+tok)
		}
		return a.FromRequest(r)
	}
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
