package passes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/scss"
	"strings"
)

// EmbedAssetsCacheKey holds `[]*EmbedRecord` — every `@embed`-decorated const the build saw, with its resolved on-disk path, declared kind, content hash, and size. Populated by `PassResolveEmbeds`; consumed by the build pipeline to stage files next to the generated Go output for `//go:embed` and (later, in P3) to feed esbuild's asset loaders. Aggregating into a single slice mirrors `ExternGoModulesCacheKey` from `pass_aggregate_extern_modules.go` so build-time tooling has one place to look.
const EmbedAssetsCacheKey = "embed_assets"

// defaultEmbedMaxBytes caps an individual embedded asset at 8 MiB so a `@embed("./bigthing.bin")` typo doesn't blow up the build. Configurable per-project via `[build.embed] max-bytes` in sova.toml (wired through the BuildConfig accessor `EmbedMaxBytes()` if present) — falling back to this constant when not set.
const defaultEmbedMaxBytes int64 = 8 * 1024 * 1024

// EmbedRecord is the build-wide view of one resolved `@embed` — a stable handle codegen and `sova dev` consume without re-walking the IR. `Decl` points at the VarDeclStmt whose `Embed` field is populated; `PackageRoot` is the absolute directory the embed resolved against (used by codegen to compute the relative path the `//go:embed` directive needs).
type EmbedRecord struct {
	Decl        *ir.VarDeclStmt
	Info        *ir.EmbedInfo
	PackageRoot string
}

// embedSizeCapper is the optional interface a BuildConfig can implement to override the global 8 MiB embed cap. Mirrors the BuildConfigGetter pattern in pass_codegen.go.
type embedSizeCapper interface {
	EmbedMaxBytes() int64
}

// PassResolveEmbeds walks every `@embed`-decorated `const` declaration, validates the surface (const-only, type ∈ `string`|`[]u8`|`bytes`, relative path, file exists, under size cap), resolves the path against the source file's directory (so library packages that ship under `.sova/deps/<pkg>/` embed correctly), computes a sha256[:16] content hash, and stores everything on the VarDeclStmt's new `Embed` field. The original Init expression is replaced with a placeholder literal of the right type so downstream passes (`infer_types`, codegen) see a well-typed declaration.
//
// Runs after `fold_annotations` (so the annotation's `"./path"` argument has been const-folded) and before any codegen pass. Aggregates every resolved record into `EmbedAssetsCacheKey` for downstream consumption.
type PassResolveEmbeds struct{}

func (p *PassResolveEmbeds) Name() string       { return "resolve_embeds" }
func (p *PassResolveEmbeds) Scope() PassScope   { return PerBuild }
func (p *PassResolveEmbeds) Requires() []string { return []string{"fold_annotations"} }
func (p *PassResolveEmbeds) NoErrors() bool     { return false }

func (p *PassResolveEmbeds) Run(pc *PassContext) error {
	sizeCap := defaultEmbedMaxBytes
	projectRoot := ""
	scssCfg := scss.Config{}
	if raw, ok := pc.Cache[buildConfigCacheKey]; ok {
		if cfg, ok := raw.(buildConfigGetter); ok {
			projectRoot = cfg.SourceDirectory()
		}
		if cfg, ok := raw.(embedSizeCapper); ok {
			if cap := cfg.EmbedMaxBytes(); cap > 0 {
				sizeCap = cap
			}
		}
		if cfg, ok := raw.(scssConfigGetter); ok {
			scssCfg.Command = cfg.SCSSCommandValue()
			scssCfg.Disabled = cfg.SCSSDisabledValue()
		}
	}
	sass := scss.New(scssCfg)
	if projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectRoot = cwd
		}
	}
	projectRootAbs, _ := filepath.Abs(projectRoot)

	var records []*EmbedRecord
	for _, pkg := range pc.Pkgs {
		if pkg == nil {
			continue
		}
		for _, f := range pkg.Files {
			if f == nil || f.Hir == nil {
				continue
			}
			if f.Hir.Side.Kind == ir.SideSynth {
				continue
			}
			fileDir := resolveSourceFileDir(f, projectRootAbs)
			for _, st := range f.Hir.Statements {
				switch s := st.(type) {
				case *ir.VarDeclStmt:
					if anno := findEmbedAnnotation(s.Annotations); anno != nil {
						if record := p.resolveTopLevel(pc, s, anno, fileDir, projectRootAbs, sizeCap, sass); record != nil {
							records = append(records, record)
						}
					}
				case *ir.TypeDeclStmt:
					for _, fld := range s.Fields {
						if fld == nil {
							continue
						}
						anno := findEmbedAnnotation(fld.Annotations)
						if anno == nil {
							continue
						}
						if record := p.resolveField(pc, fld, anno, fileDir, projectRootAbs, sizeCap, sass); record != nil {
							records = append(records, record)
						}
					}
				}
			}
		}
	}
	pc.Cache[EmbedAssetsCacheKey] = records
	return nil
}

