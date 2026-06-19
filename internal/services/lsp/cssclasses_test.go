package lsp

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func TestCSSClassCompletionInsideStringLiteral(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	cssPath := filepath.Join(tempRoot, "Button.css")
	sovaPath := filepath.Join(tempRoot, "main.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	sovaURI := uri.URI("file://" + filepath.ToSlash(sovaPath))

	cssContents := `.primary { background: rebeccapurple; }
.secondary { background: gray; }
.btn-large { padding: 16px; }
.alert.error { background: pink; }
`
	if err := os.WriteFile(cssPath, []byte(cssContents), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	sovaSrc := `package app on frontend

@embed("./Button.css")
const ButtonCSS: string = ""

type View {
    label: string = ""

    func render(): string {
        let cls = ""
        return cls
    }
}
`
	if err := os.WriteFile(sovaPath, []byte(sovaSrc), 0o644); err != nil {
		t.Fatalf("write sova: %v", err)
	}

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
		TextDocument: protocol.TextDocumentItem{URI: sovaURI, LanguageID: "sova", Version: 1, Text: sovaSrc},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 9, Character: 19},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)
	want := []string{"primary", "secondary", "btn-large", "alert", "error"}

	for _, w := range want {
		if !containsString(labels, w) {
			t.Errorf("completion missing class %q; got: %v", w, labels)
		}
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func TestCSSClassCompletionPreciseAtCSSClassParam(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	cssPath := filepath.Join(tempRoot, "Button.css")
	sovaPath := filepath.Join(tempRoot, "main.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	sovaURI := uri.URI("file://" + filepath.ToSlash(sovaPath))

	if err := os.WriteFile(cssPath, []byte(`.primary { } .secondary { }`), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	sovaSrc := `package app on frontend

@embed("./Button.css")
const ButtonCSS: string = ""

func Element(name: string, @cssClass class: string): string {
    return name
}

func render() {
    let _ = Element("button", "")
}
`
	if err := os.WriteFile(sovaPath, []byte(sovaSrc), 0o644); err != nil {
		t.Fatalf("write sova: %v", err)
	}

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
		TextDocument: protocol.TextDocumentItem{URI: sovaURI, LanguageID: "sova", Version: 1, Text: sovaSrc},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 10, Character: 31},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)
	for _, want := range []string{"primary", "secondary"} {
		if !containsString(labels, want) {
			t.Errorf("completion missing class %q; got: %v", want, labels)
		}
	}

	primary := findCompletionItem(compList.Items, "primary")
	if primary == nil {
		t.Fatalf("primary item missing")
	}

	if !strings.Contains(primary.Detail, "Element") {
		t.Errorf("detail should mention callee `Element`; got %q", primary.Detail)
	}

	if !strings.Contains(primary.Detail, "arg #2") {
		t.Errorf("detail should mention arg position `arg #2`; got %q", primary.Detail)
	}

	if primary.SortText == "" || primary.SortText[0] != '0' {
		t.Errorf("SortText should be prefixed with `0` to outrank non-class noise; got %q", primary.SortText)
	}

	compParamsFirst := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 10, Character: 25},
	}}

	var compListFirst protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParamsFirst, &compListFirst); err != nil {
		t.Fatalf("completion (first arg): %v", err)
	}

	first := findCompletionItem(compListFirst.Items, "primary")
	if first == nil {
		t.Fatalf("primary should still appear under V1 broad fallback; items: %v", completionLabels(compListFirst.Items))
	}

	if strings.Contains(first.Detail, "Element") {
		t.Errorf("first-arg detail should be the broad fallback (no callee mention); got %q", first.Detail)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func TestCSSClassHoverInsideStringLiteral(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	cssPath := filepath.Join(tempRoot, "Button.css")
	sovaPath := filepath.Join(tempRoot, "main.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	sovaURI := uri.URI("file://" + filepath.ToSlash(sovaPath))

	cssContents := ".primary { background: rebeccapurple; color: white; }\n.secondary { background: gray; }\n"
	if err := os.WriteFile(cssPath, []byte(cssContents), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	sovaSrc := `package app on frontend

@embed("./Button.css")
const ButtonCSS: string = ""

func render(): string {
    let cls = "primary"
    return cls
}
`
	if err := os.WriteFile(sovaPath, []byte(sovaSrc), 0o644); err != nil {
		t.Fatalf("write sova: %v", err)
	}

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
		TextDocument: protocol.TextDocumentItem{URI: sovaURI, LanguageID: "sova", Version: 1, Text: sovaSrc},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	hoverParams := &protocol.HoverParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 6, Character: 20},
	}}

	var hover protocol.Hover
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentHover, hoverParams, &hover); err != nil {
		t.Fatalf("hover: %v", err)
	}

	value := hover.Contents.Value
	if !strings.Contains(value, "**.primary**") {
		t.Errorf("hover should bold the class name; got %q", value)
	}

	if !strings.Contains(value, "rebeccapurple") {
		t.Errorf("hover should include the rule body containing `rebeccapurple`; got %q", value)
	}

	if !strings.Contains(value, "Button.css") {
		t.Errorf("hover should mention the source file; got %q", value)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func TestCSSClassDefinitionJumpsToCSSFile(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	cssPath := filepath.Join(tempRoot, "Button.css")
	sovaPath := filepath.Join(tempRoot, "main.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	sovaURI := uri.URI("file://" + filepath.ToSlash(sovaPath))
	cssURI := uri.URI("file://" + filepath.ToSlash(cssPath))

	if err := os.WriteFile(cssPath, []byte(".primary { color: red; }\n.secondary { color: blue; }\n"), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	sovaSrc := `package app on frontend

@embed("./Button.css")
const ButtonCSS: string = ""

func render(): string {
    let cls = "secondary"
    return cls
}
`
	if err := os.WriteFile(sovaPath, []byte(sovaSrc), 0o644); err != nil {
		t.Fatalf("write sova: %v", err)
	}

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
		TextDocument: protocol.TextDocumentItem{URI: sovaURI, LanguageID: "sova", Version: 1, Text: sovaSrc},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	defParams := &protocol.DefinitionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 6, Character: 22},
	}}

	var defLocs []protocol.Location
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentDefinition, defParams, &defLocs); err != nil {
		t.Fatalf("definition: %v", err)
	}

	if len(defLocs) != 1 {
		t.Fatalf("definition: want 1 location, got %d (%+v)", len(defLocs), defLocs)
	}

	if defLocs[0].URI != cssURI {
		t.Errorf("definition URI: want %s, got %s", cssURI, defLocs[0].URI)
	}

	if defLocs[0].Range.Start.Line != 1 {
		t.Errorf(".secondary lives on line 1 (0-indexed) of Button.css; got line %d", defLocs[0].Range.Start.Line)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func TestCSSClassCompletionPreciseAtTypeCtorNamedArg(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	cssPath := filepath.Join(tempRoot, "Button.css")
	sovaPath := filepath.Join(tempRoot, "main.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	sovaURI := uri.URI("file://" + filepath.ToSlash(sovaPath))

	if err := os.WriteFile(cssPath, []byte(`.primary { } .secondary { }`), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	sovaSrc := `package app on frontend

@embed("./Button.css")
const ButtonCSS: string = ""

type Div {
    @cssClass
    class: string = ""
    id: string = ""
}

func render() {
    let _ = new Div(class: "")
}
`
	if err := os.WriteFile(sovaPath, []byte(sovaSrc), 0o644); err != nil {
		t.Fatalf("write sova: %v", err)
	}

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
		TextDocument: protocol.TextDocumentItem{URI: sovaURI, LanguageID: "sova", Version: 1, Text: sovaSrc},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 12, Character: 28},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)
	for _, want := range []string{"primary", "secondary"} {
		if !containsString(labels, want) {
			t.Errorf("completion missing class %q; got %v", want, labels)
		}
	}

	primary := findCompletionItem(compList.Items, "primary")
	if primary == nil {
		t.Fatalf("primary item missing from completion list")
	}

	if !strings.Contains(primary.Detail, "Div") {
		t.Errorf("detail should mention the type-ctor callee `Div`; got %q", primary.Detail)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func TestCSSClassPartialFollowing(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	mainPath := filepath.Join(tempRoot, "Button.scss")
	partialPath := filepath.Join(tempRoot, "_variables.scss")
	sovaPath := filepath.Join(tempRoot, "main.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	sovaURI := uri.URI("file://" + filepath.ToSlash(sovaPath))

	if err := os.WriteFile(partialPath, []byte(`.themed-button { color: rebeccapurple; }
.themed-card { padding: 16px; }
`), 0o644); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	if err := os.WriteFile(mainPath, []byte(`@use "variables";

.local-only { color: red; }
`), 0o644); err != nil {
		t.Fatalf("write main: %v", err)
	}

	sovaSrc := `package app on frontend

@embed("./Button.scss")
const ButtonCSS: string = ""

func render(): string {
    let cls = ""
    return cls
}
`
	if err := os.WriteFile(sovaPath, []byte(sovaSrc), 0o644); err != nil {
		t.Fatalf("write sova: %v", err)
	}

	if _, err := os.Stat("/usr/local/bin/sass"); err != nil {
		if _, err := os.Stat("/usr/bin/sass"); err != nil {
			t.Skip("sass not installed; SCSS partial test requires it")
		}
	}

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
		TextDocument: protocol.TextDocumentItem{URI: sovaURI, LanguageID: "sova", Version: 1, Text: sovaSrc},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 6, Character: 16},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)
	for _, want := range []string{"local-only", "themed-button", "themed-card"} {
		if !containsString(labels, want) {
			t.Errorf("completion should include partial classes; missing %q in %v", want, labels)
		}
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func TestCSSClassReferencesReturnsAllOccurrences(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	cssPath := filepath.Join(tempRoot, "Button.css")
	sovaPath := filepath.Join(tempRoot, "main.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	sovaURI := uri.URI("file://" + filepath.ToSlash(sovaPath))
	cssURI := uri.URI("file://" + filepath.ToSlash(cssPath))

	if err := os.WriteFile(cssPath, []byte(`.btn { padding: 8px; }
.btn:hover { background: gray; }
.btn.large { font-size: 18px; }
`), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	sovaSrc := `package app on frontend

@embed("./Button.css")
const ButtonCSS: string = ""

func render(): string {
    let cls = "btn"
    return cls
}
`
	if err := os.WriteFile(sovaPath, []byte(sovaSrc), 0o644); err != nil {
		t.Fatalf("write sova: %v", err)
	}

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
		TextDocument: protocol.TextDocumentItem{URI: sovaURI, LanguageID: "sova", Version: 1, Text: sovaSrc},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	refParams := &protocol.ReferenceParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 6, Character: 16},
	}}

	var refs []protocol.Location
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentReferences, refParams, &refs); err != nil {
		t.Fatalf("references: %v", err)
	}

	if len(refs) != 3 {
		t.Fatalf("want 3 occurrences of `.btn` in Button.css, got %d (%+v)", len(refs), refs)
	}

	for _, r := range refs {
		if r.URI != cssURI {
			t.Errorf("ref URI: want %s, got %s", cssURI, r.URI)
		}
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}

func findCompletionItem(items []protocol.CompletionItem, label string) *protocol.CompletionItem {
	for i := range items {
		if items[i].Label == label {
			return &items[i]
		}
	}

	return nil
}

func TestCSSClassCompletionOutsideStringIgnored(t *testing.T) {
	restore := withTerminate(func(int) {})
	defer restore()

	tempRoot := t.TempDir()
	cssPath := filepath.Join(tempRoot, "Button.css")
	sovaPath := filepath.Join(tempRoot, "main.sova")
	rootURI := uri.URI("file://" + filepath.ToSlash(tempRoot))
	sovaURI := uri.URI("file://" + filepath.ToSlash(sovaPath))

	if err := os.WriteFile(cssPath, []byte(`.primary { }`), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	sovaSrc := `package app on frontend

@embed("./Button.css")
const ButtonCSS: string = ""

func main() {
    let x = 0
}
`
	if err := os.WriteFile(sovaPath, []byte(sovaSrc), 0o644); err != nil {
		t.Fatalf("write sova: %v", err)
	}

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
		TextDocument: protocol.TextDocumentItem{URI: sovaURI, LanguageID: "sova", Version: 1, Text: sovaSrc},
	}); err != nil {
		t.Fatalf("didOpen: %v", err)
	}

	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 6, Character: 13},
	}}

	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	labels := completionLabels(compList.Items)
	if containsString(labels, "primary") {
		t.Errorf("identifier-context completion should not include class name `primary`; got: %v", labels)
	}

	if _, err := cc.Call(ctx, protocol.MethodShutdown, nil, nil); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	_ = cc.Notify(ctx, protocol.MethodExit, nil)
	cancel()
}
