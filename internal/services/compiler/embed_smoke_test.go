package compiler

import (
	"os"
	"path/filepath"
	"sova/internal/ir"
	"sova/internal/passes"
	"strings"
	"testing"
)

type embedTestConfig struct {
	src          string
	scssCommand  string
	scssDisabled bool
}

func (c embedTestConfig) OutputDirectory() string  { return filepath.Join(c.src, ".output") }
func (c embedTestConfig) OutputBaseName() string   { return "output" }
func (c embedTestConfig) SourceDirectory() string  { return c.src }
func (c embedTestConfig) SCSSCommandValue() string { return c.scssCommand }
func (c embedTestConfig) SCSSDisabledValue() bool  { return c.scssDisabled }

func TestEmbedTextResolvesAndPopulatesInfo(t *testing.T) {
	dir := t.TempDir()
	cssPath := filepath.Join(dir, "Button.css")
	if err := os.WriteFile(cssPath, []byte(".btn { color: red; }"), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}
	srcPath := filepath.Join(dir, "main.sova")
	src := `package app on backend

@embed("./Button.css")
const ButtonCSS: string = ""
`
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write sova: %v", err)
	}

	c := newEmbedTestContext(t, dir)
	c.AddSource("main.sova", src)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}

	pkg := c.Packages["app"]
	if pkg == nil {
		t.Fatalf("package app missing")
	}
	var vd *ir.VarDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if v, ok := st.(*ir.VarDeclStmt); ok && len(v.Targets) == 1 && v.Targets[0].Name != nil && v.Targets[0].Name.Name == "ButtonCSS" {
				vd = v
			}
		}
	}
	if vd == nil {
		t.Fatalf("ButtonCSS decl missing")
	}
	if vd.Embed == nil {
		t.Fatalf("Embed info not populated; pass_resolve_embeds did not fire")
	}
	if vd.Embed.Kind != ir.EmbedKindText {
		t.Fatalf("expected text kind, got %v", vd.Embed.Kind)
	}
	if vd.Embed.SizeBytes != int64(len(".btn { color: red; }")) {
		t.Fatalf("size mismatch: want %d, got %d", len(".btn { color: red; }"), vd.Embed.SizeBytes)
	}
	if len(vd.Embed.ContentHash) != 16 {
		t.Fatalf("hash should be 16 hex chars, got %q", vd.Embed.ContentHash)
	}

	records, _ := c.Cache[passes.EmbedAssetsCacheKey].([]*passes.EmbedRecord)
	if len(records) != 1 {
		t.Fatalf("registry should contain 1 record, got %d", len(records))
	}
	if records[0].Info != vd.Embed {
		t.Fatalf("registry record info should reference the same EmbedInfo as the VarDeclStmt")
	}
}

func TestEmbedBytesResolvesWithCorrectKind(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "logo.bin")
	if err := os.WriteFile(binPath, []byte{0x89, 0x50, 0x4E, 0x47}, 0o644); err != nil {
		t.Fatalf("write bin: %v", err)
	}
	src := `package app on backend

@embed("./logo.bin")
const Logo: []byte = []
`
	c := newEmbedTestContext(t, dir)
	c.AddSource("main.sova", src)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["app"]
	var vd *ir.VarDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if v, ok := st.(*ir.VarDeclStmt); ok && len(v.Targets) == 1 && v.Targets[0].Name != nil && v.Targets[0].Name.Name == "Logo" {
				vd = v
			}
		}
	}
	if vd == nil || vd.Embed == nil {
		t.Fatalf("Logo decl or embed info missing")
	}
	if vd.Embed.Kind != ir.EmbedKindBytes {
		t.Fatalf("expected bytes kind, got %v", vd.Embed.Kind)
	}
}

func TestEmbedMissingFileDiagnoses(t *testing.T) {
	dir := t.TempDir()
	src := `package app on backend

@embed("./missing.css")
const Missing: string = ""
`
	c := newEmbedTestContext(t, dir)
	c.AddSource("main.sova", src)
	_ = c.Check()
	if !c.Diag.Errored() {
		t.Fatalf("expected file-not-found diagnostic, got none")
	}
	found := false
	for _, d := range c.Diag.Diagnostics() {
		if strings.Contains(d.Msg, "cannot find file") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no file-not-found diagnostic: %+v", c.Diag.Diagnostics())
	}
}

func TestEmbedNonConstDiagnoses(t *testing.T) {
	dir := t.TempDir()
	cssPath := filepath.Join(dir, "Button.css")
	if err := os.WriteFile(cssPath, []byte(".btn{}"), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}
	src := `package app on backend

@embed("./Button.css")
let ButtonCSS: string = ""
`
	c := newEmbedTestContext(t, dir)
	c.AddSource("main.sova", src)
	_ = c.Check()
	if !c.Diag.Errored() {
		t.Fatalf("expected non-const diagnostic, got none")
	}
	found := false
	for _, d := range c.Diag.Diagnostics() {
		if strings.Contains(d.Msg, "requires a `const`") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no non-const diagnostic: %+v", c.Diag.Diagnostics())
	}
}

func TestEmbedBadTypeDiagnoses(t *testing.T) {
	dir := t.TempDir()
	cssPath := filepath.Join(dir, "Button.css")
	if err := os.WriteFile(cssPath, []byte(".btn{}"), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}
	src := `package app on backend

@embed("./Button.css")
const ButtonCSS: int = 0
`
	c := newEmbedTestContext(t, dir)
	c.AddSource("main.sova", src)
	_ = c.Check()
	if !c.Diag.Errored() {
		t.Fatalf("expected bad-type diagnostic, got none")
	}
	found := false
	for _, d := range c.Diag.Diagnostics() {
		if strings.Contains(d.Msg, "requires a declared type") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no bad-type diagnostic: %+v", c.Diag.Diagnostics())
	}
}