// findEmbedAnnotation returns the `@embed(...)` annotation in the slice, or nil when there is none. Same check for top-level consts (`VarDeclStmt.Annotations`) and type fields (`TypeField.Annotations`); the resolver enforces the rest of the contract (const-only, type, path shape, etc.) and reports diagnostics for malformed usage rather than silently ignoring.
func findEmbedAnnotation(annos []ir.Annotation) *ir.Annotation {
	for i := range annos {
		a := &annos[i]
		if a.Name.Name == "embed" {
			return a
		}
	}
	return nil
}

// resolveSourceFileDir picks the absolute directory the source file lives in. Source filenames are stored as either project-relative paths (the common case via `collectSources`) or absolute paths (when the file came in as a one-shot entry). The result anchors the embed path resolver — `@embed("./foo.css")` resolves against this directory.
func resolveSourceFileDir(f *ir.PreparsedFile, projectRoot string) string {
	if filepath.IsAbs(f.Filename) {
		return filepath.Dir(f.Filename)
	}
	if projectRoot == "" {
		abs, err := filepath.Abs(f.Filename)
		if err != nil {
			return filepath.Dir(f.Filename)
		}
		return filepath.Dir(abs)
	}
	return filepath.Dir(filepath.Join(projectRoot, filepath.FromSlash(f.Filename)))
}

// resolveTopLevel handles `@embed` on a top-level `const` declaration. The const must be a single-target const with a `string` or `[]byte` type; failure surfaces a diagnostic and returns nil. On success, the VarDeclStmt's `Init` is replaced with a typed placeholder literal (Go and JS emitters read content via `Embed.SourcePath` instead) and a record is added to the build-wide registry so `build.go` can stage the file next to the generated `output.go`.
func (p *PassResolveEmbeds) resolveTopLevel(pc *PassContext, vd *ir.VarDeclStmt, anno *ir.Annotation, fileDir, projectRoot string, sizeCap int64, sass scss.Preprocessor) *EmbedRecord {
	label := embedDeclLabel(vd)
	if !vd.IsConst {
		pc.Diag.Report(diag.ErrEmbedNotConst, vd.Span())
		return nil
	}
	if len(vd.Targets) != 1 || vd.Targets[0].Name == nil {
		pc.Diag.Report(diag.ErrEmbedNotConst, vd.Span())
		return nil
	}
	kind := classifyEmbedTargetType(vd.Targets[0].TypeAnn)
	if kind == ir.EmbedKindUnknown {
		pc.Diag.Report(diag.ErrEmbedBadType, anno.Name.Span, label, formatTypeRefSurface(vd.Targets[0].TypeAnn))
		return nil
	}
	info, _, ok := p.validateAndRead(pc, anno, label, fileDir, projectRoot, sizeCap, kind, sass)
	if !ok {
		return nil
	}
	vd.Embed = info
	vd.Init = placeholderInitFor(kind, vd.Span(), pc.NodeAlloc)
	return &EmbedRecord{Decl: vd, Info: info, PackageRoot: fileDir}
}

// resolveField handles `@embed` on a `TypeField`. Type fields don't get the `//go:embed` directive path — the directive only works at package scope — so the resolver reads the file contents at compile time and replaces the field's `Default` with an inlined literal (string for text, ArrayLiteral of byte literals for binary). The codegen path for type fields already materialises default expressions on both backend and frontend, so no emitter changes are needed. The result is still added to the build-wide registry so `sova dev`'s watcher picks up changes to the file.
func (p *PassResolveEmbeds) resolveField(pc *PassContext, fld *ir.TypeField, anno *ir.Annotation, fileDir, projectRoot string, sizeCap int64, sass scss.Preprocessor) *EmbedRecord {
	label := fld.Name.Name
	kind := classifyEmbedTargetType(fld.Type)
	if kind == ir.EmbedKindUnknown {
		pc.Diag.Report(diag.ErrEmbedBadType, anno.Name.Span, label, formatTypeRefSurface(fld.Type))
		return nil
	}
	info, content, ok := p.validateAndRead(pc, anno, label, fileDir, projectRoot, sizeCap, kind, sass)
	if !ok {
		return nil
	}
	fld.Embed = info
	fld.Default = inlinedLiteralFor(kind, content, fld.Span(), pc.NodeAlloc)
	return &EmbedRecord{Decl: nil, Info: info, PackageRoot: fileDir}
}

