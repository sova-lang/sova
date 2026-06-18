package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"sova/internal/bundler"
	"sova/internal/services/compiler"
	"sova/internal/termui"
)

// defaultProdShell is the HTML page emitted into the prod binary when the project has no `web/index.html` override. The `__SOVA_RUNTIME__` placeholder is rewritten to the hashed `/__sova/<runtime.[hash].js>` path after bundling so the browser gets the content-addressed filename (cache-bustable for free).
const defaultProdShell = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sova App</title>
</head>
<body>
<div id="app"></div>
<script type="module" src="__SOVA_RUNTIME__"></script>
</body>
</html>`

// runtimeScriptSrcRE finds the `<script type="module" src="/__sova/runtime.js">` reference (with arbitrary whitespace + quote style) in the user's `web/index.html` so the build step can rewrite its `src` attribute to the bundler's hashed entry filename. Users that don't include this tag get an injected one before `</body>`.
var runtimeScriptSrcRE = regexp.MustCompile(`(<script[^>]*?\bsrc\s*=\s*["'])/__sova/runtime\.js(["'])`)

// runBuild is the entry point for `sova build`. It compiles Sova source in production mode (emits embedded asset helpers, no dev gates) and then invokes `go build` once per target, cross-compiling via GOOS/GOARCH.
func runBuild(cfg BuildConfig, targets []buildTarget, distDir string, stripDebug bool) error {
	cfg.ProdMode = true

	root, files, err := collectSources(cfg)
	if err != nil {
		return err
	}
	cfg.SourceDir = root

	termui.Header("sova build")
	npmResult, err := materializeNPMDeps(root)
	if err != nil {
		return err
	}
	if npmResult != nil {
		ents, _ := os.ReadDir(npmResult.BindingsRoot)
		for _, ent := range ents {
			if !ent.IsDir() {
				continue
			}
			subDir := filepath.Join(npmResult.BindingsRoot, ent.Name())
			_, subFiles, err := collectSources(BuildConfig{SourceDir: subDir, OutputDir: cfg.OutputDir})
			if err != nil {
				continue
			}
			for _, sf := range subFiles {
				files = append(files, sourceFile{RelPath: filepath.Join(".sova-npm", ent.Name(), sf.RelPath), Content: sf.Content})
			}
		}
	}
	termui.Step("compiling Sova sources")
	c := compiler.New()
	c.SetBuildConfig(CacheKey, cfg)
	c.Loader = makePackageLoader(root)
	for _, src := range files {
		c.AddSource(src.RelPath, src.Content)
	}
	if err := c.Compile(); err != nil {
		c.Diag.Print()
		return err
	}
	c.Diag.Print()
	if c.Diag.Errored() {
		return fmt.Errorf("compilation failed")
	}
	termui.Success("compiled")

	if err := stageEmbedAssets(c, cfg.OutputDir); err != nil {
		return fmt.Errorf("stage @embed assets: %w", err)
	}

	emittedJS := filepath.Join(cfg.OutputDir, cfg.OutputName+".js")
	if _, err := os.Stat(emittedJS); os.IsNotExist(err) {
		stub := filepath.Join(cfg.OutputDir, "output.js")
		if err := os.WriteFile(stub, []byte("// no frontend code\n"), 0o644); err != nil {
			return err
		}
		emittedJS = stub
	}

	termui.Step("bundling frontend (esbuild)")
	var nodePaths []string
	if npmResult != nil {
		nodePaths = []string{npmResult.NodeModulesPath}
	}
	bundleResult, err := bundler.Run(bundler.Options{
		EntryJS:   emittedJS,
		OutputDir: cfg.OutputDir,
		Minify:    true,
		KeepNames: true,
		NodePaths: nodePaths,
	})
	if err != nil {
		return err
	}
	termui.Success(fmt.Sprintf("bundled → assets/%s", bundleResult.EntryJS))

	embedHTML := filepath.Join(bundleResult.AssetsDir, "index.html")
	webDir := cfg.ServeWebDir
	if webDir == "" {
		webDir = "web"
	}
	runtimeURL := "/__sova/" + bundleResult.EntryJS
	userIndex := filepath.Join(cfg.SourceDir, webDir, "index.html")
	if data, err := os.ReadFile(userIndex); err == nil {
		if err := os.WriteFile(embedHTML, []byte(rewriteUserShell(string(data), runtimeURL)), 0o644); err != nil {
			return err
		}
		termui.Info(fmt.Sprintf("using %s as prod HTML shell", userIndex))
	} else {
		shell := strings.ReplaceAll(defaultProdShell, "__SOVA_RUNTIME__", runtimeURL)
		if err := os.WriteFile(embedHTML, []byte(shell), 0o644); err != nil {
			return err
		}
	}

	if distDir == "" {
		distDir = "dist"
	}
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		return fmt.Errorf("create dist: %w", err)
	}

	baseName := cfg.OutputName
	if baseName == "" {
		baseName = "sova-app"
	}

	for _, t := range targets {
		binaryName := baseName
		if len(targets) > 1 || !t.isHost() {
			binaryName = fmt.Sprintf("%s-%s-%s", baseName, t.OS, t.Arch)
		}
		if t.OS == "windows" {
			binaryName += ".exe"
		}
		outPath, err := filepath.Abs(filepath.Join(distDir, binaryName))
		if err != nil {
			return err
		}

		args := []string{"build", "-trimpath"}
		if stripDebug {
			args = append(args, "-ldflags=-s -w")
		}
		args = append(args, "-o", outPath, ".")
		cmd := exec.Command("go", args...)
		cmd.Dir = cfg.OutputDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		env := append([]string{}, os.Environ()...)
		env = append(env, "CGO_ENABLED=0")
		env = append(env, "GOOS="+t.OS)
		env = append(env, "GOARCH="+t.Arch)
		cmd.Env = env

		termui.Step(fmt.Sprintf("linking %s/%s → %s", t.OS, t.Arch, termui.Dim(outPath)))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go build %s/%s: %w", t.OS, t.Arch, err)
		}
	}

	termui.Success("build done")
	MaybeShowUpdateNotice()
	return nil
}

