package lobby

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// fakeRegistry is an in-memory stand-in for table.Registry that satisfies the
// lobby.Registry interface, so tests don't depend on the real table package
// (which is being refactored concurrently).
type fakeRegistry struct {
	mu     sync.Mutex
	byID   map[string]*table.Table
	byCode map[string]*table.Table
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{byID: map[string]*table.Table{}, byCode: map[string]*table.Table{}}
}

func (f *fakeRegistry) Create(cfg table.Config) *table.Table {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := table.NewWithDefaults(cfg)
	f.byID[cfg.ID] = t
	if cfg.JoinCode != "" {
		f.byCode[cfg.JoinCode] = t
	}
	return t
}

func (f *fakeRegistry) ByCode(code string) (*table.Table, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byCode[code]
	return t, ok
}

func (f *fakeRegistry) Public() []table.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []table.Config
	for _, t := range f.byID {
		if t.Cfg.Visibility == table.Public {
			out = append(out, t.Cfg)
		}
	}
	return out
}

func okAuth(*http.Request) (string, error) { return "player-1", nil }

func failAuth(*http.Request) (string, error) { return "", errors.New("no token") }

func doRequest(t *testing.T, h http.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) apiError {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v (body=%s)", err, rec.Body.String())
	}
	return env.Error
}

