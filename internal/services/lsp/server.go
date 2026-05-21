package lsp

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Server is the top-level Sova language server. It implements `protocol.Server` (60 methods); the methods we don't yet support are inherited from the embedded `noopServer` base and return `jsonrpc2.ErrMethodNotFound` so the client falls back gracefully. The actual functionality moves into separate files (`lifecycle.go`, `text_sync.go`, `diagnostics.go`, ...) as each phase lands.
type Server struct {
	noopServer

	logger *zap.Logger
	conn   jsonrpc2.Conn
	client protocol.Client

	session *Session

	mu       sync.Mutex
	shutdown bool

	diagMu        sync.Mutex
	publishedURIs map[string]struct{}

	terminate func(code int)
}

// ServeStdio runs an LSP session over stdio until the client closes the connection or sends `exit`. Returns nil on clean shutdown, error on transport failure. Logs (if any) go to `logSink` - pass `io.Discard` to silence them.
func ServeStdio(ctx context.Context, logSink io.Writer) error {
	stream := jsonrpc2.NewStream(stdioReadWriteCloser{})
	return serveStream(ctx, stream, logSink)
}

func serveStream(ctx context.Context, stream jsonrpc2.Stream, logSink io.Writer) error {
	logger := buildLogger(logSink)
	defer logger.Sync()

	conn := jsonrpc2.NewConn(stream)
	srv := &Server{
		logger:    logger,
		conn:      conn,
		client:    protocol.ClientDispatcher(conn, logger.Named("client")),
		terminate: defaultTerminate,
	}
	srv.session = NewSession(srv.client, logger.Named("session"))

	handler := protocol.ServerHandler(srv, jsonrpc2.MethodNotFoundHandler)
	conn.Go(ctx, handler)
	<-conn.Done()
	if err := conn.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("jsonrpc connection: %w", err)
	}
	return nil
}

// defaultTerminate is the process-termination hook the LSP `exit` notification invokes. Defaults to `os.Exit`; tests rebind it via `withTerminate` to keep the test runner alive.
var defaultTerminate = os.Exit

// stdioReadWriteCloser plumbs the process's stdin/stdout into a single io.ReadWriteCloser so jsonrpc2.NewStream can frame against it. Close on this is a no-op; the LSP `exit` notification ends the session via context cancellation, not by closing stdin.
type stdioReadWriteCloser struct{}

func (stdioReadWriteCloser) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdioReadWriteCloser) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdioReadWriteCloser) Close() error                { return nil }

func buildLogger(sink io.Writer) *zap.Logger {
	if sink == nil || sink == io.Discard {
		return zap.NewNop()
	}
	encCfg := zap.NewProductionEncoderConfig()
	enc := zapcore.NewJSONEncoder(encCfg)
	core := zapcore.NewCore(enc, zapcore.AddSync(sink), zap.InfoLevel)
	return zap.New(core)
}