// newEmbedTestContext creates a CompilerContext rooted at dir so the embed resolver sees the on-disk test files. The BuildConfig is wired under the same cache key the passes look up so SourceDirectory() returns the temp dir; we use a local stub rather than `cli.BuildConfig` because the cli package depends on the compiler package and a test import would form a cycle.
func newEmbedTestContext(t *testing.T, dir string) *CompilerContext {
	t.Helper()
	c := New()
	c.SetBuildConfig("build_config", embedTestConfig{src: dir})
	return c
}

func TestEmbedOnTypeFieldInlinesContent(t *testing.T) {
	dir := t.TempDir()
	cssPath := filepath.Join(dir, "Button.css")
	cssContent := ".btn { color: rebeccapurple; }"
	if err := os.WriteFile(cssPath, []byte(cssContent), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}
	src := `package app on frontend

type Button {
    @embed("./Button.css")
    __styleSource: string = ""
}
`
	c := newEmbedTestContext(t, dir)
	c.AddSource("main.sova", src)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["app"]
	var td *ir.TypeDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if t, ok := st.(*ir.TypeDeclStmt); ok && t.Name.Name == "Button" {
				td = t
			}
		}
	}
	if td == nil || len(td.Fields) != 1 {
		t.Fatalf("Button type missing or field count off")
	}
	fld := td.Fields[0]
	if fld.Embed == nil {
		t.Fatalf("field Embed info not populated")
	}
	lit, ok := fld.Default.(*ir.LitString)
	if !ok {
		t.Fatalf("field Default should be a string literal after resolution, got %T", fld.Default)
	}
	if lit.Value != cssContent {
		t.Fatalf("inlined content mismatch:\nwant %q\ngot  %q", cssContent, lit.Value)
	}
}

func TestStyleFileSynthLowersToEmbedAndStyleMethod(t *testing.T) {
	dir := t.TempDir()
	annoDir := filepath.Join(dir, "strix-annos")
	if err := os.MkdirAll(annoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(annoDir, "annotations.sova"), []byte(`package strixAnnos on synth

synth StyleFile(path: string) on frontend type T {
    emit @embed(path) private __strixStyleSource: string = ""
    emit func style(): string {
        return this.__strixStyleSource
    }
}
`), 0o644); err != nil {
		t.Fatalf("write synth: %v", err)
	}
	cssContent := ".btn { padding: 8px; }"
	if err := os.WriteFile(filepath.Join(dir, "Button.css"), []byte(cssContent), 0o644); err != nil {
		t.Fatalf("write css: %v", err)
	}
	src := `package app on frontend

import "strixAnnos" using *

@StyleFile("./Button.css")
type Button {
    label: string = ""
}
`
	c := newEmbedTestContext(t, dir)
	c.AddSource("strix-annos/annotations.sova", `package strixAnnos on synth

synth StyleFile(path: string) on frontend type T {
    emit @embed(path) private __strixStyleSource: string = ""
    emit func style(): string {
        return this.__strixStyleSource
    }
}
`)
	c.AddSource("main.sova", src)
	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
	pkg := c.Packages["app"]
	var td *ir.TypeDeclStmt
	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			if t, ok := st.(*ir.TypeDeclStmt); ok && t.Name.Name == "Button" {
				td = t
			}
		}
	}
	if td == nil {
		t.Fatalf("Button missing")
	}
	var styleField *ir.TypeField
	for _, fld := range td.Fields {
		if fld.Name.Name == "__strixStyleSource" {
			styleField = fld
		}
	}
	if styleField == nil {
		t.Fatalf("__strixStyleSource field not injected by @StyleFile; fields=%v", fieldNames(td.Fields))
	}
	if styleField.Embed == nil {
		t.Fatalf("injected field's Embed info missing — synth-param substitution into @embed(path) did not chain to resolve_embeds")
	}
	lit, ok := styleField.Default.(*ir.LitString)
	if !ok || lit.Value != cssContent {
		t.Fatalf("inlined CSS content missing; default=%T value=%v", styleField.Default, styleField.Default)
	}
	var styleMethod *ir.TypeMethodDecl
	for _, m := range td.Methods {
		if m.Func != nil && m.Func.Name.Name == "style" {
			styleMethod = m
		}
	}
	if styleMethod == nil {
		t.Fatalf("style() method not injected; methods=%v", methodNames(td.Methods))
	}
}

func fieldNames(fields []*ir.TypeField) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.Name.Name)
	}
	return out
}

func TestEmbedOnScssWithoutPreprocessorDiagnoses(t *testing.T) {
	dir := t.TempDir()
	scssPath := filepath.Join(dir, "Button.scss")
	if err := os.WriteFile(scssPath, []byte(`.btn { color: red; }`), 0o644); err != nil {
		t.Fatalf("write scss: %v", err)
	}
	src := `package app on backend

@embed("./Button.scss")
const ButtonCSS: string = ""
`
	c := New()
	c.SetBuildConfig("build_config", embedTestConfig{src: dir, scssDisabled: true})
	c.AddSource("main.sova", src)
	_ = c.Check()
	if !c.Diag.Errored() {
		t.Fatalf("expected SCSS-unavailable diagnostic, got none")
	}
	found := false
	for _, d := range c.Diag.Diagnostics() {
		if strings.Contains(d.Msg, "no Sass preprocessor is available") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no SCSS-unavailable diagnostic: %+v", c.Diag.Diagnostics())
	}
}

func methodNames(methods []*ir.TypeMethodDecl) []string {
	out := make([]string, 0, len(methods))
	for _, m := range methods {
		if m.Func != nil {
			out = append(out, m.Func.Name.Name)
		}
	}
	return out
}
