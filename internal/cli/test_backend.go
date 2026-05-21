package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sova/internal/services/compiler"
	"syscall"
	"time"
)

// maybeStartBackend, when `enabled` is true, compiles the current project in *regular* (non-test) mode into a tmp output dir, builds the resulting Go program with `go build`, spawns it on a free port with `SOVA_DEV=1` + `WIRE_PORT=<port>`, polls TCP until the port accepts a connection, and returns the resolved WS URL plus a stop func the caller defers. When `enabled` is false the function returns ("", no-op, nil) so callers can defer unconditionally. This lets `--with-backend` tests run the test bundle in the browser against a real backend without changing the main `sova test` codepath.
func maybeStartBackend(cfg BuildConfig, enabled bool) (string, func(), error) {
	if !enabled {
		return "", func() {}, nil
	}

	tmpDir, err := os.MkdirTemp("", "sova-test-backend-*")
	if err != nil {
		return "", nil, fmt.Errorf("backend tmp dir: %w", err)
	}
	cleanupTmp := func() { _ = os.RemoveAll(tmpDir) }

	backendCfg := cfg
	backendCfg.TestMode = false
	backendCfg.OutputDir = tmpDir
	backendCfg.OutputName = "output"

	c := compiler.New()
	c.SetBuildConfig(CacheKey, backendCfg)
	c.Loader = makePackageLoader(backendCfg.SourceDir)
	_, files, err := collectSources(backendCfg)
	if err != nil {
		cleanupTmp()
		return "", nil, fmt.Errorf("backend collect sources: %w", err)
	}
	for _, src := range files {
		c.AddSource(src.RelPath, src.Content)
	}
	if err := c.Compile(); err != nil {
		c.Diag.Print()
		cleanupTmp()
		return "", nil, fmt.Errorf("backend compile: %w", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		cleanupTmp()
		return "", nil, fmt.Errorf("backend compile reported errors")
	}

	port, err := resolvePort("127.0.0.1", 7080, false)
	if err != nil {
		cleanupTmp()
		return "", nil, fmt.Errorf("backend port: %w", err)
	}

	buildCmd := exec.Command("go", "build", "-o", "sovabackend", ".")
	buildCmd.Dir = tmpDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		cleanupTmp()
		return "", nil, fmt.Errorf("go build backend in %s: %w", tmpDir, err)
	}

	runCmd := exec.Command(filepath.Join(tmpDir, "sovabackend"))
	runCmd.Dir = tmpDir
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	runCmd.Env = append(os.Environ(),
		devModeEnv+"=1",
		"WIRE_PORT="+strconv.Itoa(port),
		"WIRE_HOST=127.0.0.1",
		"SOVA_WEB_DIR="+filepath.Join(tmpDir, "web"),
		"SOVA_TEST_BYPASS_AUTH=1",
	)
	runCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := runCmd.Start(); err != nil {
		cleanupTmp()
		return "", nil, fmt.Errorf("start backend: %w", err)
	}

	stop := func() {
		if runCmd.Process != nil {
			pgid, perr := syscall.Getpgid(runCmd.Process.Pid)
			if perr != nil {
				pgid = runCmd.Process.Pid
			}
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
			done := make(chan struct{})
			go func() { _ = runCmd.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
				<-done
			}
		}
		cleanupTmp()
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, derr := net.DialTimeout("tcp", addr, 250*time.Millisecond)
		if derr == nil {
			_ = conn.Close()
			fmt.Fprintf(os.Stderr, "[test] backend ready on %s\n", addr)
			return "ws://" + addr + "/__sova/ws", stop, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	stop()
	return "", nil, fmt.Errorf("backend did not become ready on %s within 10s", addr)
}
