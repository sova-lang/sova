// Code generated from /home/dasdarki/Development/DasDarki/Sova/sova/Sova.g4 by ANTLR 4.13.2. DO NOT EDIT.

package parser // Sova

import "github.com/antlr4-go/antlr/v4"

// BaseSovaListener is a complete listener for a parse tree produced by SovaParser.
type BaseSovaListener struct{}

var _ SovaListener = &BaseSovaListener{}

// VisitTerminal is called when a terminal node is visited.
func (s *BaseSovaListener) VisitTerminal(node antlr.TerminalNode) {}

// VisitErrorNode is called when an error node is visited.
func (s *BaseSovaListener) VisitErrorNode(node antlr.ErrorNode) {}

// EnterEveryRule is called when any rule is entered.
func (s *BaseSovaListener) EnterEveryRule(ctx antlr.ParserRuleContext) {}

// ExitEveryRule is called when any rule is exited.
func (s *BaseSovaListener) ExitEveryRule(ctx antlr.ParserRuleContext) {}

// EnterFile is called when production file is entered.
func (s *BaseSovaListener) EnterFile(ctx *FileContext) {}

// ExitFile is called when production file is exited.
func (s *BaseSovaListener) ExitFile(ctx *FileContext) {}

// EnterFileHeader is called when production fileHeader is entered.
func (s *BaseSovaListener) EnterFileHeader(ctx *FileHeaderContext) {}

// ExitFileHeader is called when production fileHeader is exited.
func (s *BaseSovaListener) ExitFileHeader(ctx *FileHeaderContext) {}

// EnterPackageDecl is called when production packageDecl is entered.
func (s *BaseSovaListener) EnterPackageDecl(ctx *PackageDeclContext) {}

// ExitPackageDecl is called when production packageDecl is exited.
func (s *BaseSovaListener) ExitPackageDecl(ctx *PackageDeclContext) {}

// EnterPackagePath is called when production packagePath is entered.
func (s *BaseSovaListener) EnterPackagePath(ctx *PackagePathContext) {}

// ExitPackagePath is called when production packagePath is exited.
func (s *BaseSovaListener) ExitPackagePath(ctx *PackagePathContext) {}

// EnterPkgIdent is called when production pkgIdent is entered.
func (s *BaseSovaListener) EnterPkgIdent(ctx *PkgIdentContext) {}

// ExitPkgIdent is called when production pkgIdent is exited.
func (s *BaseSovaListener) ExitPkgIdent(ctx *PkgIdentContext) {}

// EnterSideDecl is called when production sideDecl is entered.
func (s *BaseSovaListener) EnterSideDecl(ctx *SideDeclContext) {}

// ExitSideDecl is called when production sideDecl is exited.
func (s *BaseSovaListener) ExitSideDecl(ctx *SideDeclContext) {}

// EnterSide is called when production side is entered.
func (s *BaseSovaListener) EnterSide(ctx *SideContext) {}

// ExitSide is called when production side is exited.
func (s *BaseSovaListener) ExitSide(ctx *SideContext) {}

// EnterStmt is called when production stmt is entered.
func (s *BaseSovaListener) EnterStmt(ctx *StmtContext) {}

// ExitStmt is called when production stmt is exited.
func (s *BaseSovaListener) ExitStmt(ctx *StmtContext) {}

// EnterGoStmt is called when production goStmt is entered.
func (s *BaseSovaListener) EnterGoStmt(ctx *GoStmtContext) {}

// ExitGoStmt is called when production goStmt is exited.
func (s *BaseSovaListener) ExitGoStmt(ctx *GoStmtContext) {}

// EnterDeferStmt is called when production deferStmt is entered.
func (s *BaseSovaListener) EnterDeferStmt(ctx *DeferStmtContext) {}

// ExitDeferStmt is called when production deferStmt is exited.
func (s *BaseSovaListener) ExitDeferStmt(ctx *DeferStmtContext) {}

// EnterSelectStmt is called when production selectStmt is entered.
func (s *BaseSovaListener) EnterSelectStmt(ctx *SelectStmtContext) {}

// ExitSelectStmt is called when production selectStmt is exited.
func (s *BaseSovaListener) ExitSelectStmt(ctx *SelectStmtContext) {}

// EnterSelectCase is called when production selectCase is entered.
func (s *BaseSovaListener) EnterSelectCase(ctx *SelectCaseContext) {}

// ExitSelectCase is called when production selectCase is exited.
func (s *BaseSovaListener) ExitSelectCase(ctx *SelectCaseContext) {}

// EnterSelectDefaultCase is called when production selectDefaultCase is entered.
func (s *BaseSovaListener) EnterSelectDefaultCase(ctx *SelectDefaultCaseContext) {}

// ExitSelectDefaultCase is called when production selectDefaultCase is exited.
func (s *BaseSovaListener) ExitSelectDefaultCase(ctx *SelectDefaultCaseContext) {}

// EnterSelectCaseGuard is called when production selectCaseGuard is entered.
func (s *BaseSovaListener) EnterSelectCaseGuard(ctx *SelectCaseGuardContext) {}

// ExitSelectCaseGuard is called when production selectCaseGuard is exited.
func (s *BaseSovaListener) ExitSelectCaseGuard(ctx *SelectCaseGuardContext) {}

// EnterSelectRecvBinding is called when production selectRecvBinding is entered.
func (s *BaseSovaListener) EnterSelectRecvBinding(ctx *SelectRecvBindingContext) {}

// ExitSelectRecvBinding is called when production selectRecvBinding is exited.
func (s *BaseSovaListener) ExitSelectRecvBinding(ctx *SelectRecvBindingContext) {}

// EnterTestDeclStmt is called when production testDeclStmt is entered.
func (s *BaseSovaListener) EnterTestDeclStmt(ctx *TestDeclStmtContext) {}

// ExitTestDeclStmt is called when production testDeclStmt is exited.
func (s *BaseSovaListener) ExitTestDeclStmt(ctx *TestDeclStmtContext) {}

// EnterGroupDeclStmt is called when production groupDeclStmt is entered.
func (s *BaseSovaListener) EnterGroupDeclStmt(ctx *GroupDeclStmtContext) {}

// ExitGroupDeclStmt is called when production groupDeclStmt is exited.
func (s *BaseSovaListener) ExitGroupDeclStmt(ctx *GroupDeclStmtContext) {}

// EnterTestTagList is called when production testTagList is entered.
func (s *BaseSovaListener) EnterTestTagList(ctx *TestTagListContext) {}

// ExitTestTagList is called when production testTagList is exited.
func (s *BaseSovaListener) ExitTestTagList(ctx *TestTagListContext) {}

// EnterAsSessionStmt is called when production asSessionStmt is entered.
func (s *BaseSovaListener) EnterAsSessionStmt(ctx *AsSessionStmtContext) {}

