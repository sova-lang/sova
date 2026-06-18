package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"sova/internal/services/pkgmgr"
)

// NPMResult is what materializeNPMDeps returns after a successful install + generate cycle. The fields plug directly into the rest of the build flow: BindingsRoot is added as a search path for `import "npm:..."` resolution, NodeModulesPath becomes the bundler's NodePaths entry so esbuild can find the actual JS at bundle time.
type NPMResult struct {
	BindingsRoot    string
	NodeModulesPath string
}

// materializeNPMDeps reads the project manifest, installs declared npm packages into a hidden cache, runs ts2sova-generator on each, and writes the resulting Sova binding files to disk. Idempotent and cache-aware: a hash of the npm-deps section is stored in `.sova/npm/.cache-key`, and an install + regenerate only runs when the hash changes (or when a binding file is missing).
//
// Layout under `.sova/npm/`:
//
//   .sova/npm/
//     .cache-key            sha256 of the npm-deps table, written after a successful run
//     package.json          synthetic, regenerated each run from npm-deps
//     package-lock.json     npm's lockfile (auto-generated, never moved into source tree)
//     node_modules/         npm install output
//     bindings/<libname>/   generated Sova sources, one dir per declared dep
//       sova.toml
//       <libname>.sova
//
// Returns nil + no error if the manifest declares no npm deps (skipping the install entirely).
func materializeNPMDeps(projectRoot string) (*NPMResult, error) {
	m, _, err := pkgmgr.LoadManifest(filepath.Join(projectRoot, "sova.toml"))
	if err != nil || m == nil {
		return nil, nil
	}
	directDeps := map[string]pkgmgr.NPMDepSpec{}
	for k, v := range m.NPM {
		directDeps[k] = v
	}
	allDeps := map[string]pkgmgr.NPMDepSpec{}
	for k, v := range directDeps {
		allDeps[k] = v
	}
	visited := map[string]bool{}
	visited[mustAbs(projectRoot)] = true
	collectNPMDepsFromDeps(projectRoot, m, allDeps, visited)
	if len(allDeps) == 0 {
		return nil, nil
	}
	m.NPM = allDeps

	cacheDir := filepath.Join(projectRoot, ".sova", "npm")
	bindingsRoot := filepath.Join(cacheDir, "bindings")
	nodeModules := filepath.Join(cacheDir, "node_modules")

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("npm: create cache dir: %w", err)
	}

	cacheKey := computeNPMCacheKey(m.NPM)
	keyFile := filepath.Join(cacheDir, ".cache-key")
	prevKey, _ := os.ReadFile(keyFile)
	upToDate := string(prevKey) == cacheKey && allBindingsPresent(bindingsRoot, directDeps)

	if !upToDate {
		direct := len(directDeps)
		transitive := len(allDeps) - direct
		fmt.Printf("-> installing %d npm package(s) into %s (%d direct, %d transitive)\n", len(m.NPM), relPath(projectRoot, cacheDir), direct, transitive)
		if err := writeSyntheticPackageJSON(cacheDir, m.NPM); err != nil {
			return nil, err
		}
		if err := runNPMInstall(cacheDir); err != nil {
			return nil, err
		}
		installTypesFallbacks(cacheDir, m.NPM)
		if direct > 0 {
			fmt.Println("-> generating Sova bindings via ts2sova-generator (direct deps only)")
			if err := runTs2SovaForAll(cacheDir, bindingsRoot, directDeps); err != nil {
				return nil, err
			}
		}
		if err := os.WriteFile(keyFile, []byte(cacheKey), 0o644); err != nil {
			return nil, fmt.Errorf("npm: write cache key: %w", err)
		}
	} else {
		fmt.Println("ok npm bindings cache up-to-date")
	}

	return &NPMResult{
		BindingsRoot:    bindingsRoot,
		NodeModulesPath: nodeModules,
	}, nil
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// collectNPMDepsFromDeps walks `m`'s direct [dependencies] (path + workspace forms) and merges every transitively-reachable `[npm-dependencies]` entry into `collected`. First-wins on alias collisions: a consumer's pinned version overrides what a dep declares (consumer's NPM entries should already be in `collected` before this call). Git/index deps are skipped here — they'd be materialised on disk by `sova pkg install` and we don't recurse into the index cache for this prebuild step.
func collectNPMDepsFromDeps(dir string, m *pkgmgr.Manifest, collected map[string]pkgmgr.NPMDepSpec, visited map[string]bool) {
	if m == nil {
		return
	}
	for _, depSpec := range m.Dependencies {
		childDir := ""
		if depSpec.Path != "" {
			childDir = depSpec.Path
			if !filepath.IsAbs(childDir) {
				childDir = filepath.Join(dir, childDir)
			}
			if depSpec.Subdir != "" {
				childDir = filepath.Join(childDir, depSpec.Subdir)
			}
		}
		if childDir == "" {
			continue
		}
		visitDir(childDir, collected, visited)
	}

	if m.Workspace != nil {
		for _, member := range m.Workspace.Members {
			if strings.ContainsAny(member, "*?[") {
				continue
			}
			memberDir := member
			if !filepath.IsAbs(memberDir) {
				memberDir = filepath.Join(dir, memberDir)
			}
			visitDir(memberDir, collected, visited)
		}
	}
}

