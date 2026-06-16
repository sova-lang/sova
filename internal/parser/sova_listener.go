// Code generated from /home/dasdarki/Development/DasDarki/Sova/sova/Sova.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // Sova

import "github.com/antlr4-go/antlr/v4"

// SovaListener is a complete listener for a parse tree produced by SovaParser.
type SovaListener interface {
	antlr.ParseTreeListener

	// EnterFile is called when entering the file production.
	EnterFile(c *FileContext)

	// EnterFileHeader is called when entering the fileHeader production.
	EnterFileHeader(c *FileHeaderContext)

	// EnterPackageDecl is called when entering the packageDecl production.
	EnterPackageDecl(c *PackageDeclContext)

	// EnterPackagePath is called when entering the packagePath production.
	EnterPackagePath(c *PackagePathContext)

	// EnterPkgIdent is called when entering the pkgIdent production.
	EnterPkgIdent(c *PkgIdentContext)

	// EnterSoftId is called when entering the softId production.
	EnterSoftId(c *SoftIdContext)

	// EnterSideDecl is called when entering the sideDecl production.
	EnterSideDecl(c *SideDeclContext)

	// EnterSide is called when entering the side production.
	EnterSide(c *SideContext)

	// EnterStmt is called when entering the stmt production.
	EnterStmt(c *StmtContext)

	// EnterGoStmt is called when entering the goStmt production.
	EnterGoStmt(c *GoStmtContext)

	// EnterDeferStmt is called when entering the deferStmt production.
	EnterDeferStmt(c *DeferStmtContext)

	// EnterSelectStmt is called when entering the selectStmt production.
	EnterSelectStmt(c *SelectStmtContext)

	// EnterSelectCase is called when entering the selectCase production.
	EnterSelectCase(c *SelectCaseContext)

	// EnterSelectDefaultCase is called when entering the selectDefaultCase production.
	EnterSelectDefaultCase(c *SelectDefaultCaseContext)

	// EnterSelectCaseGuard is called when entering the selectCaseGuard production.
	EnterSelectCaseGuard(c *SelectCaseGuardContext)

	// EnterSelectRecvBinding is called when entering the selectRecvBinding production.
	EnterSelectRecvBinding(c *SelectRecvBindingContext)

	// EnterTestDeclStmt is called when entering the testDeclStmt production.
	EnterTestDeclStmt(c *TestDeclStmtContext)

	// EnterGroupDeclStmt is called when entering the groupDeclStmt production.
	EnterGroupDeclStmt(c *GroupDeclStmtContext)

	// EnterTestTagList is called when entering the testTagList production.
	EnterTestTagList(c *TestTagListContext)

	// EnterAsSessionStmt is called when entering the asSessionStmt production.
	EnterAsSessionStmt(c *AsSessionStmtContext)

	// EnterGroupItem is called when entering the groupItem production.
	EnterGroupItem(c *GroupItemContext)

	// EnterSetupStmt is called when entering the setupStmt production.
	EnterSetupStmt(c *SetupStmtContext)

	// EnterTeardownStmt is called when entering the teardownStmt production.
	EnterTeardownStmt(c *TeardownStmtContext)

	// EnterAssertStmt is called when entering the assertStmt production.
	EnterAssertStmt(c *AssertStmtContext)

	// EnterWireGroupStmt is called when entering the wireGroupStmt production.
	EnterWireGroupStmt(c *WireGroupStmtContext)

	// EnterWireRulesetStmt is called when entering the wireRulesetStmt production.
	EnterWireRulesetStmt(c *WireRulesetStmtContext)

	// EnterImportStmt is called when entering the importStmt production.
	EnterImportStmt(c *ImportStmtContext)

	// EnterUsingClause is called when entering the usingClause production.
	EnterUsingClause(c *UsingClauseContext)

	// EnterBlock is called when entering the block production.
	EnterBlock(c *BlockContext)

	// EnterVarDeclStmt is called when entering the varDeclStmt production.
	EnterVarDeclStmt(c *VarDeclStmtContext)

	// EnterVarDeclTarget is called when entering the varDeclTarget production.
	EnterVarDeclTarget(c *VarDeclTargetContext)

	// EnterFuncDeclStmt is called when entering the funcDeclStmt production.
	EnterFuncDeclStmt(c *FuncDeclStmtContext)

	// EnterGenericParams is called when entering the genericParams production.
	EnterGenericParams(c *GenericParamsContext)

	// EnterGenericParam is called when entering the genericParam production.
	EnterGenericParam(c *GenericParamContext)

	// EnterWireSpec is called when entering the wireSpec production.
	EnterWireSpec(c *WireSpecContext)

	// EnterWireOptions is called when entering the wireOptions production.
	EnterWireOptions(c *WireOptionsContext)

	// EnterWireOption is called when entering the wireOption production.
	EnterWireOption(c *WireOptionContext)

	// EnterFuncParamList is called when entering the funcParamList production.
	EnterFuncParamList(c *FuncParamListContext)

	// EnterFuncParam is called when entering the funcParam production.
	EnterFuncParam(c *FuncParamContext)

	// EnterExternDecl is called when entering the externDecl production.
	EnterExternDecl(c *ExternDeclContext)

	// EnterExternItem is called when entering the externItem production.
	EnterExternItem(c *ExternItemContext)

	// EnterExternFunc is called when entering the externFunc production.
	EnterExternFunc(c *ExternFuncContext)

	// EnterExternVar is called when entering the externVar production.
	EnterExternVar(c *ExternVarContext)

	// EnterSimpleExternMapping is called when entering the SimpleExternMapping production.
	EnterSimpleExternMapping(c *SimpleExternMappingContext)

	// EnterSharedExternMapping is called when entering the SharedExternMapping production.
	EnterSharedExternMapping(c *SharedExternMappingContext)

	// EnterExternSideMapping is called when entering the externSideMapping production.
	EnterExternSideMapping(c *ExternSideMappingContext)

	// EnterExternSide is called when entering the externSide production.
	EnterExternSide(c *ExternSideContext)

	// EnterEnumDeclStmt is called when entering the enumDeclStmt production.
	EnterEnumDeclStmt(c *EnumDeclStmtContext)

	// EnterEnumPayloadDef is called when entering the enumPayloadDef production.
	EnterEnumPayloadDef(c *EnumPayloadDefContext)

	// EnterEnumFieldDef is called when entering the enumFieldDef production.
	EnterEnumFieldDef(c *EnumFieldDefContext)

	// EnterEnumBody is called when entering the enumBody production.
	EnterEnumBody(c *EnumBodyContext)

	// EnterEnumCase is called when entering the enumCase production.
	EnterEnumCase(c *EnumCaseContext)

	// EnterEnumCaseArgs is called when entering the enumCaseArgs production.
	EnterEnumCaseArgs(c *EnumCaseArgsContext)

	// EnterEnumMethod is called when entering the enumMethod production.
	EnterEnumMethod(c *EnumMethodContext)

	// EnterTypeDeclStmt is called when entering the typeDeclStmt production.
	EnterTypeDeclStmt(c *TypeDeclStmtContext)

	// EnterTypeClause is called when entering the typeClause production.
	EnterTypeClause(c *TypeClauseContext)

	// EnterImplementsClause is called when entering the implementsClause production.
	EnterImplementsClause(c *ImplementsClauseContext)

	// EnterWithClause is called when entering the withClause production.
	EnterWithClause(c *WithClauseContext)

	// EnterQualifiedRef is called when entering the qualifiedRef production.
	EnterQualifiedRef(c *QualifiedRefContext)

	// EnterTypeMember is called when entering the typeMember production.
	EnterTypeMember(c *TypeMemberContext)

	// EnterFieldDecl is called when entering the fieldDecl production.
	EnterFieldDecl(c *FieldDeclContext)

	// EnterCtorDecl is called when entering the ctorDecl production.
	EnterCtorDecl(c *CtorDeclContext)

	// EnterMethodDecl is called when entering the methodDecl production.
	EnterMethodDecl(c *MethodDeclContext)

	// EnterCastDecl is called when entering the castDecl production.
	EnterCastDecl(c *CastDeclContext)

	// EnterMemberModifier is called when entering the memberModifier production.
	EnterMemberModifier(c *MemberModifierContext)

	// EnterAnnotation is called when entering the annotation production.
	EnterAnnotation(c *AnnotationContext)

	// EnterComposableBareChild is called when entering the ComposableBareChild production.
	EnterComposableBareChild(c *ComposableBareChildContext)

	// EnterComposableExprChild is called when entering the ComposableExprChild production.
	EnterComposableExprChild(c *ComposableExprChildContext)

	// EnterComposableIfChild is called when entering the ComposableIfChild production.
	EnterComposableIfChild(c *ComposableIfChildContext)

	// EnterComposableForChild is called when entering the ComposableForChild production.
	EnterComposableForChild(c *ComposableForChildContext)

	// EnterComposableWhileChild is called when entering the ComposableWhileChild production.
	EnterComposableWhileChild(c *ComposableWhileChildContext)

	// EnterComposableSwitchChild is called when entering the ComposableSwitchChild production.
	EnterComposableSwitchChild(c *ComposableSwitchChildContext)

	// EnterMethodName is called when entering the methodName production.
	EnterMethodName(c *MethodNameContext)

	// EnterOpSymbol is called when entering the opSymbol production.
	EnterOpSymbol(c *OpSymbolContext)

	// EnterInterfaceDeclStmt is called when entering the interfaceDeclStmt production.
	EnterInterfaceDeclStmt(c *InterfaceDeclStmtContext)

	// EnterMethodSignature is called when entering the methodSignature production.
	EnterMethodSignature(c *MethodSignatureContext)

	// EnterMixinDeclStmt is called when entering the mixinDeclStmt production.
	EnterMixinDeclStmt(c *MixinDeclStmtContext)

	// EnterMixinMember is called when entering the mixinMember production.
	EnterMixinMember(c *MixinMemberContext)

	// EnterSynthDeclStmt is called when entering the synthDeclStmt production.
	EnterSynthDeclStmt(c *SynthDeclStmtContext)

	// EnterSynthParams is called when entering the synthParams production.
	EnterSynthParams(c *SynthParamsContext)

	// EnterSynthRequiredSide is called when entering the synthRequiredSide production.
	EnterSynthRequiredSide(c *SynthRequiredSideContext)

	// EnterSynthTarget is called when entering the synthTarget production.
	EnterSynthTarget(c *SynthTargetContext)

	// EnterSynthTargetKind is called when entering the synthTargetKind production.
	EnterSynthTargetKind(c *SynthTargetKindContext)

	// EnterSynthBodyItem is called when entering the synthBodyItem production.
	EnterSynthBodyItem(c *SynthBodyItemContext)

	// EnterSynthEmitOn is called when entering the synthEmitOn production.
	EnterSynthEmitOn(c *SynthEmitOnContext)

	// EnterSynthEmitAppend is called when entering the synthEmitAppend production.
	EnterSynthEmitAppend(c *SynthEmitAppendContext)

	// EnterSynthEmitField is called when entering the synthEmitField production.
	EnterSynthEmitField(c *SynthEmitFieldContext)

	// EnterSynthEmitMethod is called when entering the synthEmitMethod production.
	EnterSynthEmitMethod(c *SynthEmitMethodContext)

	// EnterSynthEmitCtor is called when entering the synthEmitCtor production.
	EnterSynthEmitCtor(c *SynthEmitCtorContext)

	// EnterSynthForStmt is called when entering the synthForStmt production.
	EnterSynthForStmt(c *SynthForStmtContext)

	// EnterSynthIterable is called when entering the synthIterable production.
	EnterSynthIterable(c *SynthIterableContext)

	// EnterSynthWhere is called when entering the synthWhere production.
	EnterSynthWhere(c *SynthWhereContext)

	// EnterSynthBoolExpr is called when entering the synthBoolExpr production.
	EnterSynthBoolExpr(c *SynthBoolExprContext)

	// EnterTypeAliasStmt is called when entering the typeAliasStmt production.
	EnterTypeAliasStmt(c *TypeAliasStmtContext)

	// EnterIfStmt is called when entering the ifStmt production.
	EnterIfStmt(c *IfStmtContext)

	// EnterElseIfBranch is called when entering the elseIfBranch production.
	EnterElseIfBranch(c *ElseIfBranchContext)

	// EnterElseBranch is called when entering the elseBranch production.
	EnterElseBranch(c *ElseBranchContext)

	// EnterBreakStmt is called when entering the breakStmt production.
	EnterBreakStmt(c *BreakStmtContext)

	// EnterContinueStmt is called when entering the continueStmt production.
	EnterContinueStmt(c *ContinueStmtContext)

	// EnterReturnStmt is called when entering the returnStmt production.
	EnterReturnStmt(c *ReturnStmtContext)

	// EnterGuardStmt is called when entering the guardStmt production.
	EnterGuardStmt(c *GuardStmtContext)

	// EnterGuardReturn is called when entering the guardReturn production.
	EnterGuardReturn(c *GuardReturnContext)

	// EnterSwitchStmt is called when entering the switchStmt production.
	EnterSwitchStmt(c *SwitchStmtContext)

	// EnterSwitchCase is called when entering the switchCase production.
	EnterSwitchCase(c *SwitchCaseContext)

	// EnterDefaultCase is called when entering the defaultCase production.
	EnterDefaultCase(c *DefaultCaseContext)

	// EnterForStmt is called when entering the forStmt production.
	EnterForStmt(c *ForStmtContext)

	// EnterForCondition is called when entering the forCondition production.
	EnterForCondition(c *ForConditionContext)

	// EnterForIntCondition is called when entering the forIntCondition production.
	EnterForIntCondition(c *ForIntConditionContext)

	// EnterForIntConditionInit is called when entering the forIntConditionInit production.
	EnterForIntConditionInit(c *ForIntConditionInitContext)

	// EnterForInCondition is called when entering the forInCondition production.
	EnterForInCondition(c *ForInConditionContext)

	// EnterForInTarget is called when entering the forInTarget production.
	EnterForInTarget(c *ForInTargetContext)

	// EnterForRangeCondition is called when entering the forRangeCondition production.
	EnterForRangeCondition(c *ForRangeConditionContext)

	// EnterWhileStmt is called when entering the whileStmt production.
	EnterWhileStmt(c *WhileStmtContext)

	// EnterGenericFuncCallExprStmt is called when entering the GenericFuncCallExprStmt production.
	EnterGenericFuncCallExprStmt(c *GenericFuncCallExprStmtContext)

	// EnterFuncCallExprStmt is called when entering the FuncCallExprStmt production.
	EnterFuncCallExprStmt(c *FuncCallExprStmtContext)

	// EnterPrefixUnaryExprStmt is called when entering the PrefixUnaryExprStmt production.
	EnterPrefixUnaryExprStmt(c *PrefixUnaryExprStmtContext)

	// EnterPostfixUnaryExprStmt is called when entering the PostfixUnaryExprStmt production.
	EnterPostfixUnaryExprStmt(c *PostfixUnaryExprStmtContext)

	// EnterFieldAssignmentExprStmt is called when entering the FieldAssignmentExprStmt production.
	EnterFieldAssignmentExprStmt(c *FieldAssignmentExprStmtContext)

	// EnterMultiAssignmentExprStmt is called when entering the MultiAssignmentExprStmt production.
	EnterMultiAssignmentExprStmt(c *MultiAssignmentExprStmtContext)

	// EnterIndexAssignmentExprStmt is called when entering the IndexAssignmentExprStmt production.
	EnterIndexAssignmentExprStmt(c *IndexAssignmentExprStmtContext)

	// EnterAssignmentExprStmt is called when entering the AssignmentExprStmt production.
	EnterAssignmentExprStmt(c *AssignmentExprStmtContext)

	// EnterAssignmentTarget is called when entering the assignmentTarget production.
	EnterAssignmentTarget(c *AssignmentTargetContext)

	// EnterChanInitExpr is called when entering the ChanInitExpr production.
	EnterChanInitExpr(c *ChanInitExprContext)

	// EnterBitOrBinaryExpr is called when entering the BitOrBinaryExpr production.
	EnterBitOrBinaryExpr(c *BitOrBinaryExprContext)

	// EnterNewInstanceExpr is called when entering the NewInstanceExpr production.
	EnterNewInstanceExpr(c *NewInstanceExprContext)

	// EnterLitExpr is called when entering the LitExpr production.
	EnterLitExpr(c *LitExprContext)

	// EnterSessionExpr is called when entering the SessionExpr production.
	EnterSessionExpr(c *SessionExprContext)

	// EnterAsExpr is called when entering the AsExpr production.
	EnterAsExpr(c *AsExprContext)

	// EnterIndexExpr is called when entering the IndexExpr production.
	EnterIndexExpr(c *IndexExprContext)

	// EnterCmpBinaryExpr is called when entering the CmpBinaryExpr production.
	EnterCmpBinaryExpr(c *CmpBinaryExprContext)

	// EnterSliceRangeExpr is called when entering the SliceRangeExpr production.
	EnterSliceRangeExpr(c *SliceRangeExprContext)

	// EnterComposableCallExpr is called when entering the ComposableCallExpr production.
	EnterComposableCallExpr(c *ComposableCallExprContext)

	// EnterFuncLiteralExpr is called when entering the FuncLiteralExpr production.
	EnterFuncLiteralExpr(c *FuncLiteralExprContext)

	// EnterFieldAccessExpr is called when entering the FieldAccessExpr production.
	EnterFieldAccessExpr(c *FieldAccessExprContext)

	// EnterTernaryExpr is called when entering the TernaryExpr production.
	EnterTernaryExpr(c *TernaryExprContext)

	// EnterFuncCallExpr is called when entering the FuncCallExpr production.
	EnterFuncCallExpr(c *FuncCallExprContext)

	// EnterPrefixUnaryExpr is called when entering the PrefixUnaryExpr production.
	EnterPrefixUnaryExpr(c *PrefixUnaryExprContext)

	// EnterCoalesceExpr is called when entering the CoalesceExpr production.
	EnterCoalesceExpr(c *CoalesceExprContext)

	// EnterIdExpr is called when entering the IdExpr production.
	EnterIdExpr(c *IdExprContext)

	// EnterLOrBinaryExpr is called when entering the LOrBinaryExpr production.
	EnterLOrBinaryExpr(c *LOrBinaryExprContext)

	// EnterAddBinaryExpr is called when entering the AddBinaryExpr production.
	EnterAddBinaryExpr(c *AddBinaryExprContext)

	// EnterEqBinaryExpr is called when entering the EqBinaryExpr production.
	EnterEqBinaryExpr(c *EqBinaryExprContext)

	// EnterPostfixUnaryExpr is called when entering the PostfixUnaryExpr production.
	EnterPostfixUnaryExpr(c *PostfixUnaryExprContext)

	// EnterRangeExpr is called when entering the RangeExpr production.
	EnterRangeExpr(c *RangeExprContext)

	// EnterUnaryExpr is called when entering the UnaryExpr production.
	EnterUnaryExpr(c *UnaryExprContext)

	// EnterShiftBinaryExpr is called when entering the ShiftBinaryExpr production.
	EnterShiftBinaryExpr(c *ShiftBinaryExprContext)

	// EnterOptionUnwrapExpr is called when entering the OptionUnwrapExpr production.
	EnterOptionUnwrapExpr(c *OptionUnwrapExprContext)

	// EnterBitXorBinaryExpr is called when entering the BitXorBinaryExpr production.
	EnterBitXorBinaryExpr(c *BitXorBinaryExprContext)

	// EnterGenericFuncCallExpr is called when entering the GenericFuncCallExpr production.
	EnterGenericFuncCallExpr(c *GenericFuncCallExprContext)

	// EnterWhenExpr is called when entering the WhenExpr production.
	EnterWhenExpr(c *WhenExprContext)

	// EnterBitAndBinaryExpr is called when entering the BitAndBinaryExpr production.
	EnterBitAndBinaryExpr(c *BitAndBinaryExprContext)

	// EnterLAndBinaryExpr is called when entering the LAndBinaryExpr production.
	EnterLAndBinaryExpr(c *LAndBinaryExprContext)

	// EnterMulBinaryExpr is called when entering the MulBinaryExpr production.
	EnterMulBinaryExpr(c *MulBinaryExprContext)

	// EnterGroupedExpr is called when entering the GroupedExpr production.
	EnterGroupedExpr(c *GroupedExprContext)

	// EnterFuncArgList is called when entering the funcArgList production.
	EnterFuncArgList(c *FuncArgListContext)

	// EnterFuncArg is called when entering the funcArg production.
	EnterFuncArg(c *FuncArgContext)

	// EnterWhenCase is called when entering the whenCase production.
	EnterWhenCase(c *WhenCaseContext)

	// EnterDefaultWhenCase is called when entering the defaultWhenCase production.
	EnterDefaultWhenCase(c *DefaultWhenCaseContext)

	// EnterUnaryOp is called when entering the unaryOp production.
	EnterUnaryOp(c *UnaryOpContext)

	// EnterAssignmentOp is called when entering the assignmentOp production.
	EnterAssignmentOp(c *AssignmentOpContext)

	// EnterLiteral is called when entering the literal production.
	EnterLiteral(c *LiteralContext)

	// EnterArray_literal is called when entering the array_literal production.
	EnterArray_literal(c *Array_literalContext)

	// EnterMap_literal is called when entering the map_literal production.
	EnterMap_literal(c *Map_literalContext)

	// EnterTuple_literal is called when entering the tuple_literal production.
	EnterTuple_literal(c *Tuple_literalContext)

	// EnterTypeAnnot is called when entering the typeAnnot production.
	EnterTypeAnnot(c *TypeAnnotContext)

	// EnterType is called when entering the type production.
	EnterType(c *TypeContext)

	// EnterChanType is called when entering the chanType production.
	EnterChanType(c *ChanTypeContext)

	// EnterWildcardType is called when entering the wildcardType production.
	EnterWildcardType(c *WildcardTypeContext)

	// EnterCustomType is called when entering the customType production.
	EnterCustomType(c *CustomTypeContext)

	// EnterGenericArgs is called when entering the genericArgs production.
	EnterGenericArgs(c *GenericArgsContext)

	// EnterFuncType is called when entering the funcType production.
	EnterFuncType(c *FuncTypeContext)

	// EnterFuncTypeParamList is called when entering the funcTypeParamList production.
	EnterFuncTypeParamList(c *FuncTypeParamListContext)

	// EnterFuncTypeParam is called when entering the funcTypeParam production.
	EnterFuncTypeParam(c *FuncTypeParamContext)

	// EnterPrimitiveType is called when entering the primitiveType production.
	EnterPrimitiveType(c *PrimitiveTypeContext)

	// EnterOptionType is called when entering the optionType production.
	EnterOptionType(c *OptionTypeContext)

	// EnterArrayType is called when entering the arrayType production.
	EnterArrayType(c *ArrayTypeContext)

	// EnterSliceType is called when entering the sliceType production.
	EnterSliceType(c *SliceTypeContext)

	// EnterMapType is called when entering the mapType production.
	EnterMapType(c *MapTypeContext)

	// EnterTupleType is called when entering the tupleType production.
	EnterTupleType(c *TupleTypeContext)

	// EnterTupleField is called when entering the tupleField production.
	EnterTupleField(c *TupleFieldContext)

	// ExitFile is called when exiting the file production.
	ExitFile(c *FileContext)

	// ExitFileHeader is called when exiting the fileHeader production.
	ExitFileHeader(c *FileHeaderContext)

	// ExitPackageDecl is called when exiting the packageDecl production.
	ExitPackageDecl(c *PackageDeclContext)

	// ExitPackagePath is called when exiting the packagePath production.
	ExitPackagePath(c *PackagePathContext)

	// ExitPkgIdent is called when exiting the pkgIdent production.
	ExitPkgIdent(c *PkgIdentContext)

	// ExitSoftId is called when exiting the softId production.
	ExitSoftId(c *SoftIdContext)

	// ExitSideDecl is called when exiting the sideDecl production.
	ExitSideDecl(c *SideDeclContext)

	// ExitSide is called when exiting the side production.
	ExitSide(c *SideContext)

	// ExitStmt is called when exiting the stmt production.
	ExitStmt(c *StmtContext)

	// ExitGoStmt is called when exiting the goStmt production.
	ExitGoStmt(c *GoStmtContext)

	// ExitDeferStmt is called when exiting the deferStmt production.
	ExitDeferStmt(c *DeferStmtContext)

	// ExitSelectStmt is called when exiting the selectStmt production.
	ExitSelectStmt(c *SelectStmtContext)

	// ExitSelectCase is called when exiting the selectCase production.
	ExitSelectCase(c *SelectCaseContext)

	// ExitSelectDefaultCase is called when exiting the selectDefaultCase production.
	ExitSelectDefaultCase(c *SelectDefaultCaseContext)

	// ExitSelectCaseGuard is called when exiting the selectCaseGuard production.
	ExitSelectCaseGuard(c *SelectCaseGuardContext)

	// ExitSelectRecvBinding is called when exiting the selectRecvBinding production.
	ExitSelectRecvBinding(c *SelectRecvBindingContext)

	// ExitTestDeclStmt is called when exiting the testDeclStmt production.
	ExitTestDeclStmt(c *TestDeclStmtContext)

	// ExitGroupDeclStmt is called when exiting the groupDeclStmt production.
	ExitGroupDeclStmt(c *GroupDeclStmtContext)

	// ExitTestTagList is called when exiting the testTagList production.
	ExitTestTagList(c *TestTagListContext)

	// ExitAsSessionStmt is called when exiting the asSessionStmt production.
	ExitAsSessionStmt(c *AsSessionStmtContext)

	// ExitGroupItem is called when exiting the groupItem production.
	ExitGroupItem(c *GroupItemContext)

	// ExitSetupStmt is called when exiting the setupStmt production.
	ExitSetupStmt(c *SetupStmtContext)

	// ExitTeardownStmt is called when exiting the teardownStmt production.
	ExitTeardownStmt(c *TeardownStmtContext)

	// ExitAssertStmt is called when exiting the assertStmt production.
	ExitAssertStmt(c *AssertStmtContext)

	// ExitWireGroupStmt is called when exiting the wireGroupStmt production.
	ExitWireGroupStmt(c *WireGroupStmtContext)

	// ExitWireRulesetStmt is called when exiting the wireRulesetStmt production.
	ExitWireRulesetStmt(c *WireRulesetStmtContext)

	// ExitImportStmt is called when exiting the importStmt production.
	ExitImportStmt(c *ImportStmtContext)

	// ExitUsingClause is called when exiting the usingClause production.
	ExitUsingClause(c *UsingClauseContext)

	// ExitBlock is called when exiting the block production.
	ExitBlock(c *BlockContext)

	// ExitVarDeclStmt is called when exiting the varDeclStmt production.
	ExitVarDeclStmt(c *VarDeclStmtContext)

	// ExitVarDeclTarget is called when exiting the varDeclTarget production.
	ExitVarDeclTarget(c *VarDeclTargetContext)

	// ExitFuncDeclStmt is called when exiting the funcDeclStmt production.
	ExitFuncDeclStmt(c *FuncDeclStmtContext)

	// ExitGenericParams is called when exiting the genericParams production.
	ExitGenericParams(c *GenericParamsContext)

	// ExitGenericParam is called when exiting the genericParam production.
	ExitGenericParam(c *GenericParamContext)

	// ExitWireSpec is called when exiting the wireSpec production.
	ExitWireSpec(c *WireSpecContext)

	// ExitWireOptions is called when exiting the wireOptions production.
	ExitWireOptions(c *WireOptionsContext)

	// ExitWireOption is called when exiting the wireOption production.
	ExitWireOption(c *WireOptionContext)

	// ExitFuncParamList is called when exiting the funcParamList production.
	ExitFuncParamList(c *FuncParamListContext)

	// ExitFuncParam is called when exiting the funcParam production.
	ExitFuncParam(c *FuncParamContext)

	// ExitExternDecl is called when exiting the externDecl production.
	ExitExternDecl(c *ExternDeclContext)

	// ExitExternItem is called when exiting the externItem production.
	ExitExternItem(c *ExternItemContext)

	// ExitExternFunc is called when exiting the externFunc production.
	ExitExternFunc(c *ExternFuncContext)

	// ExitExternVar is called when exiting the externVar production.
	ExitExternVar(c *ExternVarContext)

	// ExitSimpleExternMapping is called when exiting the SimpleExternMapping production.
	ExitSimpleExternMapping(c *SimpleExternMappingContext)

	// ExitSharedExternMapping is called when exiting the SharedExternMapping production.
	ExitSharedExternMapping(c *SharedExternMappingContext)

	// ExitExternSideMapping is called when exiting the externSideMapping production.
	ExitExternSideMapping(c *ExternSideMappingContext)

	// ExitExternSide is called when exiting the externSide production.
	ExitExternSide(c *ExternSideContext)

	// ExitEnumDeclStmt is called when exiting the enumDeclStmt production.
	ExitEnumDeclStmt(c *EnumDeclStmtContext)

	// ExitEnumPayloadDef is called when exiting the enumPayloadDef production.
	ExitEnumPayloadDef(c *EnumPayloadDefContext)

	// ExitEnumFieldDef is called when exiting the enumFieldDef production.
	ExitEnumFieldDef(c *EnumFieldDefContext)

	// ExitEnumBody is called when exiting the enumBody production.
	ExitEnumBody(c *EnumBodyContext)

	// ExitEnumCase is called when exiting the enumCase production.
	ExitEnumCase(c *EnumCaseContext)

	// ExitEnumCaseArgs is called when exiting the enumCaseArgs production.
	ExitEnumCaseArgs(c *EnumCaseArgsContext)

	// ExitEnumMethod is called when exiting the enumMethod production.
	ExitEnumMethod(c *EnumMethodContext)

	// ExitTypeDeclStmt is called when exiting the typeDeclStmt production.
	ExitTypeDeclStmt(c *TypeDeclStmtContext)

	// ExitTypeClause is called when exiting the typeClause production.
	ExitTypeClause(c *TypeClauseContext)

	// ExitImplementsClause is called when exiting the implementsClause production.
	ExitImplementsClause(c *ImplementsClauseContext)

	// ExitWithClause is called when exiting the withClause production.
	ExitWithClause(c *WithClauseContext)

	// ExitQualifiedRef is called when exiting the qualifiedRef production.
	ExitQualifiedRef(c *QualifiedRefContext)

	// ExitTypeMember is called when exiting the typeMember production.
	ExitTypeMember(c *TypeMemberContext)

	// ExitFieldDecl is called when exiting the fieldDecl production.
	ExitFieldDecl(c *FieldDeclContext)

	// ExitCtorDecl is called when exiting the ctorDecl production.
	ExitCtorDecl(c *CtorDeclContext)

	// ExitMethodDecl is called when exiting the methodDecl production.
	ExitMethodDecl(c *MethodDeclContext)

	// ExitCastDecl is called when exiting the castDecl production.
	ExitCastDecl(c *CastDeclContext)

	// ExitMemberModifier is called when exiting the memberModifier production.
	ExitMemberModifier(c *MemberModifierContext)

	// ExitAnnotation is called when exiting the annotation production.
	ExitAnnotation(c *AnnotationContext)

	// ExitComposableBareChild is called when exiting the ComposableBareChild production.
	ExitComposableBareChild(c *ComposableBareChildContext)

	// ExitComposableExprChild is called when exiting the ComposableExprChild production.
	ExitComposableExprChild(c *ComposableExprChildContext)

	// ExitComposableIfChild is called when exiting the ComposableIfChild production.
	ExitComposableIfChild(c *ComposableIfChildContext)

	// ExitComposableForChild is called when exiting the ComposableForChild production.
	ExitComposableForChild(c *ComposableForChildContext)

	// ExitComposableWhileChild is called when exiting the ComposableWhileChild production.
	ExitComposableWhileChild(c *ComposableWhileChildContext)

	// ExitComposableSwitchChild is called when exiting the ComposableSwitchChild production.
	ExitComposableSwitchChild(c *ComposableSwitchChildContext)

	// ExitMethodName is called when exiting the methodName production.
	ExitMethodName(c *MethodNameContext)

	// ExitOpSymbol is called when exiting the opSymbol production.
	ExitOpSymbol(c *OpSymbolContext)

	// ExitInterfaceDeclStmt is called when exiting the interfaceDeclStmt production.
	ExitInterfaceDeclStmt(c *InterfaceDeclStmtContext)

	// ExitMethodSignature is called when exiting the methodSignature production.
	ExitMethodSignature(c *MethodSignatureContext)

	// ExitMixinDeclStmt is called when exiting the mixinDeclStmt production.
	ExitMixinDeclStmt(c *MixinDeclStmtContext)

	// ExitMixinMember is called when exiting the mixinMember production.
	ExitMixinMember(c *MixinMemberContext)

	// ExitSynthDeclStmt is called when exiting the synthDeclStmt production.
	ExitSynthDeclStmt(c *SynthDeclStmtContext)

	// ExitSynthParams is called when exiting the synthParams production.
	ExitSynthParams(c *SynthParamsContext)

	// ExitSynthRequiredSide is called when exiting the synthRequiredSide production.
	ExitSynthRequiredSide(c *SynthRequiredSideContext)

	// ExitSynthTarget is called when exiting the synthTarget production.
	ExitSynthTarget(c *SynthTargetContext)

	// ExitSynthTargetKind is called when exiting the synthTargetKind production.
	ExitSynthTargetKind(c *SynthTargetKindContext)

	// ExitSynthBodyItem is called when exiting the synthBodyItem production.
	ExitSynthBodyItem(c *SynthBodyItemContext)

	// ExitSynthEmitOn is called when exiting the synthEmitOn production.
	ExitSynthEmitOn(c *SynthEmitOnContext)

	// ExitSynthEmitAppend is called when exiting the synthEmitAppend production.
	ExitSynthEmitAppend(c *SynthEmitAppendContext)

	// ExitSynthEmitField is called when exiting the synthEmitField production.
	ExitSynthEmitField(c *SynthEmitFieldContext)

	// ExitSynthEmitMethod is called when exiting the synthEmitMethod production.
	ExitSynthEmitMethod(c *SynthEmitMethodContext)

	// ExitSynthEmitCtor is called when exiting the synthEmitCtor production.
	ExitSynthEmitCtor(c *SynthEmitCtorContext)

	// ExitSynthForStmt is called when exiting the synthForStmt production.
	ExitSynthForStmt(c *SynthForStmtContext)

	// ExitSynthIterable is called when exiting the synthIterable production.
	ExitSynthIterable(c *SynthIterableContext)

	// ExitSynthWhere is called when exiting the synthWhere production.
	ExitSynthWhere(c *SynthWhereContext)

	// ExitSynthBoolExpr is called when exiting the synthBoolExpr production.
	ExitSynthBoolExpr(c *SynthBoolExprContext)

	// ExitTypeAliasStmt is called when exiting the typeAliasStmt production.
	ExitTypeAliasStmt(c *TypeAliasStmtContext)

	// ExitIfStmt is called when exiting the ifStmt production.
	ExitIfStmt(c *IfStmtContext)

	// ExitElseIfBranch is called when exiting the elseIfBranch production.
	ExitElseIfBranch(c *ElseIfBranchContext)

	// ExitElseBranch is called when exiting the elseBranch production.
	ExitElseBranch(c *ElseBranchContext)

	// ExitBreakStmt is called when exiting the breakStmt production.
	ExitBreakStmt(c *BreakStmtContext)

	// ExitContinueStmt is called when exiting the continueStmt production.
	ExitContinueStmt(c *ContinueStmtContext)

	// ExitReturnStmt is called when exiting the returnStmt production.
	ExitReturnStmt(c *ReturnStmtContext)

	// ExitGuardStmt is called when exiting the guardStmt production.
	ExitGuardStmt(c *GuardStmtContext)

	// ExitGuardReturn is called when exiting the guardReturn production.
	ExitGuardReturn(c *GuardReturnContext)

	// ExitSwitchStmt is called when exiting the switchStmt production.
	ExitSwitchStmt(c *SwitchStmtContext)

	// ExitSwitchCase is called when exiting the switchCase production.
	ExitSwitchCase(c *SwitchCaseContext)

	// ExitDefaultCase is called when exiting the defaultCase production.
	ExitDefaultCase(c *DefaultCaseContext)

	// ExitForStmt is called when exiting the forStmt production.
	ExitForStmt(c *ForStmtContext)

	// ExitForCondition is called when exiting the forCondition production.
	ExitForCondition(c *ForConditionContext)

	// ExitForIntCondition is called when exiting the forIntCondition production.
	ExitForIntCondition(c *ForIntConditionContext)

	// ExitForIntConditionInit is called when exiting the forIntConditionInit production.
	ExitForIntConditionInit(c *ForIntConditionInitContext)

	// ExitForInCondition is called when exiting the forInCondition production.
	ExitForInCondition(c *ForInConditionContext)

	// ExitForInTarget is called when exiting the forInTarget production.
	ExitForInTarget(c *ForInTargetContext)

	// ExitForRangeCondition is called when exiting the forRangeCondition production.
	ExitForRangeCondition(c *ForRangeConditionContext)

	// ExitWhileStmt is called when exiting the whileStmt production.
	ExitWhileStmt(c *WhileStmtContext)

	// ExitGenericFuncCallExprStmt is called when exiting the GenericFuncCallExprStmt production.
	ExitGenericFuncCallExprStmt(c *GenericFuncCallExprStmtContext)

	// ExitFuncCallExprStmt is called when exiting the FuncCallExprStmt production.
	ExitFuncCallExprStmt(c *FuncCallExprStmtContext)

	// ExitPrefixUnaryExprStmt is called when exiting the PrefixUnaryExprStmt production.
	ExitPrefixUnaryExprStmt(c *PrefixUnaryExprStmtContext)

	// ExitPostfixUnaryExprStmt is called when exiting the PostfixUnaryExprStmt production.
	ExitPostfixUnaryExprStmt(c *PostfixUnaryExprStmtContext)

	// ExitFieldAssignmentExprStmt is called when exiting the FieldAssignmentExprStmt production.
	ExitFieldAssignmentExprStmt(c *FieldAssignmentExprStmtContext)

	// ExitMultiAssignmentExprStmt is called when exiting the MultiAssignmentExprStmt production.
	ExitMultiAssignmentExprStmt(c *MultiAssignmentExprStmtContext)

	// ExitIndexAssignmentExprStmt is called when exiting the IndexAssignmentExprStmt production.
	ExitIndexAssignmentExprStmt(c *IndexAssignmentExprStmtContext)

	// ExitAssignmentExprStmt is called when exiting the AssignmentExprStmt production.
	ExitAssignmentExprStmt(c *AssignmentExprStmtContext)

	// ExitAssignmentTarget is called when exiting the assignmentTarget production.
	ExitAssignmentTarget(c *AssignmentTargetContext)

	// ExitChanInitExpr is called when exiting the ChanInitExpr production.
	ExitChanInitExpr(c *ChanInitExprContext)

	// ExitBitOrBinaryExpr is called when exiting the BitOrBinaryExpr production.
	ExitBitOrBinaryExpr(c *BitOrBinaryExprContext)

	// ExitNewInstanceExpr is called when exiting the NewInstanceExpr production.
	ExitNewInstanceExpr(c *NewInstanceExprContext)

	// ExitLitExpr is called when exiting the LitExpr production.
	ExitLitExpr(c *LitExprContext)

	// ExitSessionExpr is called when exiting the SessionExpr production.
	ExitSessionExpr(c *SessionExprContext)

	// ExitAsExpr is called when exiting the AsExpr production.
	ExitAsExpr(c *AsExprContext)

	// ExitIndexExpr is called when exiting the IndexExpr production.
	ExitIndexExpr(c *IndexExprContext)

	// ExitCmpBinaryExpr is called when exiting the CmpBinaryExpr production.
	ExitCmpBinaryExpr(c *CmpBinaryExprContext)

	// ExitSliceRangeExpr is called when exiting the SliceRangeExpr production.
	ExitSliceRangeExpr(c *SliceRangeExprContext)

	// ExitComposableCallExpr is called when exiting the ComposableCallExpr production.
	ExitComposableCallExpr(c *ComposableCallExprContext)

	// ExitFuncLiteralExpr is called when exiting the FuncLiteralExpr production.
	ExitFuncLiteralExpr(c *FuncLiteralExprContext)

	// ExitFieldAccessExpr is called when exiting the FieldAccessExpr production.
	ExitFieldAccessExpr(c *FieldAccessExprContext)

	// ExitTernaryExpr is called when exiting the TernaryExpr production.
	ExitTernaryExpr(c *TernaryExprContext)

	// ExitFuncCallExpr is called when exiting the FuncCallExpr production.
	ExitFuncCallExpr(c *FuncCallExprContext)

	// ExitPrefixUnaryExpr is called when exiting the PrefixUnaryExpr production.
	ExitPrefixUnaryExpr(c *PrefixUnaryExprContext)

	// ExitCoalesceExpr is called when exiting the CoalesceExpr production.
	ExitCoalesceExpr(c *CoalesceExprContext)

	// ExitIdExpr is called when exiting the IdExpr production.
	ExitIdExpr(c *IdExprContext)

	// ExitLOrBinaryExpr is called when exiting the LOrBinaryExpr production.
	ExitLOrBinaryExpr(c *LOrBinaryExprContext)

	// ExitAddBinaryExpr is called when exiting the AddBinaryExpr production.
	ExitAddBinaryExpr(c *AddBinaryExprContext)

	// ExitEqBinaryExpr is called when exiting the EqBinaryExpr production.
	ExitEqBinaryExpr(c *EqBinaryExprContext)

	// ExitPostfixUnaryExpr is called when exiting the PostfixUnaryExpr production.
	ExitPostfixUnaryExpr(c *PostfixUnaryExprContext)

	// ExitRangeExpr is called when exiting the RangeExpr production.
	ExitRangeExpr(c *RangeExprContext)

	// ExitUnaryExpr is called when exiting the UnaryExpr production.
	ExitUnaryExpr(c *UnaryExprContext)

	// ExitShiftBinaryExpr is called when exiting the ShiftBinaryExpr production.
	ExitShiftBinaryExpr(c *ShiftBinaryExprContext)

	// ExitOptionUnwrapExpr is called when exiting the OptionUnwrapExpr production.
	ExitOptionUnwrapExpr(c *OptionUnwrapExprContext)

	// ExitBitXorBinaryExpr is called when exiting the BitXorBinaryExpr production.
	ExitBitXorBinaryExpr(c *BitXorBinaryExprContext)

	// ExitGenericFuncCallExpr is called when exiting the GenericFuncCallExpr production.
	ExitGenericFuncCallExpr(c *GenericFuncCallExprContext)

	// ExitWhenExpr is called when exiting the WhenExpr production.
	ExitWhenExpr(c *WhenExprContext)

	// ExitBitAndBinaryExpr is called when exiting the BitAndBinaryExpr production.
	ExitBitAndBinaryExpr(c *BitAndBinaryExprContext)

	// ExitLAndBinaryExpr is called when exiting the LAndBinaryExpr production.
	ExitLAndBinaryExpr(c *LAndBinaryExprContext)

	// ExitMulBinaryExpr is called when exiting the MulBinaryExpr production.
	ExitMulBinaryExpr(c *MulBinaryExprContext)

	// ExitGroupedExpr is called when exiting the GroupedExpr production.
	ExitGroupedExpr(c *GroupedExprContext)

	// ExitFuncArgList is called when exiting the funcArgList production.
	ExitFuncArgList(c *FuncArgListContext)

	// ExitFuncArg is called when exiting the funcArg production.
	ExitFuncArg(c *FuncArgContext)

	// ExitWhenCase is called when exiting the whenCase production.
	ExitWhenCase(c *WhenCaseContext)

	// ExitDefaultWhenCase is called when exiting the defaultWhenCase production.
	ExitDefaultWhenCase(c *DefaultWhenCaseContext)

	// ExitUnaryOp is called when exiting the unaryOp production.
	ExitUnaryOp(c *UnaryOpContext)

	// ExitAssignmentOp is called when exiting the assignmentOp production.
	ExitAssignmentOp(c *AssignmentOpContext)

	// ExitLiteral is called when exiting the literal production.
	ExitLiteral(c *LiteralContext)

	// ExitArray_literal is called when exiting the array_literal production.
	ExitArray_literal(c *Array_literalContext)

	// ExitMap_literal is called when exiting the map_literal production.
	ExitMap_literal(c *Map_literalContext)

	// ExitTuple_literal is called when exiting the tuple_literal production.
	ExitTuple_literal(c *Tuple_literalContext)

	// ExitTypeAnnot is called when exiting the typeAnnot production.
	ExitTypeAnnot(c *TypeAnnotContext)

	// ExitType is called when exiting the type production.
	ExitType(c *TypeContext)

	// ExitChanType is called when exiting the chanType production.
	ExitChanType(c *ChanTypeContext)

	// ExitWildcardType is called when exiting the wildcardType production.
	ExitWildcardType(c *WildcardTypeContext)

	// ExitCustomType is called when exiting the customType production.
	ExitCustomType(c *CustomTypeContext)

	// ExitGenericArgs is called when exiting the genericArgs production.
	ExitGenericArgs(c *GenericArgsContext)

	// ExitFuncType is called when exiting the funcType production.
	ExitFuncType(c *FuncTypeContext)

	// ExitFuncTypeParamList is called when exiting the funcTypeParamList production.
	ExitFuncTypeParamList(c *FuncTypeParamListContext)

	// ExitFuncTypeParam is called when exiting the funcTypeParam production.
	ExitFuncTypeParam(c *FuncTypeParamContext)

	// ExitPrimitiveType is called when exiting the primitiveType production.
	ExitPrimitiveType(c *PrimitiveTypeContext)

	// ExitOptionType is called when exiting the optionType production.
	ExitOptionType(c *OptionTypeContext)

	// ExitArrayType is called when exiting the arrayType production.
	ExitArrayType(c *ArrayTypeContext)

	// ExitSliceType is called when exiting the sliceType production.
	ExitSliceType(c *SliceTypeContext)

	// ExitMapType is called when exiting the mapType production.
	ExitMapType(c *MapTypeContext)

	// ExitTupleType is called when exiting the tupleType production.
	ExitTupleType(c *TupleTypeContext)

	// ExitTupleField is called when exiting the tupleField production.
	ExitTupleField(c *TupleFieldContext)
}