// ExitAsSessionStmt is called when production asSessionStmt is exited.
func (s *BaseSovaListener) ExitAsSessionStmt(ctx *AsSessionStmtContext) {}

// EnterGroupItem is called when production groupItem is entered.
func (s *BaseSovaListener) EnterGroupItem(ctx *GroupItemContext) {}

// ExitGroupItem is called when production groupItem is exited.
func (s *BaseSovaListener) ExitGroupItem(ctx *GroupItemContext) {}

// EnterSetupStmt is called when production setupStmt is entered.
func (s *BaseSovaListener) EnterSetupStmt(ctx *SetupStmtContext) {}

// ExitSetupStmt is called when production setupStmt is exited.
func (s *BaseSovaListener) ExitSetupStmt(ctx *SetupStmtContext) {}

// EnterTeardownStmt is called when production teardownStmt is entered.
func (s *BaseSovaListener) EnterTeardownStmt(ctx *TeardownStmtContext) {}

// ExitTeardownStmt is called when production teardownStmt is exited.
func (s *BaseSovaListener) ExitTeardownStmt(ctx *TeardownStmtContext) {}

// EnterAssertStmt is called when production assertStmt is entered.
func (s *BaseSovaListener) EnterAssertStmt(ctx *AssertStmtContext) {}

// ExitAssertStmt is called when production assertStmt is exited.
func (s *BaseSovaListener) ExitAssertStmt(ctx *AssertStmtContext) {}

// EnterWireGroupStmt is called when production wireGroupStmt is entered.
func (s *BaseSovaListener) EnterWireGroupStmt(ctx *WireGroupStmtContext) {}

// ExitWireGroupStmt is called when production wireGroupStmt is exited.
func (s *BaseSovaListener) ExitWireGroupStmt(ctx *WireGroupStmtContext) {}

// EnterWireRulesetStmt is called when production wireRulesetStmt is entered.
func (s *BaseSovaListener) EnterWireRulesetStmt(ctx *WireRulesetStmtContext) {}

// ExitWireRulesetStmt is called when production wireRulesetStmt is exited.
func (s *BaseSovaListener) ExitWireRulesetStmt(ctx *WireRulesetStmtContext) {}

// EnterImportStmt is called when production importStmt is entered.
func (s *BaseSovaListener) EnterImportStmt(ctx *ImportStmtContext) {}

// ExitImportStmt is called when production importStmt is exited.
func (s *BaseSovaListener) ExitImportStmt(ctx *ImportStmtContext) {}

// EnterUsingClause is called when production usingClause is entered.
func (s *BaseSovaListener) EnterUsingClause(ctx *UsingClauseContext) {}

// ExitUsingClause is called when production usingClause is exited.
func (s *BaseSovaListener) ExitUsingClause(ctx *UsingClauseContext) {}

// EnterBlock is called when production block is entered.
func (s *BaseSovaListener) EnterBlock(ctx *BlockContext) {}

// ExitBlock is called when production block is exited.
func (s *BaseSovaListener) ExitBlock(ctx *BlockContext) {}

// EnterVarDeclStmt is called when production varDeclStmt is entered.
func (s *BaseSovaListener) EnterVarDeclStmt(ctx *VarDeclStmtContext) {}

// ExitVarDeclStmt is called when production varDeclStmt is exited.
func (s *BaseSovaListener) ExitVarDeclStmt(ctx *VarDeclStmtContext) {}

// EnterVarDeclTarget is called when production varDeclTarget is entered.
func (s *BaseSovaListener) EnterVarDeclTarget(ctx *VarDeclTargetContext) {}

// ExitVarDeclTarget is called when production varDeclTarget is exited.
func (s *BaseSovaListener) ExitVarDeclTarget(ctx *VarDeclTargetContext) {}

// EnterFuncDeclStmt is called when production funcDeclStmt is entered.
func (s *BaseSovaListener) EnterFuncDeclStmt(ctx *FuncDeclStmtContext) {}

// ExitFuncDeclStmt is called when production funcDeclStmt is exited.
func (s *BaseSovaListener) ExitFuncDeclStmt(ctx *FuncDeclStmtContext) {}

// EnterGenericParams is called when production genericParams is entered.
func (s *BaseSovaListener) EnterGenericParams(ctx *GenericParamsContext) {}

// ExitGenericParams is called when production genericParams is exited.
func (s *BaseSovaListener) ExitGenericParams(ctx *GenericParamsContext) {}

// EnterGenericParam is called when production genericParam is entered.
func (s *BaseSovaListener) EnterGenericParam(ctx *GenericParamContext) {}

// ExitGenericParam is called when production genericParam is exited.
func (s *BaseSovaListener) ExitGenericParam(ctx *GenericParamContext) {}

// EnterWireSpec is called when production wireSpec is entered.
func (s *BaseSovaListener) EnterWireSpec(ctx *WireSpecContext) {}

// ExitWireSpec is called when production wireSpec is exited.
func (s *BaseSovaListener) ExitWireSpec(ctx *WireSpecContext) {}

// EnterWireOptions is called when production wireOptions is entered.
func (s *BaseSovaListener) EnterWireOptions(ctx *WireOptionsContext) {}

// ExitWireOptions is called when production wireOptions is exited.
func (s *BaseSovaListener) ExitWireOptions(ctx *WireOptionsContext) {}

// EnterWireOption is called when production wireOption is entered.
func (s *BaseSovaListener) EnterWireOption(ctx *WireOptionContext) {}

// ExitWireOption is called when production wireOption is exited.
func (s *BaseSovaListener) ExitWireOption(ctx *WireOptionContext) {}

// EnterFuncParamList is called when production funcParamList is entered.
func (s *BaseSovaListener) EnterFuncParamList(ctx *FuncParamListContext) {}

// ExitFuncParamList is called when production funcParamList is exited.
func (s *BaseSovaListener) ExitFuncParamList(ctx *FuncParamListContext) {}

// EnterFuncParam is called when production funcParam is entered.
func (s *BaseSovaListener) EnterFuncParam(ctx *FuncParamContext) {}

// ExitFuncParam is called when production funcParam is exited.
func (s *BaseSovaListener) ExitFuncParam(ctx *FuncParamContext) {}

// EnterExternDecl is called when production externDecl is entered.
func (s *BaseSovaListener) EnterExternDecl(ctx *ExternDeclContext) {}

// ExitExternDecl is called when production externDecl is exited.
func (s *BaseSovaListener) ExitExternDecl(ctx *ExternDeclContext) {}

// EnterExternItem is called when production externItem is entered.
func (s *BaseSovaListener) EnterExternItem(ctx *ExternItemContext) {}

// ExitExternItem is called when production externItem is exited.
func (s *BaseSovaListener) ExitExternItem(ctx *ExternItemContext) {}

// EnterExternFunc is called when production externFunc is entered.
func (s *BaseSovaListener) EnterExternFunc(ctx *ExternFuncContext) {}

