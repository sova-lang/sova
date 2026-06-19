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

func TestPhase4(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	docPath := filepath.Join(tempRoot, "p4.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	docURI := uri.URI("file://" + filepath.ToSlash(docPath))

	src := `on shared

interface Speaker {
    func say(): string
}

type Dog implements Speaker {
    private name: string = ""
    new(name: string) { this.name = name }
    func say(): string { return "woof" }
}

func add(a: int, b: int): int {
    return a + b
}

func main() {
    let d = new Dog("rex")
    let _ = d.say()
    let n = add(1, 2)
    print(n)
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

	assertCapTrue(t, "renameProvider", initResult.Capabilities.RenameProvider)
	assertCapTrue(t, "implementationProvider", initResult.Capabilities.ImplementationProvider)
	assertCapTrue(t, "signatureHelpProvider", initResult.Capabilities.SignatureHelpProvider)
	assertCapTrue(t, "completionProvider", initResult.Capabilities.CompletionProvider)

	_ = cc.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{})
	if err := cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: docURI, LanguageID: "sova", Version: 1, Text: src},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	prep := &protocol.PrepareRenameParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		Position:     protocol.Position{Line: 19, Character: 14},
	}}

	var rng protocol.Range
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentPrepareRename, prep, &rng); err != nil {
		t.Fatalf("prepareRename: %v", err)
	}

	if rng.Start.Line == 0 && rng.End.Character == 0 {
		t.Fatalf("prepareRename returned zero range")
	}

	renParams := &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 19, Character: 14},
		},
		NewName: "sum",
	}

	var we protocol.WorkspaceEdit
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentRename, renParams, &we); err != nil {
		t.Fatalf("rename: %v", err)
	}

	docEdits := we.Changes[docURI]
	if len(docEdits) < 2 {
		t.Fatalf("expected ≥2 rename edits (decl + call), got %d", len(docEdits))
	}

	for _, e := range docEdits {
		if e.NewText != "sum" {
			t.Fatalf("unexpected NewText %q", e.NewText)
		}
	}

	impParams := &protocol.ImplementationParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		Position:     protocol.Position{Line: 2, Character: 13},
	}}

	var imps []protocol.Location
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentImplementation, impParams, &imps); err != nil {
		t.Fatalf("implementation: %v", err)
	}

	if len(imps) == 0 {
		t.Fatalf("expected Dog as an implementer of Speaker, got 0 locations")
	}

	shParams := &protocol.SignatureHelpParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		Position:     protocol.Position{Line: 19, Character: 19},
	}}

	var sh protocol.SignatureHelp
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentSignatureHelp, shParams, &sh); err != nil {
		t.Fatalf("signatureHelp: %v", err)
	}

	if len(sh.Signatures) == 0 {
		t.Fatalf("expected at least one signature, got 0")
	}

	if !strings.Contains(sh.Signatures[0].Label, "add(a: int, b: int)") {
		t.Fatalf("signature label should contain `add(a: int, b: int)`, got %q", sh.Signatures[0].Label)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		Position:     protocol.Position{Line: 17, Character: 0},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)
	if !containsString(labels, "let") {
		t.Fatalf("identifier completion should include keyword `let`, got: %v", labels)
	}

	if !containsString(labels, "add") {
		t.Fatalf("identifier completion should include function `add`, got: %v", labels)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func completionLabels(items []protocol.CompletionItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}

	return out
}
