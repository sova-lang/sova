// Code generated from /home/dasdarki/Development/DasDarki/Sova/sova/Sova.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // Sova

import "github.com/antlr4-go/antlr/v4"

type BaseSovaVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseSovaVisitor) VisitFile(ctx *FileContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFileHeader(ctx *FileHeaderContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitPackageDecl(ctx *PackageDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitPackagePath(ctx *PackagePathContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitPkgIdent(ctx *PkgIdentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSoftId(ctx *SoftIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSideDecl(ctx *SideDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSide(ctx *SideContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitStmt(ctx *StmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGoStmt(ctx *GoStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitDeferStmt(ctx *DeferStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSelectStmt(ctx *SelectStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSelectCase(ctx *SelectCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSelectDefaultCase(ctx *SelectDefaultCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSelectCaseGuard(ctx *SelectCaseGuardContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSelectRecvBinding(ctx *SelectRecvBindingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTestDeclStmt(ctx *TestDeclStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGroupDeclStmt(ctx *GroupDeclStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTestTagList(ctx *TestTagListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitAsSessionStmt(ctx *AsSessionStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGroupItem(ctx *GroupItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSetupStmt(ctx *SetupStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTeardownStmt(ctx *TeardownStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitAssertStmt(ctx *AssertStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWireGroupStmt(ctx *WireGroupStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWireRulesetStmt(ctx *WireRulesetStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitImportStmt(ctx *ImportStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitUsingClause(ctx *UsingClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitBlock(ctx *BlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitVarDeclStmt(ctx *VarDeclStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitVarDeclTarget(ctx *VarDeclTargetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncDeclStmt(ctx *FuncDeclStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGenericParams(ctx *GenericParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGenericParam(ctx *GenericParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWireSpec(ctx *WireSpecContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWireOptions(ctx *WireOptionsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWireOption(ctx *WireOptionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncParamList(ctx *FuncParamListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncParam(ctx *FuncParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitExternDecl(ctx *ExternDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitExternItem(ctx *ExternItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitExternFunc(ctx *ExternFuncContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitExternVar(ctx *ExternVarContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSimpleExternMapping(ctx *SimpleExternMappingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSharedExternMapping(ctx *SharedExternMappingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitExternSideMapping(ctx *ExternSideMappingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitExternSide(ctx *ExternSideContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitEnumDeclStmt(ctx *EnumDeclStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitEnumPayloadDef(ctx *EnumPayloadDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitEnumFieldDef(ctx *EnumFieldDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitEnumBody(ctx *EnumBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitEnumCase(ctx *EnumCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitEnumCaseArgs(ctx *EnumCaseArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitEnumMethod(ctx *EnumMethodContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTypeDeclStmt(ctx *TypeDeclStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTypeClause(ctx *TypeClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitImplementsClause(ctx *ImplementsClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWithClause(ctx *WithClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitQualifiedRef(ctx *QualifiedRefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTypeMember(ctx *TypeMemberContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFieldDecl(ctx *FieldDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitCtorDecl(ctx *CtorDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMethodDecl(ctx *MethodDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitCastDecl(ctx *CastDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMemberModifier(ctx *MemberModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitAnnotation(ctx *AnnotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitComposableBareChild(ctx *ComposableBareChildContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitComposableExprChild(ctx *ComposableExprChildContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitComposableIfChild(ctx *ComposableIfChildContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitComposableForChild(ctx *ComposableForChildContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitComposableWhileChild(ctx *ComposableWhileChildContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitComposableSwitchChild(ctx *ComposableSwitchChildContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMethodName(ctx *MethodNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitOpSymbol(ctx *OpSymbolContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitInterfaceDeclStmt(ctx *InterfaceDeclStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMethodSignature(ctx *MethodSignatureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMixinDeclStmt(ctx *MixinDeclStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMixinMember(ctx *MixinMemberContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthDeclStmt(ctx *SynthDeclStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthParams(ctx *SynthParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthRequiredSide(ctx *SynthRequiredSideContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthTarget(ctx *SynthTargetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthTargetKind(ctx *SynthTargetKindContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthBodyItem(ctx *SynthBodyItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthEmitOn(ctx *SynthEmitOnContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthEmitAppend(ctx *SynthEmitAppendContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthEmitField(ctx *SynthEmitFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthEmitMethod(ctx *SynthEmitMethodContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthEmitCtor(ctx *SynthEmitCtorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthForStmt(ctx *SynthForStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthIterable(ctx *SynthIterableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthWhere(ctx *SynthWhereContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSynthBoolExpr(ctx *SynthBoolExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTypeAliasStmt(ctx *TypeAliasStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitIfStmt(ctx *IfStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitElseIfBranch(ctx *ElseIfBranchContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitElseBranch(ctx *ElseBranchContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitBreakStmt(ctx *BreakStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitContinueStmt(ctx *ContinueStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitReturnStmt(ctx *ReturnStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGuardStmt(ctx *GuardStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGuardReturn(ctx *GuardReturnContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSwitchStmt(ctx *SwitchStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSwitchCase(ctx *SwitchCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitDefaultCase(ctx *DefaultCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitForStmt(ctx *ForStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitForCondition(ctx *ForConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitForIntCondition(ctx *ForIntConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitForIntConditionInit(ctx *ForIntConditionInitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitForInCondition(ctx *ForInConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitForInTarget(ctx *ForInTargetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitForRangeCondition(ctx *ForRangeConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWhileStmt(ctx *WhileStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGenericFuncCallExprStmt(ctx *GenericFuncCallExprStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncCallExprStmt(ctx *FuncCallExprStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitPrefixUnaryExprStmt(ctx *PrefixUnaryExprStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitPostfixUnaryExprStmt(ctx *PostfixUnaryExprStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFieldAssignmentExprStmt(ctx *FieldAssignmentExprStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMultiAssignmentExprStmt(ctx *MultiAssignmentExprStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitIndexAssignmentExprStmt(ctx *IndexAssignmentExprStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitAssignmentExprStmt(ctx *AssignmentExprStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitAssignmentTarget(ctx *AssignmentTargetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitChanInitExpr(ctx *ChanInitExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitBitOrBinaryExpr(ctx *BitOrBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitNewInstanceExpr(ctx *NewInstanceExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitLitExpr(ctx *LitExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSessionExpr(ctx *SessionExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitAsExpr(ctx *AsExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitIndexExpr(ctx *IndexExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitCmpBinaryExpr(ctx *CmpBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSliceRangeExpr(ctx *SliceRangeExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitComposableCallExpr(ctx *ComposableCallExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncLiteralExpr(ctx *FuncLiteralExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFieldAccessExpr(ctx *FieldAccessExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTernaryExpr(ctx *TernaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncCallExpr(ctx *FuncCallExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitPrefixUnaryExpr(ctx *PrefixUnaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitCoalesceExpr(ctx *CoalesceExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitIdExpr(ctx *IdExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitLOrBinaryExpr(ctx *LOrBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitAddBinaryExpr(ctx *AddBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitEqBinaryExpr(ctx *EqBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitPostfixUnaryExpr(ctx *PostfixUnaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitRangeExpr(ctx *RangeExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitUnaryExpr(ctx *UnaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitShiftBinaryExpr(ctx *ShiftBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitOptionUnwrapExpr(ctx *OptionUnwrapExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitBitXorBinaryExpr(ctx *BitXorBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGenericFuncCallExpr(ctx *GenericFuncCallExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWhenExpr(ctx *WhenExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitBitAndBinaryExpr(ctx *BitAndBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitLAndBinaryExpr(ctx *LAndBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMulBinaryExpr(ctx *MulBinaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGroupedExpr(ctx *GroupedExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncArgList(ctx *FuncArgListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncArg(ctx *FuncArgContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWhenCase(ctx *WhenCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitDefaultWhenCase(ctx *DefaultWhenCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitUnaryOp(ctx *UnaryOpContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitAssignmentOp(ctx *AssignmentOpContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitLiteral(ctx *LiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitArray_literal(ctx *Array_literalContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMap_literal(ctx *Map_literalContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTuple_literal(ctx *Tuple_literalContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTypeAnnot(ctx *TypeAnnotContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitType(ctx *TypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitChanType(ctx *ChanTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitWildcardType(ctx *WildcardTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitCustomType(ctx *CustomTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitGenericArgs(ctx *GenericArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncType(ctx *FuncTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncTypeParamList(ctx *FuncTypeParamListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitFuncTypeParam(ctx *FuncTypeParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitPrimitiveType(ctx *PrimitiveTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitOptionType(ctx *OptionTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitArrayType(ctx *ArrayTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitSliceType(ctx *SliceTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitMapType(ctx *MapTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTupleType(ctx *TupleTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSovaVisitor) VisitTupleField(ctx *TupleFieldContext) interface{} {
	return v.VisitChildren(ctx)
}