func TestListTables_RequiresAuth(t *testing.T) {
	l := New(newFakeRegistry(), failAuth)
	rec := doRequest(t, l.ListTables(), http.MethodGet, "/api/tables", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestListTables_WrongMethod(t *testing.T) {
	l := New(newFakeRegistry(), okAuth)
	rec := doRequest(t, l.ListTables(), http.MethodPost, "/api/tables", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestListTables_ReturnsPublicTablesOnly(t *testing.T) {
	reg := newFakeRegistry()
	reg.Create(table.Config{ID: "pub-1", Visibility: table.Public, MaxSeats: 6, SmallBlind: 25, BigBlind: 50})
	reg.Create(table.Config{ID: "priv-1", Visibility: table.Private, MaxSeats: 6, SmallBlind: 25, BigBlind: 50, JoinCode: "ABCDEF"})

	l := New(reg, okAuth)
	rec := doRequest(t, l.ListTables(), http.MethodGet, "/api/tables", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out []publicTable
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].TableID != "pub-1" {
		t.Fatalf("expected only pub-1 listed, got %+v", out)
	}
	if out[0].SmallBlind != 25 || out[0].BigBlind != 50 || out[0].MaxSeats != 6 {
		t.Fatalf("unexpected fields: %+v", out[0])
	}
}

func TestCreateRoom_HappyPath(t *testing.T) {
	reg := newFakeRegistry()
	l := New(reg, okAuth)

	rec := doRequest(t, l.CreateRoom(), http.MethodPost, "/api/rooms", createRoomRequest{
		SmallBlind: 50, BigBlind: 100, MaxSeats: 6, Visibility: "private",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out createRoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.TableID == "" {
		t.Fatal("expected non-empty tableId")
	}
	if !isValidJoinCode(out.JoinCode) {
		t.Fatalf("join code %q does not match expected format", out.JoinCode)
	}
	if out.JoinURL != "/t/"+out.JoinCode {
		t.Fatalf("expected joinUrl /t/%s, got %s", out.JoinCode, out.JoinURL)
	}

	got, ok := reg.ByCode(out.JoinCode)
	if !ok || got.Cfg.ID != out.TableID {
		t.Fatalf("registry did not create a table reachable by the returned join code")
	}
}

func TestCreateRoom_RequiresAuth(t *testing.T) {
	l := New(newFakeRegistry(), failAuth)
	rec := doRequest(t, l.CreateRoom(), http.MethodPost, "/api/rooms", createRoomRequest{
		SmallBlind: 50, BigBlind: 100, MaxSeats: 6, Visibility: "private",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreateRoom_ValidationFailures(t *testing.T) {
	cases := []struct {
		name string
		req  createRoomRequest
	}{
		{"zero small blind", createRoomRequest{SmallBlind: 0, BigBlind: 50, MaxSeats: 6, Visibility: "private"}},
		{"negative big blind", createRoomRequest{SmallBlind: 50, BigBlind: -1, MaxSeats: 6, Visibility: "private"}},
		{"big blind not greater", createRoomRequest{SmallBlind: 50, BigBlind: 50, MaxSeats: 6, Visibility: "private"}},
		{"too few seats", createRoomRequest{SmallBlind: 50, BigBlind: 100, MaxSeats: 1, Visibility: "private"}},
		{"too many seats", createRoomRequest{SmallBlind: 50, BigBlind: 100, MaxSeats: 11, Visibility: "private"}},
		{"wrong visibility", createRoomRequest{SmallBlind: 50, BigBlind: 100, MaxSeats: 6, Visibility: "public"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := New(newFakeRegistry(), okAuth)
			rec := doRequest(t, l.CreateRoom(), http.MethodPost, "/api/rooms", tc.req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			apiErr := decodeError(t, rec)
			if apiErr.Code == "" || apiErr.Message == "" {
				t.Fatalf("expected populated error envelope, got %+v", apiErr)
			}
		})
	}
}

func TestJoinRoom_HappyPath(t *testing.T) {
	reg := newFakeRegistry()
	reg.Create(table.Config{ID: "priv-1", Visibility: table.Private, MaxSeats: 6, SmallBlind: 25, BigBlind: 50, JoinCode: "ABCDEF"})

	l := New(reg, okAuth)
	rec := doRequest(t, l.JoinRoom(), http.MethodPost, "/api/rooms/join", joinRoomRequest{Code: "ABCDEF"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out joinRoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.TableID != "priv-1" {
		t.Fatalf("expected priv-1, got %s", out.TableID)
	}
}

func TestJoinRoom_NotFound(t *testing.T) {
	l := New(newFakeRegistry(), okAuth)
	rec := doRequest(t, l.JoinRoom(), http.MethodPost, "/api/rooms/join", joinRoomRequest{Code: "ZZZZZZ"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestJoinRoom_RequiresAuth(t *testing.T) {
	l := New(newFakeRegistry(), failAuth)
	rec := doRequest(t, l.JoinRoom(), http.MethodPost, "/api/rooms/join", joinRoomRequest{Code: "ABCDEF"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestQuickseat_CreatesThenReuses(t *testing.T) {
	reg := newFakeRegistry()
	l := New(reg, okAuth)

	rec1 := doRequest(t, l.Quickseat(), http.MethodPost, "/api/quickseat", quickseatRequest{SmallBlind: 100})
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec1.Code, rec1.Body.String())
	}
	var out1 quickseatResponse
	if err := json.Unmarshal(rec1.Body.Bytes(), &out1); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out1.TableID == "" {
		t.Fatal("expected non-empty tableId")
	}

	rec2 := doRequest(t, l.Quickseat(), http.MethodPost, "/api/quickseat", quickseatRequest{SmallBlind: 100})
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var out2 quickseatResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &out2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out2.TableID != out1.TableID {
		t.Fatalf("expected quickseat to reuse existing table %s, got %s", out1.TableID, out2.TableID)
	}

	if len(reg.Public()) != 1 {
		t.Fatalf("expected exactly 1 public table after two quickseat calls, got %d", len(reg.Public()))
	}
}

func TestQuickseat_InvalidStake(t *testing.T) {
	l := New(newFakeRegistry(), okAuth)
	rec := doRequest(t, l.Quickseat(), http.MethodPost, "/api/quickseat", quickseatRequest{SmallBlind: 30})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestQuickseat_RequiresAuth(t *testing.T) {
	l := New(newFakeRegistry(), failAuth)
	rec := doRequest(t, l.Quickseat(), http.MethodPost, "/api/quickseat", quickseatRequest{SmallBlind: 100})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestQuickseat_TableConfigMatchesSpec(t *testing.T) {
	reg := newFakeRegistry()
	l := New(reg, okAuth)
	doRequest(t, l.Quickseat(), http.MethodPost, "/api/quickseat", quickseatRequest{SmallBlind: 500})

	pub := reg.Public()
	if len(pub) != 1 {
		t.Fatalf("expected 1 public table, got %d", len(pub))
	}
	cfg := pub[0]
	if cfg.MaxSeats != quickseatMaxSeats {
		t.Fatalf("expected maxSeats %d, got %d", quickseatMaxSeats, cfg.MaxSeats)
	}
	if cfg.BigBlind != 2*engine.Chips(500) {
		t.Fatalf("expected bigBlind = 2x smallBlind, got %d", cfg.BigBlind)
	}
	if cfg.Visibility != table.Public {
		t.Fatalf("expected quickseat table to be public")
	}
}

var joinCodeRe = regexp.MustCompile(`^[` + regexp.QuoteMeta(joinCodeAlphabet) + `]{6}$`)

func isValidJoinCode(code string) bool {
	if len(code) != joinCodeLength {
		return false
	}
	return joinCodeRe.MatchString(code) && !strings.ContainsAny(code, "01OI")
}

func TestNewJoinCode_FormatAndAlphabet(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := newJoinCode()
		if err != nil {
			t.Fatalf("newJoinCode: %v", err)
		}
		if !isValidJoinCode(code) {
			t.Fatalf("code %q failed format/alphabet check", code)
		}
		seen[code] = true
	}
	if len(seen) < 190 {
		t.Fatalf("expected high uniqueness across 200 draws, got %d distinct", len(seen))
	}
}
