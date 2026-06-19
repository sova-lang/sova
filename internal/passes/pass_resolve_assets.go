package passes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sova/internal/diag"
	"sova/internal/imagepipe"
	"sova/internal/ir"
	"strings"
)

const AssetsCacheKey = "static_assets"

const defaultAssetMaxBytes int64 = 32 * 1024 * 1024

type AssetRecord struct {
	Decl               *ir.VarDeclStmt
	Info               *ir.AssetInfo
	PackageRoot        string
	TransformedContent []byte
}

type PassResolveAssets struct{}

func (p *PassResolveAssets) Name() string       { return "resolve_assets" }

func (p *PassResolveAssets) Scope() PassScope   { return PerBuild }

func (p *PassResolveAssets) Requires() []string { return []string{"fold_annotations"} }

func (p *PassResolveAssets) NoErrors() bool     { return false }

func (p *PassResolveAssets) Run(pc *PassContext) error {
	sizeCap := defaultAssetMaxBytes
	projectRoot := ""
	if raw, ok := pc.Cache[buildConfigCacheKey]; ok {
		if cfg, ok := raw.(buildConfigGetter); ok {
			projectRoot = cfg.SourceDirectory()
		}
	}

	if projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectRoot = cwd
		}
	}

	projectRootAbs, _ := filepath.Abs(projectRoot)

	var records []*AssetRecord
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
				vd, ok := st.(*ir.VarDeclStmt)
				if !ok {
					continue
				}

				anno := findAssetAnnotation(vd.Annotations)
				if anno == nil {
					continue
				}

				if record := p.resolveTopLevel(pc, vd, anno, fileDir, projectRootAbs, sizeCap); record != nil {
					records = append(records, record)
				}
			}
		}
	}

	pc.Cache[AssetsCacheKey] = records
	return nil
}

func findAssetAnnotation(annos []ir.Annotation) *ir.Annotation {
	for i := range annos {
		a := &annos[i]
		if a.Name.Name == "asset" {
			return a
		}
	}

	return nil
}

func (p *PassResolveAssets) resolveTopLevel(pc *PassContext, vd *ir.VarDeclStmt, anno *ir.Annotation, fileDir, projectRoot string, sizeCap int64) *AssetRecord {
	label := embedDeclLabel(vd)
	if !vd.IsConst {
		pc.Diag.Report(diag.ErrAssetNotConst, vd.Span())
		return nil
	}

	if len(vd.Targets) != 1 || vd.Targets[0].Name == nil {
		pc.Diag.Report(diag.ErrAssetNotConst, vd.Span())
		return nil
	}

	if !isStringTypeRef(vd.Targets[0].TypeAnn) {
		pc.Diag.Report(diag.ErrAssetBadType, anno.Name.Span, label, formatTypeRefSurface(vd.Targets[0].TypeAnn))
		return nil
	}

	pathLit, ok := annotationPathArg(anno)
	if !ok {
		pc.Diag.Report(diag.ErrAssetBadPath, anno.Name.Span, "non-string or missing argument")
		return nil
	}

	if filepath.IsAbs(pathLit) || strings.HasPrefix(pathLit, "/") {
		pc.Diag.Report(diag.ErrAssetBadPath, anno.Name.Span, fmt.Sprintf("%q", pathLit))
		return nil
	}

	resolved := filepath.Clean(filepath.Join(fileDir, filepath.FromSlash(pathLit)))
	abs, err := filepath.Abs(resolved)
	if err != nil {
		pc.Diag.Report(diag.ErrAssetFileNotFound, anno.Name.Span, label, pathLit, resolved)
		return nil
	}

	if projectRoot != "" {
		rel, err := filepath.Rel(projectRoot, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			pc.Diag.Report(diag.ErrAssetPathEscapesProject, anno.Name.Span, pathLit)
			return nil
		}
	}

	stat, err := os.Stat(abs)
	if err != nil || stat.IsDir() {
		pc.Diag.Report(diag.ErrAssetFileNotFound, anno.Name.Span, label, pathLit, abs)
		return nil
	}

	if stat.Size() > sizeCap {
		pc.Diag.Report(diag.ErrAssetFileTooLarge, anno.Name.Span, label, pathLit, stat.Size(), sizeCap)
		return nil
	}

	content, err := os.ReadFile(abs)
	if err != nil {
		pc.Diag.Report(diag.ErrAssetFileNotFound, anno.Name.Span, label, pathLit, abs)
		return nil
	}

	opts, ok := parseTransformOpts(pc, anno, label)
	if !ok {
		return nil
	}

	base := filepath.Base(abs)
	ext := filepath.Ext(base)
	var transformed []byte
	if opts.NeedsTransform() {
		out, outExt, err := imagepipe.Transform(content, ext, opts)
		if err != nil {
			pc.Diag.Report(diag.ErrAssetTransformFailed, anno.Name.Span, label, err.Error())
			return nil
		}

		transformed = out
		content = out
		ext = outExt
	}

	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])[:16]
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = sanitizeAssetStem(stem)
	staged := fmt.Sprintf("%s-%s%s", stem, hash, ext)
	info := &ir.AssetInfo{
		SourcePath:  abs,
		ContentHash: hash,
		URL:         "/__sova/" + staged,
		StagedName:  staged,
		SizeBytes:   int64(len(content)),
		Span:        anno.Name.Span,
	}

	vd.Asset = info
	lit := &ir.LitString{Value: info.URL}

	nid := ir.NodeID(0)
	if pc.NodeAlloc != nil {
		nid = ir.NodeID(pc.NodeAlloc.Next())
	}

	setNodeSpan(lit, nid, vd.Span())
	vd.Init = lit
	return &AssetRecord{Decl: vd, Info: info, PackageRoot: fileDir, TransformedContent: transformed}
}

