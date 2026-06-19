package lsp

import (
	"os"
	"path/filepath"
	"sova/internal/services/compiler"
	"strings"
	"testing"
)

func TestCSSClassDiagnosticUnknownClass(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Button.css"), []byte(`.primary { } .secondary { }`), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}

	src := `package app on frontend

@embed("./Button.css")
const ButtonCSS: string = ""

func Element(name: string, @cssClass class: string): string {
    return name
}

func render() {
    let _ = Element("button", "primary missing-class")
    let _ = Element("div", "totally-unknown")
}
`
	c := compiler.New()
	c.SetBuildConfig("build_config", embedClassDiagTestConfig{root: dir})
	c.AddSource("main.sova", src)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}

	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}

	pkg := c.Packages["app"]
	if pkg == nil || len(pkg.Files) == 0 {
		t.Fatalf("package app missing")
	}

	file := pkg.Files[0].Hir
	if file == nil {
		t.Fatalf("file HIR missing")
	}

	diags := cssClassDiagnostics(c, file)
	if len(diags) != 2 {
		t.Fatalf("want 2 warnings (one per unknown token); got %d (%+v)", len(diags), diags)
	}

	msgs := make([]string, 0, len(diags))
	for _, d := range diags {
		msgs = append(msgs, d.Message)
	}

	wantSubstrings := []string{"missing-class", "totally-unknown"}

	for _, want := range wantSubstrings {
		found := false
		for _, m := range msgs {
			if strings.Contains(m, want) {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("expected warning containing %q; got %v", want, msgs)
		}
	}
}

func TestCSSClassDiagnosticSilentWithoutClassIndex(t *testing.T) {
	dir := t.TempDir()
	src := `package app on frontend

func Element(@cssClass class: string): string {
    return class
}

func render() {
    let _ = Element("anything")
}
`
	c := compiler.New()
	c.SetBuildConfig("build_config", embedClassDiagTestConfig{root: dir})
	c.AddSource("main.sova", src)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}

	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile errored")
	}

	file := c.Packages["app"].Files[0].Hir
	if diags := cssClassDiagnostics(c, file); len(diags) != 0 {
		t.Errorf("want no warnings without any CSS embed; got %+v", diags)
	}
}

type embedClassDiagTestConfig struct {
	root string
}

func (c embedClassDiagTestConfig) OutputDirectory() string  { return filepath.Join(c.root, ".output") }

func (c embedClassDiagTestConfig) OutputBaseName() string   { return "output" }

func (c embedClassDiagTestConfig) SourceDirectory() string  { return c.root }

func (c embedClassDiagTestConfig) SCSSCommandValue() string { return "" }

func (c embedClassDiagTestConfig) SCSSDisabledValue() bool  { return false }
