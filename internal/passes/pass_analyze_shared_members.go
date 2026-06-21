package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

const SharedTypeMembersCacheKey = "shared_type_members"

type PassAnalyzeSharedMembers struct{}

func (p *PassAnalyzeSharedMembers) Name() string     { return "analyze_shared_members" }

func (p *PassAnalyzeSharedMembers) Scope() PassScope { return PerPackage }

func (p *PassAnalyzeSharedMembers) Requires() []string {
	return []string{"analyze_externs", "fold_annotations"}
}

func (p *PassAnalyzeSharedMembers) NoErrors() bool { return false }

func (p *PassAnalyzeSharedMembers) Run(pc *PassContext) error {
	store, _ := pc.Cache[SharedTypeMembersCacheKey].(map[ir.TypID]*ir.SharedTypeMembers)
	if store == nil {
		store = map[ir.TypID]*ir.SharedTypeMembers{}

		pc.Cache[SharedTypeMembersCacheKey] = store
	}

	for _, f := range pc.Pkg.Files {
		fileSide := f.Hir.Side.Kind
		for _, st := range f.Hir.Statements {
			td, ok := st.(*ir.TypeDeclStmt)
			if !ok || td.IsExtern || td.Name.Sym == 0 {
				continue
			}

			summary := p.summariseType(pc, td, fileSide)
			if summary == nil {
				continue
			}

			sym, ok := pc.Pkg.Syms.GetByID(td.Name.Sym)
			if ok && sym.Typ != 0 {
				store[sym.Typ] = summary
			}
		}
	}

	return nil
}

func (p *PassAnalyzeSharedMembers) summariseType(pc *PassContext, td *ir.TypeDeclStmt, fileSide ir.SideKind) *ir.SharedTypeMembers {
	out := &ir.SharedTypeMembers{TypeDecl: td}

	for _, fld := range td.Fields {
		if !fld.IsShared && fileSide != ir.SideShared {
			continue
		}

		if fld.Type != nil && !isTransferableType(pc, fld.Type.Typ) {
			pc.Diag.Report(diag.ErrSharedFieldNonTransferable, fld.Name.Span, fld.Name.Name, formatTypeShort(pc, fld.Type.Typ))
		}

		out.Fields = append(out.Fields, fld)
	}

	for _, m := range td.Methods {
		if m.Func == nil {
			continue
		}

		if !m.IsShared && fileSide != ir.SideShared {
			continue
		}

		p.validateSignature(pc, td, m.Func)
		p.validateSharedBody(pc, td, m.Func)
		out.Methods = append(out.Methods, m)
	}

	for _, c := range td.Ctors {
		if !c.IsShared && fileSide != ir.SideShared {
			continue
		}

		p.validateCtorSignature(pc, td, c)
		p.validateSharedCtorBody(pc, td, c)
		out.Ctors = append(out.Ctors, c)
	}

	for _, cs := range td.Casts {
		if !cs.IsShared && fileSide != ir.SideShared {
			continue
		}

		out.Casts = append(out.Casts, cs)
	}

	if len(out.Fields) == 0 && len(out.Methods) == 0 && len(out.Ctors) == 0 && len(out.Casts) == 0 && fileSide != ir.SideShared {
		return nil
	}

	return out
}

func (p *PassAnalyzeSharedMembers) validateSignature(pc *PassContext, td *ir.TypeDeclStmt, fn *ir.FuncDeclStmt) {
	for _, param := range fn.Params {
		if param.Type != nil && !isTransferableType(pc, param.Type.Typ) {
			pc.Diag.Report(diag.ErrSharedSignatureNonTransferable, fn.Name.Span, td.Name.Name, fn.Name.Name, formatTypeShort(pc, param.Type.Typ))
		}
	}

	if fn.ReturnType != nil && !isTransferableType(pc, fn.ReturnType.Typ) {
		pc.Diag.Report(diag.ErrSharedSignatureNonTransferable, fn.Name.Span, td.Name.Name, fn.Name.Name, formatTypeShort(pc, fn.ReturnType.Typ))
	}
}

