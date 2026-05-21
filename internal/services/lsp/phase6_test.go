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

// TestPhase6 covers call-hierarchy navigation (prepare → incoming → outgoing) and incremental document sync. The file has `main` calling `helper`, and `helper` calling `print`; we ask for incoming calls to `helper` and outgoing calls from `main`.
func TestPhase6(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	docPath := filepath.Join(tempRoot, "p6.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	docURI := uri.URI("file://" + filepath.ToSlash(docPath))

	src := `on shared

func helper(): int {
    return 1
}

func main() {
    let a = helper()
    let b = helper()
    print(a + b)
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
	assertCapTrue(t, "callHierarchyProvider", initResult.Capabilities.CallHierarchyProvider)

	_ = cc.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{})
	_ = cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: docURI, LanguageID: "sova", Version: 1, Text: src},
	})

	// PrepareCallHierarchy on `helper` at its declaration (line 2 in 0-based).
	var items []protocol.CallHierarchyItem
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentPrepareCallHierarchy, &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 2, Character: 6},
		},
	}, &items); err != nil {
		t.Fatalf("prepareCallHierarchy: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly one call-hierarchy item, got %d", len(items))
	}
	if items[0].Name != "helper" {
		t.Fatalf("expected item Name=helper, got %q", items[0].Name)
	}

	// IncomingCalls on helper - should report `main` as caller, with 2 call sites.
	var incoming []protocol.CallHierarchyIncomingCall
	if _, err := cc.Call(ctx, protocol.MethodCallHierarchyIncomingCalls, &protocol.CallHierarchyIncomingCallsParams{
		Item: items[0],
	}, &incoming); err != nil {
		t.Fatalf("incomingCalls: %v", err)
	}
	if len(incoming) != 1 {
		t.Fatalf("expected 1 caller, got %d", len(incoming))
	}
	if incoming[0].From.Name != "main" {
		t.Fatalf("expected caller=main, got %q", incoming[0].From.Name)
	}
	if len(incoming[0].FromRanges) != 2 {
		t.Fatalf("expected 2 call sites from main, got %d", len(incoming[0].FromRanges))
	}

	// PrepareCallHierarchy on `main`, then OutgoingCalls - should include `helper` as a callee.
	var mainItems []protocol.CallHierarchyItem
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentPrepareCallHierarchy, &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
			Position:     protocol.Position{Line: 6, Character: 6},
		},
	}, &mainItems); err != nil {
		t.Fatalf("prepareCallHierarchy(main): %v", err)
	}
	if len(mainItems) != 1 {
		t.Fatalf("expected 1 main item, got %d", len(mainItems))
	}
	var outgoing []protocol.CallHierarchyOutgoingCall
	if _, err := cc.Call(ctx, protocol.MethodCallHierarchyOutgoingCalls, &protocol.CallHierarchyOutgoingCallsParams{
		Item: mainItems[0],
	}, &outgoing); err != nil {
		t.Fatalf("outgoingCalls: %v", err)
	}
	hasHelper := false
	for _, o := range outgoing {
		if o.To.Name == "helper" {
			hasHelper = true
			if len(o.FromRanges) != 2 {
				t.Fatalf("expected 2 sites for outgoing call to helper, got %d", len(o.FromRanges))
			}
		}
	}
	if !hasHelper {
		t.Fatalf("expected `helper` in outgoing calls from main, got %v", outgoingNames(outgoing))
	}

	// Incremental sync smoke: splice in deliberately-bad spacing (`return  1` with two spaces) so the formatter sees a difference and returns a normalised edit. This proves the incremental change was applied to our overlay, not just dropped on the floor.
	if err := cc.Notify(ctx, protocol.MethodTextDocumentDidChange, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: docURI},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{{
			Range: protocol.Range{
				Start: protocol.Position{Line: 3, Character: 10},
				End:   protocol.Position{Line: 3, Character: 11},
			},
			Text: "    ",
		}},
	}); err != nil {
		t.Fatalf("didChange (splice): %v", err)
	}
	var edits []protocol.TextEdit
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentFormatting, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
	}, &edits); err != nil {
		t.Fatalf("formatting after incremental edit: %v", err)
	}
	if len(edits) == 0 {
		t.Fatalf("expected formatting to return at least one edit after incremental change")
	}
	if !strings.Contains(edits[0].NewText, "return 1") {
		t.Fatalf("incremental edit didn't take effect; formatted output:\n%s", edits[0].NewText)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func outgoingNames(calls []protocol.CallHierarchyOutgoingCall) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.To.Name
	}
	return out
}
