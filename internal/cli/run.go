package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func runOnce(cfg BuildConfig) error {
	root, _, err := collectSources(cfg)
	if err != nil {
		return err
	}

	cfg.SourceDir = root

	port, err := resolvePort(cfg.ServeHost, cfg.ServePort, cfg.ServeStrictPort)
	if err != nil {
		return err
	}

	if port != cfg.ServePort && cfg.ServePort != 0 {
		fmt.Fprintf(os.Stderr, "[run] port %d in use, using %d instead\n", cfg.ServePort, port)
	}

	if _, err := compileOnce(cfg); err != nil {
		return fmt.Errorf("compile: %w", err)
	}

	outputGo := filepath.Join(cfg.OutputDir, cfg.OutputName+".go")
	if _, err := os.Stat(outputGo); err != nil {
		return fmt.Errorf("output %s not found (compile failed?): %w", outputGo, err)
	}

	runCmd := exec.Command("go", "run", ".")
	runCmd.Dir = filepath.Dir(outputGo)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	runCmd.Env = append(os.Environ(),
		"WIRE_PORT="+strconv.Itoa(port),
		"WIRE_HOST="+cfg.ServeHost,
		"SOVA_WEB_DIR="+cfg.ServeWebDir,
	)
	setProcessGroup(runCmd)
	if err := runCmd.Start(); err != nil {
		return fmt.Errorf("start backend: %w", err)
	}

	origin := fmt.Sprintf("http://%s:%d", displayHost(cfg.ServeHost), port)
	fmt.Fprintf(os.Stderr, "[run] sova app running on %s (Ctrl-C to stop)\n", origin)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	waitErr := make(chan error, 1)
	go func() { waitErr <- runCmd.Wait() }()

	select {
	case <-ctx.Done():
		if runCmd.Process != nil {
			terminateProcess(runCmd.Process)
			select {
			case <-waitErr:
			case <-time.After(2 * time.Second):
				killProcess(runCmd.Process)
				<-waitErr
			}
		}

		return nil
	case err := <-waitErr:
		return err
	}
}