func (p *PassAnalyzeSharedMembers) validateCtorSignature(pc *PassContext, td *ir.TypeDeclStmt, c *ir.CtorDecl) {
	for _, param := range c.Params {
		if param.Type != nil && !isTransferableType(pc, param.Type.Typ) {
			pc.Diag.Report(diag.ErrSharedSignatureNonTransferable, td.Name.Span, td.Name.Name, "new", formatTypeShort(pc, param.Type.Typ))
		}
	}
}

func (p *PassAnalyzeSharedMembers) validateSharedBody(pc *PassContext, td *ir.TypeDeclStmt, fn *ir.FuncDeclStmt) {
	if fn.Body == nil {
		return
	}

	allowed := buildSharedAllowedSet(pc, td, fn.Params)
	p.walkSharedStmts(pc, td, fn.Name.Name, fn.Body.Stmts, allowed)
}

func (p *PassAnalyzeSharedMembers) validateSharedCtorBody(pc *PassContext, td *ir.TypeDeclStmt, c *ir.CtorDecl) {
	if c.Body == nil {
		return
	}

	allowed := buildSharedAllowedSet(pc, td, c.Params)
	p.walkSharedStmts(pc, td, "new", c.Body.Stmts, allowed)
}

type sharedAllowedSet struct {
	allowedSyms   map[ir.SymID]bool
	sharedFields  map[ir.SymID]string
	sharedMethods map[ir.SymID]string
	allFields     map[ir.SymID]string
	allMethods    map[ir.SymID]string
}

func buildSharedAllowedSet(pc *PassContext, td *ir.TypeDeclStmt, params []*ir.FuncParam) *sharedAllowedSet {
	s := &sharedAllowedSet{
		allowedSyms:   map[ir.SymID]bool{},
		sharedFields:  map[ir.SymID]string{},
		sharedMethods: map[ir.SymID]string{},
		allFields:     map[ir.SymID]string{},
		allMethods:    map[ir.SymID]string{},
	}

	for _, fld := range td.Fields {
		if fld.Name.Sym != 0 {
			s.allFields[fld.Name.Sym] = fld.Name.Name
			if fld.IsShared {
				s.sharedFields[fld.Name.Sym] = fld.Name.Name
				s.allowedSyms[fld.Name.Sym] = true
			}
		}
	}

	for _, m := range td.Methods {
		if m.Func == nil || m.Func.Name.Sym == 0 {
			continue
		}

		s.allMethods[m.Func.Name.Sym] = m.Func.Name.Name
		if m.IsShared {
			s.sharedMethods[m.Func.Name.Sym] = m.Func.Name.Name
			s.allowedSyms[m.Func.Name.Sym] = true
		}
	}

	for _, p := range params {
		if p != nil && p.Name.Sym != 0 {
			s.allowedSyms[p.Name.Sym] = true
		}
	}

	_ = pc
	return s
}

func (p *PassAnalyzeSharedMembers) walkSharedStmts(pc *PassContext, td *ir.TypeDeclStmt, methodName string, stmts []ir.Stmt, allowed *sharedAllowedSet) {
	for _, st := range stmts {
		p.walkSharedStmt(pc, td, methodName, st, allowed)
	}
}

