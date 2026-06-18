// Package bundler turns the Sova JS emitter's single monolithic `output.js`
// into a real production bundle — minified, tree-shaken, content-hashed —
// using esbuild as an in-process Go library. The output is a small set of
// files under `<outputDir>/assets/` plus a `manifest.json` mapping logical
// names (`entry`, `entry.map`) to their hashed on-disk filenames. The
// prod_helpers.go template embeds the whole `assets/` directory via
// `//go:embed assets` (an `embed.FS`) and serves files by name from
// `manifest.json` — so a new build with new hashes "just works" without
// changing any directive paths.
//
// This package is invoked only by `sova build` (production mode). `sova dev`
// continues to serve the unbundled `output.js` from disk over SSE-driven
// live reload — fastest possible iteration without any minification noise.
package bundler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evanw/esbuild/pkg/api"
)

// Options is the user-facing knob set for one bundler invocation. `EntryJS` is the path to the emitter's `output.js` (with a sibling `output.js.map`); `OutputDir` is where the bundled assets land (the bundler creates an `assets/` subdirectory underneath); `Minify` toggles esbuild's minify suite (whitespace + identifiers + syntax). `KeepNames` preserves wired function names so backend HTTP routes stay introspectable after minification.
type Options struct {
	EntryJS   string
	OutputDir string
	Minify    bool
	KeepNames bool
	NodePaths []string
}

// Result is what the bundler returns: filenames of the bundled outputs (relative to OutputDir/assets/) plus the path to the written `manifest.json`. The HTML staging step reads these to inject hashed `<script>` tags into `index.html`.
type Result struct {
	AssetsDir    string
	EntryJS      string // e.g. "runtime.abc123.js"
	EntryJSMap   string // e.g. "runtime.abc123.js.map"
	ManifestPath string
}

// Manifest is the JSON document written to `<assetsDir>/manifest.json`. Keys are logical names (`entry`, `entry.map`, future: `css`, individual chunks); values are filenames *relative to the assets directory*. The prod runtime reads this at startup to know which hashed filename to serve for `/__sova/runtime.js`.
type Manifest struct {
	Entry    string `json:"entry"`
	EntryMap string `json:"entry.map,omitempty"`
}

// Run executes the bundle. Returns a populated Result on success; on esbuild errors the returned error wraps a multi-line message containing every esbuild diagnostic. The caller (build.go) prints them and aborts the build.
func Run(opts Options) (*Result, error) {
	if opts.EntryJS == "" {
		return nil, fmt.Errorf("bundler: EntryJS is required")
	}
	if opts.OutputDir == "" {
		return nil, fmt.Errorf("bundler: OutputDir is required")
	}
	assetsDir, err := filepath.Abs(filepath.Join(opts.OutputDir, "assets"))
	if err != nil {
		return nil, fmt.Errorf("bundler: resolve assets dir: %w", err)
	}
	if err := os.RemoveAll(assetsDir); err != nil {
		return nil, fmt.Errorf("bundler: clean assets dir: %w", err)
	}
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return nil, fmt.Errorf("bundler: create assets dir: %w", err)
	}

	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{opts.EntryJS},
		Bundle:            true,
		Outdir:            assetsDir,
		EntryNames:        "runtime.[hash]",
		ChunkNames:        "chunk.[hash]",
		AssetNames:        "asset.[hash]",
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		Target:            api.ES2022,
		Sourcemap:         api.SourceMapLinked,
		SourcesContent:    api.SourcesContentInclude,
		MinifyWhitespace:  opts.Minify,
		MinifyIdentifiers: opts.Minify,
		MinifySyntax:      opts.Minify,
		KeepNames:         opts.KeepNames,
		TreeShaking:       api.TreeShakingTrue,
		LegalComments:     api.LegalCommentsLinked,
		NodePaths:         opts.NodePaths,
		Loader:            staticAssetLoaders(),
		Write:             true,
	})
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("bundler: esbuild reported %d error(s): %s", len(result.Errors), formatEsbuildMessages(result.Errors))
	}

	manifest, err := buildManifest(result.OutputFiles, assetsDir)
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(assetsDir, "manifest.json")
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return nil, fmt.Errorf("bundler: write manifest: %w", err)
	}

	return &Result{
		AssetsDir:    assetsDir,
		EntryJS:      manifest.Entry,
		EntryJSMap:   manifest.EntryMap,
		ManifestPath: manifestPath,
	}, nil
}

// buildManifest scans the esbuild OutputFile slice and picks the entry .js + its .map. Multi-chunk and CSS handling is wired in later phases; for the first cut we expect exactly one entry + map.
func buildManifest(files []api.OutputFile, assetsDir string) (Manifest, error) {
	var m Manifest
	for _, f := range files {
		rel, err := filepath.Rel(assetsDir, f.Path)
		if err != nil {
			continue
		}
		ext := filepath.Ext(rel)
		switch ext {
		case ".js":
			if m.Entry == "" {
				m.Entry = filepath.ToSlash(rel)
			}
		case ".map":
			if m.EntryMap == "" {
				m.EntryMap = filepath.ToSlash(rel)
			}
		}
	}
	if m.Entry == "" {
		return m, fmt.Errorf("bundler: esbuild produced no .js entry under %s", assetsDir)
	}
	return m, nil
}

// staticAssetLoaders maps common static-asset file extensions to esbuild loaders. The `file` loader copies the file into the assets dir with the configured `AssetNames` hash pattern and rewrites the JS import to the resulting URL string; `text` keeps SVG as inline strings (useful for icon-as-string patterns and inline-CSS background-image data). Anything not listed here falls through to esbuild's defaults — JS/TS/CSS/JSON are handled natively, unknown extensions emit a clear esbuild error.
func staticAssetLoaders() map[string]api.Loader {
	return map[string]api.Loader{
		".png":   api.LoaderFile,
		".jpg":   api.LoaderFile,
		".jpeg":  api.LoaderFile,
		".gif":   api.LoaderFile,
		".webp":  api.LoaderFile,
		".avif":  api.LoaderFile,
		".ico":   api.LoaderFile,
		".bmp":   api.LoaderFile,
		".svg":   api.LoaderFile,
		".woff":  api.LoaderFile,
		".woff2": api.LoaderFile,
		".ttf":   api.LoaderFile,
		".otf":   api.LoaderFile,
		".eot":   api.LoaderFile,
		".mp3":   api.LoaderFile,
		".mp4":   api.LoaderFile,
		".webm":  api.LoaderFile,
		".mov":   api.LoaderFile,
		".pdf":   api.LoaderFile,
	}
}

func formatEsbuildMessages(msgs []api.Message) string {
	out := ""
	for i, msg := range msgs {
		if i > 0 {
			out += "\n"
		}
		if msg.Location != nil {
			out += fmt.Sprintf("  %s:%d:%d: %s", msg.Location.File, msg.Location.Line, msg.Location.Column, msg.Text)
		} else {
			out += "  " + msg.Text
		}
	}
	return out
}
