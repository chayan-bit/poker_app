// Package ws is the WebSocket edge: one persistent connection per client, a
// reader goroutine that routes commands to tables, and a writer goroutine that
// drains the per-connection outbound channel. No game logic lives here.
package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/time/rate"

	"github.com/chayan-bit/poker_app/server/internal/protocol"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// Gateway upgrades HTTP requests to WebSocket connections and wires them to the
// table registry.
type Gateway struct {
	Reg  *table.Registry
	Auth func(*http.Request) (playerID string, err error) // pluggable, see internal/auth

	// OnJoin/OnDisconnect are optional presence hooks (wired to the social
	// package's tracker in main.go). Called from connection goroutines; the
	// implementations must be thread-safe.
	OnJoin       func(playerID, tableID string)
	OnDisconnect func(playerID string)

	// AllowedOrigins lists the coder/websocket OriginPatterns used to verify
	// the WebSocket handshake's Origin header. If empty, defaults to
	// localhost dev patterns (see defaultAllowedOrigins) - fine for local
	// dev, but production deployments must set this explicitly via
	// OriginsFromEnv(os.Getenv("POKERD_ALLOWED_ORIGINS")).
	AllowedOrigins []string
}

// defaultAllowedOrigins is used when Gateway.AllowedOrigins is empty, so the
// gateway is safe (does not fall back to allow-any-origin) even if the
// orchestrator forgets to wire POKERD_ALLOWED_ORIGINS in a dev environment.
var defaultAllowedOrigins = []string{"localhost:*", "127.0.0.1:*"}

// OriginsFromEnv parses a comma-separated list of origin host patterns (as
// consumed by coder/websocket's AcceptOptions.OriginPatterns) from an env var
// value. Blank entries are dropped. The orchestrator wires this from
// POKERD_ALLOWED_ORIGINS in main.go.
func OriginsFromEnv(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// maxMessageBytes bounds a single client->server WebSocket frame, guarding
// against a malicious or buggy client trying to exhaust memory/CPU decoding
// an oversized payload. Protocol messages are meant to stay under 1 KB
// (see internal/protocol); 4 KB leaves generous headroom.
const maxMessageBytes = 4096

// rateLimitPerSec and rateLimitBurst bound how many commands one connection
// may submit; exceeding it repeatedly gets the connection closed rather than
// left to hammer the table loop.
const (
	rateLimitPerSec  = 10
	rateLimitBurst   = 20
	rateLimitStrikes = 3 // consecutive violations before we close the conn
)

// Liveness: the server pings a silent client periodically. A client that stops
// answering pongs within pingTimeout is treated as dead and its goroutines are
// torn down, so an authenticated-but-silent socket cannot pin a reader+writer
// pair forever. Browsers answer pings at the protocol level automatically.
const (
	pingInterval = 30 * time.Second
	pingTimeout  = 10 * time.Second
)

// TokenFromQuery adapts an Authorization-header verifier (like
// auth.Authenticator.FromRequest) to WebSocket handshakes: browsers cannot set
// headers on a WS connect, so a ?token= query parameter is accepted and promoted
// to a bearer Authorization header before the underlying verifier runs. Existing
// Authorization headers / cookies (non-browser clients) are left untouched.
func TokenFromQuery(verify func(*http.Request) (string, error)) func(*http.Request) (string, error) {
	return func(r *http.Request) (string, error) {
		if tok := r.URL.Query().Get("token"); tok != "" && r.Header.Get("Authorization") == "" {
			r.Header.Set("Authorization", "Bearer "+tok)
		}
		return verify(r)
	}
}

// Handler returns the http.Handler for the /ws endpoint.
func (g *Gateway) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if g.Auth == nil {
			// Fail closed: an unwired verifier must never admit anonymous players.
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		playerID, err := g.Auth(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		origins := g.AllowedOrigins
		if len(origins) == 0 {
			origins = defaultAllowedOrigins
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: origins,
		})
		if err != nil {
			return
		}
		c.SetReadLimit(maxMessageBytes)
		g.serve(r.Context(), c, playerID)
	}
}

