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

const EmbedAssetsCacheKey = "embed_assets"

const defaultEmbedMaxBytes int64 = 8 * 1024 * 1024

type EmbedRecord struct {
	Decl        *ir.VarDeclStmt
	Info        *ir.EmbedInfo
	PackageRoot string
}

type embedSizeCapper interface {
	EmbedMaxBytes() int64
}

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

func findEmbedAnnotation(annos []ir.Annotation) *ir.Annotation {
	for i := range annos {
		a := &annos[i]
		if a.Name.Name == "embed" {
			return a
		}
	}

	return nil
}

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

func setNodeSpan(_ ir.Node, _ ir.NodeID, _ diag.TextSpan) {

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