// visitDir loads a dep's manifest, records its own [npm-dependencies] (first-wins) into `collected`, then recurses into its own deps. Caller-side dedupe via `visited` keyed by absolute dir prevents infinite loops on cyclic graphs and re-walking shared deps.
func visitDir(dir string, collected map[string]pkgmgr.NPMDepSpec, visited map[string]bool) {
	abs := mustAbs(dir)
	if visited[abs] {
		return
	}
	visited[abs] = true
	manifest, _, err := pkgmgr.LoadManifest(filepath.Join(dir, "sova.toml"))
	if err != nil || manifest == nil {
		return
	}
	for alias, spec := range manifest.NPM {
		if _, ok := collected[alias]; !ok {
			collected[alias] = spec
		}
	}
	collectNPMDepsFromDeps(dir, manifest, collected, visited)
}

// computeNPMCacheKey produces a deterministic hash of the npm-deps table. Sorted keys → identical input yields identical key regardless of TOML decode iteration order. Changing a version, adding a dep, or flipping `default` invalidates the cache and triggers a reinstall + regenerate.
func computeNPMCacheKey(deps map[string]pkgmgr.NPMDepSpec) string {
	names := make([]string, 0, len(deps))
	for k := range deps {
		names = append(names, k)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		d := deps[name]
		fmt.Fprintf(h, "%s|%s|%s|", name, d.Version, d.Package)
		if d.Default != nil {
			fmt.Fprintf(h, "%v|", *d.Default)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func allBindingsPresent(bindingsRoot string, deps map[string]pkgmgr.NPMDepSpec) bool {
	for alias, dep := range deps {
		bindingFile := filepath.Join(bindingsRoot, alias, alias+".sova")
		_ = dep
		if _, err := os.Stat(bindingFile); err != nil {
			return false
		}
	}
	return true
}

func writeSyntheticPackageJSON(cacheDir string, deps map[string]pkgmgr.NPMDepSpec) error {
	depEntries := map[string]string{}
	for alias, dep := range deps {
		pkgName := dep.Package
		if pkgName == "" {
			pkgName = alias
		}
		ver := dep.Version
		if ver == "" {
			ver = "latest"
		}
		depEntries[pkgName] = ver
	}
	pkg := map[string]any{
		"name":         "sova-npm-cache",
		"private":      true,
		"version":      "0.0.0",
		"dependencies": depEntries,
	}
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("npm: marshal package.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "package.json"), data, 0o644); err != nil {
		return fmt.Errorf("npm: write package.json: %w", err)
	}
	return nil
}

// installTypesFallbacks probes each declared dep for missing .d.ts and tries to install the corresponding `@types/...` package. Failures are tolerated (the lib genuinely has no types package on npm). Runs after the primary install completed so node_modules exists for the probe.
func installTypesFallbacks(cacheDir string, deps map[string]pkgmgr.NPMDepSpec) {
	for alias, dep := range deps {
		pkgName := dep.Package
		if pkgName == "" {
			pkgName = alias
		}
		if hasOwnDts(cacheDir, pkgName) {
			continue
		}
		typesPkg := typesFallback(pkgName)
		if typesPkg == "" {
			continue
		}
		cmd := exec.Command("npm", "install", "--silent", "--no-audit", "--no-fund", "--no-save", "--no-progress", typesPkg)
		cmd.Dir = cacheDir
		_ = cmd.Run()
	}
}

func hasOwnDts(cacheDir, pkgName string) bool {
	pkgRoot := filepath.Join(cacheDir, "node_modules", pkgName)
	if _, err := os.Stat(filepath.Join(pkgRoot, "index.d.ts")); err == nil {
		return true
	}
	pj := filepath.Join(pkgRoot, "package.json")
	data, err := os.ReadFile(pj)
	if err != nil {
		return false
	}
	var meta struct {
		Types   string `json:"types"`
		Typings string `json:"typings"`
	}
	_ = json.Unmarshal(data, &meta)
	return meta.Types != "" || meta.Typings != ""
}

// typesFallback returns the corresponding `@types/...` package name when a lib commonly publishes its types separately. The install step always tries to pull types in parallel; a missing @types package is non-fatal (npm just errors on that one).
func typesFallback(pkgName string) string {
	if strings.HasPrefix(pkgName, "@types/") {
		return ""
	}
	if strings.HasPrefix(pkgName, "@") {
		parts := strings.SplitN(pkgName[1:], "/", 2)
		if len(parts) != 2 {
			return ""
		}
		return "@types/" + parts[0] + "__" + parts[1]
	}
	return "@types/" + pkgName
}

func runNPMInstall(cacheDir string) error {
	cmd := exec.Command("npm", "install", "--silent", "--no-audit", "--no-fund", "--no-progress")
	cmd.Dir = cacheDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm install failed in %s: %w", cacheDir, err)
	}
	return nil
}

// runTs2SovaForAll invokes the ts2sova-generator binary once per declared npm dep, writing the generated Sova source into `<bindingsRoot>/<alias>/<alias>.sova`. The generator is located via SOVA_TS2SOVA_BUNDLE (a node-compatible bundle, future shipping plan), falling back to bun-run on the in-repo dev source for local development.
func runTs2SovaForAll(cacheDir, bindingsRoot string, deps map[string]pkgmgr.NPMDepSpec) error {
	if err := os.MkdirAll(bindingsRoot, 0o755); err != nil {
		return fmt.Errorf("npm: create bindings root: %w", err)
	}
	runner, runnerArgs, err := resolveTs2SovaRunner()
	if err != nil {
		return err
	}
	aliases := make([]string, 0, len(deps))
	for k := range deps {
		aliases = append(aliases, k)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		dep := deps[alias]
		pkgName := dep.Package
		if pkgName == "" {
			pkgName = alias
		}
		outDir := filepath.Join(bindingsRoot, alias)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return fmt.Errorf("npm: create binding dir for %s: %w", alias, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, "sova.toml"), []byte(fmt.Sprintf("[package]\nname = %q\n", alias)), 0o644); err != nil {
			return fmt.Errorf("npm: write binding sova.toml for %s: %w", alias, err)
		}
		args := append([]string{}, runnerArgs...)
		args = append(args,
			"--lib", pkgName,
			"--from", cacheDir,
			"--out", outDir,
			"--package", alias,
			"--js-import", pkgName,
		)
		if dep.Default != nil {
			if *dep.Default {
				args = append(args, "--default")
			} else {
				args = append(args, "--no-default")
			}
		}
		cmd := exec.Command(runner, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ts2sova-generator failed for %s: %w", alias, err)
		}
		oldName := filepath.Join(outDir, pkgName+".sova")
		newName := filepath.Join(outDir, alias+".sova")
		if oldName != newName {
			if _, err := os.Stat(oldName); err == nil {
				_ = os.Rename(oldName, newName)
			}
		}
	}
	return nil
}

// resolveTs2SovaRunner returns the executable and its leading args to invoke the generator. Resolution order:
//  1. SOVA_TS2SOVA_BUNDLE env var → `node <bundle.js>` (production path; the bundle ships with Sova).
//  2. SOVA_TS2SOVA_DEV_REPO env var → `bun run <repo>/src/main.ts` (developer convenience).
//  3. Discovery: ../ts2sova-generator/src/main.ts next to the Sova repo (dev fallback).
//
// Returning an error here is fatal: there's no way to install npm deps without the generator.
func resolveTs2SovaRunner() (string, []string, error) {
	if bundle := os.Getenv("SOVA_TS2SOVA_BUNDLE"); bundle != "" {
		if _, err := os.Stat(bundle); err == nil {
			return "node", []string{bundle}, nil
		}
	}
	if dev := os.Getenv("SOVA_TS2SOVA_DEV_REPO"); dev != "" {
		entry := filepath.Join(dev, "src", "main.ts")
		if _, err := os.Stat(entry); err == nil {
			return "bun", []string{"run", entry}, nil
		}
	}
	exe, err := os.Executable()
	if err == nil {
		guess := filepath.Join(filepath.Dir(exe), "..", "..", "ts2sova-generator", "src", "main.ts")
		if _, err := os.Stat(guess); err == nil {
			return "bun", []string{"run", guess}, nil
		}
	}
	return "", nil, fmt.Errorf("ts2sova-generator not found: set SOVA_TS2SOVA_BUNDLE=/path/to/bundle.js (prod) or SOVA_TS2SOVA_DEV_REPO=/path/to/ts2sova-generator (dev)")
}

func relPath(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}
