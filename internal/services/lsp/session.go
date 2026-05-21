package lsp

import (
	"sync"
	"sync/atomic"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.uber.org/zap"
)

// Session is the per-process LSP state. Owns the file-overlay store, snapshot graph, client capabilities, and the back-channel `protocol.Client` for pushing notifications (diagnostics, etc.) to the editor. One Session lives for the lifetime of the LSP process.
type Session struct {
	logger *zap.Logger
	client protocol.Client

	mu              sync.Mutex
	overlays        map[uri.URI]string
	overlayVersions map[uri.URI]int32
	rootURI         uri.URI
	snapshotSeq     atomic.Int64
	current         *Snapshot

	clientCapabilities protocol.ClientCapabilities
}

// NewSession constructs an empty Session. The first `Initialize` call populates `rootURI` and `clientCapabilities`; the first `DidOpen` creates the initial Snapshot.
func NewSession(client protocol.Client, logger *zap.Logger) *Session {
	return &Session{
		logger:          logger,
		client:          client,
		overlays:        map[uri.URI]string{},
		overlayVersions: map[uri.URI]int32{},
	}
}

// SetRoot records the workspace root URI sent in `Initialize`. Subsequent snapshots use it to scope file discovery and to find `sova.toml` for dep resolution.
func (s *Session) SetRoot(root uri.URI, caps protocol.ClientCapabilities) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rootURI = root
	s.clientCapabilities = caps
}

// Root returns the workspace root URI.
func (s *Session) Root() uri.URI {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rootURI
}

// UpsertOverlay stores or replaces the editor's in-memory content for a document. Returns the resulting snapshot. Called from `didOpen` and `didChange`.
func (s *Session) UpsertOverlay(u uri.URI, version int32, text string) *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overlays[u] = text
	s.overlayVersions[u] = version
	return s.newSnapshotLocked()
}

// RemoveOverlay clears the editor overlay for `u` (the document is closed; subsequent reads fall back to disk). Returns the resulting snapshot.
func (s *Session) RemoveOverlay(u uri.URI) *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.overlays, u)
	delete(s.overlayVersions, u)
	return s.newSnapshotLocked()
}

// Snapshot returns the current snapshot. May be nil if no document has been opened yet.
func (s *Session) Snapshot() *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// OverlayVersion reports the latest version reported by the editor for `u`, or 0 if there is no overlay. Used to tag PublishDiagnostics with the correct document version so stale diagnostics get ignored by the client.
func (s *Session) OverlayVersion(u uri.URI) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overlayVersions[u]
}

// CurrentOverlayText returns the editor's current in-memory text for `u`, or `("", false)` when no overlay exists. Used by incremental `didChange` handling to start each splice from the buffer the editor thinks we have.
func (s *Session) CurrentOverlayText(u uri.URI) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, ok := s.overlays[u]
	return text, ok
}

// newSnapshotLocked builds a fresh Snapshot reflecting the current overlay store. Caller must hold the session mutex. Snapshots share overlay map by value-copy at construction time so handlers running against a snapshot see a stable view.
func (s *Session) newSnapshotLocked() *Snapshot {
	id := s.snapshotSeq.Add(1)
	snap := &Snapshot{
		ID:       id,
		Root:     s.rootURI,
		overlays: make(map[uri.URI]string, len(s.overlays)),
	}
	for k, v := range s.overlays {
		snap.overlays[k] = v
	}
	s.current = snap
	return snap
}