// ExitExternFunc is called when production externFunc is exited.
func (s *BaseSovaListener) ExitExternFunc(ctx *ExternFuncContext) {}

// EnterExternVar is called when production externVar is entered.
func (s *BaseSovaListener) EnterExternVar(ctx *ExternVarContext) {}

// ExitExternVar is called when production externVar is exited.
func (s *BaseSovaListener) ExitExternVar(ctx *ExternVarContext) {}

// EnterSimpleExternMapping is called when production SimpleExternMapping is entered.
func (s *BaseSovaListener) EnterSimpleExternMapping(ctx *SimpleExternMappingContext) {}

// ExitSimpleExternMapping is called when production SimpleExternMapping is exited.
func (s *BaseSovaListener) ExitSimpleExternMapping(ctx *SimpleExternMappingContext) {}

// EnterSharedExternMapping is called when production SharedExternMapping is entered.
func (s *BaseSovaListener) EnterSharedExternMapping(ctx *SharedExternMappingContext) {}

// ExitSharedExternMapping is called when production SharedExternMapping is exited.
func (s *BaseSovaListener) ExitSharedExternMapping(ctx *SharedExternMappingContext) {}

// EnterExternSideMapping is called when production externSideMapping is entered.
func (s *BaseSovaListener) EnterExternSideMapping(ctx *ExternSideMappingContext) {}

// ExitExternSideMapping is called when production externSideMapping is exited.
func (s *BaseSovaListener) ExitExternSideMapping(ctx *ExternSideMappingContext) {}

// EnterExternSide is called when production externSide is entered.
func (s *BaseSovaListener) EnterExternSide(ctx *ExternSideContext) {}

// ExitExternSide is called when production externSide is exited.
func (s *BaseSovaListener) ExitExternSide(ctx *ExternSideContext) {}

// EnterEnumDeclStmt is called when production enumDeclStmt is entered.
func (s *BaseSovaListener) EnterEnumDeclStmt(ctx *EnumDeclStmtContext) {}

// ExitEnumDeclStmt is called when production enumDeclStmt is exited.
func (s *BaseSovaListener) ExitEnumDeclStmt(ctx *EnumDeclStmtContext) {}

// EnterEnumPayloadDef is called when production enumPayloadDef is entered.
func (s *BaseSovaListener) EnterEnumPayloadDef(ctx *EnumPayloadDefContext) {}

// ExitEnumPayloadDef is called when production enumPayloadDef is exited.
func (s *BaseSovaListener) ExitEnumPayloadDef(ctx *EnumPayloadDefContext) {}

// EnterEnumFieldDef is called when production enumFieldDef is entered.
func (s *BaseSovaListener) EnterEnumFieldDef(ctx *EnumFieldDefContext) {}

// ExitEnumFieldDef is called when production enumFieldDef is exited.
func (s *BaseSovaListener) ExitEnumFieldDef(ctx *EnumFieldDefContext) {}

// EnterEnumBody is called when production enumBody is entered.
func (s *BaseSovaListener) EnterEnumBody(ctx *EnumBodyContext) {}

// ExitEnumBody is called when production enumBody is exited.
func (s *BaseSovaListener) ExitEnumBody(ctx *EnumBodyContext) {}

// EnterEnumCase is called when production enumCase is entered.
func (s *BaseSovaListener) EnterEnumCase(ctx *EnumCaseContext) {}

// ExitEnumCase is called when production enumCase is exited.
func (s *BaseSovaListener) ExitEnumCase(ctx *EnumCaseContext) {}

// EnterEnumCaseArgs is called when production enumCaseArgs is entered.
func (s *BaseSovaListener) EnterEnumCaseArgs(ctx *EnumCaseArgsContext) {}

// ExitEnumCaseArgs is called when production enumCaseArgs is exited.
func (s *BaseSovaListener) ExitEnumCaseArgs(ctx *EnumCaseArgsContext) {}

// EnterEnumMethod is called when production enumMethod is entered.
func (s *BaseSovaListener) EnterEnumMethod(ctx *EnumMethodContext) {}

// ExitEnumMethod is called when production enumMethod is exited.
func (s *BaseSovaListener) ExitEnumMethod(ctx *EnumMethodContext) {}

// EnterTypeDeclStmt is called when production typeDeclStmt is entered.
func (s *BaseSovaListener) EnterTypeDeclStmt(ctx *TypeDeclStmtContext) {}

// ExitTypeDeclStmt is called when production typeDeclStmt is exited.
func (s *BaseSovaListener) ExitTypeDeclStmt(ctx *TypeDeclStmtContext) {}

// EnterTypeClause is called when production typeClause is entered.
func (s *BaseSovaListener) EnterTypeClause(ctx *TypeClauseContext) {}

// ExitTypeClause is called when production typeClause is exited.
func (s *BaseSovaListener) ExitTypeClause(ctx *TypeClauseContext) {}

// EnterImplementsClause is called when production implementsClause is entered.
func (s *BaseSovaListener) EnterImplementsClause(ctx *ImplementsClauseContext) {}

// ExitImplementsClause is called when production implementsClause is exited.
func (s *BaseSovaListener) ExitImplementsClause(ctx *ImplementsClauseContext) {}

// EnterWithClause is called when production withClause is entered.
func (s *BaseSovaListener) EnterWithClause(ctx *WithClauseContext) {}

// ExitWithClause is called when production withClause is exited.
func (s *BaseSovaListener) ExitWithClause(ctx *WithClauseContext) {}

// EnterQualifiedRef is called when production qualifiedRef is entered.
func (s *BaseSovaListener) EnterQualifiedRef(ctx *QualifiedRefContext) {}

// ExitQualifiedRef is called when production qualifiedRef is exited.
func (s *BaseSovaListener) ExitQualifiedRef(ctx *QualifiedRefContext) {}

// EnterTypeMember is called when production typeMember is entered.
func (s *BaseSovaListener) EnterTypeMember(ctx *TypeMemberContext) {}

// ExitTypeMember is called when production typeMember is exited.
func (s *BaseSovaListener) ExitTypeMember(ctx *TypeMemberContext) {}

// EnterFieldDecl is called when production fieldDecl is entered.
func (s *BaseSovaListener) EnterFieldDecl(ctx *FieldDeclContext) {}

// ExitFieldDecl is called when production fieldDecl is exited.
func (s *BaseSovaListener) ExitFieldDecl(ctx *FieldDeclContext) {}

// EnterCtorDecl is called when production ctorDecl is entered.
func (s *BaseSovaListener) EnterCtorDecl(ctx *CtorDeclContext) {}

// ExitCtorDecl is called when production ctorDecl is exited.
func (s *BaseSovaListener) ExitCtorDecl(ctx *CtorDeclContext) {}

// EnterMethodDecl is called when production methodDecl is entered.
func (s *BaseSovaListener) EnterMethodDecl(ctx *MethodDeclContext) {}

