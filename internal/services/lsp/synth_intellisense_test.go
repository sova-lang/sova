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

func TestSynthAnnotationCompletionListsRegisteredSynths(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	annoPath := filepath.Join(tempRoot, "anno.sova")
	modelPath := filepath.Join(tempRoot, "model.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	annoURI := uri.URI("file://" + filepath.ToSlash(annoPath))
	modelURI := uri.URI("file://" + filepath.ToSlash(modelPath))

	anno := `package myAnno on synth

synth GormPK on field F {
    emit on F {
        @structTag("gorm", "primaryKey")
    }
}
`

	model := `package myApp on backend

import "myAnno"

type User {
    @
    id: int
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
		TextDocument: protocol.TextDocumentItem{URI: annoURI, LanguageID: "sova", Version: 1, Text: anno},
	}); err != nil {
		t.Fatalf("didOpen anno: %v", err)
	}

	if err := cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: modelURI, LanguageID: "sova", Version: 1, Text: model},
	}); err != nil {
		t.Fatalf("didOpen model: %v", err)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: modelURI},
		Position:     protocol.Position{Line: 5, Character: 5},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)

	if !containsString(labels, "GormPK") {
		t.Errorf("expected synth `GormPK` in completion, got: %v", labels)
	}

	if !containsString(labels, "structTag") {
		t.Errorf("expected built-in `structTag` still present alongside synth: %v", labels)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func TestSynthInjectedMembersAppearInCompletion(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	annoPath := filepath.Join(tempRoot, "anno.sova")
	modelPath := filepath.Join(tempRoot, "model.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	annoURI := uri.URI("file://" + filepath.ToSlash(annoPath))
	modelURI := uri.URI("file://" + filepath.ToSlash(modelPath))

	anno := `package myAnno on synth

synth Timestamps on type T {
    emit createdAt: int = 0
    emit updatedAt: int = 0
    emit func touch(): int {
        return 1
    }
}
`

	model := `package myApp on backend

import "myAnno"

@Timestamps
type User {
    id: int = 0
    name: string = ""
}

func main() {
    let u = new User(0, "")
    let _ = u.
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
	for _, doc := range []struct {
		u    uri.URI
		text string
	}{{annoURI, anno}, {modelURI, model}} {
		if err := cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{URI: doc.u, LanguageID: "sova", Version: 1, Text: doc.text},
		}); err != nil {
			t.Fatalf("didOpen %s: %v", doc.u, err)
		}
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: modelURI},
		Position:     protocol.Position{Line: 12, Character: 14},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)
	want := []string{"id", "name", "createdAt", "updatedAt", "touch"}

	for _, w := range want {
		if !containsString(labels, w) {
			t.Errorf("completion missing %q (synth-injected or hand-written): got %v", w, labels)
		}
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func TestSynthAnnotationHoverAndDefinition(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	annoPath := filepath.Join(tempRoot, "anno.sova")
	modelPath := filepath.Join(tempRoot, "model.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	annoURI := uri.URI("file://" + filepath.ToSlash(annoPath))
	modelURI := uri.URI("file://" + filepath.ToSlash(modelPath))

	anno := `package myAnno on synth

synth GormPK on field F {
    emit on F {
        @structTag("gorm", "primaryKey")
    }
}
`

	model := `package myApp on backend

import "myAnno"

type User {
    @GormPK
    id: int
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
	for _, doc := range []struct {
		u    uri.URI
		text string
	}{{annoURI, anno}, {modelURI, model}} {
		if err := cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{URI: doc.u, LanguageID: "sova", Version: 1, Text: doc.text},
		}); err != nil {
			t.Fatalf("didOpen %s: %v", doc.u, err)
		}
	}

	hoverParams := &protocol.HoverParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: modelURI},
		Position:     protocol.Position{Line: 5, Character: 8},
	}}

	var hover protocol.Hover
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentHover, hoverParams, &hover); err != nil {
		t.Fatalf("hover: %v", err)
	}

	value := hover.Contents.Value
	if !strings.Contains(value, "@GormPK") {
		t.Errorf("hover content missing @GormPK signature, got %q", value)
	}

	if !strings.Contains(value, "synth GormPK on field F") {
		t.Errorf("hover content missing synth body summary, got %q", value)
	}

	defParams := &protocol.DefinitionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: modelURI},
		Position:     protocol.Position{Line: 5, Character: 8},
	}}

	var defLocs []protocol.Location
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentDefinition, defParams, &defLocs); err != nil {
		t.Fatalf("definition: %v", err)
	}

	if len(defLocs) != 1 {
		t.Fatalf("definition: want 1 location, got %d (%+v)", len(defLocs), defLocs)
	}

	if defLocs[0].URI != annoURI {
		t.Errorf("definition URI: want %s, got %s", annoURI, defLocs[0].URI)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}
