package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"sova/internal/services/fmtsrv"

	"github.com/spf13/cobra"
)

// newFmtCmd registers `sova fmt` - Sova's source-code formatter. Default: rewrite each given file in place. Flags toggle two read-only modes (`-l` lists files that would change, `-d` prints a diff to stdout) and one stdin/stdout mode (no args → read source from stdin, write formatted source to stdout).
func newFmtCmd() *cobra.Command {
	var listOnly, showDiff, writeIfChange bool
	cmd := &cobra.Command{
		Use:   "fmt [paths...]",
		Short: "Format Sova source code",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFmt(args, listOnly, showDiff, writeIfChange)
		},
	}
	cmd.Flags().BoolVarP(&listOnly, "list", "l", false, "list files whose formatting differs (no rewrite)")
	cmd.Flags().BoolVarP(&showDiff, "diff", "d", false, "print a unified-style diff per file instead of rewriting")
	cmd.Flags().BoolVarP(&writeIfChange, "write", "w", true, "write the formatted result back to the source file (default true)")
	return cmd
}

// runFmt dispatches `sova fmt` based on the args + flags. No args + a tty stdin reads from stdin and writes to stdout. Path args walk the filesystem: directories are recursed (skipping `.git`, `.sova`, hidden dirs); files must end in `.sova`.
func runFmt(args []string, listOnly, showDiff, writeIfChange bool) error {
	if len(args) == 0 {
		if err := formatStream(os.Stdin, os.Stdout); err != nil {
			return err
		}
		return nil
	}
	var paths []string
	for _, arg := range args {
		matches, err := expandPath(arg)
		if err != nil {
			return err
		}
		paths = append(paths, matches...)
	}
	if len(paths) == 0 {
		return fmt.Errorf("no .sova files found in %v", args)
	}
	changed := 0
	for _, p := range paths {
		modified, err := formatFile(p, listOnly, showDiff, writeIfChange)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[fmt] %s: %v\n", p, err)
			continue
		}
		if modified {
			changed++
		}
	}
	if listOnly && changed > 0 {
		return nil
	}
	return nil
}

// formatStream reads source from `in`, formats it, writes the result to `out`. Parse errors leave the original unchanged on the wire.
func formatStream(in io.Reader, out io.Writer) error {
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	formatted, err := fmtsrv.Source(string(data))
	if err != nil {
		_, _ = out.Write(data)
		return err
	}
	_, _ = out.Write([]byte(formatted))
	return nil
}

// formatFile formats a single file according to the chosen mode. Returns true when the on-disk content changed (or would change in `--list`/`--diff` modes).
func formatFile(path string, listOnly, showDiff, writeIfChange bool) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	src := string(data)
	formatted, err := fmtsrv.Source(src)
	if err != nil {
		return false, err
	}
	if formatted == src {
		return false, nil
	}
	switch {
	case listOnly:
		fmt.Println(path)
	case showDiff:
		fmt.Printf("--- %s (original)\n+++ %s (formatted)\n", path, path)
		fmt.Println(simpleDiff(src, formatted))
	case writeIfChange:
		if err := os.WriteFile(path, []byte(formatted), 0o644); err != nil {
			return false, err
		}
	}
	return true, nil
}

// expandPath turns a CLI argument into the list of `.sova` files it covers. A file path returns just that file (if it ends in `.sova`); a directory recurses, skipping hidden dirs and `.sova/deps/`.
func expandPath(arg string) ([]string, error) {
	info, err := os.Stat(arg)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !strings.HasSuffix(arg, ".sova") {
			return nil, fmt.Errorf("%s: not a .sova file", arg)
		}
		return []string{arg}, nil
	}
	var out []string
	err = filepath.WalkDir(arg, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != arg && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".sova") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// simpleDiff is a minimal line-by-line diff for the `--diff` mode. Not unified-diff format (no context lines, no `@@` headers); good enough to spot per-line changes without pulling in a diff library for v1.
func simpleDiff(a, b string) string {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	var out strings.Builder
	n := len(aLines)
	if len(bLines) > n {
		n = len(bLines)
	}
	for i := 0; i < n; i++ {
		var av, bv string
		if i < len(aLines) {
			av = aLines[i]
		}
		if i < len(bLines) {
			bv = bLines[i]
		}
		if av == bv {
			continue
		}
		if av != "" {
			out.WriteString("- " + av + "\n")
		}
		if bv != "" {
			out.WriteString("+ " + bv + "\n")
		}
	}
	return out.String()
}