// ExitMethodDecl is called when production methodDecl is exited.
func (s *BaseSovaListener) ExitMethodDecl(ctx *MethodDeclContext) {}

// EnterCastDecl is called when production castDecl is entered.
func (s *BaseSovaListener) EnterCastDecl(ctx *CastDeclContext) {}

// ExitCastDecl is called when production castDecl is exited.
func (s *BaseSovaListener) ExitCastDecl(ctx *CastDeclContext) {}

// EnterMemberModifier is called when production memberModifier is entered.
func (s *BaseSovaListener) EnterMemberModifier(ctx *MemberModifierContext) {}

// ExitMemberModifier is called when production memberModifier is exited.
func (s *BaseSovaListener) ExitMemberModifier(ctx *MemberModifierContext) {}

// EnterAnnotation is called when production annotation is entered.
func (s *BaseSovaListener) EnterAnnotation(ctx *AnnotationContext) {}

// ExitAnnotation is called when production annotation is exited.
func (s *BaseSovaListener) ExitAnnotation(ctx *AnnotationContext) {}

// EnterComposableBareChild is called when production ComposableBareChild is entered.
func (s *BaseSovaListener) EnterComposableBareChild(ctx *ComposableBareChildContext) {}

// ExitComposableBareChild is called when production ComposableBareChild is exited.
func (s *BaseSovaListener) ExitComposableBareChild(ctx *ComposableBareChildContext) {}

// EnterComposableExprChild is called when production ComposableExprChild is entered.
func (s *BaseSovaListener) EnterComposableExprChild(ctx *ComposableExprChildContext) {}

// ExitComposableExprChild is called when production ComposableExprChild is exited.
func (s *BaseSovaListener) ExitComposableExprChild(ctx *ComposableExprChildContext) {}

// EnterComposableIfChild is called when production ComposableIfChild is entered.
func (s *BaseSovaListener) EnterComposableIfChild(ctx *ComposableIfChildContext) {}

// ExitComposableIfChild is called when production ComposableIfChild is exited.
func (s *BaseSovaListener) ExitComposableIfChild(ctx *ComposableIfChildContext) {}

// EnterComposableForChild is called when production ComposableForChild is entered.
func (s *BaseSovaListener) EnterComposableForChild(ctx *ComposableForChildContext) {}

// ExitComposableForChild is called when production ComposableForChild is exited.
func (s *BaseSovaListener) ExitComposableForChild(ctx *ComposableForChildContext) {}

// EnterComposableWhileChild is called when production ComposableWhileChild is entered.
func (s *BaseSovaListener) EnterComposableWhileChild(ctx *ComposableWhileChildContext) {}

// ExitComposableWhileChild is called when production ComposableWhileChild is exited.
func (s *BaseSovaListener) ExitComposableWhileChild(ctx *ComposableWhileChildContext) {}

// EnterComposableSwitchChild is called when production ComposableSwitchChild is entered.
func (s *BaseSovaListener) EnterComposableSwitchChild(ctx *ComposableSwitchChildContext) {}

// ExitComposableSwitchChild is called when production ComposableSwitchChild is exited.
func (s *BaseSovaListener) ExitComposableSwitchChild(ctx *ComposableSwitchChildContext) {}

// EnterMethodName is called when production methodName is entered.
func (s *BaseSovaListener) EnterMethodName(ctx *MethodNameContext) {}

// ExitMethodName is called when production methodName is exited.
func (s *BaseSovaListener) ExitMethodName(ctx *MethodNameContext) {}

// EnterOpSymbol is called when production opSymbol is entered.
func (s *BaseSovaListener) EnterOpSymbol(ctx *OpSymbolContext) {}

// ExitOpSymbol is called when production opSymbol is exited.
func (s *BaseSovaListener) ExitOpSymbol(ctx *OpSymbolContext) {}

// EnterInterfaceDeclStmt is called when production interfaceDeclStmt is entered.
func (s *BaseSovaListener) EnterInterfaceDeclStmt(ctx *InterfaceDeclStmtContext) {}

// ExitInterfaceDeclStmt is called when production interfaceDeclStmt is exited.
func (s *BaseSovaListener) ExitInterfaceDeclStmt(ctx *InterfaceDeclStmtContext) {}

// EnterMethodSignature is called when production methodSignature is entered.
func (s *BaseSovaListener) EnterMethodSignature(ctx *MethodSignatureContext) {}

// ExitMethodSignature is called when production methodSignature is exited.
func (s *BaseSovaListener) ExitMethodSignature(ctx *MethodSignatureContext) {}

// EnterMixinDeclStmt is called when production mixinDeclStmt is entered.
func (s *BaseSovaListener) EnterMixinDeclStmt(ctx *MixinDeclStmtContext) {}

// ExitMixinDeclStmt is called when production mixinDeclStmt is exited.
func (s *BaseSovaListener) ExitMixinDeclStmt(ctx *MixinDeclStmtContext) {}

// EnterMixinMember is called when production mixinMember is entered.
func (s *BaseSovaListener) EnterMixinMember(ctx *MixinMemberContext) {}

// ExitMixinMember is called when production mixinMember is exited.
func (s *BaseSovaListener) ExitMixinMember(ctx *MixinMemberContext) {}

// EnterTypeAliasStmt is called when production typeAliasStmt is entered.
func (s *BaseSovaListener) EnterTypeAliasStmt(ctx *TypeAliasStmtContext) {}

// ExitTypeAliasStmt is called when production typeAliasStmt is exited.
func (s *BaseSovaListener) ExitTypeAliasStmt(ctx *TypeAliasStmtContext) {}

// EnterIfStmt is called when production ifStmt is entered.
func (s *BaseSovaListener) EnterIfStmt(ctx *IfStmtContext) {}

// ExitIfStmt is called when production ifStmt is exited.
func (s *BaseSovaListener) ExitIfStmt(ctx *IfStmtContext) {}

// EnterElseIfBranch is called when production elseIfBranch is entered.
func (s *BaseSovaListener) EnterElseIfBranch(ctx *ElseIfBranchContext) {}

// ExitElseIfBranch is called when production elseIfBranch is exited.
func (s *BaseSovaListener) ExitElseIfBranch(ctx *ElseIfBranchContext) {}

// EnterElseBranch is called when production elseBranch is entered.
func (s *BaseSovaListener) EnterElseBranch(ctx *ElseBranchContext) {}

// ExitElseBranch is called when production elseBranch is exited.
func (s *BaseSovaListener) ExitElseBranch(ctx *ElseBranchContext) {}

// EnterBreakStmt is called when production breakStmt is entered.
func (s *BaseSovaListener) EnterBreakStmt(ctx *BreakStmtContext) {}

// ExitBreakStmt is called when production breakStmt is exited.
func (s *BaseSovaListener) ExitBreakStmt(ctx *BreakStmtContext) {}

