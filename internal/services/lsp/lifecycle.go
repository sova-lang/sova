package lsp

import (
	"context"
	"os"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.uber.org/zap"
)

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

func (s *Server) Initialized(ctx context.Context, params *protocol.InitializedParams) error {
	s.logger.Info("initialized")
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shutdown = true
	s.logger.Info("shutdown")
	return nil
}

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