func parseTransformOpts(pc *PassContext, a *ir.Annotation, label string) (imagepipe.Options, bool) {
	var opts imagepipe.Options
	for i, val := range a.ResolvedArgs {
		name := ""
		if i < len(a.ArgNames) {
			name = a.ArgNames[i]
		}

		if i == 0 && name == "" {
			continue
		}

		if name == "" {
			switch i {
			case 1:
				name = "to"
			case 2:
				name = "quality"
			case 3:
				name = "maxWidth"
			case 4:
				name = "maxHeight"
			default:
				pc.Diag.Report(diag.ErrAssetBadArg, a.Name.Span, label, fmt.Sprintf("extra positional argument at position %d", i))
				return opts, false
			}
		}

		switch name {
		case "path":

		case "to":
			if val.Kind != ir.AnnotationValueString {
				pc.Diag.Report(diag.ErrAssetBadArg, a.Name.Span, label, "`to:` must be a string")
				return opts, false
			}

			f, ok := imagepipe.NormalizeFormat(val.Str)
			if !ok {
				pc.Diag.Report(diag.ErrAssetBadArg, a.Name.Span, label, fmt.Sprintf("unknown `to:` format %q (supported: png, jpeg, gif, webp)", val.Str))
				return opts, false
			}

			opts.To = f
		case "quality":
			if val.Kind != ir.AnnotationValueInt {
				pc.Diag.Report(diag.ErrAssetBadArg, a.Name.Span, label, "`quality:` must be an int")
				return opts, false
			}

			if val.Int < 1 || val.Int > 100 {
				pc.Diag.Report(diag.ErrAssetBadArg, a.Name.Span, label, fmt.Sprintf("`quality:` must be 1..100 (got %d)", val.Int))
				return opts, false
			}

			opts.Quality = int(val.Int)
		case "maxWidth":
			if val.Kind != ir.AnnotationValueInt || val.Int < 0 {
				pc.Diag.Report(diag.ErrAssetBadArg, a.Name.Span, label, "`maxWidth:` must be a non-negative int")
				return opts, false
			}

			opts.MaxWidth = int(val.Int)
		case "maxHeight":
			if val.Kind != ir.AnnotationValueInt || val.Int < 0 {
				pc.Diag.Report(diag.ErrAssetBadArg, a.Name.Span, label, "`maxHeight:` must be a non-negative int")
				return opts, false
			}

			opts.MaxHeight = int(val.Int)
		default:
			pc.Diag.Report(diag.ErrAssetBadArg, a.Name.Span, label, fmt.Sprintf("unknown argument %q (supported: to, quality, maxWidth, maxHeight)", name))
			return opts, false
		}
	}

	return opts, true
}

func isStringTypeRef(t *ir.TypeRef) bool {
	if t == nil {
		return false
	}

	return t.Kind == ir.TK_PrimitiveString
}

func sanitizeAssetStem(s string) string {
	if s == "" {
		return "asset"
	}

	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	if b.Len() == 0 {
		return "asset"
	}

	return b.String()
}
