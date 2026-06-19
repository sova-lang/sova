package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"sova/internal/services/compiler"
	"sova/internal/services/lsp"

	"github.com/spf13/cobra"
)

func newLSPCmd() *cobra.Command {
	var logPath string
	var checkPath string
	var stdio bool
	var nodeIPC bool
	var socketPort int
	var clientPID int
	cmd := &cobra.Command{
		Use:   "lsp",
		Short: "Run the Sova language server (stdio JSON-RPC)",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = stdio
			_ = nodeIPC
			_ = socketPort
			_ = clientPID
			if checkPath != "" {
				return runLSPCheck(checkPath)
			}

			var logSink io.Writer = io.Discard
			if logPath != "" {
				f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
				if err != nil {
					return fmt.Errorf("open log %s: %w", logPath, err)
				}

				defer f.Close()
				logSink = f
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return lsp.ServeStdio(ctx, logSink)
		},
	}

	cmd.Flags().StringVar(&logPath, "log", "", "mirror jsonrpc traffic into this file for debugging")
	cmd.Flags().StringVar(&checkPath, "check", "", "one-shot: run the check pipeline on the given file or directory, print diagnostics, exit (smoke test)")
	cmd.Flags().BoolVar(&stdio, "stdio", false, "use stdio transport (default; accepted for LSP-client convention)")
	cmd.Flags().BoolVar(&nodeIPC, "node-ipc", false, "(unsupported, accepted for LSP-client convention)")
	cmd.Flags().IntVar(&socketPort, "socket", 0, "(unsupported, accepted for LSP-client convention)")
	cmd.Flags().IntVar(&clientPID, "clientProcessId", 0, "PID of the parent editor process (accepted for LSP-client convention)")
	return cmd
}

func runLSPCheck(target string) (retErr error) {
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("stat %s: %w", target, err)
	}

	c := compiler.New()
	defer func() {
		if r := recover(); r != nil {
			c.Diag.Print()
			fmt.Fprintf(os.Stderr, "[lsp --check] panic during compile: %v\n", r)
			retErr = fmt.Errorf("check failed")
		}
	}()
	if info.IsDir() {
		abs, _ := filepath.Abs(target)
		files, err := crawlSova(abs)
		if err != nil {
			return err
		}

		for _, src := range files {
			c.AddSource(src.RelPath, src.Content)
		}

		c.Loader = makePackageLoader(abs)
	} else {
		data, err := os.ReadFile(target)
		if err != nil {
			return err
		}

		display := target
		if wd, werr := os.Getwd(); werr == nil {
			if rel, rerr := filepath.Rel(wd, target); rerr == nil && !strings.HasPrefix(rel, "..") {
				display = rel
			} else if abs, aerr := filepath.Abs(target); aerr == nil {
				display = abs
			}
		}

		c.AddSource(display, string(data))
		c.Loader = makePackageLoader(filepath.Dir(target))
	}

	_ = c.Check()
	c.Diag.Print()
	if c.Diag.Errored() {
		return fmt.Errorf("check failed")
	}

	fmt.Fprintln(os.Stderr, "[lsp --check] no errors")
	return nil
}