func (p *PassAnalyzeSharedMembers) walkSharedStmt(pc *PassContext, td *ir.TypeDeclStmt, methodName string, st ir.Stmt, allowed *sharedAllowedSet) {
	switch s := st.(type) {
	case nil:
		return
	case *ir.BlockStmt:
		p.walkSharedStmts(pc, td, methodName, s.Stmts, allowed)
	case *ir.VarDeclStmt:
		for _, tgt := range s.Targets {
			if tgt.Name != nil && tgt.Name.Sym != 0 {
				allowed.allowedSyms[tgt.Name.Sym] = true
			}
		}

		p.walkSharedExpr(pc, td, methodName, s.Init, allowed)
	case *ir.MultiAssignmentStmt:
		for _, tgt := range s.Targets {
			if tgt.Name != nil && tgt.Name.Sym != 0 {
				if fname, ok := allowed.allFields[tgt.Name.Sym]; ok && !allowed.allowedSyms[tgt.Name.Sym] {
					pc.Diag.Report(diag.ErrSharedReferencesBackendField, tgt.Name.Span, td.Name.Name, methodName, fname)
				}
			}
		}

		p.walkSharedExpr(pc, td, methodName, s.Value, allowed)
	case *ir.FieldAssignmentStmt:
		p.walkSharedExpr(pc, td, methodName, s.Value, allowed)
	case *ir.ExprStmt:
		p.walkSharedExpr(pc, td, methodName, s.Expr, allowed)
	case *ir.ReturnStmt:
		for _, r := range s.Results {
			p.walkSharedExpr(pc, td, methodName, r, allowed)
		}

	case *ir.IfStmt:
		p.walkSharedExpr(pc, td, methodName, s.Cond, allowed)
		if s.Then != nil {
			p.walkSharedStmts(pc, td, methodName, s.Then.Stmts, allowed)
		}

		for _, eb := range s.ElseIfs {
			p.walkSharedExpr(pc, td, methodName, eb.Cond, allowed)
			if eb.Then != nil {
				p.walkSharedStmts(pc, td, methodName, eb.Then.Stmts, allowed)
			}
		}

		if s.Else != nil {
			p.walkSharedStmts(pc, td, methodName, s.Else.Stmts, allowed)
		}

	case *ir.ForStmt:
		if s.Body != nil {
			p.walkSharedStmts(pc, td, methodName, s.Body.Stmts, allowed)
		}

	case *ir.WhileStmt:
		p.walkSharedExpr(pc, td, methodName, s.Cond, allowed)
		if s.Body != nil {
			p.walkSharedStmts(pc, td, methodName, s.Body.Stmts, allowed)
		}
	}
}

func (p *PassAnalyzeSharedMembers) walkSharedExpr(pc *PassContext, td *ir.TypeDeclStmt, methodName string, e ir.Expr, allowed *sharedAllowedSet) {
	if ir.IsNilExpr(e) {
		return
	}

	switch x := e.(type) {
	case *ir.VarRef:
		p.checkSymReference(pc, td, methodName, x.Ref, allowed)
	case *ir.FieldAccessExpr:
		p.walkSharedExpr(pc, td, methodName, x.Expr, allowed)
	case *ir.FuncCallExpr:
		p.walkSharedExpr(pc, td, methodName, x.Callee, allowed)
		for _, a := range x.Args {
			p.walkSharedExpr(pc, td, methodName, a.Expr, allowed)
		}

	case *ir.IndexExpr:
		p.walkSharedExpr(pc, td, methodName, x.Expr, allowed)
		p.walkSharedExpr(pc, td, methodName, x.Index, allowed)
	case *ir.BinaryExpr:
		p.walkSharedExpr(pc, td, methodName, x.Left, allowed)
		p.walkSharedExpr(pc, td, methodName, x.Right, allowed)
	case *ir.UnaryExpr:
		p.walkSharedExpr(pc, td, methodName, x.Expr, allowed)
	case *ir.PrefixUnaryExpr:
		p.walkSharedExpr(pc, td, methodName, x.Expr, allowed)
	case *ir.PostfixUnaryExpr:
		p.walkSharedExpr(pc, td, methodName, x.Expr, allowed)
	case *ir.AsExpr:
		p.walkSharedExpr(pc, td, methodName, x.Expr, allowed)
	case *ir.InstanceofExpr:
		p.walkSharedExpr(pc, td, methodName, x.Expr, allowed)
	case *ir.CoalesceExpr:
		p.walkSharedExpr(pc, td, methodName, x.Left, allowed)
		p.walkSharedExpr(pc, td, methodName, x.Default, allowed)
	case *ir.TenaryExpr:
		p.walkSharedExpr(pc, td, methodName, x.Cond, allowed)
		p.walkSharedExpr(pc, td, methodName, x.Then, allowed)
		p.walkSharedExpr(pc, td, methodName, x.Else, allowed)
	case *ir.GroupedExpr:
		p.walkSharedExpr(pc, td, methodName, x.Expr, allowed)
	case *ir.ArrayLiteral:
		for _, el := range x.Elems {
			p.walkSharedExpr(pc, td, methodName, el, allowed)
		}

	case *ir.MapLiteral:
		for _, entry := range x.Entries {
			p.walkSharedExpr(pc, td, methodName, entry.Key, allowed)
			p.walkSharedExpr(pc, td, methodName, entry.Value, allowed)
		}

	case *ir.TupleLiteral:
		for _, el := range x.Elems {
			p.walkSharedExpr(pc, td, methodName, el, allowed)
		}

	case *ir.NewExpr:
		for _, a := range x.Args {
			p.walkSharedExpr(pc, td, methodName, a.Expr, allowed)
		}

	case *ir.AssignmentExpr:
		p.walkSharedExpr(pc, td, methodName, x.Right, allowed)
	}
}

