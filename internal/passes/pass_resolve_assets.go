package passes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"sova/internal/diag"
	"sova/internal/imagepipe"
	"sova/internal/ir"
)

const AssetsCacheKey = "static_assets"

const defaultAssetMaxBytes int64 = 32 * 1024 * 1024

type AssetRecord struct {
	Decl               *ir.VarDeclStmt
	Info               *ir.AssetInfo
	PackageRoot        string
	TransformedContent []byte
}

var assetCodes = FileAnnotationCodes{
	BadPath:            diag.ErrAssetBadPath,
	FileNotFound:       diag.ErrAssetFileNotFound,
	FileTooLarge:       diag.ErrAssetFileTooLarge,
	PathEscapesProject: diag.ErrAssetPathEscapesProject,
}

type PassResolveAssets struct{}

func (p *PassResolveAssets) Name() string { return "resolve_assets" }

func (p *PassResolveAssets) Scope() PassScope { return PerBuild }

func (p *PassResolveAssets) Requires() []string { return []string{"fold_annotations"} }

func (p *PassResolveAssets) NoErrors() bool { return false }

func (p *PassResolveAssets) Run(pc *PassContext) error {
	sizeCap := defaultAssetMaxBytes
	projectRoot := ResolveProjectRootAndCwd(pc)

	var records []*AssetRecord
	WalkAnnotatedDecls(pc, "asset", projectRoot,
		func(vd *ir.VarDeclStmt, anno *ir.Annotation, fileDir string) {
			if rec := p.resolveTopLevel(pc, vd, anno, fileDir, projectRoot, sizeCap); rec != nil {
				records = append(records, rec)
			}
		},
		nil,
	)

	pc.Cache[AssetsCacheKey] = records
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

	fa, ok := ResolveAnnotatedFile(pc, anno, label, fileDir, projectRoot, sizeCap, assetCodes)
	if !ok {
		return nil
	}

	opts, ok := parseTransformOpts(pc, anno, label)
	if !ok {
		return nil
	}

	base := filepath.Base(fa.AbsPath)
	ext := filepath.Ext(base)
	content := fa.Content
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
		SourcePath:  fa.AbsPath,
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
