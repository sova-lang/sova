package lsp

import (
	"context"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// noopServer is the default implementation of every `protocol.Server` method. Each method returns `jsonrpc2.ErrMethodNotFound` (for request-style methods) or no-op (for notifications), so unimplemented LSP features cause the editor to fall back gracefully instead of breaking the session. The real `Server` embeds this and overrides one method per supported feature; new features are simply additions, not edits to a giant switch.
type noopServer struct{}

func (noopServer) Initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Initialized(ctx context.Context, params *protocol.InitializedParams) error {
	return nil
}
func (noopServer) Shutdown(ctx context.Context) error                                       { return nil }
func (noopServer) Exit(ctx context.Context) error                                           { return nil }
func (noopServer) WorkDoneProgressCancel(ctx context.Context, _ *protocol.WorkDoneProgressCancelParams) error {
	return nil
}
func (noopServer) LogTrace(ctx context.Context, _ *protocol.LogTraceParams) error          { return nil }
func (noopServer) SetTrace(ctx context.Context, _ *protocol.SetTraceParams) error          { return nil }
func (noopServer) CodeAction(ctx context.Context, _ *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) CodeLens(ctx context.Context, _ *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) CodeLensResolve(ctx context.Context, _ *protocol.CodeLens) (*protocol.CodeLens, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) ColorPresentation(ctx context.Context, _ *protocol.ColorPresentationParams) ([]protocol.ColorPresentation, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Completion(ctx context.Context, _ *protocol.CompletionParams) (*protocol.CompletionList, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) CompletionResolve(ctx context.Context, _ *protocol.CompletionItem) (*protocol.CompletionItem, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Declaration(ctx context.Context, _ *protocol.DeclarationParams) ([]protocol.Location, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Definition(ctx context.Context, _ *protocol.DefinitionParams) ([]protocol.Location, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) DidChange(ctx context.Context, _ *protocol.DidChangeTextDocumentParams) error {
	return nil
}
func (noopServer) DidChangeConfiguration(ctx context.Context, _ *protocol.DidChangeConfigurationParams) error {
	return nil
}
func (noopServer) DidChangeWatchedFiles(ctx context.Context, _ *protocol.DidChangeWatchedFilesParams) error {
	return nil
}
func (noopServer) DidChangeWorkspaceFolders(ctx context.Context, _ *protocol.DidChangeWorkspaceFoldersParams) error {
	return nil
}
func (noopServer) DidClose(ctx context.Context, _ *protocol.DidCloseTextDocumentParams) error {
	return nil
}
func (noopServer) DidOpen(ctx context.Context, _ *protocol.DidOpenTextDocumentParams) error {
	return nil
}
func (noopServer) DidSave(ctx context.Context, _ *protocol.DidSaveTextDocumentParams) error {
	return nil
}
func (noopServer) DocumentColor(ctx context.Context, _ *protocol.DocumentColorParams) ([]protocol.ColorInformation, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) DocumentHighlight(ctx context.Context, _ *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) DocumentLink(ctx context.Context, _ *protocol.DocumentLinkParams) ([]protocol.DocumentLink, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) DocumentLinkResolve(ctx context.Context, _ *protocol.DocumentLink) (*protocol.DocumentLink, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) DocumentSymbol(ctx context.Context, _ *protocol.DocumentSymbolParams) ([]interface{}, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) ExecuteCommand(ctx context.Context, _ *protocol.ExecuteCommandParams) (interface{}, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) FoldingRanges(ctx context.Context, _ *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Formatting(ctx context.Context, _ *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Hover(ctx context.Context, _ *protocol.HoverParams) (*protocol.Hover, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Implementation(ctx context.Context, _ *protocol.ImplementationParams) ([]protocol.Location, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) OnTypeFormatting(ctx context.Context, _ *protocol.DocumentOnTypeFormattingParams) ([]protocol.TextEdit, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) PrepareRename(ctx context.Context, _ *protocol.PrepareRenameParams) (*protocol.Range, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) RangeFormatting(ctx context.Context, _ *protocol.DocumentRangeFormattingParams) ([]protocol.TextEdit, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) References(ctx context.Context, _ *protocol.ReferenceParams) ([]protocol.Location, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Rename(ctx context.Context, _ *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) SignatureHelp(ctx context.Context, _ *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Symbols(ctx context.Context, _ *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) TypeDefinition(ctx context.Context, _ *protocol.TypeDefinitionParams) ([]protocol.Location, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) WillSave(ctx context.Context, _ *protocol.WillSaveTextDocumentParams) error {
	return nil
}
func (noopServer) WillSaveWaitUntil(ctx context.Context, _ *protocol.WillSaveTextDocumentParams) ([]protocol.TextEdit, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) ShowDocument(ctx context.Context, _ *protocol.ShowDocumentParams) (*protocol.ShowDocumentResult, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) WillCreateFiles(ctx context.Context, _ *protocol.CreateFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) DidCreateFiles(ctx context.Context, _ *protocol.CreateFilesParams) error {
	return nil
}
func (noopServer) WillRenameFiles(ctx context.Context, _ *protocol.RenameFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) DidRenameFiles(ctx context.Context, _ *protocol.RenameFilesParams) error {
	return nil
}
func (noopServer) WillDeleteFiles(ctx context.Context, _ *protocol.DeleteFilesParams) (*protocol.WorkspaceEdit, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) DidDeleteFiles(ctx context.Context, _ *protocol.DeleteFilesParams) error {
	return nil
}
func (noopServer) CodeLensRefresh(ctx context.Context) error { return nil }
func (noopServer) PrepareCallHierarchy(ctx context.Context, _ *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) IncomingCalls(ctx context.Context, _ *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) OutgoingCalls(ctx context.Context, _ *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) SemanticTokensFull(ctx context.Context, _ *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) SemanticTokensFullDelta(ctx context.Context, _ *protocol.SemanticTokensDeltaParams) (interface{}, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) SemanticTokensRange(ctx context.Context, _ *protocol.SemanticTokensRangeParams) (*protocol.SemanticTokens, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) SemanticTokensRefresh(ctx context.Context) error { return nil }
func (noopServer) LinkedEditingRange(ctx context.Context, _ *protocol.LinkedEditingRangeParams) (*protocol.LinkedEditingRanges, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Moniker(ctx context.Context, _ *protocol.MonikerParams) ([]protocol.Moniker, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
func (noopServer) Request(ctx context.Context, method string, params interface{}) (interface{}, error) {
	return nil, jsonrpc2.ErrMethodNotFound
}
