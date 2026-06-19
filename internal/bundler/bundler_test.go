package bundler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundlerProducesHashedEntryAndManifest(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "output.js")
	if err := os.WriteFile(entry, []byte(`export const hello = () => "world";
console.log(hello());`), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	res, err := Run(Options{
		EntryJS:   entry,
		OutputDir: dir,
		Minify:    true,
	})
	if err != nil {
		t.Fatalf("bundler: %v", err)
	}

	if res.EntryJS == "" {
		t.Fatalf("entry filename should be populated")
	}

	if !strings.HasPrefix(res.EntryJS, "runtime.") || !strings.HasSuffix(res.EntryJS, ".js") {
		t.Errorf("entry filename should be runtime.[hash].js, got %q", res.EntryJS)
	}

	entryPath := filepath.Join(res.AssetsDir, res.EntryJS)
	if info, err := os.Stat(entryPath); err != nil || info.Size() == 0 {
		t.Fatalf("entry file should exist and be non-empty at %s", entryPath)
	}

	manifestData, err := os.ReadFile(res.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}

	if manifest.Entry != res.EntryJS {
		t.Errorf("manifest entry %q != result entry %q", manifest.Entry, res.EntryJS)
	}

	if manifest.EntryMap == "" || !strings.HasSuffix(manifest.EntryMap, ".js.map") {
		t.Errorf("manifest should contain a sourcemap path, got %q", manifest.EntryMap)
	}
}

func TestBundlerMinifiesAndDropsWhitespace(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "output.js")
	verbose := `
		const computeSomethingExpensive = (input) => {
			const intermediate = input * 2;
			const result = intermediate + 1;
			return result;
		};
		console.log(computeSomethingExpensive(21));
	`
	if err := os.WriteFile(entry, []byte(verbose), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	res, err := Run(Options{EntryJS: entry, OutputDir: dir, Minify: true})
	if err != nil {
		t.Fatalf("bundler: %v", err)
	}

	bundled, err := os.ReadFile(filepath.Join(res.AssetsDir, res.EntryJS))
	if err != nil {
		t.Fatalf("read bundled: %v", err)
	}

	bundledStr := string(bundled)
	if len(bundledStr) >= len(verbose) {
		t.Errorf("minified bundle should be smaller than source; src=%d bundle=%d", len(verbose), len(bundledStr))
	}

	if strings.Contains(bundledStr, "computeSomethingExpensive") {
		t.Errorf("minified bundle should rename or inline the identifier, got verbatim: %s", bundledStr)
	}
}

func TestBundlerErrorsSurfaceAsGoError(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "output.js")
	if err := os.WriteFile(entry, []byte(`import { missing } from "./does-not-exist";`), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	_, err := Run(Options{EntryJS: entry, OutputDir: dir, Minify: false})
	if err == nil {
		t.Fatalf("expected esbuild error for missing import, got nil")
	}

	if !strings.Contains(err.Error(), "esbuild") {
		t.Errorf("error should mention esbuild, got: %v", err)
	}
}