// validateAndRead is the shared validate-path-and-load step both entry points share. Returns the populated EmbedInfo, the raw file contents (so field-level callers can build an inlined literal without re-reading), and an ok flag — on failure a diagnostic has already been emitted and ok=false.
//
// SCSS handling: when the resolved path ends in `.scss` or `.sass`, the file is run through the configured Sass preprocessor before its contents are hashed/returned. The size cap check applies to the *source* size (the user's SCSS file) rather than the compiled CSS, so a user pinning the cap to 100 KB doesn't have a 50 KB SCSS file fail the check because its compiled CSS happens to be 120 KB. The hash is computed over the *compiled* CSS so cache invalidation still triggers when the resulting output changes.
func (p *PassResolveEmbeds) validateAndRead(pc *PassContext, anno *ir.Annotation, label, fileDir, projectRoot string, sizeCap int64, kind ir.EmbedKind, sass scss.Preprocessor) (*ir.EmbedInfo, []byte, bool) {
	pathLit, ok := annotationPathArg(anno)
	if !ok {
		pc.Diag.Report(diag.ErrEmbedBadPath, anno.Name.Span, "non-string or missing argument")
		return nil, nil, false
	}
	if filepath.IsAbs(pathLit) || strings.HasPrefix(pathLit, "/") {
		pc.Diag.Report(diag.ErrEmbedBadPath, anno.Name.Span, fmt.Sprintf("%q", pathLit))
		return nil, nil, false
	}
	resolved := filepath.Clean(filepath.Join(fileDir, filepath.FromSlash(pathLit)))
	resolvedAbs, err := filepath.Abs(resolved)
	if err != nil {
		pc.Diag.Report(diag.ErrEmbedFileNotFound, anno.Name.Span, label, pathLit, resolved)
		return nil, nil, false
	}
	if projectRoot != "" {
		rel, err := filepath.Rel(projectRoot, resolvedAbs)
		if err != nil || strings.HasPrefix(rel, "..") {
			pc.Diag.Report(diag.ErrEmbedPathEscapesProject, anno.Name.Span, pathLit)
			return nil, nil, false
		}
	}
	stat, err := os.Stat(resolvedAbs)
	if err != nil {
		pc.Diag.Report(diag.ErrEmbedFileNotFound, anno.Name.Span, label, pathLit, resolvedAbs)
		return nil, nil, false
	}
	if stat.IsDir() {
		pc.Diag.Report(diag.ErrEmbedFileNotFound, anno.Name.Span, label, pathLit, resolvedAbs+" (is a directory)")
		return nil, nil, false
	}
	if stat.Size() > sizeCap {
		pc.Diag.Report(diag.ErrEmbedFileTooLarge, anno.Name.Span, label, pathLit, stat.Size(), sizeCap)
		return nil, nil, false
	}
	content, err := os.ReadFile(resolvedAbs)
	if err != nil {
		pc.Diag.Report(diag.ErrEmbedFileNotFound, anno.Name.Span, label, pathLit, resolvedAbs)
		return nil, nil, false
	}
	if scss.IsSassPath(resolvedAbs) {
		if !sass.Available() {
			pc.Diag.Report(diag.ErrEmbedSassUnavailable, anno.Name.Span, label, pathLit)
			return nil, nil, false
		}
		compiled, err := sass.Compile(resolvedAbs)
		if err != nil {
			pc.Diag.Report(diag.ErrEmbedSassFailed, anno.Name.Span, label, pathLit, err.Error())
			return nil, nil, false
		}
		content = compiled
	}
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])[:16]
	return &ir.EmbedInfo{
		SourcePath:  resolvedAbs,
		Kind:        kind,
		ContentHash: hash,
		SizeBytes:   stat.Size(),
		Span:        anno.Name.Span,
	}, content, true
}

// inlinedLiteralFor builds an Expr that materialises the literal file contents at the call site of a type-field embed (in contrast to `placeholderInitFor` which returns an empty literal because top-level VarDecls use the `//go:embed` directive on the backend). Text embeds become a `*LitString` containing the file content verbatim; binary embeds become an `*ArrayLiteral` of `*LitInt` byte values — the latter is verbose but produces correct code on both sides without requiring codegen to know about embed-typed defaults.
func inlinedLiteralFor(kind ir.EmbedKind, content []byte, span diag.TextSpan, alloc *ir.IdAlloc) ir.Expr {
	switch kind {
	case ir.EmbedKindText:
		lit := &ir.LitString{Value: string(content)}
		setNodeSpan(lit, freshNodeID(alloc), span)
		return lit
	case ir.EmbedKindBytes:
		list := &ir.ArrayLiteral{}
		setNodeSpan(list, freshNodeID(alloc), span)
		for _, b := range content {
			item := &ir.LitInt{Value: int64(b)}
			setNodeSpan(item, freshNodeID(alloc), span)
			list.Elems = append(list.Elems, item)
		}
		return list
	}
	return nil
}

