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

func TestSharedMemberIntellisense(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	backendPath := filepath.Join(tempRoot, "user.sova")
	frontendPath := filepath.Join(tempRoot, "client.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	backendURI := uri.URI("file://" + filepath.ToSlash(backendPath))
	frontendURI := uri.URI("file://" + filepath.ToSlash(frontendPath))

	backend := `package myapp on backend

type User {
    shared id: int = 0
    shared name: string = ""
    passwordHash: string = ""

    shared func display(): string {
        return name + " (#" + (id as string) + ")"
    }

    func internalSave() {
        println(passwordHash)
    }
}

wire(authn: false) func getUser(): User {
    return new User(7, "Bob", "secret")
}
`

	frontend := `package myapp/client on frontend

import "myapp"

func main() {
    let u, _ = myapp.getUser()
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

	if err := cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: backendURI, LanguageID: "sova", Version: 1, Text: backend},
	}); err != nil {
		t.Fatalf("didOpen backend: %v", err)
	}

	if err := cc.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: frontendURI, LanguageID: "sova", Version: 1, Text: frontend},
	}); err != nil {
		t.Fatalf("didOpen frontend: %v", err)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: frontendURI},
		Position:     protocol.Position{Line: 6, Character: 14},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)

	if !containsString(labels, "id") {
		t.Errorf("expected shared field `id` in completion, got: %v", labels)
	}

	if !containsString(labels, "name") {
		t.Errorf("expected shared field `name` in completion, got: %v", labels)
	}

	if !containsString(labels, "display") {
		t.Errorf("expected shared method `display` in completion, got: %v", labels)
	}

	if containsString(labels, "passwordHash") {
		t.Errorf("backend-only field `passwordHash` leaked into frontend completion: %v", labels)
	}

	if containsString(labels, "internalSave") {
		t.Errorf("backend-only method `internalSave` leaked into frontend completion: %v", labels)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func TestAnnotationCompletion(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	docPath := filepath.Join(tempRoot, "ann.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	docURI := uri.URI("file://" + filepath.ToSlash(docPath))

	src := `package ann on backend

type T {
    @
    id: int = 0
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
		TextDocument: protocol.TextDocumentItem{URI: docURI, LanguageID: "sova", Version: 1, Text: src},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: docURI},
		Position:     protocol.Position{Line: 3, Character: 5},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)

	if !containsString(labels, "reactive") {
		t.Errorf("annotation completion missing `reactive`, got: %v", labels)
	}

	if !containsString(labels, "structTag") {
		t.Errorf("annotation completion missing `structTag`, got: %v", labels)
	}

	for _, l := range labels {
		if strings.HasPrefix(l, "@") {
			t.Errorf("annotation labels should be bare names (no leading @), got %q", l)
		}
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}
