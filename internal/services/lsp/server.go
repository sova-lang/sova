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

var defaultTerminate = os.Exit

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
