package lobby

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/tourney"
)

// fakeSNG is an in-memory stand-in for tourney.Manager satisfying SNGManager.
type fakeSNG struct {
	created   []tourney.SNGConfig
	registers []struct{ sngID, playerID string }
	registerErr error
	list        []tourney.View
	nextSngID   string
	nextTableID string
}

func (f *fakeSNG) Create(cfg tourney.SNGConfig) (string, string, error) {
	f.created = append(f.created, cfg)
	sid, tid := f.nextSngID, f.nextTableID
	if sid == "" {
		sid = "sng-1"
	}
	if tid == "" {
		tid = "tbl-1"
	}
	return sid, tid, nil
}

func (f *fakeSNG) Register(sngID, playerID string) error {
	f.registers = append(f.registers, struct{ sngID, playerID string }{sngID, playerID})
	return f.registerErr
}

func (f *fakeSNG) List() []tourney.View { return f.list }

func newSNGLobby(m SNGManager, auth AuthFunc) *Lobby {
	return New(newFakeRegistry(), auth).WithSNG(m)
}

func TestCreateSNG_HappyPath(t *testing.T) {
	f := &fakeSNG{nextSngID: "sng-9", nextTableID: "tbl-9"}
	l := newSNGLobby(f, okAuth)

	rec := doRequest(t, l.CreateSNG(), http.MethodPost, "/api/sng", createSNGRequest{Name: "Fri", Seats: 6, BuyIn: 250})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out createSNGResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.SngID != "sng-9" || out.TableID != "tbl-9" {
		t.Fatalf("unexpected ids: %+v", out)
	}
	// Defaults were applied to the config the manager received.
	if len(f.created) != 1 {
		t.Fatalf("expected one create, got %d", len(f.created))
	}
	cfg := f.created[0]
	if cfg.Seats != 6 || cfg.BuyIn != 250 || cfg.StartingStack != tourney.DefaultStartingStack {
		t.Fatalf("config defaults not applied: %+v", cfg)
	}
	if len(cfg.PayoutPct) != 2 { // 6 players -> top two
		t.Fatalf("expected 2 payout places for 6 seats, got %v", cfg.PayoutPct)
	}
}

func TestCreateSNG_RequiresAuth(t *testing.T) {
	l := newSNGLobby(&fakeSNG{}, failAuth)
	rec := doRequest(t, l.CreateSNG(), http.MethodPost, "/api/sng", createSNGRequest{Seats: 6, BuyIn: 250})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateSNG_Validation(t *testing.T) {
	cases := []struct {
		name string
		req  createSNGRequest
	}{
		{"too few seats", createSNGRequest{Seats: 1, BuyIn: 100}},
		{"too many seats", createSNGRequest{Seats: 10, BuyIn: 100}},
		{"non-positive buyin", createSNGRequest{Seats: 6, BuyIn: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := newSNGLobby(&fakeSNG{}, okAuth)
			rec := doRequest(t, l.CreateSNG(), http.MethodPost, "/api/sng", tc.req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", rec.Code)
			}
		})
	}
}

func TestCreateSNG_DisabledWhenNoManager(t *testing.T) {
	l := New(newFakeRegistry(), okAuth) // no WithSNG
	rec := doRequest(t, l.CreateSNG(), http.MethodPost, "/api/sng", createSNGRequest{Seats: 6, BuyIn: 250})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when tournaments disabled, got %d", rec.Code)
	}
}

func TestRegisterSNG_HappyPath(t *testing.T) {
	f := &fakeSNG{}
	l := newSNGLobby(f, okAuth)
	rec := doRequest(t, l.RegisterSNG(), http.MethodPost, "/api/sng/register", registerSNGRequest{SngID: "sng-1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(f.registers) != 1 || f.registers[0].sngID != "sng-1" || f.registers[0].playerID != "player-1" {
		t.Fatalf("register not forwarded with caller identity: %+v", f.registers)
	}
}

func TestRegisterSNG_ErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
		code string
	}{
		{"not found", tourney.ErrNotFound, http.StatusNotFound, "not_found"},
		{"full", tourney.ErrFull, http.StatusConflict, "sng_full"},
		{"already", tourney.ErrAlreadyRegistered, http.StatusConflict, "already_registered"},
		{"insufficient", economy.ErrInsufficientFunds, http.StatusConflict, "insufficient_funds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := newSNGLobby(&fakeSNG{registerErr: tc.err}, okAuth)
			rec := doRequest(t, l.RegisterSNG(), http.MethodPost, "/api/sng/register", registerSNGRequest{SngID: "s"})
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if got := decodeError(t, rec); got.Code != tc.code {
				t.Fatalf("error code = %q, want %q", got.Code, tc.code)
			}
		})
	}
}

func TestRegisterSNG_RequiresAuthAndSngID(t *testing.T) {
	l := newSNGLobby(&fakeSNG{}, failAuth)
	if rec := doRequest(t, l.RegisterSNG(), http.MethodPost, "/api/sng/register", registerSNGRequest{SngID: "s"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	l = newSNGLobby(&fakeSNG{}, okAuth)
	if rec := doRequest(t, l.RegisterSNG(), http.MethodPost, "/api/sng/register", registerSNGRequest{SngID: ""}); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing sngId, got %d", rec.Code)
	}
}

func TestListSNG_ReturnsOpenSNGs(t *testing.T) {
	f := &fakeSNG{list: []tourney.View{{SngID: "s1", Name: "A", Seats: 6, Registered: 2, BuyIn: 100}}}
	l := newSNGLobby(f, okAuth)
	rec := doRequest(t, l.ListSNG(), http.MethodGet, "/api/sng", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var out []tourney.View
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].SngID != "s1" || out[0].Registered != 2 {
		t.Fatalf("unexpected list: %+v", out)
	}
}

func TestListSNG_RequiresAuth(t *testing.T) {
	l := newSNGLobby(&fakeSNG{}, failAuth)
	rec := doRequest(t, l.ListSNG(), http.MethodGet, "/api/sng", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
