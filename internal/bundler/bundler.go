package bundler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evanw/esbuild/pkg/api"
)

type Options struct {
	EntryJS   string
	OutputDir string
	Minify    bool
	KeepNames bool
	NodePaths []string
}

type Result struct {
	AssetsDir    string
	EntryJS      string
	EntryJSMap   string
	ManifestPath string
}

type Manifest struct {
	Entry    string `json:"entry"`
	EntryMap string `json:"entry.map,omitempty"`
}

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
