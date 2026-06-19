package lsp

import "encoding/json"

func jsonMarshalDecode(raw json.RawMessage, out any) error {
	return json.Unmarshal(raw, out)
}

func withTerminate(fn func(int)) func() {
	prev := defaultTerminate
	defaultTerminate = fn
	return func() { defaultTerminate = prev }
}
