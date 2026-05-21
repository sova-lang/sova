package lsp

import "encoding/json"

// jsonMarshalDecode decodes raw JSON bytes (as supplied by jsonrpc2 in request params) into the target struct. Used by tests to inspect strongly-typed protocol payloads without depending on the production protocol.Server dispatch logic.
func jsonMarshalDecode(raw json.RawMessage, out any) error {
	return json.Unmarshal(raw, out)
}

// withTerminate swaps the package-level terminate hook used by `Server.Exit`. Returns a restore function the test calls on cleanup; never leak the override into other tests.
func withTerminate(fn func(int)) func() {
	prev := defaultTerminate
	defaultTerminate = fn
	return func() { defaultTerminate = prev }
}
