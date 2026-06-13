package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sova/internal/ir"
	"sova/internal/passes"
	"sova/internal/services/compiler"
	"sova/internal/services/fmtsrv"
	"sova/internal/termui"
	"strings"

	"github.com/spf13/cobra"
)

func newSynthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "synth",
		Short: "Inspect and validate custom-annotation (synth) packages",
	}
	cmd.AddCommand(newSynthCheckCmd())
	cmd.AddCommand(newSynthListCmd())
	cmd.AddCommand(newSynthExpandCmd())
	return cmd
}

func newSynthExpandCmd() *cobra.Command {
	var outDir string
	var onlyFile string
	cmd := &cobra.Command{
		Use:   "expand [file|dir]",
		Short: "Re-emit the project's Sova source after running synth expansion (debug view of what every `@CustomAnnotation` lowered to)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfig(args, "", "", cmd)
			if err != nil {
				return err
			}
			return runSynthExpand(cfg, outDir, onlyFile)
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "write each expanded file under this directory (mirroring the source tree); default is stdout with file-header banners")
	cmd.Flags().StringVar(&onlyFile, "file", "", "restrict output to one source file (matched against the file's stored relative path)")
	return cmd
}

func newSynthCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check [file|dir]",
		Short: "Validate a project's synth declarations end-to-end (parse, register, expand) without emitting code",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfig(args, "", "", cmd)
			if err != nil {
				return err
			}
			return runSynthCheck(cfg)
		},
	}
	return cmd
}

func newSynthListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [file|dir]",
		Short: "List every `synth` declaration the build sees, with target kind and param signature",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfig(args, "", "", cmd)
			if err != nil {
				return err
			}
			return runSynthList(cfg)
		},
	}
	return cmd
}

func runSynthCheck(cfg BuildConfig) error {
	c, err := compileForSynth(cfg)
	if err != nil {
		return err
	}
	c.Diag.Print()
	if c.Diag.Errored() {
		return fmt.Errorf("synth check failed")
	}
	reg, _ := c.Cache[passes.SynthRegistryCacheKey].(map[string]*ir.SynthDeclStmt)
	termui.Success(fmt.Sprintf("synth check ok: %d declaration(s) registered, no errors", len(reg)))
	return nil
}

func runSynthList(cfg BuildConfig) error {
	c, err := compileForSynth(cfg)
	if err != nil {
		return err
	}
	c.Diag.Print()
	reg, _ := c.Cache[passes.SynthRegistryCacheKey].(map[string]*ir.SynthDeclStmt)
	if len(reg) == 0 {
		fmt.Fprintln(os.Stdout, "(no synth declarations)")
		return nil
	}
	names := make([]string, 0, len(reg))
	for n := range reg {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		sd := reg[n]
		fmt.Fprintf(os.Stdout, "%s%s on %s %s\n", n, formatSynthParams(sd.Params), sd.Target.Kind.String(), sd.Target.BindName)
	}
	return nil
}

func runSynthExpand(cfg BuildConfig, outDir, onlyFile string) error {
	c, err := compileForSynth(cfg)
	if err != nil {
		return err
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		return fmt.Errorf("synth expansion failed: compilation has errors")
	}
	keys := make([]string, 0, len(c.Packages))
	for k := range c.Packages {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	wrote := 0
	for _, k := range keys {
		pkg := c.Packages[k]
		if pkg == nil {
			continue
		}
		for _, file := range pkg.Files {
			if file == nil || file.Hir == nil {
				continue
			}
			if file.Hir.Side.Kind == ir.SideSynth {
				continue
			}
			if onlyFile != "" && !strings.Contains(file.Filename, onlyFile) {
				continue
			}
			materializeFoldedAnnotations(file.Hir)
			rendered := fmtsrv.File(file.Hir, "")
			if outDir == "" {
				fmt.Fprintf(os.Stdout, "// === %s ===\n%s\n", file.Filename, rendered)
				wrote++
				continue
			}
			dest := filepath.Join(outDir, file.Filename)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
			}
			if err := os.WriteFile(dest, []byte(rendered), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", dest, err)
			}
			termui.Info(fmt.Sprintf("wrote %s", dest))
			wrote++
		}
	}
	if wrote == 0 {
		return fmt.Errorf("no source files matched (synth-side files are skipped; check --file filter)")
	}
	return nil
}

