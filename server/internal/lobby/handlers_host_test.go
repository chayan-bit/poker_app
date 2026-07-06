package lobby

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateRoom_SetsHostToAuthenticatedCreator(t *testing.T) {
	// Arrange: okAuth resolves every caller to "player-1".
	reg := newFakeRegistry()
	l := New(reg, okAuth)

	// Act.
	rec := doRequest(t, l.CreateRoom(), http.MethodPost, "/api/rooms", createRoomRequest{
		SmallBlind: 25, BigBlind: 50, MaxSeats: 6, Visibility: "private",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out createRoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Assert: the created table's host is the authenticated creator, not left for
	// the first sit_down to claim (issue: createroom-host-race).
	tbl, ok := reg.ByCode(out.JoinCode)
	if !ok {
		t.Fatal("created room not reachable by join code")
	}
	if tbl.Cfg.HostPlayerID != "player-1" {
		t.Fatalf("HostPlayerID = %q, want the authenticated creator player-1", tbl.Cfg.HostPlayerID)
	}
}
