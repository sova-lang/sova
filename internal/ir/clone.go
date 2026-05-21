package ir

// ExtendSyntheticCtor appends a constructor parameter and `this.<field> = <field>` body assignment for each new field, mutating the synthetic ctor in place. When the type has no constructor yet (e.g. a type whose only fields come from mixed-in mixins), a fresh synthetic ctor is created. Types with a user-written constructor keep their explicit param lists untouched. Used by `pass_inline_mixins` so mixed-in fields participate in the auto-generated named-args ctor like any locally-declared field would.
func ExtendSyntheticCtor(td *TypeDeclStmt, newFields []*TypeField, alloc *IdAlloc) {
	if td.IsExtern {
		return
	}
	if len(td.Ctors) == 0 {
		ctor := &CtorDecl{
			node:        node{id: NodeID(alloc.Next()), span: td.Name.Span},
			IsSynthetic: true,
			Body: &BlockStmt{
				node: node{id: NodeID(alloc.Next()), span: td.Name.Span},
			},
		}
		td.Ctors = append(td.Ctors, ctor)
	}
	ctor := td.Ctors[0]
	if !ctor.IsSynthetic {
		return
	}
	for _, fld := range newFields {
		param := &FuncParam{
			node: node{id: NodeID(alloc.Next()), span: fld.Name.Span},
			Name: NameRef{Name: fld.Name.Name, Span: fld.Name.Span},
			Type: cloneTypeRef(fld.Type, alloc),
		}
		if fld.Default != nil {
			param.Default = CloneExpr(fld.Default, alloc)
		}
		ctor.Params = append(ctor.Params, param)
		if ctor.Body == nil {
			continue
		}
		rhs := &VarRef{
			node: node{id: NodeID(alloc.Next()), span: fld.Name.Span},
			Ref:  NameRef{Name: fld.Name.Name, Span: fld.Name.Span},
		}
		assign := &FieldAssignmentStmt{
			node:     node{id: NodeID(alloc.Next()), span: fld.Name.Span},
			Receiver: NameRef{Name: "this", Span: fld.Name.Span},
			Fields:   []FieldName{{Name: fld.Name.Name, Span: fld.Name.Span}},
			Op:       OpAssign,
			Value:    rhs,
		}
		ctor.Body.Stmts = append(ctor.Body.Stmts, assign)
	}
}

// CloneTypeField produces a deep copy of a TypeField with fresh node IDs allocated from alloc. Symbol IDs in NameRefs are reset to 0 so that a subsequent bind pass can re-declare them in the target scope.
func CloneTypeField(f *TypeField, alloc *IdAlloc) *TypeField {
	if f == nil {
		return nil
	}
	return &TypeField{
		node:    node{id: NodeID(alloc.Next()), span: f.span},
		Name:    NameRef{Name: f.Name.Name, Span: f.Name.Span},
		Type:    cloneTypeRef(f.Type, alloc),
		Default: CloneExpr(f.Default, alloc),
		Private: f.Private,
	}
}

// CloneTypeMethodDecl produces a deep copy of a TypeMethodDecl with fresh node IDs. The receiver symbol is reset; a later bind pass re-declares it.
func CloneTypeMethodDecl(m *TypeMethodDecl, alloc *IdAlloc) *TypeMethodDecl {
	if m == nil {
		return nil
	}
	return &TypeMethodDecl{
		node:    node{id: NodeID(alloc.Next()), span: m.span},
		Private: m.Private,
		Func:    cloneFuncDeclStmt(m.Func, alloc),
	}
}

func cloneFuncDeclStmt(s *FuncDeclStmt, alloc *IdAlloc) *FuncDeclStmt {
	if s == nil {
		return nil
	}
	out := &FuncDeclStmt{
		node:       node{id: NodeID(alloc.Next()), span: s.span},
		Side:       s.Side,
		Name:       NameRef{Name: s.Name.Name, Span: s.Name.Span},
		ReturnType: cloneTypeRef(s.ReturnType, alloc),
		Body:       cloneBlockStmt(s.Body, alloc),
	}
	for _, p := range s.Params {
		out.Params = append(out.Params, cloneFuncParam(p, alloc))
	}
	return out
}

