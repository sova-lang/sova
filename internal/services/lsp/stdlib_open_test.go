package lsp

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestStdLibFileOpenProducesNoSyntaxErrors verifies that opening a stdlib file
// (std/list.sova) directly in the editor doesn't produce spurious parse/type
// diagnostics. The user-reported regression: the `>` in `if x >= len(...)`
// was being marked red because the file's package couldn't resolve cleanly
// when opened outside a user project.
func TestStdLibFileOpenProducesNoSyntaxErrors(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	listPath, err := filepath.Abs("../../../std/list.sova")
	if err != nil {
		t.Fatalf("abs list path: %v", err)
	}
	if _, err := os.Stat(listPath); err != nil {
		t.Skipf("std/list.sova not at expected path: %v", err)
	}
	listText, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("read list: %v", err)
	}

	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}
	rootURI := uri.URI("file://" + filepath.ToSlash(repoRoot))
	listURI := uri.URI("file://" + filepath.ToSlash(listPath))

	clientSide, serverSide := newPipeDuplex()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	go func() { _ = serveStream(ctx, jsonrpc2.NewStream(serverSide), io.Discard) }()
	cc := jsonrpc2.NewConn(jsonrpc2.NewStream(clientSide))
	diagCh := make(chan *protocol.PublishDiagnosticsParams, 16)
	cc.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		if req.Method() == protocol.MethodTextDocumentPublishDiagnostics {
			var pd protocol.PublishDiagnosticsParams
			if err := jsonMarshalDecode(req.Params(), &pd); err == nil {
				select {
				case diagCh <- &pd:
				default:
				}
			}
		}
		return reply(ctx, nil, nil)
	})

	var initResult protocol.InitializeResult
	if _, err := cc.Call(ctx, protocol.MethodInitialize, &protocol.InitializeParams{RootURI: rootURI}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	_ = cc.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{})
	if err := cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: listURI, LanguageID: "sova", Version: 1, Text: string(listText)},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	deadline := time.After(8 * time.Second)
	var got *protocol.PublishDiagnosticsParams
loop:
	for {
		select {
		case pd := <-diagCh:
			if pd.URI == listURI {
				got = pd
				break loop
			}
		case <-deadline:
			t.Fatalf("no diagnostics published for %s within deadline", listURI)
		}
	}

	if got == nil {
		t.Fatalf("no diagnostics seen")
	}
	for _, d := range got.Diagnostics {
		if d.Severity == protocol.DiagnosticSeverityError {
			t.Errorf("unexpected error diagnostic in std/list.sova: [%d:%d-%d:%d] %s",
				d.Range.Start.Line, d.Range.Start.Character,
				d.Range.End.Line, d.Range.End.Character, d.Message)
		}
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}
