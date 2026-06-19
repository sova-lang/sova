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

type CodegenStep struct {
	Name    string
	Command string
	Inputs  []string
	Outputs []string
	Always  bool
	Manual  bool
}

type CodegenMode int

const (
	CodegenModeAuto CodegenMode = iota

	CodegenModeForce
)

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

func runCodegenSteps(cfg *BuildConfig, mode CodegenMode) (ran int, skipped int, err error) {
	if len(cfg.Codegen) == 0 {
		return 0, 0, nil
	}

	for _, step := range cfg.Codegen {
		shouldRun, why := codegenShouldRun(step, mode)
		if !shouldRun {
			skipped++
			termui.Info(fmt.Sprintf("codegen %s - skipped (%s)", step.Name, why))
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
