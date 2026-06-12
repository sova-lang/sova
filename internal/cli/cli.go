package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sova/internal/services/compiler"
	"sova/internal/services/pkgmgr"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// Execute is the main entry point for the Sova CLI.
func Execute() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sova",
		Short:         "Sova multi-tier language compiler",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	cmd.AddCommand(newCompileCmd())
	cmd.AddCommand(newCheckCmd())
	cmd.AddCommand(newDevCmd())
	cmd.AddCommand(newBuildCmd())
	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newTestCmd())
	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newRemoveCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newLinkCmd())
	cmd.AddCommand(newUnlinkCmd())
	cmd.AddCommand(newOutdatedCmd())
	cmd.AddCommand(newIndexCmd())
	cmd.AddCommand(newLSPCmd())
	cmd.AddCommand(newFmtCmd())
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newUpgradeCmd())
	return cmd
}

func newBuildCmd() *cobra.Command {
	var targetSpec, distDir, outName string
	var stripDebug bool
	cmd := &cobra.Command{
		Use:   "build [file|dir]",
		Short: "Compile Sova in production mode and produce a deployable binary",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfig(args, "", "", cmd)
			if err != nil {
				return err
			}
			if outName != "" {
				cfg.OutputName = outName
			}
			targets, err := parseTargets(targetSpec)
			if err != nil {
				return err
			}
			return runBuild(cfg, targets, distDir, stripDebug)
		},
	}
	cmd.Flags().StringVar(&targetSpec, "target", "", "cross-compile target(s): os/arch, comma-separated for multi (e.g. linux/amd64,darwin/arm64)")
	cmd.Flags().StringVar(&distDir, "dist", "dist", "output directory for produced binaries")
	cmd.Flags().StringVar(&outName, "name", "", "binary base name (default: sova-app or [build].output_name)")
	cmd.Flags().BoolVar(&stripDebug, "strip", true, "pass -ldflags=\"-s -w\" to shrink the binary")
	return cmd
}

// newRunCmd registers `sova run` - a one-shot prod-style local execution. Compiles the project once (no watcher, no SSE-reload helpers, no dev origin overrides), then spawns the resulting backend binary in the foreground and forwards its stdout/stderr until Ctrl-C. Useful for "run my app like prod, locally, with one command" workflows that don't want `sova dev`'s file-watching + auto-reload overhead. Cross-compile and stripping are NOT involved - for a deployable artifact use `sova build`.
func newRunCmd() *cobra.Command {
	var port int
	var host string
	var strict bool
	cmd := &cobra.Command{
		Use:   "run [file|dir]",
		Short: "Compile once in prod-style mode and run the resulting backend binary in the foreground (no file watcher, no live reload)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfig(args, "", "", cmd)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("port") {
				cfg.ServePort = port
			}
			if cmd.Flags().Changed("host") {
				cfg.ServeHost = host
			}
			if cmd.Flags().Changed("strict-port") {
				cfg.ServeStrictPort = strict
			}
			return runOnce(cfg)
		},
	}
	cmd.Flags().IntVar(&port, "port", 5173, "preferred server port (falls back to next free unless --strict-port)")
	cmd.Flags().StringVar(&host, "host", "", "server bind host (default: all interfaces)")
	cmd.Flags().BoolVar(&strict, "strict-port", false, "fail instead of incrementing when --port is taken")
	return cmd
}

func newDevCmd() *cobra.Command {
	var port int
	var host string
	var strict bool
	cmd := &cobra.Command{
		Use:   "dev [file|dir]",
		Short: "Run the Sova compiler in watch+serve mode",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfig(args, "", "", cmd)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("port") {
				cfg.ServePort = port
			}
			if cmd.Flags().Changed("host") {
				cfg.ServeHost = host
			}
			if cmd.Flags().Changed("strict-port") {
				cfg.ServeStrictPort = strict
			}
			return runDev(cfg)
		},
	}
	cmd.Flags().IntVar(&port, "port", 5173, "preferred dev server port (falls back to next free unless --strict-port)")
	cmd.Flags().StringVar(&host, "host", "", "dev server bind host (default: all interfaces)")
	cmd.Flags().BoolVar(&strict, "strict-port", false, "fail instead of incrementing when --port is taken")
	return cmd
}

func newCompileCmd() *cobra.Command {
	var outDir, outName string
	cmd := &cobra.Command{
		Use:   "compile [file]",
		Short: "Compile a Sova source file to Go and JavaScript",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfig(args, outDir, outName, cmd)
			if err != nil {
				return err
			}
			return runCompile(cfg, false)
		},
	}
	cmd.Flags().StringVarP(&outDir, "out", "o", "", "output directory (default: .output, or sova.toml [build].output_dir)")
	cmd.Flags().StringVarP(&outName, "name", "n", "", "output basename without extension (default: output, or sova.toml [build].output_name)")
	return cmd
}

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check [file]",
		Short: "Type-check a Sova source file without emitting code",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfig(args, "", "", cmd)
			if err != nil {
				return err
			}
			return runCompile(cfg, true)
		},
	}
	return cmd
}

