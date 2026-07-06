package localcore

import (
	"encoding/json"

	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// out accumulates the per-recipient event envelopes produced while processing a
// single Submit/Tick/VoidHand call. It replaces the table package's per-
// connection channels: instead of writing to a chan, each event is appended to a
// slice keyed by recipient player ID (or Broadcast for table-wide events).
//
// The result is the facade's return shape: map[playerID] -> []Envelope-JSON,
// with Broadcast ("*") carrying table-wide events. A client merges the two
// streams by Seq exactly as it does online.
type out struct {
	lt     *LocalTable
	events map[string][]json.RawMessage
}

func (lt *LocalTable) newOut() *out {
	return &out{lt: lt, events: map[string][]json.RawMessage{}}
}

// envelope wraps a payload into a versioned, sequenced protocol.Envelope, using
// the table's monotonic sequence so clients can detect gaps.
func (o *out) envelope(typ string, data any) json.RawMessage {
	env := protocol.Envelope{
		V:    protocol.ProtocolVersion,
		Type: typ,
		Seq:  o.lt.nextSeq(),
		Data: mustJSON(data),
	}
	return mustJSON(env)
}

// broadcast records one event, sharing a single Seq, for every recipient (the
// Broadcast key). Only players actually connected will consume it, but the
// facade returns it under "*" so the host can fan it out.
func (o *out) broadcast(typ string, data any) {
	o.events[Broadcast] = append(o.events[Broadcast], o.envelope(typ, data))
}

// sendTo records one event addressed to a single player, with its own Seq (used
// for privacy sends: hole cards, targeted snapshots, errors).
func (o *out) sendTo(playerID, typ string, data any) {
	o.events[playerID] = append(o.events[playerID], o.envelope(typ, data))
}

// sendError records a non-fatal error event addressed to a single player.
func (o *out) sendError(playerID, code, msg string) {
	o.sendTo(playerID, protocol.EvError, protocol.ErrorEvent{Code: code, Message: msg})
}

// result finalizes the accumulator into the facade's return map.
func (o *out) result() map[string][]json.RawMessage { return o.events }

// mustJSON marshals an outbound payload; the payloads here are always
// serializable, so an error is a programming bug.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("localcore: outbound payload not serializable: " + err.Error())
	}
	return b
}
