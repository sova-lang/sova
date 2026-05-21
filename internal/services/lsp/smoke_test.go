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

// pipeStream wires two io.Pipe pairs into one duplex io.ReadWriteCloser so jsonrpc2.NewStream can frame against it. Used to drive the LSP server in-process for tests.
type pipeStream struct {
	r io.Reader
	w io.WriteCloser
}

func (p pipeStream) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p pipeStream) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p pipeStream) Close() error                { return p.w.Close() }

// newPipeDuplex returns two paired stream endpoints; what one writes the other reads. The "client" side acts as the editor; the "server" side is what we hand to serveStream.
func newPipeDuplex() (clientStream, serverStream pipeStream) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	return pipeStream{r: cr, w: cw}, pipeStream{r: sr, w: sw}
}

// TestStdioSmoke drives the LSP through a complete editor session: initialize → didOpen with a known-broken file → wait for publishDiagnostics → assert the diagnostic is correct → shutdown. Verifies the entire phase-1 surface (jsonrpc framing, lifecycle, document sync, compiler integration, diagnostic mapping) end-to-end without spawning a real editor.
func TestStdioSmoke(t *testing.T) {
	restoreTerminate := withTerminate(func(int) {})
	defer restoreTerminate()

	tempRoot := t.TempDir()
	docPath := filepath.Join(tempRoot, "smoke.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	docURI := uri.URI("file://" + filepath.ToSlash(docPath))

	clientSide, serverSide := newPipeDuplex()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- serveStream(ctx, jsonrpc2.NewStream(serverSide), io.Discard)
	}()

	clientConn := jsonrpc2.NewConn(jsonrpc2.NewStream(clientSide))
	var (
		diagsMu  sync.Mutex
		diagsCh  = make(chan *protocol.PublishDiagnosticsParams, 4)
	)
	clientHandler := func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		if req.Method() == protocol.MethodTextDocumentPublishDiagnostics {
			var params protocol.PublishDiagnosticsParams
			if err := unmarshalParams(req, &params); err != nil {
				return reply(ctx, nil, err)
			}
			diagsMu.Lock()
			diagsCh <- &params
			diagsMu.Unlock()
			return reply(ctx, nil, nil)
		}
		return reply(ctx, nil, jsonrpc2.ErrMethodNotFound)
	}
	clientConn.Go(ctx, clientHandler)

	var initResult protocol.InitializeResult
	if _, err := clientConn.Call(ctx, protocol.MethodInitialize, &protocol.InitializeParams{
		RootURI: rootURI,
	}, &initResult); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if initResult.ServerInfo == nil || initResult.ServerInfo.Name != "sova-lsp" {
		t.Fatalf("expected ServerInfo name=sova-lsp, got %+v", initResult.ServerInfo)
	}
	if err := clientConn.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}

	brokenText := "on shared\n\nfunc main() {\n    let x = unknown_symbol()\n}\n"
	if err := clientConn.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        docURI,
			LanguageID: "sova",
			Version:    1,
			Text:       brokenText,
		},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	var diags *protocol.PublishDiagnosticsParams
	select {
	case diags = <-diagsCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("did not receive publishDiagnostics within timeout")
	}
	if diags.URI != docURI {
		t.Fatalf("unexpected diag URI: got %s want %s", diags.URI, docURI)
	}
	if len(diags.Diagnostics) == 0 {
		t.Fatalf("expected at least one diagnostic for broken file, got none")
	}
	gotErr := false
	for _, d := range diags.Diagnostics {
		if d.Severity == protocol.DiagnosticSeverityError && strings.Contains(strings.ToLower(d.Message), "undeclared") {
			gotErr = true
			break
		}
	}
	if !gotErr {
		t.Fatalf("expected an error diagnostic about an undeclared symbol; got %+v", diags.Diagnostics)
	}

	if _, err := clientConn.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_ = clientConn.Notify(ctx, protocol.MethodExit, nil)
	cancel()
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
	}
}

// unmarshalParams decodes a jsonrpc2.Request's params into `out`. The raw API exposes params as json.RawMessage; we just delegate to json.Unmarshal.
func unmarshalParams(req jsonrpc2.Request, out any) error {
	return jsonMarshalDecode(req.Params(), out)
}
