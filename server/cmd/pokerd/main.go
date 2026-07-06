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
	"strconv"
	"syscall"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/auth"
	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/handsapi"
	"github.com/chayan-bit/poker_app/server/internal/history"
	"github.com/chayan-bit/poker_app/server/internal/httpmw"
	"github.com/chayan-bit/poker_app/server/internal/lobby"
	"github.com/chayan-bit/poker_app/server/internal/postgres"
	"github.com/chayan-bit/poker_app/server/internal/social"
	"github.com/chayan-bit/poker_app/server/internal/table"
	"github.com/chayan-bit/poker_app/server/internal/tourney"
	"github.com/chayan-bit/poker_app/server/internal/ws"
)

// version is the build identifier, stamped at link time via
// -ldflags "-X main.version=<tag>" (see Makefile / Dockerfile). Defaults to
// "dev" for un-stamped local builds. Reported by GET /healthz.
var version = "dev"

// Timeouts and shutdown budgets. Global http.Server ReadTimeout/WriteTimeout are
// deliberately NOT set: they place a hard deadline on the underlying connection
// that would kill long-lived WebSocket sockets after hijack. Instead, REST
// bodies are bounded by size (httpmw.BodyLimit) and duration (http.TimeoutHandler),
// header slowloris by ReadHeaderTimeout, and idle keep-alives by IdleTimeout.
const (
	defaultRequestTimeout = 15 * time.Second
	defaultIdleTimeout    = 60 * time.Second
	defaultShutdownBudget = 15 * time.Second
	tableDrainDeadline    = 5 * time.Second
)

func main() {
	addr := envOr("POKERD_ADDR", ":8080")
	prod := isProduction()

	authStore, econStore, histStore, closeStores := buildStores(prod)

	ledger := economy.NewLedger(econStore, time.Now)
	reg := table.NewRegistryWithDeps(table.Deps{Ledger: ledger, History: histStore})
	reg.SetMaxPerCreator(envInt("POKERD_MAX_TABLES_PER_CREATOR", 10))

	authn := auth.NewAuthenticator(authSecret(prod), authStore)
	gw := &ws.Gateway{
		Reg:            reg,
		Auth:           ws.TokenFromQuery(authn.FromRequest),
		AllowedOrigins: ws.OriginsFromEnv(os.Getenv("POKERD_ALLOWED_ORIGINS")),
		OnJoin:         social.Presence.SetTable,
		OnDisconnect:   social.Presence.SetOffline,
	}
	sngMgr := tourney.NewManager(ledger, reg)
	lob := lobby.New(reg, authn.FromRequest).WithSNG(sngMgr)
	hands := handsapi.New(histStore, authn.FromRequest)

	// REST surface (everything but /ws and /healthz).
	apiMux := http.NewServeMux()
	apiMux.Handle("/api/auth/guest", authn.GuestHandler())
	apiMux.Handle("/api/auth/upgrade", authn.UpgradeHandler())
	apiMux.Handle("/api/tables", lob.ListTables())
	apiMux.Handle("/api/rooms", lob.CreateRoom())
	apiMux.Handle("/api/rooms/join", lob.JoinRoom())
	apiMux.Handle("/api/quickseat", lob.Quickseat())
	apiMux.Handle("POST /api/sng", lob.CreateSNG())
	apiMux.Handle("GET /api/sng", lob.ListSNG())
	apiMux.Handle("POST /api/sng/register", lob.RegisterSNG())
	hands.Register(apiMux)
	social.New(social.NewMemFriendStore(), social.Presence, authn.FromRequest, nil).Register(apiMux)

	// Edge middleware: per-IP rate limiting (strict tier for unauthenticated /
	// resource-creating routes), a per-IP concurrent-WebSocket cap, request-body
	// size caps, and a per-request duration cap.
	trustProxy := envBool("POKERD_TRUST_PROXY", false)
	rl := httpmw.NewIPRateLimiter(httpmw.RateLimitConfig{
		GlobalRPS:      envFloat("POKERD_RATE_RPS", 20),
		GlobalBurst:    envInt("POKERD_RATE_BURST", 40),
		StrictRPS:      envFloat("POKERD_RATE_STRICT_RPS", 1),
		StrictBurst:    envInt("POKERD_RATE_STRICT_BURST", 5),
		StrictPrefixes: []string{"/api/auth", "/api/rooms", "/api/sng"},
		TrustProxy:     trustProxy,
		IdleTTL:        10 * time.Minute,
	})
	wsCap := httpmw.NewWSConnLimiter(envInt("POKERD_MAX_WS_PER_IP", 20), trustProxy)

	maxBody := int64(envInt("POKERD_MAX_BODY_BYTES", 64*1024))
	reqTimeout := envDuration("POKERD_REQUEST_TIMEOUT", defaultRequestTimeout)
	restHandler := httpmw.BodyLimit(maxBody)(
		http.TimeoutHandler(apiMux, reqTimeout, `{"error":{"code":"timeout","message":"request timed out"}}`),
	)

	root := http.NewServeMux()
	root.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":` + strconv.Quote(version) + `}`))
	})
	root.Handle("/ws", rl.Middleware(wsCap.Middleware(gw.Handler())))
	root.Handle("/", rl.Middleware(restHandler))

	srv := &http.Server{
		Addr:              addr,
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       envDuration("POKERD_IDLE_TIMEOUT", defaultIdleTimeout),
	}

	go func() {
		log.Printf("pokerd listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown: stop accepting new work, then drain live tables so
	// seated chips are cashed out and in-flight hands refunded, refund tournament
	// buy-ins, and only then close the durable pool (so all ledger writes flush).
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Print("shutdown signal received; draining")

	ctx, cancel := context.WithTimeout(context.Background(), envDuration("POKERD_SHUTDOWN_TIMEOUT", defaultShutdownBudget))
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("http server shutdown: %v", err)
	}
	reg.DrainAll(tableDrainDeadline)
	sngMgr.Shutdown()
	rl.Close()
	closeStores()
	log.Print("pokerd stopped")
}

