package lsp

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestReferencesSmoke exercises Phase 3: open a file with a function declared once and called twice, then request References, DocumentHighlight, and workspace/Symbol. Asserts each handler returns the expected entries.
func TestReferencesSmoke(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	docPath := filepath.Join(tempRoot, "refs.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	docURI := uri.URI("file://" + filepath.ToSlash(docPath))

	src := `on shared

func helper(): int {
    return 1
}

func main() {
    let a = helper()
    let b = helper()
    print(a)
    print(b)
}
`

	clientSide, serverSide := newPipeDuplex()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		_ = serveStream(ctx, jsonrpc2.NewStream(serverSide), io.Discard)
	}()

	clientConn := jsonrpc2.NewConn(jsonrpc2.NewStream(clientSide))
	clientConn.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		return reply(ctx, nil, nil)
	})

	var initResult protocol.InitializeResult
	if _, err := clientConn.Call(ctx, protocol.MethodInitialize, &protocol.InitializeParams{
		RootURI: rootURI,
	}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	assertCapTrue(t, "referencesProvider", initResult.Capabilities.ReferencesProvider)
	assertCapTrue(t, "documentHighlightProvider", initResult.Capabilities.DocumentHighlightProvider)
	assertCapTrue(t, "workspaceSymbolProvider", initResult.Capabilities.WorkspaceSymbolProvider)

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

	// References: cursor on `helper` inside the first call (line 7, col 12).
	refParams := &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 7, Character: 14},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	}
	var refs []protocol.Location
	if _, err := clientConn.Call(ctx, protocol.MethodTextDocumentReferences, refParams, &refs); err != nil {
		t.Fatalf("references: %v", err)
	}
	if len(refs) < 3 {
		t.Fatalf("expected at least 3 references (decl + 2 call sites), got %d: %+v", len(refs), refs)
	}

	// References excluding declaration.
	refParams.Context.IncludeDeclaration = false
	var refsNoDecl []protocol.Location
	if _, err := clientConn.Call(ctx, protocol.MethodTextDocumentReferences, refParams, &refsNoDecl); err != nil {
		t.Fatalf("references no-decl: %v", err)
	}
	if len(refsNoDecl) != len(refs)-1 {
		t.Fatalf("expected exactly one fewer entry when excluding decl; got %d (with decl: %d)", len(refsNoDecl), len(refs))
	}

	// DocumentHighlight at the same position.
	hlParams := &protocol.DocumentHighlightParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 7, Character: 14},
		},
	}
	var hls []protocol.DocumentHighlight
	if _, err := clientConn.Call(ctx, protocol.MethodTextDocumentDocumentHighlight, hlParams, &hls); err != nil {
		t.Fatalf("documentHighlight: %v", err)
	}
	if len(hls) < 3 {
		t.Fatalf("expected at least 3 highlights, got %d", len(hls))
	}
	gotDeclHL := false
	for _, h := range hls {
		if h.Kind == protocol.DocumentHighlightKindWrite {
			gotDeclHL = true
			break
		}
	}
	if !gotDeclHL {
		t.Fatalf("expected one declaration-kind highlight in the set; got kinds: %+v", hls)
	}

	// Workspace symbols - empty query returns the file's decls.
	wsParams := &protocol.WorkspaceSymbolParams{Query: ""}
	var wsSyms []protocol.SymbolInformation
	if _, err := clientConn.Call(ctx, protocol.MethodWorkspaceSymbol, wsParams, &wsSyms); err != nil {
		t.Fatalf("workspace/symbol: %v", err)
	}
	if !workspaceHasSymbol(wsSyms, "helper") || !workspaceHasSymbol(wsSyms, "main") {
		t.Fatalf("expected workspace symbols to include `helper` and `main`, got %v", symbolNames(wsSyms))
	}

	// Filtered workspace symbol query.
	wsParams.Query = "hel"
	var wsFiltered []protocol.SymbolInformation
	if _, err := clientConn.Call(ctx, protocol.MethodWorkspaceSymbol, wsParams, &wsFiltered); err != nil {
		t.Fatalf("workspace/symbol filtered: %v", err)
	}
	if !workspaceHasSymbol(wsFiltered, "helper") {
		t.Fatalf("query 'hel' should match `helper`, got %v", symbolNames(wsFiltered))
	}
	for _, s := range wsFiltered {
		if !strings.Contains(strings.ToLower(s.Name), "hel") {
			t.Fatalf("query 'hel' returned non-matching symbol %s", s.Name)
		}
	}

	if _, err := clientConn.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_ = clientConn.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func workspaceHasSymbol(syms []protocol.SymbolInformation, name string) bool {
	for _, s := range syms {
		if s.Name == name {
			return true
		}
	}
	return false
}

func symbolNames(syms []protocol.SymbolInformation) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.Name
	}
	return out
}
