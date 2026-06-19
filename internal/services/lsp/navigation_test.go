package lsp

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func TestNavigationSmoke(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	docPath := filepath.Join(tempRoot, "nav.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	docURI := uri.URI("file://" + filepath.ToSlash(docPath))

	src := `on shared

func add(a: int, b: int): int {
    return a + b
}

func main() {
    let x = add(1, 2)
    print(x)
}
`

	clientSide, serverSide := newPipeDuplex()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		_ = serveStream(ctx, jsonrpc2.NewStream(serverSide), io.Discard)
	}()

	clientConn := jsonrpc2.NewConn(jsonrpc2.NewStream(clientSide))
	var (
		diagMu sync.Mutex
		diags  []protocol.PublishDiagnosticsParams
	)
	clientConn.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		if req.Method() == protocol.MethodTextDocumentPublishDiagnostics {
			var params protocol.PublishDiagnosticsParams
			if err := jsonMarshalDecode(req.Params(), &params); err == nil {
				diagMu.Lock()
				diags = append(diags, params)
				diagMu.Unlock()
			}
		}

		return reply(ctx, nil, nil)
	})

	var initResult protocol.InitializeResult
	if _, err := clientConn.Call(ctx, protocol.MethodInitialize, &protocol.InitializeParams{
		RootURI: rootURI,
	}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	assertCapTrue(t, "hoverProvider", initResult.Capabilities.HoverProvider)
	assertCapTrue(t, "definitionProvider", initResult.Capabilities.DefinitionProvider)
	assertCapTrue(t, "documentSymbolProvider", initResult.Capabilities.DocumentSymbolProvider)

	_ = clientConn.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{})
	if err := clientConn.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        docURI,
			LanguageID: "sova",
			Version:    1,
			Text:       src,
		},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	hoverParams := &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 7, Character: 13},
		},
	}

	var hover protocol.Hover
	if _, err := clientConn.Call(ctx, protocol.MethodTextDocumentHover, hoverParams, &hover); err != nil {
		t.Fatalf("hover call: %v", err)
	}

	if hover.Contents.Value == "" {
		t.Fatalf("hover on `add` returned empty contents")
	}

	if !strings.Contains(hover.Contents.Value, "add") {
		t.Fatalf("hover should mention `add`, got: %q", hover.Contents.Value)
	}

	defParams := &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 7, Character: 13},
		},
	}

	var defs []protocol.Location
	if _, err := clientConn.Call(ctx, protocol.MethodTextDocumentDefinition, defParams, &defs); err != nil {
		t.Fatalf("definition call: %v", err)
	}

	if len(defs) == 0 {
		t.Fatalf("definition returned no locations")
	}

	if defs[0].URI != docURI {
		t.Fatalf("definition URI mismatch: got %s want %s", defs[0].URI, docURI)
	}

	if defs[0].Range.Start.Line != 2 {
		t.Fatalf("definition should point at line 2 (func add), got line %d", defs[0].Range.Start.Line)
	}

	symParams := &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	}

	var rawSyms []protocol.DocumentSymbol
	if _, err := clientConn.Call(ctx, protocol.MethodTextDocumentDocumentSymbol, symParams, &rawSyms); err != nil {
		t.Fatalf("documentSymbol call: %v", err)
	}

	if len(rawSyms) < 2 {
		t.Fatalf("expected at least 2 top-level symbols (add, main), got %d", len(rawSyms))
	}

	names := []string{rawSyms[0].Name, rawSyms[1].Name}

	if !containsString(names, "add") || !containsString(names, "main") {
		t.Fatalf("expected `add` and `main` in document symbols, got %v", names)
	}

	if _, err := clientConn.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = clientConn.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func containsString(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}

	return false
}

func assertCapTrue(t *testing.T, name string, cap interface{}) {
	t.Helper()
	switch v := cap.(type) {
	case bool:
		if !v {
			t.Fatalf("server didn't advertise %s", name)
		}

	case nil:
		t.Fatalf("server didn't advertise %s", name)
	}
}