func resolveConfig(args []string, outDir, outName string, cmd *cobra.Command) (BuildConfig, error) {
	cfg := DefaultBuildConfig()
	if len(args) == 1 {
		cfg.Entry = args[0]
	}
	if m, ok, err := LoadManifest(ManifestFilename); err != nil {
		return cfg, fmt.Errorf("read %s: %w", ManifestFilename, err)
	} else if ok {
		applyManifest(&cfg, m)
	}
	if outDir != "" {
		cfg.OutputDir = outDir
	}
	if outName != "" {
		cfg.OutputName = outName
	}
	return cfg, nil
}

func runCompile(cfg BuildConfig, checkOnly bool) error {
	root, files, err := collectSources(cfg)
	if err != nil {
		return err
	}
	cfg.SourceDir = root

	c := compiler.New()
	c.SetBuildConfig(CacheKey, cfg)
	c.Loader = makePackageLoader(root)
	for _, src := range files {
		c.AddSource(src.RelPath, src.Content)
	}

	var compileErr error
	if checkOnly {
		compileErr = c.Check()
	} else {
		compileErr = c.Compile()
	}
	c.Diag.Print()
	if compileErr != nil {
		return compileErr
	}
	if c.Diag.Errored() {
		return fmt.Errorf("compilation failed")
	}
	return nil
}

type sourceFile struct {
	RelPath string
	Content string
}

func collectSources(cfg BuildConfig) (string, []sourceFile, error) {
	if cfg.Entry != "" {
		info, err := os.Stat(cfg.Entry)
		if err != nil {
			return "", nil, fmt.Errorf("stat %s: %w", cfg.Entry, err)
		}
		if !info.IsDir() {
			entryAbs, err := filepath.Abs(cfg.Entry)
			if err != nil {
				return "", nil, err
			}
			fileDir := filepath.Dir(entryAbs)
			if projectRoot := findUpwardManifest(fileDir); projectRoot != "" {
				files, err := crawlSova(projectRoot)
				if err != nil {
					return "", nil, err
				}
				return projectRoot, files, nil
			}
			data, err := os.ReadFile(entryAbs)
			if err != nil {
				return "", nil, err
			}
			rel, err := filepath.Rel(fileDir, entryAbs)
			if err != nil {
				rel = entryAbs
			}
			return fileDir, []sourceFile{{RelPath: rel, Content: string(data)}}, nil
		}
		root, err := filepath.Abs(cfg.Entry)
		if err != nil {
			return "", nil, err
		}
		files, err := crawlSova(root)
		if err != nil {
			return "", nil, err
		}
		return root, files, nil
	}
	root, err := filepath.Abs(cfg.SourceDir)
	if err != nil {
		return "", nil, err
	}
	files, err := crawlSova(root)
	if err != nil {
		return "", nil, err
	}
	if len(files) == 0 {
		return root, nil, fmt.Errorf("no .sova files found under %s", root)
	}
	return root, files, nil
}

func makePackageLoader(root string) compiler.PackageLoader {
	var indexOnce sync.Once
	var pkgIndex map[string][]string
	return func(c *compiler.CompilerContext, pkgPath string) error {
		if dir, ok := resolveDepPath(root, pkgPath); ok {
			return loadFromDir(c, root, dir)
		}
		dir := filepath.Join(root, filepath.FromSlash(pkgPath))
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return loadFromDir(c, root, dir)
		}
		indexOnce.Do(func() {
			pkgIndex = buildPackageIndex(root)
		})
		if files, ok := pkgIndex[pkgPath]; ok && len(files) > 0 {
			for _, abs := range files {
				data, err := os.ReadFile(abs)
				if err != nil {
					return fmt.Errorf("import %q: %w", pkgPath, err)
				}
				rel, rerr := filepath.Rel(root, abs)
				if rerr != nil {
					rel = abs
				}
				c.AddSource(rel, string(data))
			}
			return nil
		}
		return fmt.Errorf("import %q: package not found (no directory %s and no file declaring `package %s` under the project)", pkgPath, dir, pkgPath)
	}
}

// buildPackageIndex walks the project source tree starting at the nearest
// sova.toml ancestor of `loaderRoot` (or `loaderRoot` itself if no manifest is
// found) and maps each `package <name>` declaration it finds to the absolute
// paths of the files that declare it. Dotted package paths preserve their full
// slash-joined form, matching what `import "<path>"` resolves to. Used by the
// loader as a fallback so projects with mixed directory layouts (e.g. `src/back`
// holding `package testBackend`) can be imported by package name regardless of
// where they live on disk.
func buildPackageIndex(loaderRoot string) map[string][]string {
	scanRoot := findUpwardManifest(loaderRoot)
	if scanRoot == "" {
		scanRoot = loaderRoot
	}
	out := map[string][]string{}
	_ = filepath.WalkDir(scanRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != scanRoot && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			if name == ".output" || name == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".sova") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if pkg := extractPackageDecl(string(data)); pkg != "" {
			out[pkg] = append(out[pkg], path)
		}
		return nil
	})
	return out
}

