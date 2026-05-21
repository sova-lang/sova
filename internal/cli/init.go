package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// newInitCmd registers `sova init [name]`. With no argument it scaffolds the current working directory (no `mkdir`). With a name it creates a fresh `<name>/` subdirectory and scaffolds there. In both cases the scaffold writes a minimal `sova.toml`, a `src/main.sova` hello-world, and a `.gitignore` covering the generated/derived directories. Refuses to overwrite an existing `sova.toml` so re-running on a populated project doesn't silently clobber.
func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [name]",
		Short: "Scaffold a new Sova project (name → new dir; no name → current dir)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return runInit(args[0], true)
			}
			return runInit("", false)
		},
	}
	return cmd
}

// runInit performs the actual scaffold. When `createDir` is true, it creates a fresh `<name>/` subdirectory of the current working directory and scaffolds there; otherwise it uses the cwd as-is. The package name in the generated manifest is `name` when provided, or the basename of the target directory when it isn't.
func runInit(name string, createDir bool) error {
	target := "."
	pkgName := name
	if createDir {
		if name == "" {
			return errors.New("init: missing project name")
		}
		if _, err := os.Stat(name); err == nil {
			return fmt.Errorf("init: %s already exists", name)
		}
		if err := os.MkdirAll(name, 0o755); err != nil {
			return fmt.Errorf("init: mkdir %s: %w", name, err)
		}
		target = name
	} else {
		abs, err := filepath.Abs(".")
		if err == nil {
			pkgName = filepath.Base(abs)
		}
	}

	manifestPath := filepath.Join(target, "sova.toml")
	if _, err := os.Stat(manifestPath); err == nil {
		return fmt.Errorf("init: %s already has a sova.toml - refusing to overwrite", target)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	files := map[string]string{
		"sova.toml":     scaffoldManifest(pkgName),
		"src/main.sova": scaffoldMain(),
		".gitignore":    scaffoldGitignore(),
	}
	for rel, body := range files {
		path := filepath.Join(target, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("init: mkdir %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return fmt.Errorf("init: write %s: %w", path, err)
		}
	}

	if createDir {
		fmt.Fprintf(os.Stderr, "[init] scaffolded %s/\n", target)
		fmt.Fprintf(os.Stderr, "       cd %s && sova run\n", target)
	} else {
		fmt.Fprintf(os.Stderr, "[init] scaffolded in current directory\n")
		fmt.Fprintf(os.Stderr, "       run with: sova run\n")
	}
	return nil
}

func scaffoldManifest(name string) string {
	if name == "" {
		name = "my-sova-app"
	}
	return `[package]
name = "` + name + `"
version = "0.1.0"

[project]
entry = "src/main.sova"

[dependencies]
`
}

func scaffoldMain() string {
	return `on shared

func main() {
    print("hello, sova")
}
`
}

func scaffoldGitignore() string {
	return `# Sova-managed directories
.sova/
.output/
dist/

# Editor / OS noise
.DS_Store
Thumbs.db
*.swp
.idea/
.vscode/

# Local overrides (kept per-developer, never committed)
.sova/local-links.toml
`
}