// EnterContinueStmt is called when production continueStmt is entered.
func (s *BaseSovaListener) EnterContinueStmt(ctx *ContinueStmtContext) {}

// ExitContinueStmt is called when production continueStmt is exited.
func (s *BaseSovaListener) ExitContinueStmt(ctx *ContinueStmtContext) {}

// EnterReturnStmt is called when production returnStmt is entered.
func (s *BaseSovaListener) EnterReturnStmt(ctx *ReturnStmtContext) {}

// ExitReturnStmt is called when production returnStmt is exited.
func (s *BaseSovaListener) ExitReturnStmt(ctx *ReturnStmtContext) {}

// EnterGuardStmt is called when production guardStmt is entered.
func (s *BaseSovaListener) EnterGuardStmt(ctx *GuardStmtContext) {}

// ExitGuardStmt is called when production guardStmt is exited.
func (s *BaseSovaListener) ExitGuardStmt(ctx *GuardStmtContext) {}

// EnterGuardReturn is called when production guardReturn is entered.
func (s *BaseSovaListener) EnterGuardReturn(ctx *GuardReturnContext) {}

// ExitGuardReturn is called when production guardReturn is exited.
func (s *BaseSovaListener) ExitGuardReturn(ctx *GuardReturnContext) {}

// EnterSwitchStmt is called when production switchStmt is entered.
func (s *BaseSovaListener) EnterSwitchStmt(ctx *SwitchStmtContext) {}

// ExitSwitchStmt is called when production switchStmt is exited.
func (s *BaseSovaListener) ExitSwitchStmt(ctx *SwitchStmtContext) {}

// EnterSwitchCase is called when production switchCase is entered.
func (s *BaseSovaListener) EnterSwitchCase(ctx *SwitchCaseContext) {}

// ExitSwitchCase is called when production switchCase is exited.
func (s *BaseSovaListener) ExitSwitchCase(ctx *SwitchCaseContext) {}

// EnterDefaultCase is called when production defaultCase is entered.
func (s *BaseSovaListener) EnterDefaultCase(ctx *DefaultCaseContext) {}

// ExitDefaultCase is called when production defaultCase is exited.
func (s *BaseSovaListener) ExitDefaultCase(ctx *DefaultCaseContext) {}

// EnterForStmt is called when production forStmt is entered.
func (s *BaseSovaListener) EnterForStmt(ctx *ForStmtContext) {}

// ExitForStmt is called when production forStmt is exited.
func (s *BaseSovaListener) ExitForStmt(ctx *ForStmtContext) {}

// EnterForCondition is called when production forCondition is entered.
func (s *BaseSovaListener) EnterForCondition(ctx *ForConditionContext) {}

// ExitForCondition is called when production forCondition is exited.
func (s *BaseSovaListener) ExitForCondition(ctx *ForConditionContext) {}

// EnterForIntCondition is called when production forIntCondition is entered.
func (s *BaseSovaListener) EnterForIntCondition(ctx *ForIntConditionContext) {}

// ExitForIntCondition is called when production forIntCondition is exited.
func (s *BaseSovaListener) ExitForIntCondition(ctx *ForIntConditionContext) {}

// EnterForIntConditionInit is called when production forIntConditionInit is entered.
func (s *BaseSovaListener) EnterForIntConditionInit(ctx *ForIntConditionInitContext) {}

// ExitForIntConditionInit is called when production forIntConditionInit is exited.
func (s *BaseSovaListener) ExitForIntConditionInit(ctx *ForIntConditionInitContext) {}

// EnterForInCondition is called when production forInCondition is entered.
func (s *BaseSovaListener) EnterForInCondition(ctx *ForInConditionContext) {}

// ExitForInCondition is called when production forInCondition is exited.
func (s *BaseSovaListener) ExitForInCondition(ctx *ForInConditionContext) {}

// EnterForInTarget is called when production forInTarget is entered.
func (s *BaseSovaListener) EnterForInTarget(ctx *ForInTargetContext) {}

// ExitForInTarget is called when production forInTarget is exited.
func (s *BaseSovaListener) ExitForInTarget(ctx *ForInTargetContext) {}

// EnterForRangeCondition is called when production forRangeCondition is entered.
func (s *BaseSovaListener) EnterForRangeCondition(ctx *ForRangeConditionContext) {}

// ExitForRangeCondition is called when production forRangeCondition is exited.
func (s *BaseSovaListener) ExitForRangeCondition(ctx *ForRangeConditionContext) {}

// EnterWhileStmt is called when production whileStmt is entered.
func (s *BaseSovaListener) EnterWhileStmt(ctx *WhileStmtContext) {}

// ExitWhileStmt is called when production whileStmt is exited.
func (s *BaseSovaListener) ExitWhileStmt(ctx *WhileStmtContext) {}

// EnterGenericFuncCallExprStmt is called when production GenericFuncCallExprStmt is entered.
func (s *BaseSovaListener) EnterGenericFuncCallExprStmt(ctx *GenericFuncCallExprStmtContext) {}

// ExitGenericFuncCallExprStmt is called when production GenericFuncCallExprStmt is exited.
func (s *BaseSovaListener) ExitGenericFuncCallExprStmt(ctx *GenericFuncCallExprStmtContext) {}

// EnterFuncCallExprStmt is called when production FuncCallExprStmt is entered.
func (s *BaseSovaListener) EnterFuncCallExprStmt(ctx *FuncCallExprStmtContext) {}

// ExitFuncCallExprStmt is called when production FuncCallExprStmt is exited.
func (s *BaseSovaListener) ExitFuncCallExprStmt(ctx *FuncCallExprStmtContext) {}

// EnterPrefixUnaryExprStmt is called when production PrefixUnaryExprStmt is entered.
func (s *BaseSovaListener) EnterPrefixUnaryExprStmt(ctx *PrefixUnaryExprStmtContext) {}

// ExitPrefixUnaryExprStmt is called when production PrefixUnaryExprStmt is exited.
func (s *BaseSovaListener) ExitPrefixUnaryExprStmt(ctx *PrefixUnaryExprStmtContext) {}

// EnterPostfixUnaryExprStmt is called when production PostfixUnaryExprStmt is entered.
func (s *BaseSovaListener) EnterPostfixUnaryExprStmt(ctx *PostfixUnaryExprStmtContext) {}

// ExitPostfixUnaryExprStmt is called when production PostfixUnaryExprStmt is exited.
func (s *BaseSovaListener) ExitPostfixUnaryExprStmt(ctx *PostfixUnaryExprStmtContext) {}

// EnterFieldAssignmentExprStmt is called when production FieldAssignmentExprStmt is entered.
func (s *BaseSovaListener) EnterFieldAssignmentExprStmt(ctx *FieldAssignmentExprStmtContext) {}

