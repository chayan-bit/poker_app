package social

import "testing"

func TestPresenceDefaultOffline(t *testing.T) {
	p := NewPresenceTracker()
	st := p.Get("unknown")
	if st.State != StateOffline || st.TableID != "" {
		t.Fatalf("Get(unseen player) = %+v, want offline with no table", st)
	}
}

func TestPresenceTransitions(t *testing.T) {
	p := NewPresenceTracker()

	p.SetLobby("alice")
	if st := p.Get("alice"); st.State != StateLobby || st.TableID != "" {
		t.Fatalf("after SetLobby: %+v, want {lobby, \"\"}", st)
	}

	p.SetTable("alice", "t1")
	if st := p.Get("alice"); st.State != StateTable || st.TableID != "t1" {
		t.Fatalf("after SetTable: %+v, want {table, t1}", st)
	}

	p.SetOffline("alice")
	if st := p.Get("alice"); st.State != StateOffline || st.TableID != "" {
		t.Fatalf("after SetOffline: %+v, want {offline, \"\"}", st)
	}
}

func TestPresenceIndependentPerPlayer(t *testing.T) {
	p := NewPresenceTracker()
	p.SetTable("alice", "t1")
	p.SetLobby("bob")

	if st := p.Get("alice"); st.State != StateTable || st.TableID != "t1" {
		t.Fatalf("alice = %+v", st)
	}
	if st := p.Get("bob"); st.State != StateLobby {
		t.Fatalf("bob = %+v", st)
	}
}
