package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sova/internal/termui"
	"strings"
	"time"
)

// CodegenStep is the resolved form of one `[[build.codegen]]` entry from sova.toml. All paths are absolute (resolved against the project root). The runner walks this slice in declaration order — sequential, not parallel, because steps may depend on each other (e.g. a font generator feeding a schema generator).
type CodegenStep struct {
	Name    string
	Command string
	Inputs  []string
	Outputs []string
	Always  bool
	Manual  bool
}

// CodegenMode selects which subset of configured codegen steps the runner executes.
type CodegenMode int

const (
	// CodegenModeAuto runs only steps that are stale (inputs newer than outputs) AND not marked `manual = true`. This is the default when invoked from `sova build` or `sova dev`.
	CodegenModeAuto CodegenMode = iota
	// CodegenModeForce runs every step regardless of staleness or the `manual` flag. Used by `sova generate` so the user can deterministically refresh everything.
	CodegenModeForce
)

// resolveCodegenSteps converts the raw manifest entries into CodegenStep values rooted at projectRoot. Empty `command` makes a step invalid; missing `name` defaults to "codegen[i]" so the runner output stays attributable. Returns the parsed list and a slice of validation errors (one per malformed entry) so the caller can present them all at once.
func resolveCodegenSteps(entries []manifestCodegen, projectRoot string) ([]CodegenStep, []error) {
	if len(entries) == 0 {
		return nil, nil
	}
	rootAbs, err := filepath.Abs(projectRoot)
	if err != nil || rootAbs == "" {
		rootAbs = projectRoot
	}
	out := make([]CodegenStep, 0, len(entries))
	var errs []error
	for i, e := range entries {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			name = fmt.Sprintf("codegen[%d]", i)
		}
		if strings.TrimSpace(e.Command) == "" {
			errs = append(errs, fmt.Errorf("[[build.codegen]] %q: missing `command`", name))
			continue
		}
		step := CodegenStep{
			Name:    name,
			Command: e.Command,
			Always:  e.Always,
			Manual:  e.Manual,
		}
		for _, in := range e.Inputs {
			step.Inputs = append(step.Inputs, absJoin(rootAbs, in))
		}
		for _, o := range e.Outputs {
			step.Outputs = append(step.Outputs, absJoin(rootAbs, o))
		}
		out = append(out, step)
	}
	return out, errs
}

func absJoin(root, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(root, p))
}

// runCodegenSteps walks cfg.Codegen and executes each entry that needs to run under the given mode. Output (stdout + stderr) from each command is streamed live so long-running generators (font downloads, GraphQL schema fetches) report progress to the user. A step that fails aborts the whole runner — subsequent steps may depend on it.
//
// Returns the count of steps that actually ran, the count that were skipped (up-to-date or manual), and the first error encountered. The caller decides how to react: build.go treats any error as a build failure; the `sova generate` subcommand prints the error and exits non-zero.
func runCodegenSteps(cfg *BuildConfig, mode CodegenMode) (ran int, skipped int, err error) {
	if len(cfg.Codegen) == 0 {
		return 0, 0, nil
	}
	for _, step := range cfg.Codegen {
		shouldRun, why := codegenShouldRun(step, mode)
		if !shouldRun {
			skipped++
			termui.Info(fmt.Sprintf("codegen %s — skipped (%s)", step.Name, why))
			continue
		}
		termui.Step(fmt.Sprintf("codegen %s", step.Name))
		if err := runCodegenCommand(step, cfg.SourceDir); err != nil {
			return ran, skipped, fmt.Errorf("[[build.codegen]] %s: %w", step.Name, err)
		}
		ran++
	}
	return ran, skipped, nil
}

// codegenShouldRun decides whether a single step needs to fire under the requested mode. Force mode runs everything (including Manual). Auto mode skips Manual entries and uses mtime-based staleness on the rest; Always entries bypass the staleness check but are still skipped in Auto mode if marked Manual.
func codegenShouldRun(step CodegenStep, mode CodegenMode) (bool, string) {
	if mode == CodegenModeForce {
		return true, ""
	}
	if step.Manual {
		return false, "manual = true; run `sova generate` to invoke"
	}
	if step.Always {
		return true, ""
	}
	if len(step.Outputs) == 0 {
		return true, "no outputs declared"
	}
	newestIn, ok := newestMtime(step.Inputs)
	if !ok && len(step.Inputs) > 0 {
		return true, "input(s) missing"
	}
	oldestOut, allPresent := oldestMtime(step.Outputs)
	if !allPresent {
		return true, "output(s) missing"
	}
	if len(step.Inputs) == 0 {
		return false, "up to date (no inputs)"
	}
	if newestIn.After(oldestOut) {
		return true, "input newer than output"
	}
	return false, "up to date"
}

// newestMtime returns the most recent modification time across the listed paths and whether at least one path existed. For a directory the mtime of the newest file (recursively) is used so generators that emit many files into a tree (`assets/fonts/*.woff2`) trigger correctly on any internal change.
func newestMtime(paths []string) (time.Time, bool) {
	var newest time.Time
	any := false
	for _, p := range paths {
		t, ok := walkNewestMtime(p)
		if !ok {
			continue
		}
		any = true
		if t.After(newest) {
			newest = t
		}
	}
	return newest, any
}

// oldestMtime returns the oldest modification time across the listed paths plus a flag indicating every path existed. Missing entries cause allPresent=false so the caller treats the outputs as stale.
func oldestMtime(paths []string) (time.Time, bool) {
	var oldest time.Time
	first := true
	allPresent := true
	for _, p := range paths {
		t, ok := walkNewestMtime(p)
		if !ok {
			allPresent = false
			continue
		}
		if first || t.Before(oldest) {
			oldest = t
			first = false
		}
	}
	return oldest, allPresent && !first
}

// walkNewestMtime returns the mtime of a file or, for a directory, the newest mtime of any file inside it (recursively). Hidden files and `__sova_*` sentinel files are walked the same as everything else — generators are responsible for putting only their real outputs in declared output paths.
func walkNewestMtime(p string) (time.Time, bool) {
	info, err := os.Stat(p)
	if err != nil {
		return time.Time{}, false
	}
	if !info.IsDir() {
		return info.ModTime(), true
	}
	var newest time.Time
	any := false
	_ = filepath.Walk(p, func(_ string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		any = true
		if t := fi.ModTime(); t.After(newest) {
			newest = t
		}
		return nil
	})
	if !any {
		return info.ModTime(), true
	}
	return newest, true
}

// runCodegenCommand executes the step's command string via the platform shell, with the project source dir as the working directory and stdout/stderr inherited so the user sees output live. Inheriting the env means generators can read SOVA_* variables or anything else the parent shell defines — including PATH entries pinned in the user's shell config.
func runCodegenCommand(step CodegenStep, sourceDir string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", step.Command)
	} else {
		cmd = exec.Command("sh", "-c", step.Command)
	}
	cmd.Dir = sourceDir
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	return cmd.Run()
}