func cloneFuncParam(p *FuncParam, alloc *IdAlloc) *FuncParam {
	if p == nil {
		return nil
	}
	return &FuncParam{
		node:       node{id: NodeID(alloc.Next()), span: p.span},
		IsVariadic: p.IsVariadic,
		Name:       NameRef{Name: p.Name.Name, Span: p.Name.Span},
		Type:       cloneTypeRef(p.Type, alloc),
		Default:    CloneExpr(p.Default, alloc),
	}
}

func cloneTypeRef(t *TypeRef, alloc *IdAlloc) *TypeRef {
	if t == nil {
		return nil
	}
	out := &TypeRef{
		node:            node{id: NodeID(alloc.Next()), span: t.span},
		Kind:            t.Kind,
		Typ:             t.Typ,
		Dim:             t.Dim,
		CustomName:      t.CustomName,
		CustomQualifier: t.CustomQualifier,
		Elem:            cloneTypeRef(t.Elem, alloc),
		Key:             cloneTypeRef(t.Key, alloc),
		Value:           cloneTypeRef(t.Value, alloc),
		FuncReturn:      cloneTypeRef(t.FuncReturn, alloc),
	}
	for _, tf := range t.Tuple {
		out.Tuple = append(out.Tuple, TupleFieldRef{Name: tf.Name, Type: cloneTypeRef(tf.Type, alloc)})
	}
	for _, fp := range t.FuncParams {
		out.FuncParams = append(out.FuncParams, FuncTypeParamRef{Name: fp.Name, Type: cloneTypeRef(fp.Type, alloc)})
	}
	for _, ta := range t.TypeArgs {
		out.TypeArgs = append(out.TypeArgs, cloneTypeRef(ta, alloc))
	}
	return out
}

func cloneBlockStmt(b *BlockStmt, alloc *IdAlloc) *BlockStmt {
	if b == nil {
		return nil
	}
	out := &BlockStmt{node: node{id: NodeID(alloc.Next()), span: b.span}}
	for _, s := range b.Stmts {
		out.Stmts = append(out.Stmts, CloneStmt(s, alloc))
	}
	return out
}