func (p *PassAnalyzeSharedMembers) checkSymReference(pc *PassContext, td *ir.TypeDeclStmt, methodName string, ref ir.NameRef, allowed *sharedAllowedSet) {
	if ref.Sym == 0 {
		return
	}

	if allowed.allowedSyms[ref.Sym] {
		return
	}

	if fname, ok := allowed.allFields[ref.Sym]; ok {
		pc.Diag.Report(diag.ErrSharedReferencesBackendField, ref.Span, td.Name.Name, methodName, fname)
		return
	}

	if mname, ok := allowed.allMethods[ref.Sym]; ok {
		pc.Diag.Report(diag.ErrSharedReferencesBackendMethod, ref.Span, td.Name.Name, methodName, mname)
		return
	}

	if ref.Name == "this" || ref.Name == "" {
		return
	}

	sym, ok := lookupSymGlobal(pc, ref.Sym)
	if !ok {
		pc.Diag.Report(diag.ErrSharedReferencesBackendSymbol, ref.Span, td.Name.Name, methodName, ref.Name)
		return
	}

	declSide := lookupDeclSide(pc, sym)
	if declSide == ir.SideShared {
		return
	}

	if declSide == ir.SideBackend || declSide == ir.SideFrontend {
		pc.Diag.Report(diag.ErrSharedReferencesBackendPackage, ref.Span, td.Name.Name, methodName, ref.Name, sideLabel(declSide))
		return
	}

	pc.Diag.Report(diag.ErrSharedReferencesBackendSymbol, ref.Span, td.Name.Name, methodName, ref.Name)
}

func lookupDeclSide(pc *PassContext, sym *ir.Symbol) ir.SideKind {
	if sym == nil {
		return ir.SideShared
	}

	for _, otherPkg := range pc.Pkgs {
		for _, f := range otherPkg.Files {
			if f.Hir == nil {
				continue
			}

			for _, st := range f.Hir.Statements {
				if topLevelSymMatches(st, sym.ID) {
					return f.Hir.Side.Kind
				}
			}
		}
	}

	return ir.SideShared
}

func topLevelSymMatches(st ir.Stmt, sym ir.SymID) bool {
	switch s := st.(type) {
	case *ir.FuncDeclStmt:
		return s.Name.Sym == sym
	case *ir.VarDeclStmt:
		for _, tgt := range s.Targets {
			if tgt.Name != nil && tgt.Name.Sym == sym {
				return true
			}
		}

	case *ir.TypeDeclStmt:
		return s.Name.Sym == sym
	case *ir.EnumDeclStmt:
		return s.Name.Sym == sym
	case *ir.InterfaceDeclStmt:
		return s.Name.Sym == sym
	case *ir.MixinDeclStmt:
		return s.Name.Sym == sym
	case *ir.ExternDeclStmt:
		for _, fn := range s.Funcs {
			if fn.Name.Sym == sym {
				return true
			}
		}

		for _, v := range s.Vars {
			if v.Name.Sym == sym {
				return true
			}
		}
	}

	return false
}