// ExitFieldAssignmentExprStmt is called when production FieldAssignmentExprStmt is exited.
func (s *BaseSovaListener) ExitFieldAssignmentExprStmt(ctx *FieldAssignmentExprStmtContext) {}

// EnterMultiAssignmentExprStmt is called when production MultiAssignmentExprStmt is entered.
func (s *BaseSovaListener) EnterMultiAssignmentExprStmt(ctx *MultiAssignmentExprStmtContext) {}

// ExitMultiAssignmentExprStmt is called when production MultiAssignmentExprStmt is exited.
func (s *BaseSovaListener) ExitMultiAssignmentExprStmt(ctx *MultiAssignmentExprStmtContext) {}

// EnterAssignmentExprStmt is called when production AssignmentExprStmt is entered.
func (s *BaseSovaListener) EnterAssignmentExprStmt(ctx *AssignmentExprStmtContext) {}

// ExitAssignmentExprStmt is called when production AssignmentExprStmt is exited.
func (s *BaseSovaListener) ExitAssignmentExprStmt(ctx *AssignmentExprStmtContext) {}

// EnterAssignmentTarget is called when production assignmentTarget is entered.
func (s *BaseSovaListener) EnterAssignmentTarget(ctx *AssignmentTargetContext) {}

// ExitAssignmentTarget is called when production assignmentTarget is exited.
func (s *BaseSovaListener) ExitAssignmentTarget(ctx *AssignmentTargetContext) {}

// EnterChanInitExpr is called when production ChanInitExpr is entered.
func (s *BaseSovaListener) EnterChanInitExpr(ctx *ChanInitExprContext) {}

// ExitChanInitExpr is called when production ChanInitExpr is exited.
func (s *BaseSovaListener) ExitChanInitExpr(ctx *ChanInitExprContext) {}

// EnterBitOrBinaryExpr is called when production BitOrBinaryExpr is entered.
func (s *BaseSovaListener) EnterBitOrBinaryExpr(ctx *BitOrBinaryExprContext) {}

// ExitBitOrBinaryExpr is called when production BitOrBinaryExpr is exited.
func (s *BaseSovaListener) ExitBitOrBinaryExpr(ctx *BitOrBinaryExprContext) {}

// EnterNewInstanceExpr is called when production NewInstanceExpr is entered.
func (s *BaseSovaListener) EnterNewInstanceExpr(ctx *NewInstanceExprContext) {}

// ExitNewInstanceExpr is called when production NewInstanceExpr is exited.
func (s *BaseSovaListener) ExitNewInstanceExpr(ctx *NewInstanceExprContext) {}

// EnterLitExpr is called when production LitExpr is entered.
func (s *BaseSovaListener) EnterLitExpr(ctx *LitExprContext) {}

// ExitLitExpr is called when production LitExpr is exited.
func (s *BaseSovaListener) ExitLitExpr(ctx *LitExprContext) {}

// EnterSessionExpr is called when production SessionExpr is entered.
func (s *BaseSovaListener) EnterSessionExpr(ctx *SessionExprContext) {}

// ExitSessionExpr is called when production SessionExpr is exited.
func (s *BaseSovaListener) ExitSessionExpr(ctx *SessionExprContext) {}

// EnterAsExpr is called when production AsExpr is entered.
func (s *BaseSovaListener) EnterAsExpr(ctx *AsExprContext) {}

// ExitAsExpr is called when production AsExpr is exited.
func (s *BaseSovaListener) ExitAsExpr(ctx *AsExprContext) {}

// EnterIndexExpr is called when production IndexExpr is entered.
func (s *BaseSovaListener) EnterIndexExpr(ctx *IndexExprContext) {}

// ExitIndexExpr is called when production IndexExpr is exited.
func (s *BaseSovaListener) ExitIndexExpr(ctx *IndexExprContext) {}

// EnterCmpBinaryExpr is called when production CmpBinaryExpr is entered.
func (s *BaseSovaListener) EnterCmpBinaryExpr(ctx *CmpBinaryExprContext) {}

// ExitCmpBinaryExpr is called when production CmpBinaryExpr is exited.
func (s *BaseSovaListener) ExitCmpBinaryExpr(ctx *CmpBinaryExprContext) {}

// EnterComposableCallExpr is called when production ComposableCallExpr is entered.
func (s *BaseSovaListener) EnterComposableCallExpr(ctx *ComposableCallExprContext) {}

// ExitComposableCallExpr is called when production ComposableCallExpr is exited.
func (s *BaseSovaListener) ExitComposableCallExpr(ctx *ComposableCallExprContext) {}

// EnterFuncLiteralExpr is called when production FuncLiteralExpr is entered.
func (s *BaseSovaListener) EnterFuncLiteralExpr(ctx *FuncLiteralExprContext) {}

// ExitFuncLiteralExpr is called when production FuncLiteralExpr is exited.
func (s *BaseSovaListener) ExitFuncLiteralExpr(ctx *FuncLiteralExprContext) {}

// EnterFieldAccessExpr is called when production FieldAccessExpr is entered.
func (s *BaseSovaListener) EnterFieldAccessExpr(ctx *FieldAccessExprContext) {}

// ExitFieldAccessExpr is called when production FieldAccessExpr is exited.
func (s *BaseSovaListener) ExitFieldAccessExpr(ctx *FieldAccessExprContext) {}

// EnterTernaryExpr is called when production TernaryExpr is entered.
func (s *BaseSovaListener) EnterTernaryExpr(ctx *TernaryExprContext) {}

// ExitTernaryExpr is called when production TernaryExpr is exited.
func (s *BaseSovaListener) ExitTernaryExpr(ctx *TernaryExprContext) {}

// EnterFuncCallExpr is called when production FuncCallExpr is entered.
func (s *BaseSovaListener) EnterFuncCallExpr(ctx *FuncCallExprContext) {}

// ExitFuncCallExpr is called when production FuncCallExpr is exited.
func (s *BaseSovaListener) ExitFuncCallExpr(ctx *FuncCallExprContext) {}

// EnterPrefixUnaryExpr is called when production PrefixUnaryExpr is entered.
func (s *BaseSovaListener) EnterPrefixUnaryExpr(ctx *PrefixUnaryExprContext) {}

// ExitPrefixUnaryExpr is called when production PrefixUnaryExpr is exited.
func (s *BaseSovaListener) ExitPrefixUnaryExpr(ctx *PrefixUnaryExprContext) {}

// EnterCoalesceExpr is called when production CoalesceExpr is entered.
func (s *BaseSovaListener) EnterCoalesceExpr(ctx *CoalesceExprContext) {}

// ExitCoalesceExpr is called when production CoalesceExpr is exited.
func (s *BaseSovaListener) ExitCoalesceExpr(ctx *CoalesceExprContext) {}

// EnterIdExpr is called when production IdExpr is entered.
func (s *BaseSovaListener) EnterIdExpr(ctx *IdExprContext) {}

