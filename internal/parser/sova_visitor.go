// Code generated from /home/dasdarki/Development/DasDarki/Sova/sova/Sova.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // Sova

import "github.com/antlr4-go/antlr/v4"

// A complete Visitor for a parse tree produced by SovaParser.
type SovaVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by SovaParser#file.
	VisitFile(ctx *FileContext) interface{}

	// Visit a parse tree produced by SovaParser#fileHeader.
	VisitFileHeader(ctx *FileHeaderContext) interface{}

	// Visit a parse tree produced by SovaParser#packageDecl.
	VisitPackageDecl(ctx *PackageDeclContext) interface{}

	// Visit a parse tree produced by SovaParser#packagePath.
	VisitPackagePath(ctx *PackagePathContext) interface{}

	// Visit a parse tree produced by SovaParser#pkgIdent.
	VisitPkgIdent(ctx *PkgIdentContext) interface{}

	// Visit a parse tree produced by SovaParser#sideDecl.
	VisitSideDecl(ctx *SideDeclContext) interface{}

	// Visit a parse tree produced by SovaParser#side.
	VisitSide(ctx *SideContext) interface{}

	// Visit a parse tree produced by SovaParser#stmt.
	VisitStmt(ctx *StmtContext) interface{}

	// Visit a parse tree produced by SovaParser#goStmt.
	VisitGoStmt(ctx *GoStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#deferStmt.
	VisitDeferStmt(ctx *DeferStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#selectStmt.
	VisitSelectStmt(ctx *SelectStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#selectCase.
	VisitSelectCase(ctx *SelectCaseContext) interface{}

	// Visit a parse tree produced by SovaParser#selectDefaultCase.
	VisitSelectDefaultCase(ctx *SelectDefaultCaseContext) interface{}

	// Visit a parse tree produced by SovaParser#selectCaseGuard.
	VisitSelectCaseGuard(ctx *SelectCaseGuardContext) interface{}

	// Visit a parse tree produced by SovaParser#selectRecvBinding.
	VisitSelectRecvBinding(ctx *SelectRecvBindingContext) interface{}

	// Visit a parse tree produced by SovaParser#testDeclStmt.
	VisitTestDeclStmt(ctx *TestDeclStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#groupDeclStmt.
	VisitGroupDeclStmt(ctx *GroupDeclStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#testTagList.
	VisitTestTagList(ctx *TestTagListContext) interface{}

	// Visit a parse tree produced by SovaParser#asSessionStmt.
	VisitAsSessionStmt(ctx *AsSessionStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#groupItem.
	VisitGroupItem(ctx *GroupItemContext) interface{}

	// Visit a parse tree produced by SovaParser#setupStmt.
	VisitSetupStmt(ctx *SetupStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#teardownStmt.
	VisitTeardownStmt(ctx *TeardownStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#assertStmt.
	VisitAssertStmt(ctx *AssertStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#wireGroupStmt.
	VisitWireGroupStmt(ctx *WireGroupStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#wireRulesetStmt.
	VisitWireRulesetStmt(ctx *WireRulesetStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#importStmt.
	VisitImportStmt(ctx *ImportStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#usingClause.
	VisitUsingClause(ctx *UsingClauseContext) interface{}

	// Visit a parse tree produced by SovaParser#block.
	VisitBlock(ctx *BlockContext) interface{}

	// Visit a parse tree produced by SovaParser#varDeclStmt.
	VisitVarDeclStmt(ctx *VarDeclStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#varDeclTarget.
	VisitVarDeclTarget(ctx *VarDeclTargetContext) interface{}

	// Visit a parse tree produced by SovaParser#funcDeclStmt.
	VisitFuncDeclStmt(ctx *FuncDeclStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#genericParams.
	VisitGenericParams(ctx *GenericParamsContext) interface{}

	// Visit a parse tree produced by SovaParser#genericParam.
	VisitGenericParam(ctx *GenericParamContext) interface{}

	// Visit a parse tree produced by SovaParser#wireSpec.
	VisitWireSpec(ctx *WireSpecContext) interface{}

	// Visit a parse tree produced by SovaParser#wireOptions.
	VisitWireOptions(ctx *WireOptionsContext) interface{}

	// Visit a parse tree produced by SovaParser#wireOption.
	VisitWireOption(ctx *WireOptionContext) interface{}

	// Visit a parse tree produced by SovaParser#funcParamList.
	VisitFuncParamList(ctx *FuncParamListContext) interface{}

	// Visit a parse tree produced by SovaParser#funcParam.
	VisitFuncParam(ctx *FuncParamContext) interface{}

	// Visit a parse tree produced by SovaParser#externDecl.
	VisitExternDecl(ctx *ExternDeclContext) interface{}

	// Visit a parse tree produced by SovaParser#externItem.
	VisitExternItem(ctx *ExternItemContext) interface{}

	// Visit a parse tree produced by SovaParser#externFunc.
	VisitExternFunc(ctx *ExternFuncContext) interface{}

	// Visit a parse tree produced by SovaParser#externVar.
	VisitExternVar(ctx *ExternVarContext) interface{}

	// Visit a parse tree produced by SovaParser#SimpleExternMapping.
	VisitSimpleExternMapping(ctx *SimpleExternMappingContext) interface{}

	// Visit a parse tree produced by SovaParser#SharedExternMapping.
	VisitSharedExternMapping(ctx *SharedExternMappingContext) interface{}

	// Visit a parse tree produced by SovaParser#externSideMapping.
	VisitExternSideMapping(ctx *ExternSideMappingContext) interface{}

	// Visit a parse tree produced by SovaParser#externSide.
	VisitExternSide(ctx *ExternSideContext) interface{}

	// Visit a parse tree produced by SovaParser#enumDeclStmt.
	VisitEnumDeclStmt(ctx *EnumDeclStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#enumPayloadDef.
	VisitEnumPayloadDef(ctx *EnumPayloadDefContext) interface{}

	// Visit a parse tree produced by SovaParser#enumFieldDef.
	VisitEnumFieldDef(ctx *EnumFieldDefContext) interface{}

	// Visit a parse tree produced by SovaParser#enumBody.
	VisitEnumBody(ctx *EnumBodyContext) interface{}

	// Visit a parse tree produced by SovaParser#enumCase.
	VisitEnumCase(ctx *EnumCaseContext) interface{}

	// Visit a parse tree produced by SovaParser#enumCaseArgs.
	VisitEnumCaseArgs(ctx *EnumCaseArgsContext) interface{}

	// Visit a parse tree produced by SovaParser#enumMethod.
	VisitEnumMethod(ctx *EnumMethodContext) interface{}

	// Visit a parse tree produced by SovaParser#typeDeclStmt.
	VisitTypeDeclStmt(ctx *TypeDeclStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#typeClause.
	VisitTypeClause(ctx *TypeClauseContext) interface{}

	// Visit a parse tree produced by SovaParser#implementsClause.
	VisitImplementsClause(ctx *ImplementsClauseContext) interface{}

	// Visit a parse tree produced by SovaParser#withClause.
	VisitWithClause(ctx *WithClauseContext) interface{}

	// Visit a parse tree produced by SovaParser#qualifiedRef.
	VisitQualifiedRef(ctx *QualifiedRefContext) interface{}

	// Visit a parse tree produced by SovaParser#typeMember.
	VisitTypeMember(ctx *TypeMemberContext) interface{}

	// Visit a parse tree produced by SovaParser#fieldDecl.
	VisitFieldDecl(ctx *FieldDeclContext) interface{}

	// Visit a parse tree produced by SovaParser#ctorDecl.
	VisitCtorDecl(ctx *CtorDeclContext) interface{}

	// Visit a parse tree produced by SovaParser#methodDecl.
	VisitMethodDecl(ctx *MethodDeclContext) interface{}

	// Visit a parse tree produced by SovaParser#castDecl.
	VisitCastDecl(ctx *CastDeclContext) interface{}

	// Visit a parse tree produced by SovaParser#memberModifier.
	VisitMemberModifier(ctx *MemberModifierContext) interface{}

	// Visit a parse tree produced by SovaParser#annotation.
	VisitAnnotation(ctx *AnnotationContext) interface{}

	// Visit a parse tree produced by SovaParser#ComposableBareChild.
	VisitComposableBareChild(ctx *ComposableBareChildContext) interface{}

	// Visit a parse tree produced by SovaParser#ComposableExprChild.
	VisitComposableExprChild(ctx *ComposableExprChildContext) interface{}

	// Visit a parse tree produced by SovaParser#ComposableIfChild.
	VisitComposableIfChild(ctx *ComposableIfChildContext) interface{}

	// Visit a parse tree produced by SovaParser#ComposableForChild.
	VisitComposableForChild(ctx *ComposableForChildContext) interface{}

	// Visit a parse tree produced by SovaParser#ComposableWhileChild.
	VisitComposableWhileChild(ctx *ComposableWhileChildContext) interface{}

	// Visit a parse tree produced by SovaParser#ComposableSwitchChild.
	VisitComposableSwitchChild(ctx *ComposableSwitchChildContext) interface{}

	// Visit a parse tree produced by SovaParser#methodName.
	VisitMethodName(ctx *MethodNameContext) interface{}

	// Visit a parse tree produced by SovaParser#opSymbol.
	VisitOpSymbol(ctx *OpSymbolContext) interface{}

	// Visit a parse tree produced by SovaParser#interfaceDeclStmt.
	VisitInterfaceDeclStmt(ctx *InterfaceDeclStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#methodSignature.
	VisitMethodSignature(ctx *MethodSignatureContext) interface{}

	// Visit a parse tree produced by SovaParser#mixinDeclStmt.
	VisitMixinDeclStmt(ctx *MixinDeclStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#mixinMember.
	VisitMixinMember(ctx *MixinMemberContext) interface{}

	// Visit a parse tree produced by SovaParser#typeAliasStmt.
	VisitTypeAliasStmt(ctx *TypeAliasStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#ifStmt.
	VisitIfStmt(ctx *IfStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#elseIfBranch.
	VisitElseIfBranch(ctx *ElseIfBranchContext) interface{}

	// Visit a parse tree produced by SovaParser#elseBranch.
	VisitElseBranch(ctx *ElseBranchContext) interface{}

	// Visit a parse tree produced by SovaParser#breakStmt.
	VisitBreakStmt(ctx *BreakStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#continueStmt.
	VisitContinueStmt(ctx *ContinueStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#returnStmt.
	VisitReturnStmt(ctx *ReturnStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#guardStmt.
	VisitGuardStmt(ctx *GuardStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#guardReturn.
	VisitGuardReturn(ctx *GuardReturnContext) interface{}

	// Visit a parse tree produced by SovaParser#switchStmt.
	VisitSwitchStmt(ctx *SwitchStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#switchCase.
	VisitSwitchCase(ctx *SwitchCaseContext) interface{}

	// Visit a parse tree produced by SovaParser#defaultCase.
	VisitDefaultCase(ctx *DefaultCaseContext) interface{}

	// Visit a parse tree produced by SovaParser#forStmt.
	VisitForStmt(ctx *ForStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#forCondition.
	VisitForCondition(ctx *ForConditionContext) interface{}

	// Visit a parse tree produced by SovaParser#forIntCondition.
	VisitForIntCondition(ctx *ForIntConditionContext) interface{}

	// Visit a parse tree produced by SovaParser#forIntConditionInit.
	VisitForIntConditionInit(ctx *ForIntConditionInitContext) interface{}

	// Visit a parse tree produced by SovaParser#forInCondition.
	VisitForInCondition(ctx *ForInConditionContext) interface{}

	// Visit a parse tree produced by SovaParser#forInTarget.
	VisitForInTarget(ctx *ForInTargetContext) interface{}

	// Visit a parse tree produced by SovaParser#forRangeCondition.
	VisitForRangeCondition(ctx *ForRangeConditionContext) interface{}

	// Visit a parse tree produced by SovaParser#whileStmt.
	VisitWhileStmt(ctx *WhileStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#GenericFuncCallExprStmt.
	VisitGenericFuncCallExprStmt(ctx *GenericFuncCallExprStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#FuncCallExprStmt.
	VisitFuncCallExprStmt(ctx *FuncCallExprStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#PrefixUnaryExprStmt.
	VisitPrefixUnaryExprStmt(ctx *PrefixUnaryExprStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#PostfixUnaryExprStmt.
	VisitPostfixUnaryExprStmt(ctx *PostfixUnaryExprStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#FieldAssignmentExprStmt.
	VisitFieldAssignmentExprStmt(ctx *FieldAssignmentExprStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#MultiAssignmentExprStmt.
	VisitMultiAssignmentExprStmt(ctx *MultiAssignmentExprStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#AssignmentExprStmt.
	VisitAssignmentExprStmt(ctx *AssignmentExprStmtContext) interface{}

	// Visit a parse tree produced by SovaParser#assignmentTarget.
	VisitAssignmentTarget(ctx *AssignmentTargetContext) interface{}

	// Visit a parse tree produced by SovaParser#ChanInitExpr.
	VisitChanInitExpr(ctx *ChanInitExprContext) interface{}

	// Visit a parse tree produced by SovaParser#BitOrBinaryExpr.
	VisitBitOrBinaryExpr(ctx *BitOrBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#NewInstanceExpr.
	VisitNewInstanceExpr(ctx *NewInstanceExprContext) interface{}

	// Visit a parse tree produced by SovaParser#LitExpr.
	VisitLitExpr(ctx *LitExprContext) interface{}

	// Visit a parse tree produced by SovaParser#SessionExpr.
	VisitSessionExpr(ctx *SessionExprContext) interface{}

	// Visit a parse tree produced by SovaParser#AsExpr.
	VisitAsExpr(ctx *AsExprContext) interface{}

	// Visit a parse tree produced by SovaParser#IndexExpr.
	VisitIndexExpr(ctx *IndexExprContext) interface{}

	// Visit a parse tree produced by SovaParser#CmpBinaryExpr.
	VisitCmpBinaryExpr(ctx *CmpBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#ComposableCallExpr.
	VisitComposableCallExpr(ctx *ComposableCallExprContext) interface{}

	// Visit a parse tree produced by SovaParser#FuncLiteralExpr.
	VisitFuncLiteralExpr(ctx *FuncLiteralExprContext) interface{}

	// Visit a parse tree produced by SovaParser#FieldAccessExpr.
	VisitFieldAccessExpr(ctx *FieldAccessExprContext) interface{}

	// Visit a parse tree produced by SovaParser#TernaryExpr.
	VisitTernaryExpr(ctx *TernaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#FuncCallExpr.
	VisitFuncCallExpr(ctx *FuncCallExprContext) interface{}

	// Visit a parse tree produced by SovaParser#PrefixUnaryExpr.
	VisitPrefixUnaryExpr(ctx *PrefixUnaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#CoalesceExpr.
	VisitCoalesceExpr(ctx *CoalesceExprContext) interface{}

	// Visit a parse tree produced by SovaParser#IdExpr.
	VisitIdExpr(ctx *IdExprContext) interface{}

	// Visit a parse tree produced by SovaParser#LOrBinaryExpr.
	VisitLOrBinaryExpr(ctx *LOrBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#AddBinaryExpr.
	VisitAddBinaryExpr(ctx *AddBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#EqBinaryExpr.
	VisitEqBinaryExpr(ctx *EqBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#PostfixUnaryExpr.
	VisitPostfixUnaryExpr(ctx *PostfixUnaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#RangeExpr.
	VisitRangeExpr(ctx *RangeExprContext) interface{}

	// Visit a parse tree produced by SovaParser#UnaryExpr.
	VisitUnaryExpr(ctx *UnaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#ShiftBinaryExpr.
	VisitShiftBinaryExpr(ctx *ShiftBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#OptionUnwrapExpr.
	VisitOptionUnwrapExpr(ctx *OptionUnwrapExprContext) interface{}

	// Visit a parse tree produced by SovaParser#BitXorBinaryExpr.
	VisitBitXorBinaryExpr(ctx *BitXorBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#GenericFuncCallExpr.
	VisitGenericFuncCallExpr(ctx *GenericFuncCallExprContext) interface{}

	// Visit a parse tree produced by SovaParser#WhenExpr.
	VisitWhenExpr(ctx *WhenExprContext) interface{}

	// Visit a parse tree produced by SovaParser#BitAndBinaryExpr.
	VisitBitAndBinaryExpr(ctx *BitAndBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#LAndBinaryExpr.
	VisitLAndBinaryExpr(ctx *LAndBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#MulBinaryExpr.
	VisitMulBinaryExpr(ctx *MulBinaryExprContext) interface{}

	// Visit a parse tree produced by SovaParser#GroupedExpr.
	VisitGroupedExpr(ctx *GroupedExprContext) interface{}

	// Visit a parse tree produced by SovaParser#funcArgList.
	VisitFuncArgList(ctx *FuncArgListContext) interface{}

	// Visit a parse tree produced by SovaParser#funcArg.
	VisitFuncArg(ctx *FuncArgContext) interface{}

	// Visit a parse tree produced by SovaParser#whenCase.
	VisitWhenCase(ctx *WhenCaseContext) interface{}

	// Visit a parse tree produced by SovaParser#defaultWhenCase.
	VisitDefaultWhenCase(ctx *DefaultWhenCaseContext) interface{}

	// Visit a parse tree produced by SovaParser#unaryOp.
	VisitUnaryOp(ctx *UnaryOpContext) interface{}

	// Visit a parse tree produced by SovaParser#assignmentOp.
	VisitAssignmentOp(ctx *AssignmentOpContext) interface{}

	// Visit a parse tree produced by SovaParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}

	// Visit a parse tree produced by SovaParser#array_literal.
	VisitArray_literal(ctx *Array_literalContext) interface{}

	// Visit a parse tree produced by SovaParser#map_literal.
	VisitMap_literal(ctx *Map_literalContext) interface{}

	// Visit a parse tree produced by SovaParser#tuple_literal.
	VisitTuple_literal(ctx *Tuple_literalContext) interface{}

	// Visit a parse tree produced by SovaParser#typeAnnot.
	VisitTypeAnnot(ctx *TypeAnnotContext) interface{}

	// Visit a parse tree produced by SovaParser#type.
	VisitType(ctx *TypeContext) interface{}

	// Visit a parse tree produced by SovaParser#chanType.
	VisitChanType(ctx *ChanTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#wildcardType.
	VisitWildcardType(ctx *WildcardTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#customType.
	VisitCustomType(ctx *CustomTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#genericArgs.
	VisitGenericArgs(ctx *GenericArgsContext) interface{}

	// Visit a parse tree produced by SovaParser#funcType.
	VisitFuncType(ctx *FuncTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#funcTypeParamList.
	VisitFuncTypeParamList(ctx *FuncTypeParamListContext) interface{}

	// Visit a parse tree produced by SovaParser#funcTypeParam.
	VisitFuncTypeParam(ctx *FuncTypeParamContext) interface{}

	// Visit a parse tree produced by SovaParser#primitiveType.
	VisitPrimitiveType(ctx *PrimitiveTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#optionType.
	VisitOptionType(ctx *OptionTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#arrayType.
	VisitArrayType(ctx *ArrayTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#sliceType.
	VisitSliceType(ctx *SliceTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#mapType.
	VisitMapType(ctx *MapTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#tupleType.
	VisitTupleType(ctx *TupleTypeContext) interface{}

	// Visit a parse tree produced by SovaParser#tupleField.
	VisitTupleField(ctx *TupleFieldContext) interface{}
}