// CloneStmt produces a deep copy of any statement, assigning fresh node IDs from alloc. Symbol IDs on identifiers are reset to 0.
func CloneStmt(s Stmt, alloc *IdAlloc) Stmt {
	if s == nil {
		return nil
	}
	switch v := s.(type) {
	case *BlockStmt:
		return cloneBlockStmt(v, alloc)
	case *VarDeclStmt:
		out := &VarDeclStmt{node: node{id: NodeID(alloc.Next()), span: v.span}, IsConst: v.IsConst, Init: CloneExpr(v.Init, alloc)}
		for _, t := range v.Targets {
			out.Targets = append(out.Targets, VarDeclTarget{
				Name:    cloneNameRefPtr(t.Name),
				TypeAnn: cloneTypeRef(t.TypeAnn, alloc),
			})
		}
		return out
	case *ExprStmt:
		return &ExprStmt{node: node{id: NodeID(alloc.Next()), span: v.span}, Expr: CloneExpr(v.Expr, alloc)}
	case *FieldAssignmentStmt:
		out := &FieldAssignmentStmt{
			node:     node{id: NodeID(alloc.Next()), span: v.span},
			Receiver: NameRef{Name: v.Receiver.Name, Span: v.Receiver.Span},
			Op:       v.Op,
			Value:    CloneExpr(v.Value, alloc),
		}
		for _, f := range v.Fields {
			out.Fields = append(out.Fields, FieldName{Name: f.Name, Span: f.Span})
		}
		return out
	case *MultiAssignmentStmt:
		out := &MultiAssignmentStmt{node: node{id: NodeID(alloc.Next()), span: v.span}, Value: CloneExpr(v.Value, alloc)}
		for _, t := range v.Targets {
			out.Targets = append(out.Targets, AssignmentTarget{Name: cloneNameRefPtr(t.Name)})
		}
		return out
	case *IfStmt:
		out := &IfStmt{
			node: node{id: NodeID(alloc.Next()), span: v.span},
			Cond: CloneExpr(v.Cond, alloc),
			Then: cloneBlockStmt(v.Then, alloc),
			Else: cloneBlockStmt(v.Else, alloc),
		}
		for _, eb := range v.ElseIfs {
			out.ElseIfs = append(out.ElseIfs, ElseIfBranch{
				Cond: CloneExpr(eb.Cond, alloc),
				Then: cloneBlockStmt(eb.Then, alloc),
			})
		}
		return out
	case *SwitchStmt:
		out := &SwitchStmt{node: node{id: NodeID(alloc.Next()), span: v.span}, Expr: CloneExpr(v.Expr, alloc)}
		for _, c := range v.Cases {
			sc := SwitchCase{}
			for _, val := range c.Values {
				sc.Values = append(sc.Values, CloneExpr(val, alloc))
			}
			for _, st := range c.Stmts {
				sc.Stmts = append(sc.Stmts, CloneStmt(st, alloc))
			}
			out.Cases = append(out.Cases, sc)
		}
		for _, st := range v.Default {
			out.Default = append(out.Default, CloneStmt(st, alloc))
		}
		return out
	case *BreakStmt:
		return &BreakStmt{node: node{id: NodeID(alloc.Next()), span: v.span}, Depth: v.Depth}
	case *ContinueStmt:
		return &ContinueStmt{node: node{id: NodeID(alloc.Next()), span: v.span}, Depth: v.Depth}
	case *ReturnStmt:
		out := &ReturnStmt{node: node{id: NodeID(alloc.Next()), span: v.span}}
		for _, r := range v.Results {
			out.Results = append(out.Results, CloneExpr(r, alloc))
		}
		return out
	case *GuardStmt:
		out := &GuardStmt{node: node{id: NodeID(alloc.Next()), span: v.span}, Cond: CloneExpr(v.Cond, alloc)}
		for _, r := range v.Returns {
			out.Returns = append(out.Returns, CloneExpr(r, alloc))
		}
		return out
	case *ForStmt:
		out := &ForStmt{
			node:     node{id: NodeID(alloc.Next()), span: v.span},
			CondType: v.CondType,
			Body:     cloneBlockStmt(v.Body, alloc),
		}
		if v.CondInt != nil {
			out.CondInt = &ForCondIntDecl{
				Init: cloneVarDeclStmtRaw(v.CondInt.Init, alloc),
				Cond: CloneExpr(v.CondInt.Cond, alloc),
				Post: CloneExpr(v.CondInt.Post, alloc),
			}
		}
		if v.CondRange != nil {
			out.CondRange = &ForCondRangeDecl{
				RangeVar:   NameRef{Name: v.CondRange.RangeVar.Name, Span: v.CondRange.RangeVar.Span},
				RangeStart: CloneExpr(v.CondRange.RangeStart, alloc),
				RangeEnd:   CloneExpr(v.CondRange.RangeEnd, alloc),
			}
		}
		if v.CondIn != nil {
			out.CondIn = &ForCondInDecl{
				InFirstVar:  NameRef{Name: v.CondIn.InFirstVar.Name, Span: v.CondIn.InFirstVar.Span},
				InSecondVar: cloneNameRefPtr(v.CondIn.InSecondVar),
				InThirdVar:  cloneNameRefPtr(v.CondIn.InThirdVar),
				IterExpr:    CloneExpr(v.CondIn.IterExpr, alloc),
			}
		}
		return out
	case *WhileStmt:
		return &WhileStmt{
			node: node{id: NodeID(alloc.Next()), span: v.span},
			Cond: CloneExpr(v.Cond, alloc),
			Body: cloneBlockStmt(v.Body, alloc),
		}
	case *FuncDeclStmt:
		return cloneFuncDeclStmt(v, alloc)
	}
	return s
}