// ExitIdExpr is called when production IdExpr is exited.
func (s *BaseSovaListener) ExitIdExpr(ctx *IdExprContext) {}

// EnterLOrBinaryExpr is called when production LOrBinaryExpr is entered.
func (s *BaseSovaListener) EnterLOrBinaryExpr(ctx *LOrBinaryExprContext) {}

// ExitLOrBinaryExpr is called when production LOrBinaryExpr is exited.
func (s *BaseSovaListener) ExitLOrBinaryExpr(ctx *LOrBinaryExprContext) {}

// EnterAddBinaryExpr is called when production AddBinaryExpr is entered.
func (s *BaseSovaListener) EnterAddBinaryExpr(ctx *AddBinaryExprContext) {}

// ExitAddBinaryExpr is called when production AddBinaryExpr is exited.
func (s *BaseSovaListener) ExitAddBinaryExpr(ctx *AddBinaryExprContext) {}

// EnterEqBinaryExpr is called when production EqBinaryExpr is entered.
func (s *BaseSovaListener) EnterEqBinaryExpr(ctx *EqBinaryExprContext) {}

// ExitEqBinaryExpr is called when production EqBinaryExpr is exited.
func (s *BaseSovaListener) ExitEqBinaryExpr(ctx *EqBinaryExprContext) {}

// EnterPostfixUnaryExpr is called when production PostfixUnaryExpr is entered.
func (s *BaseSovaListener) EnterPostfixUnaryExpr(ctx *PostfixUnaryExprContext) {}

// ExitPostfixUnaryExpr is called when production PostfixUnaryExpr is exited.
func (s *BaseSovaListener) ExitPostfixUnaryExpr(ctx *PostfixUnaryExprContext) {}

// EnterRangeExpr is called when production RangeExpr is entered.
func (s *BaseSovaListener) EnterRangeExpr(ctx *RangeExprContext) {}

// ExitRangeExpr is called when production RangeExpr is exited.
func (s *BaseSovaListener) ExitRangeExpr(ctx *RangeExprContext) {}

// EnterUnaryExpr is called when production UnaryExpr is entered.
func (s *BaseSovaListener) EnterUnaryExpr(ctx *UnaryExprContext) {}

// ExitUnaryExpr is called when production UnaryExpr is exited.
func (s *BaseSovaListener) ExitUnaryExpr(ctx *UnaryExprContext) {}

// EnterShiftBinaryExpr is called when production ShiftBinaryExpr is entered.
func (s *BaseSovaListener) EnterShiftBinaryExpr(ctx *ShiftBinaryExprContext) {}

// ExitShiftBinaryExpr is called when production ShiftBinaryExpr is exited.
func (s *BaseSovaListener) ExitShiftBinaryExpr(ctx *ShiftBinaryExprContext) {}

// EnterOptionUnwrapExpr is called when production OptionUnwrapExpr is entered.
func (s *BaseSovaListener) EnterOptionUnwrapExpr(ctx *OptionUnwrapExprContext) {}

// ExitOptionUnwrapExpr is called when production OptionUnwrapExpr is exited.
func (s *BaseSovaListener) ExitOptionUnwrapExpr(ctx *OptionUnwrapExprContext) {}

// EnterBitXorBinaryExpr is called when production BitXorBinaryExpr is entered.
func (s *BaseSovaListener) EnterBitXorBinaryExpr(ctx *BitXorBinaryExprContext) {}

// ExitBitXorBinaryExpr is called when production BitXorBinaryExpr is exited.
func (s *BaseSovaListener) ExitBitXorBinaryExpr(ctx *BitXorBinaryExprContext) {}

// EnterGenericFuncCallExpr is called when production GenericFuncCallExpr is entered.
func (s *BaseSovaListener) EnterGenericFuncCallExpr(ctx *GenericFuncCallExprContext) {}

// ExitGenericFuncCallExpr is called when production GenericFuncCallExpr is exited.
func (s *BaseSovaListener) ExitGenericFuncCallExpr(ctx *GenericFuncCallExprContext) {}

// EnterWhenExpr is called when production WhenExpr is entered.
func (s *BaseSovaListener) EnterWhenExpr(ctx *WhenExprContext) {}

// ExitWhenExpr is called when production WhenExpr is exited.
func (s *BaseSovaListener) ExitWhenExpr(ctx *WhenExprContext) {}

// EnterBitAndBinaryExpr is called when production BitAndBinaryExpr is entered.
func (s *BaseSovaListener) EnterBitAndBinaryExpr(ctx *BitAndBinaryExprContext) {}

// ExitBitAndBinaryExpr is called when production BitAndBinaryExpr is exited.
func (s *BaseSovaListener) ExitBitAndBinaryExpr(ctx *BitAndBinaryExprContext) {}

// EnterLAndBinaryExpr is called when production LAndBinaryExpr is entered.
func (s *BaseSovaListener) EnterLAndBinaryExpr(ctx *LAndBinaryExprContext) {}

// ExitLAndBinaryExpr is called when production LAndBinaryExpr is exited.
func (s *BaseSovaListener) ExitLAndBinaryExpr(ctx *LAndBinaryExprContext) {}

// EnterMulBinaryExpr is called when production MulBinaryExpr is entered.
func (s *BaseSovaListener) EnterMulBinaryExpr(ctx *MulBinaryExprContext) {}

// ExitMulBinaryExpr is called when production MulBinaryExpr is exited.
func (s *BaseSovaListener) ExitMulBinaryExpr(ctx *MulBinaryExprContext) {}

// EnterGroupedExpr is called when production GroupedExpr is entered.
func (s *BaseSovaListener) EnterGroupedExpr(ctx *GroupedExprContext) {}

// ExitGroupedExpr is called when production GroupedExpr is exited.
func (s *BaseSovaListener) ExitGroupedExpr(ctx *GroupedExprContext) {}

// EnterFuncArgList is called when production funcArgList is entered.
func (s *BaseSovaListener) EnterFuncArgList(ctx *FuncArgListContext) {}

// ExitFuncArgList is called when production funcArgList is exited.
func (s *BaseSovaListener) ExitFuncArgList(ctx *FuncArgListContext) {}

// EnterFuncArg is called when production funcArg is entered.
func (s *BaseSovaListener) EnterFuncArg(ctx *FuncArgContext) {}

// ExitFuncArg is called when production funcArg is exited.
func (s *BaseSovaListener) ExitFuncArg(ctx *FuncArgContext) {}

// EnterWhenCase is called when production whenCase is entered.
func (s *BaseSovaListener) EnterWhenCase(ctx *WhenCaseContext) {}

// ExitWhenCase is called when production whenCase is exited.
func (s *BaseSovaListener) ExitWhenCase(ctx *WhenCaseContext) {}

// EnterDefaultWhenCase is called when production defaultWhenCase is entered.
func (s *BaseSovaListener) EnterDefaultWhenCase(ctx *DefaultWhenCaseContext) {}