// materializeFoldedAnnotations rewrites every annotation in the file so that its `Args` slice is the literal form of `ResolvedArgs` (when fold_annotations actually folded it). This gives `sova synth expand` a cleaner debug view: `@structTag("gorm", "column:id")` instead of `@structTag("gorm", "column:" + "id")` — what the rest of the pipeline (codegen, etc.) sees in practice, since downstream consumers read `ResolvedArgs`, not `Args`. Annotations whose args didn't fold (e.g. a non-const expression that pass_fold_annotations rejected) keep their original Args verbatim so the user can still see what was there.
func materializeFoldedAnnotations(f *ir.File) {
	walkFileAnnotations(f, func(a *ir.Annotation) {
		if len(a.ResolvedArgs) == 0 {
			return
		}
		newArgs := make([]ir.Expr, 0, len(a.ResolvedArgs))
		for _, rv := range a.ResolvedArgs {
			lit, ok := resolvedToLiteral(rv)
			if !ok {
				return
			}
			newArgs = append(newArgs, lit)
		}
		a.Args = newArgs
	})
}

func resolvedToLiteral(rv ir.AnnotationValue) (ir.Expr, bool) {
	switch rv.Kind {
	case ir.AnnotationValueString:
		return &ir.LitString{Value: rv.Str}, true
	case ir.AnnotationValueInt:
		return &ir.LitInt{Value: rv.Int}, true
	case ir.AnnotationValueBool:
		return &ir.LitBool{Value: rv.Bool}, true
	}
	return nil, false
}

// walkFileAnnotations visits every annotation slot in the file's HIR — top-level type/func/let decls plus their inner members (fields, methods, ctors, params). The callback receives a pointer into the original slice so it can mutate in place.
func walkFileAnnotations(f *ir.File, fn func(*ir.Annotation)) {
	if f == nil {
		return
	}
	for _, st := range f.Statements {
		switch s := st.(type) {
		case *ir.TypeDeclStmt:
			for i := range s.Annotations {
				fn(&s.Annotations[i])
			}
			for _, fld := range s.Fields {
				if fld == nil {
					continue
				}
				for i := range fld.Annotations {
					fn(&fld.Annotations[i])
				}
			}
			for _, m := range s.Methods {
				if m == nil {
					continue
				}
				for i := range m.Annotations {
					fn(&m.Annotations[i])
				}
				if m.Func != nil {
					for _, prm := range m.Func.Params {
						if prm == nil {
							continue
						}
						for i := range prm.Annotations {
							fn(&prm.Annotations[i])
						}
					}
				}
			}
			for _, c := range s.Ctors {
				if c == nil {
					continue
				}
				for i := range c.Annotations {
					fn(&c.Annotations[i])
				}
				for _, prm := range c.Params {
					if prm == nil {
						continue
					}
					for i := range prm.Annotations {
						fn(&prm.Annotations[i])
					}
				}
			}
		case *ir.FuncDeclStmt:
			for i := range s.Annotations {
				fn(&s.Annotations[i])
			}
			for _, prm := range s.Params {
				if prm == nil {
					continue
				}
				for i := range prm.Annotations {
					fn(&prm.Annotations[i])
				}
			}
		case *ir.VarDeclStmt:
			for i := range s.Annotations {
				fn(&s.Annotations[i])
			}
		}
	}
}

func compileForSynth(cfg BuildConfig) (*compiler.CompilerContext, error) {
	root, files, err := collectSources(cfg)
	if err != nil {
		return nil, err
	}
	cfg.SourceDir = root
	c := compiler.New()
	c.SetBuildConfig(CacheKey, cfg)
	c.Loader = makePackageLoader(root)
	for _, src := range files {
		c.AddSource(src.RelPath, src.Content)
	}
	if err := c.Check(); err != nil {
		return c, err
	}
	return c, nil
}

func formatSynthParams(params []*ir.FuncParam) string {
	if len(params) == 0 {
		return ""
	}
	out := "("
	for i, p := range params {
		if i > 0 {
			out += ", "
		}
		if p == nil {
			out += "?"
			continue
		}
		out += p.Name.Name
		if p.Type != nil {
			out += ": " + typeRefSummary(p.Type)
		}
	}
	return out + ")"
}

func typeRefSummary(t *ir.TypeRef) string {
	if t == nil {
		return "?"
	}
	if t.CustomName != "" {
		if t.CustomQualifier != "" {
			return t.CustomQualifier + "." + t.CustomName
		}
		return t.CustomName
	}
	return "?"
}
