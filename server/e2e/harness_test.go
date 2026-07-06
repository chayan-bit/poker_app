package e2e_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/auth"
	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/history"
	"github.com/chayan-bit/poker_app/server/internal/lobby"
	"github.com/chayan-bit/poker_app/server/internal/table"
	"github.com/chayan-bit/poker_app/server/internal/ws"
)

// harness boots the real HTTP+WS stack in-process, wired exactly like
// cmd/pokerd/main.go: signed guest auth, the lobby REST surface, and /ws backed
// by the real gateway. The registry is built with NewRegistryWithDeps sharing a
// single economy ledger and in-memory history, and a real clock with a 30s turn
// timeout so scripted actions never race the auto-fold timer.
type harness struct {
	ts     *httptest.Server
	wsURL  string
	ledger *economy.Ledger
	hist   history.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	secret := []byte("e2e-test-secret-not-for-production-0123456789")
	authn := auth.NewAuthenticator(secret, auth.NewMemStore())

	ledger := economy.NewLedger(economy.NewMemoryStore(), time.Now)
	hist := history.NewMemStore()
	reg := table.NewRegistryWithDeps(table.Deps{
		Ledger:      ledger,
		History:     hist,
		Now:         time.Now,
		TurnTimeout: 30 * time.Second, // real clock; never fires during the script
	})

	gw := &ws.Gateway{Reg: reg, Auth: authn.FromRequest}
	lob := lobby.New(reg, authn.FromRequest)

	mux := http.NewServeMux()
	mux.Handle("/ws", gw.Handler())
	mux.Handle("/api/auth/guest", authn.GuestHandler())
	mux.Handle("/api/auth/upgrade", authn.UpgradeHandler())
	mux.Handle("/api/tables", lob.ListTables())
	mux.Handle("/api/rooms", lob.CreateRoom())
	mux.Handle("/api/rooms/join", lob.JoinRoom())
	mux.Handle("/api/quickseat", lob.Quickseat())

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return &harness{
		ts:     ts,
		wsURL:  "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws",
		ledger: ledger,
		hist:   hist,
	}
}

func authHeader(token string) http.Header {
	return http.Header{"Authorization": {"Bearer " + token}}
}

// guest issues a fresh guest identity via POST /api/auth/guest.
func (h *harness) guest(t *testing.T) (token, playerID string) {
	t.Helper()
	var out struct {
		Token    string `json:"token"`
		PlayerID string `json:"playerId"`
	}
	h.postJSON(t, "/api/auth/guest", "", nil, &out)
	if out.Token == "" || out.PlayerID == "" {
		t.Fatalf("guest: empty token/playerId in response")
	}
	return out.Token, out.PlayerID
}

// createRoom creates a private room and returns its tableId and join code.
func (h *harness) createRoom(t *testing.T, token string, small, big, maxSeats int) (tableID, code string) {
	t.Helper()
	body := map[string]any{
		"smallBlind": small,
		"bigBlind":   big,
		"maxSeats":   maxSeats,
		"visibility": "private",
	}
	var out struct {
		TableID  string `json:"tableId"`
		JoinCode string `json:"joinCode"`
	}
	h.postJSON(t, "/api/rooms", token, body, &out)
	if out.TableID == "" || out.JoinCode == "" {
		t.Fatalf("createRoom: empty tableId/joinCode")
	}
	return out.TableID, out.JoinCode
}

// joinRoom resolves a join code to a tableId via POST /api/rooms/join.
func (h *harness) joinRoom(t *testing.T, token, code string) string {
	t.Helper()
	var out struct {
		TableID string `json:"tableId"`
	}
	h.postJSON(t, "/api/rooms/join", token, map[string]any{"code": code}, &out)
	if out.TableID == "" {
		t.Fatalf("joinRoom: empty tableId for code %q", code)
	}
	return out.TableID
}

func (h *harness) postJSON(t *testing.T, path, token string, body, out any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body for %s: %v", path, err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, h.ts.URL+path, &buf)
	if err != nil {
		t.Fatalf("new request %s: %v", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: status %d", path, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s response: %v", path, err)
		}
	}
}
