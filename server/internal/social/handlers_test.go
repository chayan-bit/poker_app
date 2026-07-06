package social

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func authAs(playerID string) AuthFunc {
	return func(*http.Request) (string, error) { return playerID, nil }
}

func failAuth(*http.Request) (string, error) { return "", errors.New("no token") }

func names(m map[string]string) func(string) string {
	return func(id string) string { return m[id] }
}

func newTestServer(store FriendStore, presence *PresenceTracker, auth AuthFunc, resolveName func(string) string) *httptest.Server {
	mux := http.NewServeMux()
	New(store, presence, auth, resolveName).Register(mux)
	return httptest.NewServer(mux)
}

func postJSON(t *testing.T, srv *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func getJSON(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestEndpoints_RequireAuth(t *testing.T) {
	srv := newTestServer(NewMemFriendStore(), NewPresenceTracker(), failAuth, nil)
	defer srv.Close()

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/friends/request"},
		{http.MethodPost, "/api/friends/accept"},
		{http.MethodPost, "/api/friends/decline"},
		{http.MethodPost, "/api/friends/remove"},
		{http.MethodGet, "/api/friends"},
		{http.MethodGet, "/api/friends/pending"},
		{http.MethodGet, "/api/friends/bob/table"},
	}
	for _, c := range cases {
		var resp *http.Response
		if c.method == http.MethodGet {
			resp = getJSON(t, srv, c.path)
		} else {
			resp = postJSON(t, srv, c.path, map[string]string{"playerId": "bob"})
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without auth = %d, want 401", c.method, c.path, resp.StatusCode)
		}
	}
}

func TestRequestAcceptFlow(t *testing.T) {
	store := NewMemFriendStore()
	presence := NewPresenceTracker()
	srv := newTestServer(store, presence, authAs("alice"), names(map[string]string{"bob": "Bob"}))
	defer srv.Close()

	resp := postJSON(t, srv, "/api/friends/request", map[string]string{"playerId": "bob"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("request status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Bob sees the pending request.
	srvBob := newTestServer(store, presence, authAs("bob"), names(map[string]string{"alice": "Alice"}))
	defer srvBob.Close()
	pendResp := getJSON(t, srvBob, "/api/friends/pending")
	pend := decode[[]pendingEntry](t, pendResp)
	if len(pend) != 1 || pend[0].PlayerID != "alice" || pend[0].Name != "Alice" {
		t.Fatalf("pending = %+v, want one entry from alice named Alice", pend)
	}

	acceptResp := postJSON(t, srvBob, "/api/friends/accept", map[string]string{"playerId": "alice"})
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("accept status = %d, want 200", acceptResp.StatusCode)
	}
	acceptResp.Body.Close()

	listResp := getJSON(t, srv, "/api/friends")
	list := decode[[]friendEntry](t, listResp)
	if len(list) != 1 || list[0].PlayerID != "bob" || list[0].Name != "Bob" {
		t.Fatalf("friends list = %+v, want one entry for bob named Bob", list)
	}
}

func TestDeclineFlow(t *testing.T) {
	store := NewMemFriendStore()
	presence := NewPresenceTracker()
	srv := newTestServer(store, presence, authAs("alice"), nil)
	defer srv.Close()
	srvBob := newTestServer(store, presence, authAs("bob"), nil)
	defer srvBob.Close()

	postJSON(t, srv, "/api/friends/request", map[string]string{"playerId": "bob"}).Body.Close()
	resp := postJSON(t, srvBob, "/api/friends/decline", map[string]string{"playerId": "alice"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("decline status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	list := decode[[]friendEntry](t, getJSON(t, srv, "/api/friends"))
	if len(list) != 0 {
		t.Fatalf("friends list after decline = %+v, want empty", list)
	}
}

func TestRemoveFlow(t *testing.T) {
	store := NewMemFriendStore()
	presence := NewPresenceTracker()
	srv := newTestServer(store, presence, authAs("alice"), nil)
	defer srv.Close()
	srvBob := newTestServer(store, presence, authAs("bob"), nil)
	defer srvBob.Close()

	postJSON(t, srv, "/api/friends/request", map[string]string{"playerId": "bob"}).Body.Close()
	postJSON(t, srvBob, "/api/friends/accept", map[string]string{"playerId": "alice"}).Body.Close()

	resp := postJSON(t, srv, "/api/friends/remove", map[string]string{"playerId": "bob"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remove status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	list := decode[[]friendEntry](t, getJSON(t, srv, "/api/friends"))
	if len(list) != 0 {
		t.Fatalf("friends list after remove = %+v, want empty", list)
	}
}

func TestFriendTable_Forbidden(t *testing.T) {
	store := NewMemFriendStore()
	presence := NewPresenceTracker()
	presence.SetTable("bob", "t1")
	srv := newTestServer(store, presence, authAs("alice"), nil)
	defer srv.Close()

	resp := getJSON(t, srv, "/api/friends/bob/table")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("friendTable for non-friend = %d, want 403", resp.StatusCode)
	}
}

func TestFriendTable_NotFoundWhenOfflineOrNoTable(t *testing.T) {
	store := NewMemFriendStore()
	presence := NewPresenceTracker()
	srv := newTestServer(store, presence, authAs("alice"), nil)
	defer srv.Close()
	srvBob := newTestServer(store, presence, authAs("bob"), nil)
	defer srvBob.Close()

	postJSON(t, srv, "/api/friends/request", map[string]string{"playerId": "bob"}).Body.Close()
	postJSON(t, srvBob, "/api/friends/accept", map[string]string{"playerId": "alice"}).Body.Close()

	// bob is offline (default) - expect 404.
	resp := getJSON(t, srv, "/api/friends/bob/table")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("friendTable for offline friend = %d, want 404", resp.StatusCode)
	}

	// bob is in the lobby (no table) - still 404.
	presence.SetLobby("bob")
	resp2 := getJSON(t, srv, "/api/friends/bob/table")
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("friendTable for lobby friend = %d, want 404", resp2.StatusCode)
	}
}

func TestFriendTable_HappyPath(t *testing.T) {
	store := NewMemFriendStore()
	presence := NewPresenceTracker()
	srv := newTestServer(store, presence, authAs("alice"), nil)
	defer srv.Close()
	srvBob := newTestServer(store, presence, authAs("bob"), nil)
	defer srvBob.Close()

	postJSON(t, srv, "/api/friends/request", map[string]string{"playerId": "bob"}).Body.Close()
	postJSON(t, srvBob, "/api/friends/accept", map[string]string{"playerId": "alice"}).Body.Close()
	presence.SetTable("bob", "table-42")

	resp := getJSON(t, srv, "/api/friends/bob/table")
	body := decode[friendTableResponse](t, resp)
	if body.TableID != "table-42" {
		t.Fatalf("friendTable = %+v, want tableId table-42", body)
	}
}

func TestRequest_InvalidBody(t *testing.T) {
	srv := newTestServer(NewMemFriendStore(), NewPresenceTracker(), authAs("alice"), nil)
	defer srv.Close()

	resp := postJSON(t, srv, "/api/friends/request", map[string]string{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("request with empty playerId = %d, want 400", resp.StatusCode)
	}
}

func TestRequest_SelfAndDuplicateErrors(t *testing.T) {
	store := NewMemFriendStore()
	srv := newTestServer(store, NewPresenceTracker(), authAs("alice"), nil)
	defer srv.Close()

	resp := postJSON(t, srv, "/api/friends/request", map[string]string{"playerId": "alice"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("self request = %d, want 400", resp.StatusCode)
	}

	postJSON(t, srv, "/api/friends/request", map[string]string{"playerId": "bob"}).Body.Close()
	dup := postJSON(t, srv, "/api/friends/request", map[string]string{"playerId": "bob"})
	defer dup.Body.Close()
	if dup.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate request = %d, want 409", dup.StatusCode)
	}
}
