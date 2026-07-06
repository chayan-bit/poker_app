// Command tablewasm compiles the pure localcore facade to WebAssembly and
// exposes it to a JS host. Build with:
//
//	GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w" -o tablecore.wasm ./cmd/tablewasm
//
// The JS boundary is deliberately all-strings/JSON: every argument and return
// value that crosses is a string. The host wires the seed (from
// crypto.getRandomValues) and all time (nowMs) in explicitly, so an offline hand
// is byte-identical to an online one and replicable across peers (issue #27).
//
// JS API (installed on globalThis.tablecore):
//
//	newTable(configJSON, seedHex) -> handle
//	handle.submit(playerId, envelopeJSON) -> resultJSON
//	handle.tick(nowMs) -> resultJSON
//	handle.stateHash() -> hex string
//	handle.voidHand() -> resultJSON
//	handle.setSeed(seedHex) -> void
//
// resultJSON is a JSON object mapping recipient ("*" for broadcast, else a
// player ID) to an array of event-envelope JSON objects.
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/chayan-bit/poker_app/server/internal/localcore"
)

func main() {
	js.Global().Set("tablecore", js.ValueOf(map[string]any{
		"newTable": js.FuncOf(newTable),
	}))
	// Block forever: a WASM module's exports must stay callable for the tab's
	// lifetime, so main must not return.
	select {}
}

// newTable(configJSON, seedHex) constructs a LocalTable and returns a JS handle
// object whose methods drive it. Errors surface as a JS object {error: "..."}.
func newTable(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return jsError("newTable(configJSON, seedHex) requires two arguments")
	}
	var cfg localcore.Config
	if err := json.Unmarshal([]byte(args[0].String()), &cfg); err != nil {
		return jsError("invalid config JSON: " + err.Error())
	}
	lt := localcore.NewLocalTable(cfg, args[1].String())
	return handle(lt)
}

// handle builds the JS object exposing one table's methods.
func handle(lt *localcore.LocalTable) map[string]any {
	return map[string]any{
		"submit": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) < 2 {
				return jsError("submit(playerId, envelopeJSON) requires two arguments")
			}
			res, err := lt.Submit(args[0].String(), []byte(args[1].String()))
			if err != nil {
				return jsError(err.Error())
			}
			return marshalResult(res)
		}),
		"tick": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) < 1 {
				return jsError("tick(nowMs) requires one argument")
			}
			// Epoch milliseconds exceed 2^31, so read as float then narrow.
			return marshalResult(lt.Tick(int64(args[0].Float())))
		}),
		"stateHash": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			return lt.StateHash()
		}),
		"voidHand": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			return marshalResult(lt.VoidHand())
		}),
		"setSeed": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) >= 1 {
				lt.SetSeed(args[0].String())
			}
			return nil
		}),
	}
}

// marshalResult renders a per-recipient event map as a JSON string for JS.
func marshalResult(res map[string][]json.RawMessage) string {
	b, err := json.Marshal(res)
	if err != nil {
		return `{"*":[]}`
	}
	return string(b)
}

func jsError(msg string) map[string]any { return map[string]any{"error": msg} }