func isTransferableType(pc *PassContext, t ir.TypID) bool {
	if t == 0 {
		return false
	}

	ty, ok := pc.Types.GetByID(t)
	if !ok {
		return false
	}

	switch ty.Kind {
	case ir.TK_PrimitiveInt, ir.TK_PrimitiveFloat, ir.TK_PrimitiveBool, ir.TK_PrimitiveString, ir.TK_PrimitiveChar, ir.TK_PrimitiveByte, ir.TK_PrimitiveAny, ir.TK_PrimitiveNone:
		return true
	case ir.TK_Option, ir.TK_Slice, ir.TK_Array:
		return isTransferableType(pc, ty.ElemType)
	case ir.TK_Map:
		return isTransferableType(pc, ty.KeyType) && isTransferableType(pc, ty.ValueType)
	case ir.TK_Tuple:
		for _, fld := range ty.Fields {
			if !isTransferableType(pc, fld.Type) {
				return false
			}
		}

		return true
	case ir.TK_Struct, ir.TK_Enum:
		return true
	case ir.TK_TypeParam:
		return true
	case ir.TK_Function:
		for _, p := range ty.Func.Params {
			if p == nil || p.Type == nil {
				continue
			}

			if !isTransferableType(pc, p.Type.Typ) {
				return false
			}
		}

		if ty.Func.ReturnType != 0 {
			return isTransferableType(pc, ty.Func.ReturnType)
		}

		return true
	}

	return false
}

func lookupSymGlobal(pc *PassContext, symID ir.SymID) (*ir.Symbol, bool) {
	if sym, ok := pc.Pkg.Syms.GetByID(symID); ok {
		return sym, true
	}

	for _, pkg := range pc.Pkgs {
		if pkg == pc.Pkg {
			continue
		}

		if sym, ok := pkg.Syms.GetByID(symID); ok {
			return sym, true
		}
	}

	return nil, false
}

func formatTypeShort(pc *PassContext, t ir.TypID) string {
	if t == 0 {
		return "?"
	}

	ty, ok := pc.Types.GetByID(t)
	if !ok {
		return "?"
	}

	switch ty.Kind {
	case ir.TK_PrimitiveInt:
		return "int"
	case ir.TK_PrimitiveFloat:
		return "float"
	case ir.TK_PrimitiveBool:
		return "bool"
	case ir.TK_PrimitiveString:
		return "string"
	case ir.TK_PrimitiveChar:
		return "char"
	case ir.TK_PrimitiveByte:
		return "byte"
	case ir.TK_PrimitiveAny:
		return "any"
	case ir.TK_PrimitiveNone:
		return "none"
	case ir.TK_Option:
		return "option<" + formatTypeShort(pc, ty.ElemType) + ">"
	case ir.TK_Slice:
		return "[]" + formatTypeShort(pc, ty.ElemType)
	case ir.TK_Array:
		return "[N]" + formatTypeShort(pc, ty.ElemType)
	case ir.TK_Map:
		return "map<" + formatTypeShort(pc, ty.KeyType) + ", " + formatTypeShort(pc, ty.ValueType) + ">"
	case ir.TK_Struct:
		if ty.Struct.Name != "" {
			return ty.Struct.Name
		}

		return "struct"
	case ir.TK_Enum:
		if ty.Enum.Name != "" {
			return ty.Enum.Name
		}

		return "enum"
	case ir.TK_Function:
		return "func"
	case ir.TK_Interface:
		if ty.Interface.Name != "" {
			return ty.Interface.Name
		}

		return "interface"
	case ir.TK_Chan:
		return "chan<" + formatTypeShort(pc, ty.ElemType) + ">"
	case ir.TK_TypeParam:
		return ty.ParamName
	}

	return "?"
}
