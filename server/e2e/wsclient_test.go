package e2e_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// waitTimeout is the per-event deadline for every waitFor call. The whole test
// must finish well under 30s, so a generous-but-bounded 5s catches a hung stack
// without masking real slowness.
const waitTimeout = 5 * time.Second

// wsClient is a scripted WebSocket test client. A single reader goroutine drains
// the socket into an ordered channel (for in-order consumption via next/waitFor)
// and, independently, into a complete raw-message log (for the hole-card privacy
// scan, which must see every byte the server ever sent this connection).
type wsClient struct {
	t      testing.TB
	name   string
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	ch     chan protocol.Envelope

	mu  sync.Mutex
	raw [][]byte
}

// dialClient opens a WS connection to wsURL authenticated as token and starts
// its reader goroutine. name is used only in failure messages.
func dialClient(t testing.TB, wsURL, token, name string) *wsClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: authHeader(token),
	})
	if err != nil {
		cancel()
		t.Fatalf("%s: dial %s: %v", name, wsURL, err)
	}
	c := &wsClient{
		t:      t,
		name:   name,
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
		ch:     make(chan protocol.Envelope, 256),
	}
	go c.readLoop()
	return c
}

func (c *wsClient) readLoop() {
	for {
		_, b, err := c.conn.Read(c.ctx)
		if err != nil {
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(b, &env); err != nil {
			continue
		}
		c.mu.Lock()
		c.raw = append(c.raw, b)
		c.mu.Unlock()
		select {
		case c.ch <- env:
		case <-c.ctx.Done():
			return
		}
	}
}

// close tears down the connection and its reader goroutine.
func (c *wsClient) close() {
	c.cancel()
	_ = c.conn.Close(websocket.StatusNormalClosure, "bye")
}

// cmd sends an imperative command with data as its Data payload. Callers must
// include a "tableId" in data: the gateway's route() resolves the table from it
// for every command (join_table, resync, sit_down, place_bet alike).
func (c *wsClient) cmd(typ string, data any) {
	c.t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		c.t.Fatalf("%s: marshal %s payload: %v", c.name, typ, err)
	}
	env := protocol.Envelope{V: protocol.ProtocolVersion, Type: typ, Data: raw}
	b, err := json.Marshal(env)
	if err != nil {
		c.t.Fatalf("%s: marshal %s envelope: %v", c.name, typ, err)
	}
	wctx, cancel := context.WithTimeout(c.ctx, waitTimeout)
	defer cancel()
	if err := c.conn.Write(wctx, websocket.MessageText, b); err != nil {
		c.t.Fatalf("%s: write %s: %v", c.name, typ, err)
	}
}

// next returns the next envelope in arrival order, or fails after waitTimeout.
func (c *wsClient) next() protocol.Envelope {
	c.t.Helper()
	select {
	case env := <-c.ch:
		return env
	case <-time.After(waitTimeout):
		c.t.Fatalf("%s: timed out after %s waiting for next event", c.name, waitTimeout)
		return protocol.Envelope{}
	}
}

// waitFor consumes events in order until one of type typ arrives, then returns
// it. On timeout it fails naming the expected type and the types actually seen.
func (c *wsClient) waitFor(typ string) protocol.Envelope {
	c.t.Helper()
	deadline := time.After(waitTimeout)
	var seen []string
	for {
		select {
		case env := <-c.ch:
			if env.Type == typ {
				return env
			}
			seen = append(seen, env.Type)
		case <-deadline:
			c.t.Fatalf("%s: timed out waiting for %q; saw %v", c.name, typ, seen)
			return protocol.Envelope{}
		}
	}
}

// expectNone asserts that no event of type typ arrives within dur. Other events
// are consumed and ignored (the callers use it only at points where the client's
// stream is otherwise expected to be idle).
func (c *wsClient) expectNone(typ string, dur time.Duration) {
	c.t.Helper()
	deadline := time.After(dur)
	for {
		select {
		case env := <-c.ch:
			if env.Type == typ {
				c.t.Fatalf("%s: received unexpected %q event: %s", c.name, typ, string(env.Data))
			}
		case <-deadline:
			return
		}
	}
}

// rawLen returns how many raw messages have been received so far.
func (c *wsClient) rawLen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.raw)
}

// rawWindowUntilShowdown returns copies of every raw message received from index
// start up to (but excluding) the first message of type "showdown". This bounds
// the privacy scan to a single hand's pre-showdown traffic.
func (c *wsClient) rawWindowUntilShowdown(start int) [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out [][]byte
	for i := start; i < len(c.raw); i++ {
		var env protocol.Envelope
		if json.Unmarshal(c.raw[i], &env) == nil && env.Type == protocol.EvShowdown {
			break
		}
		cp := make([]byte, len(c.raw[i]))
		copy(cp, c.raw[i])
		out = append(out, cp)
	}
	return out
}

// decodeData unmarshals an envelope's Data payload into T, failing the test on
// malformed JSON.
func decodeData[T any](t testing.TB, env protocol.Envelope) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("decode %q payload: %v (raw: %s)", env.Type, err, string(env.Data))
	}
	return v
}