type buildTarget struct {
	OS, Arch string
}

func (t buildTarget) isHost() bool {
	return t.OS == runtime.GOOS && t.Arch == runtime.GOARCH
}

// rewriteUserShell rewrites the user's `web/index.html` to reference the bundler's hashed runtime entry. The user is expected to keep a `<script type="module" src="/__sova/runtime.js"></script>` tag in their shell; we swap the static `runtime.js` for the content-hashed `runtime.[hash].js` produced by the bundler. If the tag is missing entirely we inject one before `</body>` so the page still boots.
func rewriteUserShell(html, runtimeURL string) string {
	rewritten, n := replaceFirstRuntimeSrc(html, runtimeURL)
	if n > 0 {
		return rewritten
	}
	injected := `<script type="module" src="` + runtimeURL + `"></script>`
	if idx := strings.LastIndex(html, "</body>"); idx >= 0 {
		return html[:idx] + injected + "\n" + html[idx:]
	}
	return html + "\n" + injected + "\n"
}

func replaceFirstRuntimeSrc(html, runtimeURL string) (string, int) {
	count := 0
	rewritten := runtimeScriptSrcRE.ReplaceAllStringFunc(html, func(match string) string {
		count++
		return runtimeScriptSrcRE.ReplaceAllString(match, "${1}"+runtimeURL+"${2}")
	})
	return rewritten, count
}

// parseTargets turns a comma-separated --target list into buildTarget structs. Empty input yields the host target.
func parseTargets(raw string) ([]buildTarget, error) {
	if strings.TrimSpace(raw) == "" {
		return []buildTarget{{OS: runtime.GOOS, Arch: runtime.GOARCH}}, nil
	}
	var out []buildTarget
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		parts := strings.SplitN(tok, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid target %q (expected os/arch, e.g. linux/amd64)", tok)
		}
		out = append(out, buildTarget{OS: parts[0], Arch: parts[1]})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid targets in %q", raw)
	}
	return out, nil
}
