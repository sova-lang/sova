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

// TestFormattingSmoke drives the LSP `textDocument/formatting` request end-to-end: open a deliberately-messy document, ask the server to format it, assert the returned TextEdit replaces the whole document with the canonical form.
func TestFormattingSmoke(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	docPath := filepath.Join(tempRoot, "messy.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	docURI := uri.URI("file://" + filepath.ToSlash(docPath))

	src := "on shared\n\nfunc   main(){\nlet x=1+2\nprint(x)\n}\n"

	clientSide, serverSide := newPipeDuplex()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() { _ = serveStream(ctx, jsonrpc2.NewStream(serverSide), io.Discard) }()
	clientConn := jsonrpc2.NewConn(jsonrpc2.NewStream(clientSide))
	clientConn.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		return reply(ctx, nil, nil)
	})

	var initResult protocol.InitializeResult
	if _, err := clientConn.Call(ctx, protocol.MethodInitialize, &protocol.InitializeParams{RootURI: rootURI}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	assertCapTrue(t, "documentFormattingProvider", initResult.Capabilities.DocumentFormattingProvider)

	_ = clientConn.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{})
	if err := clientConn.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: docURI, LanguageID: "sova", Version: 1, Text: src,
		},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	var edits []protocol.TextEdit
	if _, err := clientConn.Call(ctx, protocol.MethodTextDocumentFormatting, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	}, &edits); err != nil {
		t.Fatalf("formatting: %v", err)
	}
	if len(edits) != 1 {
		t.Fatalf("expected exactly one full-replacement edit, got %d", len(edits))
	}
	out := edits[0].NewText
	if !strings.Contains(out, "func main() {") {
		t.Fatalf("formatted output should contain canonical `func main() {`, got:\n%s", out)
	}
	if !strings.Contains(out, "let x = 1 + 2") {
		t.Fatalf("formatted output should normalize spacing around `let x = 1 + 2`, got:\n%s", out)
	}
	if strings.Contains(out, "  func   main") {
		t.Fatalf("formatted output still contains pre-formatted whitespace:\n%s", out)
	}

	if _, err := clientConn.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_ = clientConn.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}