// outboundBuffer bounds how far a slow client may fall behind before we drop
// events (it will Seq-gap and resync). Keeps a stalled client from pinning RAM.
const outboundBuffer = 128

// closeInfo is a requested WebSocket close: the writer flushes every queued
// event, then sends the close frame with this code+reason. Routing the close
// through the writer (the single socket-write owner) guarantees a queued error
// event - like rate_limited - is delivered to the client BEFORE the close frame.
type closeInfo struct {
	code   websocket.StatusCode
	reason string
}

func (g *Gateway) serve(ctx context.Context, c *websocket.Conn, playerID string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // eventual: unblocks the pinger and, as a fallback, the writer

	out := make(chan protocol.Envelope, outboundBuffer)
	closeReq := make(chan closeInfo, 1)
	writerDone := make(chan struct{})

	// joined tracks every table this connection addressed, so we can notify each
	// of them when the socket closes (disconnect-grace flow, issue #16).
	joined := map[string]*table.Table{}
	defer g.notifyDisconnect(playerID, joined, out)

	go g.writePump(ctx, cancel, c, out, closeReq, writerDone)
	go g.pingPump(ctx, cancel, c)

	limiter := rate.NewLimiter(rate.Limit(rateLimitPerSec), rateLimitBurst)
	strikes := 0
	closeWith := closeInfo{code: websocket.StatusNormalClosure}

	// Reader loop: decode commands, route to the addressed table.
	for {
		var env protocol.Envelope
		if err := readJSON(ctx, c, &env); err != nil {
			break
		}

		if !limiter.Allow() {
			strikes++
			sendError(ctx, out, "rate_limited", "too many commands, slow down")
			if strikes >= rateLimitStrikes {
				closeWith = closeInfo{code: websocket.StatusPolicyViolation, reason: "rate limit exceeded"}
				break
			}
			continue
		}

		if env.V != protocol.ProtocolVersion {
			sendError(ctx, out, "bad_version", "unsupported protocol version")
			continue
		}

		g.route(ctx, playerID, env, out, joined)
	}

	// Hand the close to the writer so every queued event (including any
	// rate_limited error) is flushed BEFORE the close frame. Then wait for the
	// writer to finish, with a cancel fallback so teardown can never hang.
	select {
	case closeReq <- closeWith:
	default:
	}
	select {
	case <-writerDone:
	case <-time.After(pingTimeout):
		cancel()
		<-writerDone
	}
}

// writePump is the single owner of the socket write side. It drains outbound
// events until the context is cancelled or a close is requested. On a close
// request it first flushes every still-queued event (so the client sees why it
// is being closed) and then sends the close frame. A deferred recover keeps a
// panic here from crashing the process.
func (g *Gateway) writePump(ctx context.Context, cancel context.CancelFunc, c *websocket.Conn, out <-chan protocol.Envelope, closeReq <-chan closeInfo, done chan<- struct{}) {
	defer close(done)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ws writer PANIC for connection: %v\n%s", r, debug.Stack())
			cancel()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			// Normal disconnect: close politely; a prior explicit close is a no-op.
			_ = c.Close(websocket.StatusNormalClosure, "")
			return
		case ci := <-closeReq:
			g.flushThenClose(c, out, ci)
			cancel()
			return
		case ev := <-out:
			if !g.writeOne(ctx, c, ev) {
				cancel()
				return
			}
		}
	}
}

// writeOne writes a single event with a bounded deadline. Returns false on any
// write failure so the caller can tear the connection down.
func (g *Gateway) writeOne(ctx context.Context, c *websocket.Conn, ev protocol.Envelope) bool {
	wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
	err := writeJSON(wctx, c, ev)
	wcancel()
	return err == nil
}