// extractPackageDecl returns the package path declared at the head of `content`
// (e.g. `package foo/bar` → `"foo/bar"`), or "" when the file uses the implicit
// default. Skips blank lines and `// ...` / `/* ... */` comments before the
// declaration so the scan tolerates the user's header style.
func extractPackageDecl(content string) string {
	i := 0
	n := len(content)
	for i < n {
		for i < n && (content[i] == ' ' || content[i] == '\t' || content[i] == '\r' || content[i] == '\n') {
			i++
		}
		if i+1 < n && content[i] == '/' && content[i+1] == '/' {
			for i < n && content[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < n && content[i] == '/' && content[i+1] == '*' {
			i += 2
			for i+1 < n && !(content[i] == '*' && content[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			}
			continue
		}
		break
	}
	const kw = "package"
	if i+len(kw) > n || content[i:i+len(kw)] != kw {
		return ""
	}
	j := i + len(kw)
	if j >= n || (content[j] != ' ' && content[j] != '\t') {
		return ""
	}
	for j < n && (content[j] == ' ' || content[j] == '\t') {
		j++
	}
	start := j
	for j < n {
		ch := content[j]
		if ch == '_' || ch == '/' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			j++
			continue
		}
		break
	}
	if j == start {
		return ""
	}
	return content[start:j]
}

// loadFromDir crawls all .sova files under `dir` and adds them to the compiler context, registering them by their path relative to `root` so diagnostic spans point at user-recognisable file names.
func loadFromDir(c *compiler.CompilerContext, root, dir string) error {
	files, err := crawlSova(dir)
	if err != nil {
		return err
	}
	for _, src := range files {
		rel, rerr := filepath.Rel(root, filepath.Join(dir, src.RelPath))
		if rerr != nil {
			rel = src.RelPath
		}
		c.AddSource(rel, src.Content)
	}
	return nil
}

// resolveDepPath consults the package-manager layers in order:
//  1. local-link override at `<projectRoot>/.sova/local-links.toml`
//  2. workspace member listed in `<projectRoot>/sova.toml`
//  3. resolved dep view at `<projectRoot>/.sova/deps/<name>/`
//
// The "project root" is found by walking upward from `sourceRoot` looking for the nearest `sova.toml`. Returns the on-disk directory and true on first hit; (`""`, false) when none of the layers match (caller falls back to the project-internal source-root scan).
func resolveDepPath(sourceRoot, pkgPath string) (string, bool) {
	projectRoot := findUpwardManifest(sourceRoot)
	if projectRoot == "" {
		return "", false
	}
	if links, err := pkgmgr.LoadLocalLinks(projectRoot); err == nil {
		if dir, ok := links.Lookup(pkgPath, projectRoot); ok {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir, true
			}
		}
	}
	if m, ok, err := pkgmgr.LoadManifest(filepath.Join(projectRoot, pkgmgr.ManifestFilename)); err == nil && ok && m.Workspace != nil {
		if members, err := m.ResolveWorkspaceMembers(); err == nil {
			for _, dir := range members {
				if mm, ok, err := pkgmgr.LoadManifest(filepath.Join(dir, pkgmgr.ManifestFilename)); err == nil && ok && mm.PackageName() == pkgPath {
					return dir, true
				}
			}
		}
	}
	candidate := filepath.Join(projectRoot, pkgmgr.DepsDirname, filepath.FromSlash(pkgPath))
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, true
	}
	return "", false
}

// findUpwardManifest walks from `start` up to the filesystem root looking for `sova.toml`. Returns the containing directory or "" when no manifest is found. Used by the package loader so dep resolution works no matter how deep inside the source tree the entry file lives.
func findUpwardManifest(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, pkgmgr.ManifestFilename)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func crawlSova(root string) ([]sourceFile, error) {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	memberAllowed := workspaceMemberSet(root)
	var out []sourceFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name != "." && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			if path != root {
				if _, statErr := os.Stat(filepath.Join(path, pkgmgr.ManifestFilename)); statErr == nil {
					rel, _ := filepath.Rel(root, path)
					rel = filepath.ToSlash(rel)
					if memberAllowed != nil {
						if _, ok := memberAllowed[rel]; !ok {
							return fs.SkipDir
						}
					} else {
						return fs.SkipDir
					}
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".sova") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		out = append(out, sourceFile{RelPath: rel, Content: string(data)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// workspaceMemberSet loads the root's `sova.toml` and, when it carries a `[workspace]` section, returns a set of slash-form member paths relative to the root. Returns nil when the root is not a workspace, signalling to `crawlSova` that any sub-manifest is an unrelated sub-project (example, scratch, sample) and must not be pulled into the build. Treats every manifest discovered below the root as a hard boundary by default; workspace members opt back in by being explicitly listed.
func workspaceMemberSet(root string) map[string]bool {
	m, ok, err := pkgmgr.LoadManifest(filepath.Join(root, pkgmgr.ManifestFilename))
	if err != nil || !ok || m == nil || !m.IsWorkspaceRoot() {
		return nil
	}
	abs, err := m.ResolveWorkspaceMembers()
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, dir := range abs {
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			continue
		}
		out[filepath.ToSlash(rel)] = true
	}
	return out
}