// ExitDefaultWhenCase is called when production defaultWhenCase is exited.
func (s *BaseSovaListener) ExitDefaultWhenCase(ctx *DefaultWhenCaseContext) {}

// EnterUnaryOp is called when production unaryOp is entered.
func (s *BaseSovaListener) EnterUnaryOp(ctx *UnaryOpContext) {}

// ExitUnaryOp is called when production unaryOp is exited.
func (s *BaseSovaListener) ExitUnaryOp(ctx *UnaryOpContext) {}

// EnterAssignmentOp is called when production assignmentOp is entered.
func (s *BaseSovaListener) EnterAssignmentOp(ctx *AssignmentOpContext) {}

// ExitAssignmentOp is called when production assignmentOp is exited.
func (s *BaseSovaListener) ExitAssignmentOp(ctx *AssignmentOpContext) {}

// EnterLiteral is called when production literal is entered.
func (s *BaseSovaListener) EnterLiteral(ctx *LiteralContext) {}

// ExitLiteral is called when production literal is exited.
func (s *BaseSovaListener) ExitLiteral(ctx *LiteralContext) {}

// EnterArray_literal is called when production array_literal is entered.
func (s *BaseSovaListener) EnterArray_literal(ctx *Array_literalContext) {}

// ExitArray_literal is called when production array_literal is exited.
func (s *BaseSovaListener) ExitArray_literal(ctx *Array_literalContext) {}

// EnterMap_literal is called when production map_literal is entered.
func (s *BaseSovaListener) EnterMap_literal(ctx *Map_literalContext) {}

// ExitMap_literal is called when production map_literal is exited.
func (s *BaseSovaListener) ExitMap_literal(ctx *Map_literalContext) {}

// EnterTuple_literal is called when production tuple_literal is entered.
func (s *BaseSovaListener) EnterTuple_literal(ctx *Tuple_literalContext) {}

// ExitTuple_literal is called when production tuple_literal is exited.
func (s *BaseSovaListener) ExitTuple_literal(ctx *Tuple_literalContext) {}

// EnterTypeAnnot is called when production typeAnnot is entered.
func (s *BaseSovaListener) EnterTypeAnnot(ctx *TypeAnnotContext) {}

// ExitTypeAnnot is called when production typeAnnot is exited.
func (s *BaseSovaListener) ExitTypeAnnot(ctx *TypeAnnotContext) {}

// EnterType is called when production type is entered.
func (s *BaseSovaListener) EnterType(ctx *TypeContext) {}

// ExitType is called when production type is exited.
func (s *BaseSovaListener) ExitType(ctx *TypeContext) {}

// EnterChanType is called when production chanType is entered.
func (s *BaseSovaListener) EnterChanType(ctx *ChanTypeContext) {}

// ExitChanType is called when production chanType is exited.
func (s *BaseSovaListener) ExitChanType(ctx *ChanTypeContext) {}

// EnterWildcardType is called when production wildcardType is entered.
func (s *BaseSovaListener) EnterWildcardType(ctx *WildcardTypeContext) {}

// ExitWildcardType is called when production wildcardType is exited.
func (s *BaseSovaListener) ExitWildcardType(ctx *WildcardTypeContext) {}

// EnterCustomType is called when production customType is entered.
func (s *BaseSovaListener) EnterCustomType(ctx *CustomTypeContext) {}

// ExitCustomType is called when production customType is exited.
func (s *BaseSovaListener) ExitCustomType(ctx *CustomTypeContext) {}

// EnterGenericArgs is called when production genericArgs is entered.
func (s *BaseSovaListener) EnterGenericArgs(ctx *GenericArgsContext) {}

// ExitGenericArgs is called when production genericArgs is exited.
func (s *BaseSovaListener) ExitGenericArgs(ctx *GenericArgsContext) {}

// EnterFuncType is called when production funcType is entered.
func (s *BaseSovaListener) EnterFuncType(ctx *FuncTypeContext) {}

// ExitFuncType is called when production funcType is exited.
func (s *BaseSovaListener) ExitFuncType(ctx *FuncTypeContext) {}

// EnterFuncTypeParamList is called when production funcTypeParamList is entered.
func (s *BaseSovaListener) EnterFuncTypeParamList(ctx *FuncTypeParamListContext) {}

// ExitFuncTypeParamList is called when production funcTypeParamList is exited.
func (s *BaseSovaListener) ExitFuncTypeParamList(ctx *FuncTypeParamListContext) {}

// EnterFuncTypeParam is called when production funcTypeParam is entered.
func (s *BaseSovaListener) EnterFuncTypeParam(ctx *FuncTypeParamContext) {}

// ExitFuncTypeParam is called when production funcTypeParam is exited.
func (s *BaseSovaListener) ExitFuncTypeParam(ctx *FuncTypeParamContext) {}

// EnterPrimitiveType is called when production primitiveType is entered.
func (s *BaseSovaListener) EnterPrimitiveType(ctx *PrimitiveTypeContext) {}

// ExitPrimitiveType is called when production primitiveType is exited.
func (s *BaseSovaListener) ExitPrimitiveType(ctx *PrimitiveTypeContext) {}

// EnterOptionType is called when production optionType is entered.
func (s *BaseSovaListener) EnterOptionType(ctx *OptionTypeContext) {}

// ExitOptionType is called when production optionType is exited.
func (s *BaseSovaListener) ExitOptionType(ctx *OptionTypeContext) {}

// EnterArrayType is called when production arrayType is entered.
func (s *BaseSovaListener) EnterArrayType(ctx *ArrayTypeContext) {}

// ExitArrayType is called when production arrayType is exited.
func (s *BaseSovaListener) ExitArrayType(ctx *ArrayTypeContext) {}

// EnterSliceType is called when production sliceType is entered.
func (s *BaseSovaListener) EnterSliceType(ctx *SliceTypeContext) {}

// ExitSliceType is called when production sliceType is exited.
func (s *BaseSovaListener) ExitSliceType(ctx *SliceTypeContext) {}

// EnterMapType is called when production mapType is entered.
func (s *BaseSovaListener) EnterMapType(ctx *MapTypeContext) {}

// ExitMapType is called when production mapType is exited.
func (s *BaseSovaListener) ExitMapType(ctx *MapTypeContext) {}

// EnterTupleType is called when production tupleType is entered.
func (s *BaseSovaListener) EnterTupleType(ctx *TupleTypeContext) {}

// ExitTupleType is called when production tupleType is exited.
func (s *BaseSovaListener) ExitTupleType(ctx *TupleTypeContext) {}

// EnterTupleField is called when production tupleField is entered.
func (s *BaseSovaListener) EnterTupleField(ctx *TupleFieldContext) {}

// ExitTupleField is called when production tupleField is exited.
func (s *BaseSovaListener) ExitTupleField(ctx *TupleFieldContext) {}
