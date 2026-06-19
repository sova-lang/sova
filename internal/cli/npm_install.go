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

type NPMResult struct {
	BindingsRoot    string
	NodeModulesPath string
}

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