func freshNodeID(alloc *ir.IdAlloc) ir.NodeID {
	if alloc == nil {
		return 0
	}
	return ir.NodeID(alloc.Next())
}

// classifyEmbedTargetType inspects the declared type annotation on the const and decides whether to treat the embed as text or bytes. `string` → text, `[]u8` → bytes. Unknown kinds make the caller diagnose.
func classifyEmbedTargetType(t *ir.TypeRef) ir.EmbedKind {
	if t == nil {
		return ir.EmbedKindUnknown
	}
	if t.Kind == ir.TK_PrimitiveString {
		return ir.EmbedKindText
	}
	if t.Kind == ir.TK_Slice && t.Elem != nil && t.Elem.Kind == ir.TK_PrimitiveByte {
		return ir.EmbedKindBytes
	}
	if t.CustomName == "bytes" && t.CustomQualifier == "" {
		return ir.EmbedKindBytes
	}
	return ir.EmbedKindUnknown
}

// annotationPathArg pulls the first argument of a `@embed("./path")` annotation as a string literal. Returns ok=false when the annotation has no args or the first arg is not a string after const folding. The fold pass has already run by the time this resolver fires, so we can read `ResolvedArgs` directly.
func annotationPathArg(a *ir.Annotation) (string, bool) {
	if len(a.ResolvedArgs) > 0 {
		if a.ResolvedArgs[0].Kind == ir.AnnotationValueString {
			return a.ResolvedArgs[0].Str, true
		}
		return "", false
	}
	if len(a.Args) == 0 {
		return "", false
	}
	if lit, ok := a.Args[0].(*ir.LitString); ok {
		return lit.Value, true
	}
	return "", false
}

// placeholderInitFor builds an empty literal of the right type so downstream passes (infer_types, codegen) see a well-typed const. The real content lives in `vd.Embed`; codegen replaces the initializer with the actual file payload when emitting Go or JS.
func placeholderInitFor(kind ir.EmbedKind, span diag.TextSpan, alloc *ir.IdAlloc) ir.Expr {
	id := ir.NodeID(0)
	if alloc != nil {
		id = ir.NodeID(alloc.Next())
	}
	switch kind {
	case ir.EmbedKindText:
		lit := &ir.LitString{Value: ""}
		setNodeSpan(lit, id, span)
		return lit
	case ir.EmbedKindBytes:
		lit := &ir.ArrayLiteral{}
		setNodeSpan(lit, id, span)
		return lit
	}
	return nil
}

// setNodeSpan stamps an ID + span onto any IR node that embeds the unexported `node` struct. Uses the same trick the visitor does (creating a `node` value via the public `mkNode`-style allocator) but applied locally because the IR types don't expose setters.
func setNodeSpan(_ ir.Node, _ ir.NodeID, _ diag.TextSpan) {
	// No-op: literal nodes built without IDs still satisfy the Expr/Node interfaces;
	// the visitor's normal nodes get fresh IDs, but synthetic placeholders here can
	// share NodeID(0) without breaking downstream passes (they don't index by ID).
}

func embedDeclLabel(vd *ir.VarDeclStmt) string {
	if len(vd.Targets) == 0 || vd.Targets[0].Name == nil {
		return "?"
	}
	return vd.Targets[0].Name.Name
}

func formatTypeRefSurface(t *ir.TypeRef) string {
	if t == nil {
		return "<inferred>"
	}
	if t.CustomName != "" {
		if t.CustomQualifier != "" {
			return t.CustomQualifier + "." + t.CustomName
		}
		return t.CustomName
	}
	switch t.Kind {
	case ir.TK_PrimitiveString:
		return "string"
	case ir.TK_PrimitiveInt:
		return "int"
	case ir.TK_PrimitiveFloat:
		return "float"
	case ir.TK_PrimitiveBool:
		return "bool"
	case ir.TK_PrimitiveByte:
		return "byte"
	case ir.TK_PrimitiveAny:
		return "any"
	case ir.TK_PrimitiveNone:
		return "none"
	case ir.TK_Slice:
		if t.Elem != nil {
			return "[]" + formatTypeRefSurface(t.Elem)
		}
		return "[]?"
	}
	return "?"
}
