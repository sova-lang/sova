package lsp

import (
	"sync"
	"sync/atomic"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.uber.org/zap"
)

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

func NewSession(client protocol.Client, logger *zap.Logger) *Session {
	return &Session{
		logger:          logger,
		client:          client,
		overlays:        map[uri.URI]string{},
		overlayVersions: map[uri.URI]int32{},
	}
}

func (s *Session) SetRoot(root uri.URI, caps protocol.ClientCapabilities) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rootURI = root
	s.clientCapabilities = caps
}

func (s *Session) Root() uri.URI {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rootURI
}

func (s *Session) UpsertOverlay(u uri.URI, version int32, text string) *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overlays[u] = text
	s.overlayVersions[u] = version
	return s.newSnapshotLocked()
}

func (s *Session) RemoveOverlay(u uri.URI) *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.overlays, u)
	delete(s.overlayVersions, u)
	return s.newSnapshotLocked()
}

func (s *Session) Snapshot() *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *Session) OverlayVersion(u uri.URI) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overlayVersions[u]
}

func (s *Session) CurrentOverlayText(u uri.URI) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, ok := s.overlays[u]
	return text, ok
}

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
