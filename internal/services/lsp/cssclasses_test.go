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

// TestCSSClassCompletionInsideStringLiteral verifies the end-to-end LSP path
// for class-name completion: a project has a `.css` file with several class
// selectors, a Sova source file `@embed`s it (so the embed resolver sees
// the file at build time), and typing inside a string literal in any
// method body of a frontend type returns the class names as completion
// items. The completion classifier sees the cursor sitting between the
// quotes of `"<here>"` and routes through `cssClassCompletions`, which
// reads the class set out of the embed registry cache and feeds it back
// to the editor.
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

	// `        let cls = ""` is line 9 (0-indexed); the cursor sits inside the empty string at column 19 (between the two quotes).
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

// TestCSSClassCompletionPreciseAtCSSClassParam verifies P2's precision
// path: when the cursor is inside a string literal at an argument
// position whose corresponding parameter carries `@cssClass`, the
// completion items show the call's callee + arg index in their detail
// line (so the user sees *why* the suggestion is being offered) and the
// SortText prefix puts class items ahead of any identifier-fallback
// noise. The broad in-string fallback still works elsewhere — this test
// only asserts the precise-context enrichment kicks in.
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

	// `    let _ = Element("button", "")` is line 10 (0-indexed). The second
	// string's opening quote is at column 30; the cursor between the two
	// quotes is column 31.
	compParams := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 10, Character: 31},
	}}
	var compList protocol.CompletionList
	if _, err := cc.Call(ctx, protocol.MethodTextDocumentCompletion, compParams, &compList); err != nil {
		t.Fatalf("completion: %v", err)
	}

	// Both classes must show up.
	labels := completionLabels(compList.Items)
	for _, want := range []string{"primary", "secondary"} {
		if !containsString(labels, want) {
			t.Errorf("completion missing class %q; got: %v", want, labels)
		}
	}

	// The detail must mention the callee + arg index (P2 precision).
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

	// At the FIRST arg (the tag, not a @cssClass param), the detail should be the broad fallback ("CSS class") rather than the precise one — V1's behavior still applies because the first param has no @cssClass.
	compParamsFirst := &protocol.CompletionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: sovaURI},
		Position:     protocol.Position{Line: 10, Character: 25}, // between the quotes of `"button"`
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

// TestCSSClassHoverInsideStringLiteral verifies P3 hover: cursor on a known
// class name inside a string literal returns a markdown popup with the
// rule body, sourced from the corresponding `.css` file.
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

	// `    let cls = "primary"` is line 6 (0-indexed). The `p` of "primary" sits at column 20.
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

// TestCSSClassDefinitionJumpsToCSSFile verifies P3 go-to-def: cursor on a
// known class name returns a Location pointing into the `.css` file at
// the selector's line.
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

	// `    let cls = "secondary"` is line 6 (0-indexed); cursor at column 22 sits on the `o` of "secondary".
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

// TestCSSClassCompletionPreciseAtTypeCtorNamedArg verifies the Strix-style
// path: a type with a `@cssClass`-marked field (think `Div`'s `class`
// field via the `HtmlElement` mixin) called with a named argument
// (`Div(class: "primary")`) gets the same precise detection as a
// top-level function with a `@cssClass`-marked param. Type-ctor +
// named-arg is the actual surface Strix uses today, so this test
// guards the everyday case.
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

	// `    let _ = new Div(class: "")` is line 12 (0-indexed). The opening
	// quote of "" sits at column 27, cursor between the quotes is 28.
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

// TestCSSClassPartialFollowing verifies P4 SCSS-partial following: a main
// stylesheet `@use`s a partial; classes defined only in the partial show
// up in the project index and surface in hover. The LSP probes the four
// standard Sass spellings (`name.scss`, `_name.scss`, `name.sass`,
// `_name.sass`) relative to the importing file's directory.
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

	// Skip if no sass binary — SCSS embed needs preprocessing, and without
	// the binary the @embed fails before the LSP can build the class index.
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

	// `    let cls = ""` is line 6; cursor between the quotes at column 16.
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

// TestCSSClassReferencesReturnsAllOccurrences verifies P4 references:
// asking for references on a class name returns every occurrence across
// the project's stylesheets.
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

	// `    let cls = "btn"` is line 6 (0-indexed). The string body sits at
	// columns 15-17 (`b`, `t`, `n`); column 16 puts the cursor on the `t`
	// of "btn", which `classNameAtCursor` walks left+right to capture the
	// full token.
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

// TestCSSClassCompletionOutsideStringIgnored asserts that completion in a
// regular identifier position (not inside a string) does NOT surface CSS
// class names — they'd pollute the regular identifier completion list.
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

	// Cursor at `let x = 0` end-of-line — pure identifier completion context.
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