func cloneVarDeclStmtRaw(v *VarDeclStmt, alloc *IdAlloc) *VarDeclStmt {
	if v == nil {
		return nil
	}
	out := &VarDeclStmt{node: node{id: NodeID(alloc.Next()), span: v.span}, IsConst: v.IsConst, Init: CloneExpr(v.Init, alloc)}
	for _, t := range v.Targets {
		out.Targets = append(out.Targets, VarDeclTarget{
			Name:    cloneNameRefPtr(t.Name),
			TypeAnn: cloneTypeRef(t.TypeAnn, alloc),
		})
	}
	return out
}

func cloneNameRefPtr(n *NameRef) *NameRef {
	if n == nil {
		return nil
	}
	return &NameRef{Name: n.Name, Span: n.Span}
}

// CloneExpr produces a deep copy of any expression, assigning fresh node IDs from alloc. Resolved types and symbol IDs are reset so that the clone can be re-analyzed in a new context.
func CloneExpr(e Expr, alloc *IdAlloc) Expr {
	if e == nil {
		return nil
	}
	switch v := e.(type) {
	case *LitInt:
		return &LitInt{node: node{id: NodeID(alloc.Next()), span: v.span}, Value: v.Value}
	case *LitFloat:
		return &LitFloat{node: node{id: NodeID(alloc.Next()), span: v.span}, Value: v.Value}
	case *LitString:
		return &LitString{node: node{id: NodeID(alloc.Next()), span: v.span}, Value: v.Value}
	case *LitChar:
		return &LitChar{node: node{id: NodeID(alloc.Next()), span: v.span}, Value: v.Value}
	case *LitBool:
		return &LitBool{node: node{id: NodeID(alloc.Next()), span: v.span}, Value: v.Value}
	case *LitNone:
		return &LitNone{node: node{id: NodeID(alloc.Next()), span: v.span}}
	case *VarRef:
		return &VarRef{
			node: node{id: NodeID(alloc.Next()), span: v.span},
			Ref:  NameRef{Name: v.Ref.Name, Span: v.Ref.Span},
		}
	case *BinaryExpr:
		return &BinaryExpr{
			node:  node{id: NodeID(alloc.Next()), span: v.span},
			Left:  CloneExpr(v.Left, alloc),
			Op:    v.Op,
			Right: CloneExpr(v.Right, alloc),
		}
	case *UnaryExpr:
		return &UnaryExpr{node: node{id: NodeID(alloc.Next()), span: v.span}, Op: v.Op, Expr: CloneExpr(v.Expr, alloc)}
	case *PrefixUnaryExpr:
		return &PrefixUnaryExpr{node: node{id: NodeID(alloc.Next()), span: v.span}, Op: v.Op, Expr: CloneExpr(v.Expr, alloc)}
	case *PostfixUnaryExpr:
		return &PostfixUnaryExpr{node: node{id: NodeID(alloc.Next()), span: v.span}, Op: v.Op, Expr: CloneExpr(v.Expr, alloc)}
	case *AssignmentExpr:
		return &AssignmentExpr{
			node:  node{id: NodeID(alloc.Next()), span: v.span},
			Left:  NameRef{Name: v.Left.Name, Span: v.Left.Span},
			Op:    v.Op,
			Right: CloneExpr(v.Right, alloc),
		}
	case *IndexExpr:
		return &IndexExpr{
			node:  node{id: NodeID(alloc.Next()), span: v.span},
			Expr:  CloneExpr(v.Expr, alloc),
			Index: CloneExpr(v.Index, alloc),
		}
	case *FieldAccessExpr:
		out := &FieldAccessExpr{node: node{id: NodeID(alloc.Next()), span: v.span}, Expr: CloneExpr(v.Expr, alloc)}
		for _, f := range v.Fields {
			out.Fields = append(out.Fields, FieldName{Name: f.Name, Span: f.Span})
		}
		return out
	case *FuncCallExpr:
		out := &FuncCallExpr{node: node{id: NodeID(alloc.Next()), span: v.span}, Callee: CloneExpr(v.Callee, alloc)}
		for _, a := range v.Args {
			out.Args = append(out.Args, FuncCallArg{Name: a.Name, Expr: CloneExpr(a.Expr, alloc)})
		}
		return out
	case *FuncLitExpr:
		out := &FuncLitExpr{
			node:       node{id: NodeID(alloc.Next()), span: v.span},
			ReturnType: cloneTypeRef(v.ReturnType, alloc),
			Body:       cloneBlockStmt(v.Body, alloc),
		}
		for _, p := range v.Params {
			out.Params = append(out.Params, cloneFuncParam(p, alloc))
		}
		return out
	case *RangeExpr:
		return &RangeExpr{
			node:  node{id: NodeID(alloc.Next()), span: v.span},
			Start: CloneExpr(v.Start, alloc),
			End:   CloneExpr(v.End, alloc),
			Inc:   CloneExpr(v.Inc, alloc),
		}
	case *TenaryExpr:
		return &TenaryExpr{
			node: node{id: NodeID(alloc.Next()), span: v.span},
			Cond: CloneExpr(v.Cond, alloc),
			Then: CloneExpr(v.Then, alloc),
			Else: CloneExpr(v.Else, alloc),
		}
	case *CoalesceExpr:
		return &CoalesceExpr{
			node:    node{id: NodeID(alloc.Next()), span: v.span},
			Left:    CloneExpr(v.Left, alloc),
			Default: CloneExpr(v.Default, alloc),
		}
	case *GroupedExpr:
		return &GroupedExpr{node: node{id: NodeID(alloc.Next()), span: v.span}, Expr: CloneExpr(v.Expr, alloc)}
	case *WhenExpr:
		out := &WhenExpr{
			node:    node{id: NodeID(alloc.Next()), span: v.span},
			Expr:    CloneExpr(v.Expr, alloc),
			Default: CloneExpr(v.Default, alloc),
		}
		for _, c := range v.Cases {
			wc := WhenCase{Then: CloneExpr(c.Then, alloc)}
			for _, val := range c.Values {
				wc.Values = append(wc.Values, CloneExpr(val, alloc))
			}
			out.Cases = append(out.Cases, wc)
		}
		return out
	case *StringTemplateExpr:
		out := &StringTemplateExpr{node: node{id: NodeID(alloc.Next()), span: v.span}}
		for _, part := range v.Parts {
			out.Parts = append(out.Parts, StringTemplatePart{Lit: part.Lit, Expr: CloneExpr(part.Expr, alloc)})
		}
		return out
	case *SessionExpr:
		return &SessionExpr{node: node{id: NodeID(alloc.Next()), span: v.span}}
	case *NewExpr:
		out := &NewExpr{
			node:      node{id: NodeID(alloc.Next()), span: v.span},
			Qualifier: v.Qualifier,
			TypeName:  NameRef{Name: v.TypeName.Name, Span: v.TypeName.Span},
		}
		for _, a := range v.Args {
			out.Args = append(out.Args, FuncCallArg{Name: a.Name, Expr: CloneExpr(a.Expr, alloc)})
		}
		return out
	case *ArrayLiteral:
		out := &ArrayLiteral{node: node{id: NodeID(alloc.Next()), span: v.span}}
		for _, el := range v.Elems {
			out.Elems = append(out.Elems, CloneExpr(el, alloc))
		}
		return out
	case *MapLiteral:
		out := &MapLiteral{node: node{id: NodeID(alloc.Next()), span: v.span}}
		for _, en := range v.Entries {
			out.Entries = append(out.Entries, MapEntry{Key: CloneExpr(en.Key, alloc), Value: CloneExpr(en.Value, alloc)})
		}
		return out
	case *TupleLiteral:
		out := &TupleLiteral{node: node{id: NodeID(alloc.Next()), span: v.span}}
		for _, el := range v.Elems {
			out.Elems = append(out.Elems, CloneExpr(el, alloc))
		}
		return out
	}
	return e
}
