package localcore

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// StateHash returns the SHA-256 of a deterministic canonical serialization of
// the authoritative table state: the sequence counter, hand number, button, the
// seats (sorted by ID) with their durable state, and the in-flight HandState
// (stacks, committed, status, hole cards, board, pot, street, to-act). Two peers
// whose visible + hidden state agree produce the same hash, so a snapshot sync
// or a divergence is detectable without trusting any single peer (issue #27).
//
// The serialization is hand-written (not JSON) so it is independent of map
// iteration order and stable across Go versions.
func (lt *LocalTable) StateHash() string {
	h := sha256.New()
	var scratch [8]byte
	putU64 := func(v uint64) {
		binary.BigEndian.PutUint64(scratch[:], v)
		h.Write(scratch[:])
	}
	putI64 := func(v int64) { putU64(uint64(v)) }
	putStr := func(s string) {
		putU64(uint64(len(s)))
		h.Write([]byte(s))
	}
	putCard := func(c engine.Card) {
		h.Write([]byte{byte(c.Rank), byte(c.Suit)})
	}

	putU64(lt.seq)
	putI64(int64(lt.handNum))
	putI64(int64(lt.button))

	ids := lt.sortedSeatIDs()
	putU64(uint64(len(ids)))
	for _, id := range ids {
		s := lt.seats[id]
		putI64(int64(id))
		putStr(s.playerID)
		putI64(int64(s.stack))
		putBool(h, s.sittingOut)
		putBool(h, s.disconnected)
	}

	if lt.hand == nil {
		putU64(0) // hand-present flag
		return hex.EncodeToString(h.Sum(nil))
	}
	putU64(1)
	hs := lt.hand
	putStr(lt.handID)
	putI64(int64(hs.Street))
	putI64(int64(hs.Pot))
	putI64(int64(hs.CurrentBet))
	putI64(int64(hs.MinRaise))
	putI64(int64(hs.ButtonPos))
	putI64(int64(hs.ToActPos))

	putU64(uint64(len(hs.Board)))
	for _, c := range hs.Board {
		putCard(c)
	}
	putU64(uint64(len(hs.Players)))
	for _, p := range hs.Players {
		putI64(int64(p.SeatID))
		putI64(int64(p.Stack))
		putI64(int64(p.Committed))
		putI64(int64(p.TotalBet))
		putI64(int64(p.Status))
		putCard(p.Hole[0])
		putCard(p.Hole[1])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func putBool(h interface{ Write([]byte) (int, error) }, b bool) {
	if b {
		h.Write([]byte{1})
		return
	}
	h.Write([]byte{0})
}
