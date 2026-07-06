// Package ws is the WebSocket edge: one persistent connection per client, a
// reader goroutine that routes commands to tables, and a writer goroutine that
// drains the per-connection outbound channel. No game logic lives here.
package ws

import (
	"context"
	"encoding/json"
	"net/http"
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

// Handler returns the http.Handler for the /ws endpoint.
func (g *Gateway) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

func (g *Gateway) serve(ctx context.Context, c *websocket.Conn, playerID string) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer c.Close(websocket.StatusNormalClosure, "")

	out := make(chan protocol.Envelope, outboundBuffer)

	// joined tracks every table this connection addressed, so we can notify each
	// of them when the socket closes (disconnect-grace flow, issue #16).
	joined := map[string]*table.Table{}
	defer g.notifyDisconnect(playerID, joined, out)

	// Writer goroutine: single owner of the socket write side.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-out:
				wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
				err := writeJSON(wctx, c, ev)
				wcancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	limiter := rate.NewLimiter(rate.Limit(rateLimitPerSec), rateLimitBurst)
	strikes := 0

	// Reader loop: decode commands, route to the addressed table.
	for {
		var env protocol.Envelope
		if err := readJSON(ctx, c, &env); err != nil {
			return
		}

		if !limiter.Allow() {
			strikes++
			sendError(ctx, out, "rate_limited", "too many commands, slow down")
			if strikes >= rateLimitStrikes {
				c.Close(websocket.StatusPolicyViolation, "rate limit exceeded")
				return
			}
			continue
		}

		if env.V != protocol.ProtocolVersion {
			sendError(ctx, out, "bad_version", "unsupported protocol version")
			continue
		}

		g.route(playerID, env, out, joined)
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

func (g *Gateway) route(playerID string, env protocol.Envelope, out chan<- protocol.Envelope, joined map[string]*table.Table) {
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
	joined[ref.TableID] = t
	t.Submit(table.Command{PlayerID: playerID, Msg: env, Reply: out})
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
