package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
	"strconv"
	"strings"
)

// PassFoldAnnotations walks every annotation in the package and folds its argument expressions into compile-time constant values. Annotations whose arguments depend on runtime state produce a diagnostic; downstream consumers (Go struct tags, future route hints) only see annotations with ResolvedArgs populated.
type PassFoldAnnotations struct{}

func (p *PassFoldAnnotations) Name() string       { return "fold_annotations" }
func (p *PassFoldAnnotations) Scope() PassScope   { return PerPackage }
func (p *PassFoldAnnotations) Requires() []string { return []string{"infer_types"} }
func (p *PassFoldAnnotations) NoErrors() bool     { return false }

func (p *PassFoldAnnotations) Run(pc *PassContext) error {
	for _, f := range pc.Pkg.Files {
		for _, st := range f.Hir.Statements {
			p.walkStmt(pc, st)
		}
	}
	return nil
}

func (p *PassFoldAnnotations) walkStmt(pc *PassContext, st ir.Stmt) {
	switch s := st.(type) {
	case *ir.FuncDeclStmt:
		p.foldList(pc, s.Annotations)
	case *ir.VarDeclStmt:
		p.foldList(pc, s.Annotations)
	case *ir.TypeDeclStmt:
		p.foldList(pc, s.Annotations)
		for _, fld := range s.Fields {
			p.foldList(pc, fld.Annotations)
			p.validateTagShape(pc, fld.Annotations, fld.Name.Name, s.Name.Name)
		}
		for _, m := range s.Methods {
			p.foldList(pc, m.Annotations)
			if m.Func != nil {
				p.foldList(pc, m.Func.Annotations)
			}
		}
		for _, c := range s.Ctors {
			p.foldList(pc, c.Annotations)
		}
	}
}

func (p *PassFoldAnnotations) foldList(pc *PassContext, annos []ir.Annotation) {
	for i := range annos {
		a := &annos[i]
		a.ResolvedArgs = make([]ir.AnnotationValue, 0, len(a.Args))
		for _, arg := range a.Args {
			val, ok := foldAnnotationExpr(pc, arg)
			if !ok {
				pc.Diag.Report(diag.ErrAnnotationNotConst, arg.Span(), a.Name.Name)
				continue
			}
			a.ResolvedArgs = append(a.ResolvedArgs, val)
		}
	}
}

// validateTagShape enforces the `@tag` annotation's two-string-arg contract on every annotation in `annos` whose name is `tag`. The annotation is the user-facing surface for Go struct tags emitted on type fields: each occurrence contributes one `key:"value"` pair, where `key` is the tag namespace (`json`, `gorm`, `validate`, ...) and `value` is the literal tag content. Restricting to exactly two string args keeps the surface free of library-specific knowledge in the compiler — the codegen only joins the recorded key/value pairs into the Go tag string at struct emission. Reports a per-annotation diagnostic on shape violations; the codegen later skips any malformed `@tag` so the rendered Go does not carry partial tag data.
func (p *PassFoldAnnotations) validateTagShape(pc *PassContext, annos []ir.Annotation, fieldName, typeName string) {
	for i := range annos {
		a := &annos[i]
		if a.Name.Name != "structTag" {
			continue
		}
		ctx := fieldName
		if typeName != "" {
			ctx = typeName + "." + fieldName
		}
		if len(a.ResolvedArgs) != 2 {
			pc.Diag.Report(diag.ErrTagAnnotationMalformed, a.Name.Span, ctx)
			continue
		}
		if a.ResolvedArgs[0].Kind != ir.AnnotationValueString || a.ResolvedArgs[1].Kind != ir.AnnotationValueString {
			pc.Diag.Report(diag.ErrTagAnnotationMalformed, a.Name.Span, ctx)
			continue
		}
		if a.ResolvedArgs[0].Str == "" {
			pc.Diag.Report(diag.ErrTagAnnotationMalformed, a.Name.Span, ctx)
		}
	}
}

// foldAnnotationExpr reduces an annotation argument to a compile-time constant. It handles the literal kinds Sova currently exposes plus string concatenation via `+`, which is the common case for assembling tags from a fixed base and a const-named segment. Anything else returns ok=false.
func foldAnnotationExpr(pc *PassContext, e ir.Expr) (ir.AnnotationValue, bool) {
	switch x := e.(type) {
	case *ir.LitString:
		return ir.AnnotationValue{Kind: ir.AnnotationValueString, Str: x.Value}, true
	case *ir.LitInt:
		return ir.AnnotationValue{Kind: ir.AnnotationValueInt, Int: x.Value}, true
	case *ir.LitBool:
		return ir.AnnotationValue{Kind: ir.AnnotationValueBool, Bool: x.Value}, true
	case *ir.BinaryExpr:
		if x.Op != ir.OpAdd {
			return ir.AnnotationValue{}, false
		}
		l, lok := foldAnnotationExpr(pc, x.Left)
		r, rok := foldAnnotationExpr(pc, x.Right)
		if !lok || !rok {
			return ir.AnnotationValue{}, false
		}
		if l.Kind == ir.AnnotationValueString && r.Kind == ir.AnnotationValueString {
			return ir.AnnotationValue{Kind: ir.AnnotationValueString, Str: l.Str + r.Str}, true
		}
		if l.Kind == ir.AnnotationValueInt && r.Kind == ir.AnnotationValueInt {
			return ir.AnnotationValue{Kind: ir.AnnotationValueInt, Int: l.Int + r.Int}, true
		}
		return ir.AnnotationValue{}, false
	case *ir.VarRef:
		if x.Ref.Sym == 0 {
			return ir.AnnotationValue{}, false
		}
		sym, ok := pc.Pkg.Syms.GetByID(x.Ref.Sym)
		if !ok || sym.Kind != ir.SK_Variable || !sym.IsConst() {
			return ir.AnnotationValue{}, false
		}
		init := findConstInitExpr(pc, sym.ID)
		if init == nil {
			return ir.AnnotationValue{}, false
		}
		return foldAnnotationExpr(pc, init)
	case *ir.GroupedExpr:
		return foldAnnotationExpr(pc, x.Expr)
	case *ir.StringTemplateExpr:
		var sb strings.Builder
		for _, part := range x.Parts {
			if part.Expr == nil {
				sb.WriteString(part.Lit)
				continue
			}
			val, ok := foldAnnotationExpr(pc, part.Expr)
			if !ok {
				return ir.AnnotationValue{}, false
			}
			switch val.Kind {
			case ir.AnnotationValueString:
				sb.WriteString(val.Str)
			case ir.AnnotationValueInt:
				sb.WriteString(strconv.FormatInt(val.Int, 10))
			case ir.AnnotationValueBool:
				sb.WriteString(strconv.FormatBool(val.Bool))
			default:
				return ir.AnnotationValue{}, false
			}
		}
		return ir.AnnotationValue{Kind: ir.AnnotationValueString, Str: sb.String()}, true
	}
	return ir.AnnotationValue{}, false
}

// findConstInitExpr scans file-level VarDeclStmt entries in the current package and returns the init expression bound to sym, or nil when sym isn't a single-target top-level const declaration.
func findConstInitExpr(pc *PassContext, sym ir.SymID) ir.Expr {
	for _, f := range pc.Pkg.Files {
		for _, st := range f.Hir.Statements {
			vd, ok := st.(*ir.VarDeclStmt)
			if !ok || !vd.IsConst || len(vd.Targets) != 1 {
				continue
			}
			if vd.Targets[0].Name == nil || vd.Targets[0].Name.Sym != sym {
				continue
			}
			return vd.Init
		}
	}
	return nil
}
