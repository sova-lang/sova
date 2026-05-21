package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"sova/internal/services/compiler"
	"sova/internal/termui"
)

// defaultProdShell is the HTML page emitted into the prod binary when the project has no `web/index.html` override. Loads the embedded JS bundle from `/__sova/runtime.js` into a `#app` mount point - matches the dev-mode default shape so frameworks bind to the same DOM ID in either mode.
const defaultProdShell = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sova App</title>
</head>
<body>
<div id="app"></div>
<script type="module" src="/__sova/runtime.js"></script>
</body>
</html>`

// runBuild is the entry point for `sova build`. It compiles Sova source in production mode (emits embedded asset helpers, no dev gates) and then invokes `go build` once per target, cross-compiling via GOOS/GOARCH.
func runBuild(cfg BuildConfig, targets []buildTarget, distDir string, stripDebug bool) error {
	cfg.ProdMode = true

	root, files, err := collectSources(cfg)
	if err != nil {
		return err
	}
	cfg.SourceDir = root

	termui.Header("sova build")
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

	emittedJS := filepath.Join(cfg.OutputDir, cfg.OutputName+".js")
	embedJS := filepath.Join(cfg.OutputDir, "output.js")
	if _, err := os.Stat(emittedJS); err == nil && emittedJS != embedJS {
		if data, err := os.ReadFile(emittedJS); err == nil {
			_ = os.WriteFile(embedJS, data, 0o644)
		}
	}
	if _, err := os.Stat(embedJS); os.IsNotExist(err) {
		if err := os.WriteFile(embedJS, []byte("// no frontend code\n"), 0o644); err != nil {
			return err
		}
	}
	emittedMap := emittedJS + ".map"
	embedMap := embedJS + ".map"
	if _, err := os.Stat(emittedMap); err == nil && emittedMap != embedMap {
		if data, err := os.ReadFile(emittedMap); err == nil {
			_ = os.WriteFile(embedMap, data, 0o644)
		}
	}
	if _, err := os.Stat(embedMap); os.IsNotExist(err) {
		if err := os.WriteFile(embedMap, []byte(`{"version":3,"sources":[],"names":[],"mappings":""}`), 0o644); err != nil {
			return err
		}
	}

	embedHTML := filepath.Join(cfg.OutputDir, "output.html")
	webDir := cfg.ServeWebDir
	if webDir == "" {
		webDir = "web"
	}
	userIndex := filepath.Join(cfg.SourceDir, webDir, "index.html")
	if data, err := os.ReadFile(userIndex); err == nil {
		_ = os.WriteFile(embedHTML, data, 0o644)
		termui.Info(fmt.Sprintf("using %s as prod HTML shell", userIndex))
	} else {
		if err := os.WriteFile(embedHTML, []byte(defaultProdShell), 0o644); err != nil {
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
