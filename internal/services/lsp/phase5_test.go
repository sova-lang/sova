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

func TestPhase5(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	docPath := filepath.Join(tempRoot, "p5.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	docURI := uri.URI("file://" + filepath.ToSlash(docPath))

	src := `on shared

import "std/strings"
import "std/errors"

interface Greeter {
    func greet(): string
}

type Dog implements Greeter {
    private name: string = ""
    new(name: string) { this.name = name }
    func greet(): string {
        return "woof"
    }
}

func main() {
    let d = new Dog("rex")
    print(d.greet())
}

test "dog greets" {
    let d = new Dog("rex")
    assert d.greet() == "woof"
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

	assertCapTrue(t, "foldingRangeProvider", initResult.Capabilities.FoldingRangeProvider)
	assertCapTrue(t, "codeLensProvider", initResult.Capabilities.CodeLensProvider)
	assertCapTrue(t, "codeActionProvider", initResult.Capabilities.CodeActionProvider)
	assertCapTrue(t, "semanticTokensProvider", initResult.Capabilities.SemanticTokensProvider)

	_ = cc.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{})
	_ = cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: docURI, LanguageID: "sova", Version: 1, Text: src},
	})

	var fr []protocol.FoldingRange
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentFoldingRange, &protocol.FoldingRangeParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		},
	}, &fr); err != nil {
		t.Fatalf("foldingRange: %v", err)
	}

	if len(fr) < 4 {
		t.Fatalf("expected ≥4 folding ranges (imports + interface + type + main + test), got %d", len(fr))
	}

	hasImports := false
	for _, r := range fr {
		if r.Kind == protocol.ImportsFoldingRange {
			hasImports = true
		}
	}

	if !hasImports {
		t.Fatalf("expected an imports-kind folding range, got none")
	}

	var cl []protocol.CodeLens
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCodeLens, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	}, &cl); err != nil {
		t.Fatalf("codeLens: %v", err)
	}

	hasRunMain := false
	hasRunTest := false
	for _, l := range cl {
		if l.Command == nil {
			continue
		}

		if l.Command.Command == "sova.runMain" {
			hasRunMain = true
		}

		if l.Command.Command == "sova.runTest" {
			hasRunTest = true
		}
	}

	if !hasRunMain {
		t.Fatalf("expected a Run-main code lens")
	}

	if !hasRunTest {
		t.Fatalf("expected a Run-test code lens")
	}

	var actions []protocol.CodeAction
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCodeAction, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		Range:        protocol.Range{Start: protocol.Position{Line: 2}, End: protocol.Position{Line: 3}},
		Context:      protocol.CodeActionContext{},
	}, &actions); err != nil {
		t.Fatalf("codeAction: %v", err)
	}

	hasOrganize := false
	for _, a := range actions {
		if a.Kind == protocol.SourceOrganizeImports {
			hasOrganize = true
		}
	}

	if !hasOrganize {
		t.Fatalf("expected a Source.OrganizeImports action, got %d actions", len(actions))
	}

	var stokens protocol.SemanticTokens
	if _, err := cc.Call(ctx, protocol.MethodSemanticTokensFull, &protocol.SemanticTokensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	}, &stokens); err != nil {
		t.Fatalf("semanticTokens/full: %v", err)
	}

	if len(stokens.Data)%5 != 0 {
		t.Fatalf("semantic tokens data length %d is not a multiple of 5", len(stokens.Data))
	}

	if len(stokens.Data) < 5 {
		t.Fatalf("expected at least one semantic token, got %d", len(stokens.Data)/5)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}
