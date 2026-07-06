package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/chayan-bit/poker_app/server/internal/protocol"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

func stubAuth(playerID string) func(*http.Request) (string, error) {
	return func(r *http.Request) (string, error) {
		return playerID, nil
	}
}

func newTestServer(t *testing.T, gw *Gateway) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dial(t *testing.T, ctx context.Context, srv *httptest.Server, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	opts := &websocket.DialOptions{}
	if origin != "" {
		opts.HTTPHeader = http.Header{"Origin": []string{origin}}
	}
	return websocket.Dial(ctx, wsURL(srv), opts)
}

func TestHandler_RejectsBadOrigin(t *testing.T) {
	gw := &Gateway{
		Reg:            table.NewRegistry(),
		Auth:           stubAuth("p1"),
		AllowedOrigins: []string{"localhost:*"},
	}
	srv := newTestServer(t, gw)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := dial(t, ctx, srv, "http://evil.example.com")
	if err == nil {
		t.Fatal("expected handshake to fail for disallowed origin, got nil error")
	}
}

func TestHandler_AllowsDefaultOriginWhenUnset(t *testing.T) {
	gw := &Gateway{
		Reg:  table.NewRegistry(),
		Auth: stubAuth("p1"),
	}
	srv := newTestServer(t, gw)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// A different port than the server's own, so this exercises the
	// "127.0.0.1:*" wildcard pattern rather than the same-host shortcut.
	c, _, err := dial(t, ctx, srv, "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("expected handshake to succeed with default origins, got: %v", err)
	}
	c.Close(websocket.StatusNormalClosure, "")
}

func TestServe_OversizedMessageCloses(t *testing.T) {
	gw := &Gateway{
		Reg:  table.NewRegistry(),
		Auth: stubAuth("p1"),
	}
	srv := newTestServer(t, gw)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := dial(t, ctx, srv, "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close(websocket.StatusInternalError, "")

	// Build an envelope whose Data payload exceeds maxMessageBytes.
	big := make([]byte, maxMessageBytes+1024)
	for i := range big {
		big[i] = 'a'
	}
	env := protocol.Envelope{V: protocol.ProtocolVersion, Type: "junk", Data: mustJSONString(t, string(big))}
	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := c.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The server should close the connection (SetReadLimit trips) rather
	// than accept the oversized frame; the next read must fail.
	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rcancel()
	_, _, readErr := c.Read(rctx)
	if readErr == nil {
		t.Fatal("expected connection to be closed after oversized message, but read succeeded")
	}
}

func TestServe_RateLimitTriggersErrorThenClose(t *testing.T) {
	gw := &Gateway{
		Reg:  table.NewRegistry(),
		Auth: stubAuth("p1"),
	}
	srv := newTestServer(t, gw)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, _, err := dial(t, ctx, srv, "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close(websocket.StatusInternalError, "")

	// Burst is 20; flood well past it plus the 3-strike close threshold with
	// unknown-table commands so nothing blocks on table submission.
	env := protocol.Envelope{
		V:    protocol.ProtocolVersion,
		Type: protocol.CmdJoinTable,
		Data: mustJSON(t, map[string]string{"tableId": "does-not-exist"}),
	}
	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Read concurrently: a short-deadline Read on coder/websocket tears down
	// the connection on timeout, so a single long-lived reader goroutine is
	// required instead of polling Read with a fresh short-lived context per
	// message.
	type msgOrErr struct {
		env protocol.Envelope
		err error
	}
	msgs := make(chan msgOrErr, 256)
	go func() {
		for {
			_, raw, err := c.Read(ctx)
			if err != nil {
				msgs <- msgOrErr{err: err}
				return
			}
			var got protocol.Envelope
			if err := json.Unmarshal(raw, &got); err != nil {
				continue
			}
			msgs <- msgOrErr{env: got}
		}
	}()

	// Flood past burst (20) and the 3-strike close threshold.
	for i := 0; i < 60; i++ {
		if err := c.Write(ctx, websocket.MessageText, payload); err != nil {
			break
		}
	}

	sawRateLimited := false
	sawClose := false
	timeout := time.After(5 * time.Second)
loop:
	for {
		select {
		case m := <-msgs:
			if m.err != nil {
				sawClose = true
				break loop
			}
			if m.env.Type == protocol.EvError {
				var errEv protocol.ErrorEvent
				_ = json.Unmarshal(m.env.Data, &errEv)
				if errEv.Code == "rate_limited" {
					sawRateLimited = true
				}
			}
		case <-timeout:
			break loop
		}
	}

	if !sawRateLimited {
		t.Fatal("expected at least one rate_limited error event")
	}
	if !sawClose {
		t.Fatal("expected the connection to be closed after repeated rate limit abuse")
	}
}

func TestServe_UnknownVersionRejected(t *testing.T) {
	gw := &Gateway{
		Reg:  table.NewRegistry(),
		Auth: stubAuth("p1"),
	}
	srv := newTestServer(t, gw)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := dial(t, ctx, srv, "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close(websocket.StatusInternalError, "")

	env := protocol.Envelope{V: protocol.ProtocolVersion + 99, Type: protocol.CmdJoinTable}
	payload, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := c.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rcancel()
	_, msg, readErr := c.Read(rctx)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	var got protocol.Envelope
	if err := json.Unmarshal(msg, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != protocol.EvError {
		t.Fatalf("type = %q, want %q", got.Type, protocol.EvError)
	}
	var errEv protocol.ErrorEvent
	if err := json.Unmarshal(got.Data, &errEv); err != nil {
		t.Fatalf("unmarshal error event: %v", err)
	}
	if errEv.Code != "bad_version" {
		t.Fatalf("code = %q, want bad_version", errEv.Code)
	}
}

func TestOriginsFromEnv(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"  ", nil},
		{"localhost:*", []string{"localhost:*"}},
		{"a.com,b.com", []string{"a.com", "b.com"}},
		{" a.com , ,b.com ", []string{"a.com", "b.com"}},
	}
	for _, tc := range cases {
		got := OriginsFromEnv(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("OriginsFromEnv(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("OriginsFromEnv(%q) = %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func mustJSONString(t *testing.T, s string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// errorsAs is a tiny wrapper so we don't need an extra import alias juggling
// act in each call site above.
func errorsAs(err error, target *websocket.CloseError) bool {
	for err != nil {
		if ce, ok := err.(websocket.CloseError); ok {
			*target = ce
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