// isProduction reports whether pokerd is running in a production environment,
// which flips the missing-config behavior from "warn and use ephemeral dev
// defaults" to "fail fast". Keyed on POKERD_ENV=production.
func isProduction() bool {
	return os.Getenv("POKERD_ENV") == "production"
}

// buildStores returns persistent stores when POKERD_DATABASE_URL is set,
// otherwise in-memory ones (dev mode: everything resets on restart). In
// production a missing database URL is fatal: the server must never silently run
// on ephemeral storage. The returned close func releases the pool (no-op for
// in-memory stores).
func buildStores(prod bool) (auth.Store, economy.Store, history.Store, func()) {
	dsn := os.Getenv("POKERD_DATABASE_URL")
	if dsn == "" {
		if prod {
			log.Fatal("POKERD_DATABASE_URL is required in production (POKERD_ENV=production); refusing to run on in-memory storage")
		}
		log.Print("WARNING: POKERD_DATABASE_URL not set; using in-memory stores. " +
			"Accounts, balances, and hand histories reset on restart.")
		return auth.NewMemStore(), economy.NewMemoryStore(), history.NewMemStore(), func() {}
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
	return postgres.NewAuthStore(db), postgres.NewEconomyStore(db), postgres.NewHistoryStore(db), db.Close
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envInt reads an int env var, falling back to def on empty or malformed input.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("WARNING: %s=%q is not an integer; using default %d", key, v, def)
	}
	return def
}

// envFloat reads a float env var, falling back to def on empty or malformed input.
func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		log.Printf("WARNING: %s=%q is not a number; using default %v", key, v, def)
	}
	return def
}

// envBool reads a bool env var (1/t/true/...), falling back to def.
func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		log.Printf("WARNING: %s=%q is not a boolean; using default %v", key, v, def)
	}
	return def
}

// envDuration reads a Go duration env var (e.g. "15s"), falling back to def.
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("WARNING: %s=%q is not a duration; using default %s", key, v, def)
	}
	return def
}

// authSecret reads POKERD_AUTH_SECRET, or (only outside production) generates a
// random ephemeral secret with a warning so the server still starts in dev. An
// ephemeral secret invalidates existing tokens on every restart, so in
// production a missing secret is fatal.
func authSecret(prod bool) []byte {
	if v := os.Getenv("POKERD_AUTH_SECRET"); v != "" {
		return []byte(v)
	}
	if prod {
		log.Fatal("POKERD_AUTH_SECRET is required in production (POKERD_ENV=production); refusing to run with an ephemeral secret")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatalf("failed to generate ephemeral auth secret: %v", err)
	}
	log.Print("WARNING: POKERD_AUTH_SECRET not set; using a random ephemeral secret. " +
		"All auth tokens will be invalidated on restart. Set POKERD_AUTH_SECRET in production.")
	return secret
}
