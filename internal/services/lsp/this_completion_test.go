package lsp

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestThisCompletionInsideMethodBody verifies that typing `this.` inside a struct method body
// lists the type's fields and methods. `this` is a synthetic receiver with no VarRef of its
// own, so the symbol-by-name fallback misses it; member-completion must consult the enclosing
// TypeDeclStmt to recover the receiver type.
func TestThisCompletionInsideMethodBody(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	modelPath := filepath.Join(tempRoot, "model.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	modelURI := uri.URI("file://" + filepath.ToSlash(modelPath))

	model := `package myApp on backend

type User {
    id: int = 0
    name: string = ""

    func describe(): string {
        let x = this.
        return ""
    }
}
`

	clientSide, serverSide := newPipeDuplex()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() { _ = serveStream(ctx, jsonrpc2.NewStream(serverSide), io.Discard) }()
	cc := jsonrpc2.NewConn(jsonrpc2.NewStream(clientSide))
	cc.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		return reply(ctx, nil, nil)
	})

	var initResult protocol.InitializeResult
	if _, err := cc.Call(ctx, protocol.MethodInitialize, &protocol.InitializeParams{RootURI: rootURI}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	_ = cc.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{})
	if err := cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: modelURI, LanguageID: "sova", Version: 1, Text: model},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: modelURI},
		Position:     protocol.Position{Line: 7, Character: 21},
	}}
	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}
	labels := completionLabels(compList.Items)
	for _, want := range []string{"id", "name", "describe"} {
		if !containsString(labels, want) {
			t.Errorf("this.<TAB> missing %q: got %v", want, labels)
		}
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}