// flushThenClose drains and writes every buffered event, then sends the close
// frame. It uses a fresh background-derived deadline (not the serve context, which
// is about to be cancelled) so the flush+close still completes on teardown.
func (g *Gateway) flushThenClose(c *websocket.Conn, out <-chan protocol.Envelope, ci closeInfo) {
	fctx, fcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fcancel()
	for {
		select {
		case ev := <-out:
			if !g.writeOne(fctx, c, ev) {
				_ = c.Close(ci.code, ci.reason)
				return
			}
		default:
			_ = c.Close(ci.code, ci.reason)
			return
		}
	}
}

// pingPump keeps a server-side liveness check running: it pings the client on
// pingInterval and cancels the connection if a pong does not return within
// pingTimeout, so a silent (or half-open) socket cannot pin goroutines forever.
// coder/websocket requires Ping to run concurrently with the reader (it does).
func (g *Gateway) pingPump(ctx context.Context, cancel context.CancelFunc, c *websocket.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ws pinger PANIC for connection: %v\n%s", r, debug.Stack())
			cancel()
		}
	}()
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, pcancel := context.WithTimeout(ctx, pingTimeout)
			err := c.Ping(pctx)
			pcancel()
			if err != nil {
				cancel()
				return
			}
		}
	}
}

// notifyDisconnect submits an internal CmdDisconnected to every table this
// connection had addressed, so each can start the seat's disconnect-grace
// window. Called once from serve's defer when the socket closes.
func (g *Gateway) notifyDisconnect(playerID string, joined map[string]*table.Table, out chan<- protocol.Envelope) {
	for id, t := range joined {
		data, err := json.Marshal(struct {
			TableID string `json:"tableId"`
		}{id})
		if err != nil {
			continue
		}
		env := protocol.Envelope{V: protocol.ProtocolVersion, Type: protocol.CmdDisconnected, Data: data}
		t.Submit(table.Command{PlayerID: playerID, Msg: env, Reply: out})
	}
	if g.OnDisconnect != nil {
		g.OnDisconnect(playerID)
	}
}

// sendError enqueues a non-fatal protocol error event for the client. It
// never blocks: if the outbound buffer is full the event is dropped, since a
// stalled writer will already be tearing the connection down.
func sendError(ctx context.Context, out chan<- protocol.Envelope, code, message string) {
	data, err := json.Marshal(protocol.ErrorEvent{Code: code, Message: message})
	if err != nil {
		return
	}
	env := protocol.Envelope{V: protocol.ProtocolVersion, Type: protocol.EvError, Data: data}
	select {
	case out <- env:
	case <-ctx.Done():
	default:
	}
}

func (g *Gateway) route(ctx context.Context, playerID string, env protocol.Envelope, out chan<- protocol.Envelope, joined map[string]*table.Table) {
	// Minimal routing: bet/join carry a tableId in Data; resolve and Submit.
	var ref struct {
		TableID string `json:"tableId"`
	}
	_ = json.Unmarshal(env.Data, &ref)
	t, ok := g.Reg.Get(ref.TableID)
	if !ok {
		return
	}
	// Remember this table so the socket close can notify it (issue #16).
	if _, seen := joined[ref.TableID]; !seen && g.OnJoin != nil {
		g.OnJoin(playerID, ref.TableID)
	}
	joined[ref.TableID] = t
	// Submit returns false when the table's loop has stopped (idle/drain
	// shutdown) between our Get and here; tell the client the table is gone
	// rather than silently dropping the command.
	if !t.Submit(table.Command{PlayerID: playerID, Msg: env, Reply: out}) {
		delete(joined, ref.TableID)
		sendError(ctx, out, "table_gone", "that table is no longer available")
	}
}

func writeJSON(ctx context.Context, c *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Write(ctx, websocket.MessageText, b)
}

func readJSON(ctx context.Context, c *websocket.Conn, v any) error {
	_, b, err := c.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
