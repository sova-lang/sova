package lsp

import (
	"context"
	"os"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.uber.org/zap"
)

// Initialize handles the first request from the client: negotiates capabilities and stores the workspace root. The returned `InitializeResult` lists exactly the features Sova's LSP implements today; everything else is silently omitted so the editor doesn't probe for unsupported requests.
func (s *Server) Initialize(ctx context.Context, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	s.logger.Info("initialize", zap.String("client", clientName(params)))

	if params.RootURI != "" {
		s.session.SetRoot(params.RootURI, params.Capabilities)
	} else if len(params.WorkspaceFolders) > 0 {
		s.session.SetRoot(uri.URI(params.WorkspaceFolders[0].URI), params.Capabilities)
	}

	syncKind := protocol.TextDocumentSyncKindIncremental
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    syncKind,
				Save: &protocol.SaveOptions{
					IncludeText: false,
				},
			},
			HoverProvider:              true,
			DefinitionProvider:         true,
			TypeDefinitionProvider:     true,
			DocumentSymbolProvider:     true,
			ReferencesProvider:         true,
			DocumentHighlightProvider:  true,
			WorkspaceSymbolProvider:    true,
			DocumentFormattingProvider: true,
			ImplementationProvider:     true,
			RenameProvider: &protocol.RenameOptions{
				PrepareProvider: true,
			},
			SignatureHelpProvider: &protocol.SignatureHelpOptions{
				TriggerCharacters:   []string{"(", ","},
				RetriggerCharacters: []string{",", " ", ")"},
			},
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", "@", "\""},
				ResolveProvider:   false,
			},
			FoldingRangeProvider: true,
			CodeLensProvider: &protocol.CodeLensOptions{
				ResolveProvider: false,
			},
			CodeActionProvider: &protocol.CodeActionOptions{
				CodeActionKinds: []protocol.CodeActionKind{
					protocol.QuickFix,
					protocol.SourceOrganizeImports,
				},
			},
			SemanticTokensProvider: map[string]interface{}{
				"legend": map[string]interface{}{
					"tokenTypes":     semanticTokenLegend,
					"tokenModifiers": []string{},
				},
				"range": true,
				"full":  true,
			},
			CallHierarchyProvider: true,
		},
		ServerInfo: &protocol.ServerInfo{
			Name:    "sova-lsp",
			Version: "0.1.0",
		},
	}, nil
}

// Initialized is the client's ack that it has digested our capabilities; we use it to log readiness and schedule the first diagnostics pass for any documents already open.
func (s *Server) Initialized(ctx context.Context, params *protocol.InitializedParams) error {
	s.logger.Info("initialized")
	return nil
}

// Shutdown signals the client is about to exit. Mark the server shutting-down; subsequent requests should return InvalidRequest, but for v1 we accept everything and just stop publishing diagnostics.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shutdown = true
	s.logger.Info("shutdown")
	return nil
}

// Exit terminates the server process. Per the LSP spec, exit code 0 only if shutdown was called first; otherwise 1. The actual termination goes through the injected `terminate` function so tests can substitute a no-op.
func (s *Server) Exit(ctx context.Context) error {
	s.mu.Lock()
	doneCleanly := s.shutdown
	term := s.terminate
	s.mu.Unlock()
	s.logger.Info("exit", zap.Bool("clean", doneCleanly))
	if term == nil {
		term = os.Exit
	}
	code := 1
	if doneCleanly {
		code = 0
	}
	go term(code)
	return nil
}

func clientName(params *protocol.InitializeParams) string {
	if params == nil || params.ClientInfo == nil {
		return "unknown"
	}
	if params.ClientInfo.Version != "" {
		return params.ClientInfo.Name + " " + params.ClientInfo.Version
	}
	return params.ClientInfo.Name
}
