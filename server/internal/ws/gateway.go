// Package ws is the WebSocket edge: one persistent connection per client, a
// reader goroutine that routes commands to tables, and a writer goroutine that
// drains the per-connection outbound channel. No game logic lives here.
package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/chayan-bit/poker_app/server/internal/protocol"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// Gateway upgrades HTTP requests to WebSocket connections and wires them to the
// table registry.
type Gateway struct {
	Reg  *table.Registry
	Auth func(*http.Request) (playerID string, err error) // pluggable, see internal/auth
}

// Handler returns the http.Handler for the /ws endpoint.
func (g *Gateway) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		playerID, err := g.Auth(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			// Tighten OriginPatterns in production.
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
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

	// Reader loop: decode commands, route to the addressed table.
	for {
		var env protocol.Envelope
		if err := readJSON(ctx, c, &env); err != nil {
			return
		}
		g.route(playerID, env, out)
	}
}

func (g *Gateway) route(playerID string, env protocol.Envelope, out chan<- protocol.Envelope) {
	// Minimal routing: bet/join carry a tableId in Data; resolve and Submit.
	var ref struct {
		TableID string `json:"tableId"`
	}
	_ = json.Unmarshal(env.Data, &ref)
	t, ok := g.Reg.Get(ref.TableID)
	if !ok {
		return
	}
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
