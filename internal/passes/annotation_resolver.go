package passes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sova/internal/diag"
	"sova/internal/ir"
)

type FileAnnotationCodes struct {
	BadPath            diag.DiagnosticCode
	FileNotFound       diag.DiagnosticCode
	FileTooLarge       diag.DiagnosticCode
	PathEscapesProject diag.DiagnosticCode
}

type FileAnnotation struct {
	PathLit   string
	AbsPath   string
	Content   []byte
	SizeBytes int64
}

func ResolveProjectRootAndCwd(pc *PassContext) string {
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

	abs, _ := filepath.Abs(projectRoot)
	return abs
}

type AnnotatedVarVisitor func(vd *ir.VarDeclStmt, anno *ir.Annotation, fileDir string)

type AnnotatedFieldVisitor func(fld *ir.TypeField, anno *ir.Annotation, fileDir string)

func WalkAnnotatedDecls(pc *PassContext, annoName, projectRoot string, onVar AnnotatedVarVisitor, onField AnnotatedFieldVisitor) {
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

			fileDir := resolveSourceFileDir(f, projectRoot)
			for _, st := range f.Hir.Statements {
				switch s := st.(type) {
				case *ir.VarDeclStmt:
					if onVar == nil {
						continue
					}

					if anno := FindAnnotationByName(s.Annotations, annoName); anno != nil {
						onVar(s, anno, fileDir)
					}

				case *ir.TypeDeclStmt:
					if onField == nil {
						continue
					}

					for _, fld := range s.Fields {
						if fld == nil {
							continue
						}

						if anno := FindAnnotationByName(fld.Annotations, annoName); anno != nil {
							onField(fld, anno, fileDir)
						}
					}
				}
			}
		}
	}
}

func FindAnnotationByName(annos []ir.Annotation, name string) *ir.Annotation {
	for i := range annos {
		if annos[i].Name.Name == name {
			return &annos[i]
		}
	}

	return nil
}

func ResolveAnnotatedFile(pc *PassContext, anno *ir.Annotation, label, fileDir, projectRoot string, sizeCap int64, codes FileAnnotationCodes) (*FileAnnotation, bool) {
	pathLit, ok := annotationPathArg(anno)
	if !ok {
		pc.Diag.Report(codes.BadPath, anno.Name.Span, "non-string or missing argument")
		return nil, false
	}

	if filepath.IsAbs(pathLit) || strings.HasPrefix(pathLit, "/") {
		pc.Diag.Report(codes.BadPath, anno.Name.Span, fmt.Sprintf("%q", pathLit))
		return nil, false
	}

	resolved := filepath.Clean(filepath.Join(fileDir, filepath.FromSlash(pathLit)))
	abs, err := filepath.Abs(resolved)
	if err != nil {
		pc.Diag.Report(codes.FileNotFound, anno.Name.Span, label, pathLit, resolved)
		return nil, false
	}

	if projectRoot != "" {
		rel, err := filepath.Rel(projectRoot, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			pc.Diag.Report(codes.PathEscapesProject, anno.Name.Span, pathLit)
			return nil, false
		}
	}

	stat, err := os.Stat(abs)
	if err != nil {
		pc.Diag.Report(codes.FileNotFound, anno.Name.Span, label, pathLit, abs)
		return nil, false
	}

	if stat.IsDir() {
		pc.Diag.Report(codes.FileNotFound, anno.Name.Span, label, pathLit, abs+" (is a directory)")
		return nil, false
	}

	if stat.Size() > sizeCap {
		pc.Diag.Report(codes.FileTooLarge, anno.Name.Span, label, pathLit, stat.Size(), sizeCap)
		return nil, false
	}

	content, err := os.ReadFile(abs)
	if err != nil {
		pc.Diag.Report(codes.FileNotFound, anno.Name.Span, label, pathLit, abs)
		return nil, false
	}

	return &FileAnnotation{
		PathLit:   pathLit,
		AbsPath:   abs,
		Content:   content,
		SizeBytes: stat.Size(),
	}, true
}
