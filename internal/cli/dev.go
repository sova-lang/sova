package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"sova/internal/passes"
	"sova/internal/services/compiler"
	"sova/internal/termui"

	"github.com/fsnotify/fsnotify"
)

// portHolder tries to name the process currently holding `port` so the dev
// CLI can tell the user *who* is blocking the default port rather than just
// "in use". Best-effort: relies on `lsof` (preferred) or `ss`; returns "" on
// platforms or systems where neither tool is reachable. We deliberately
// avoid pulling in a netlink/proc-scanning dependency for what is purely a
// UX nicety.
func portHolder(host string, port int) string {
	if port <= 0 {
		return ""
	}
	if h := portHolderViaLsof(host, port); h != "" {
		return h
	}
	if h := portHolderViaSS(port); h != "" {
		return h
	}
	return ""
}

func portHolderViaLsof(host string, port int) string {
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		return ""
	}
	args := []string{"-nP", "-iTCP:" + strconv.Itoa(port), "-sTCP:LISTEN", "-Fpcn"}
	if host != "" && host != "0.0.0.0" && host != "::" {
		args = []string{"-nP", "-iTCP@" + host + ":" + strconv.Itoa(port), "-sTCP:LISTEN", "-Fpcn"}
	}
	cmd := exec.Command(lsof, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var pid, name string
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid = line[1:]
		case 'c':
			name = line[1:]
		}
		if pid != "" && name != "" {
			return fmt.Sprintf("%s (pid %s)", name, pid)
		}
	}
	return ""
}

func portHolderViaSS(port int) string {
	ss, err := exec.LookPath("ss")
	if err != nil {
		return ""
	}
	cmd := exec.Command(ss, "-Hltnp", "sport", "=", ":"+strconv.Itoa(port))
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		// users:(("process",pid=1234,fd=5))
		idx := strings.Index(line, `users:((`)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(`users:((`):]
		end := strings.Index(rest, `))`)
		if end < 0 {
			continue
		}
		tuple := rest[:end]
		fields := strings.Split(tuple, ",")
		if len(fields) < 2 {
			continue
		}
		name := strings.Trim(strings.TrimSpace(fields[0]), `"`)
		pid := strings.TrimSpace(fields[1])
		pid = strings.TrimPrefix(pid, "pid=")
		return fmt.Sprintf("%s (pid %s)", name, pid)
	}
	return ""
}

const (
	portProbeAttempts = 100
	reloadSignalEnv   = "SOVA_RELOAD_SIGFILE"
	devModeEnv        = "SOVA_DEV"
	devOriginEnv      = "SOVA_DEV_ORIGIN"
)

// resolvePort walks upward from start until it finds a port that can be bound on the given host. It gives up after portProbeAttempts steps. When strict is true the function tries only the start port and returns an error if it is taken.
func resolvePort(host string, start int, strict bool) (int, error) {
	if start <= 0 {
		start = 5173
	}
	limit := portProbeAttempts
	if strict {
		limit = 1
	}
	addrHost := host
	if addrHost == "" {
		addrHost = "0.0.0.0"
	}
	for i := 0; i < limit; i++ {
		port := start + i
		ln, err := net.Listen("tcp", net.JoinHostPort(addrHost, strconv.Itoa(port)))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	if strict {
		return 0, fmt.Errorf("strict port %d is already in use", start)
	}
	return 0, fmt.Errorf("no free port found in range %d..%d", start, start+limit-1)
}

// runDev is the entry point for `sova dev`. It compiles once, spawns the backend in dev mode, and respawns on every .sova change. The signal file mechanism lets the running backend push a reload event to connected browsers without a full restart when only the JS bundle changed.
func runDev(cfg BuildConfig) error {
	root, _, err := collectSources(cfg)
	if err != nil {
		return err
	}
	cfg.SourceDir = root

	termui.Header("sova dev")
	termui.Step("compiling project")
	firstCtx, err := compileOnce(cfg)
	if err != nil {
		termui.Failure("initial compile failed - fix the diagnostics above and rerun")
		return err
	}
	termui.Success("compiled")

	embedWatch := &embedWatchSet{}
	embedWatch.Set(embedSourcePaths(firstCtx))

	port, err := resolvePort(cfg.ServeHost, cfg.ServePort, cfg.ServeStrictPort)
	if err != nil {
		if holder := portHolder(cfg.ServeHost, cfg.ServePort); holder != "" {
			termui.Failure(fmt.Sprintf("port %d is already in use by %s", cfg.ServePort, holder))
		}
		return err
	}
	if port != cfg.ServePort {
		if holder := portHolder(cfg.ServeHost, cfg.ServePort); holder != "" {
			termui.WarnMsg(fmt.Sprintf("port %d is held by %s - falling back to %d", cfg.ServePort, holder, port))
		} else {
			termui.WarnMsg(fmt.Sprintf("port %d is in use - falling back to %d", cfg.ServePort, port))
		}
	}

	sigFile, err := os.CreateTemp("", "sova-reload-*.sig")
	if err != nil {
		return fmt.Errorf("create reload sigfile: %w", err)
	}
	sigFile.Close()
	defer os.Remove(sigFile.Name())

	mgr := &devProcess{
		outputGo: filepath.Join(cfg.OutputDir, cfg.OutputName+".go"),
		port:     port,
		host:     cfg.ServeHost,
		sigFile:  sigFile.Name(),
		webDir:   cfg.ServeWebDir,
	}
	if err := mgr.start(); err != nil {
		return fmt.Errorf("spawn backend: %w", err)
	}
	defer mgr.stop()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watcher: %w", err)
	}
	defer watcher.Close()

	if err := watchTree(watcher, root); err != nil {
		return fmt.Errorf("watch tree: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	origin := fmt.Sprintf("http://%s:%d", displayHost(cfg.ServeHost), port)
	termui.Success(fmt.Sprintf("dev server ready on %s", termui.Bold(termui.Cyan(origin))))

	var (
		debounce  = 80 * time.Millisecond
		fireTimer *time.Timer
		mu        sync.Mutex
	)
	queueRecompile := func() {
		mu.Lock()
		defer mu.Unlock()
		if fireTimer != nil {
			fireTimer.Stop()
		}
		fireTimer = time.AfterFunc(debounce, func() {
			termui.Step("recompiling")
			ctx, err := compileOnce(cfg)
			if err != nil {
				termui.Failure("compile failed - waiting for the next change")
				return
			}
			embedWatch.Set(embedSourcePaths(ctx))
			termui.Success("recompiled")
			if err := mgr.signalReload(); err != nil {
				termui.WarnMsg(fmt.Sprintf("reload signal failed: %v", err))
			}
		})
	}

	for {
		select {
		case <-ctx.Done():
			termui.Step("shutting down")
			return nil
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if shouldTriggerRecompile(ev, root, cfg.OutputDir, embedWatch) {
				queueRecompile()
			}
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = watchTree(watcher, ev.Name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintln(os.Stderr, "[dev] watcher:", err)
		}
	}
}

func compileOnce(cfg BuildConfig) (*compiler.CompilerContext, error) {
	_, files, err := collectSources(cfg)
	if err != nil {
		return nil, err
	}
	c := compiler.New()
	c.SetBuildConfig(CacheKey, cfg)
	c.Loader = makePackageLoader(cfg.SourceDir)
	for _, src := range files {
		c.AddSource(src.RelPath, src.Content)
	}
	compileErr := c.Compile()
	c.Diag.Print()
	if compileErr != nil {
		return c, compileErr
	}
	if c.Diag.Errored() {
		return c, fmt.Errorf("compilation failed")
	}
	if err := stageEmbedAssets(c, cfg.OutputDir); err != nil {
		return c, fmt.Errorf("stage @embed assets: %w", err)
	}
	return c, nil
}

// embedSourcePaths returns the absolute source paths of every `@embed`-decorated const the build resolved. The dev watcher uses this set to trigger a recompile when an embedded asset's contents change (otherwise the runtime keeps the stale value baked in from the previous build).
func embedSourcePaths(c *compiler.CompilerContext) []string {
	if c == nil {
		return nil
	}
	raw, ok := c.Cache[passes.EmbedAssetsCacheKey]
	if !ok {
		return nil
	}
	records, ok := raw.([]*passes.EmbedRecord)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(records))
	for _, rec := range records {
		if rec == nil || rec.Info == nil {
			continue
		}
		out = append(out, rec.Info.SourcePath)
	}
	return out
}

func watchTree(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base == ".output" || base == ".bin" || base == ".git" || base == "node_modules" {
			return filepath.SkipDir
		}
		return w.Add(path)
	})
}

func shouldTriggerRecompile(ev fsnotify.Event, root, outputDir string, embeds *embedWatchSet) bool {
	if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}
	abs, err := filepath.Abs(ev.Name)
	if err != nil {
		return false
	}
	outAbs, err := filepath.Abs(outputDir)
	if err == nil {
		if rel, err := filepath.Rel(outAbs, abs); err == nil && !startsWithDotDot(rel) {
			return false
		}
	}
	if embeds != nil && embeds.Has(abs) {
		return true
	}
	ext := filepath.Ext(ev.Name)
	return ext == ".sova" || ext == ".toml"
}

// embedWatchSet is the dev-mode set of absolute paths the watcher should treat as recompile triggers in addition to `.sova` / `.toml`. Populated from `passes.EmbedAssetsCacheKey` after every successful compile; reads + writes are guarded so the watcher goroutine can poll while the compile goroutine swaps the contents.
type embedWatchSet struct {
	mu    sync.RWMutex
	paths map[string]struct{}
}

func (e *embedWatchSet) Set(paths []string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(paths) == 0 {
		e.paths = nil
		return
	}
	out := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		out[abs] = struct{}{}
	}
	e.paths = out
}

func (e *embedWatchSet) Has(absPath string) bool {
	if e == nil {
		return false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.paths == nil {
		return false
	}
	_, ok := e.paths[absPath]
	return ok
}

func startsWithDotDot(rel string) bool {
	return len(rel) >= 2 && rel[0] == '.' && rel[1] == '.'
}

func displayHost(host string) string {
	if host == "" {
		return "localhost"
	}
	return host
}

// devProcess wraps the spawned backend so we can restart it on Go-level changes and poke it via the reload sigfile for JS-only changes.
type devProcess struct {
	outputGo string
	port     int
	host     string
	sigFile  string
	webDir   string

	mu  sync.Mutex
	cmd *exec.Cmd
}

func (d *devProcess) start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := os.Stat(d.outputGo); err != nil {
		return fmt.Errorf("output %s not found (compile failed?): %w", d.outputGo, err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Dir(d.outputGo)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		devModeEnv+"=1",
		"WIRE_PORT="+strconv.Itoa(d.port),
		"WIRE_HOST="+d.host,
		reloadSignalEnv+"="+d.sigFile,
		"SOVA_WEB_DIR="+d.webDir,
		devOriginEnv+"=http://"+displayHost(d.host)+":"+strconv.Itoa(d.port),
	)
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	d.cmd = cmd
	return nil
}

func (d *devProcess) stop() {
	d.mu.Lock()
	cmd := d.cmd
	d.cmd = nil
	d.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	terminateProcess(cmd.Process)
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		killProcess(cmd.Process)
		<-done
	}
}

// signalReload restarts the spawned backend so Go-side changes take effect and bumps the sigfile so any SSE clients still attached to the previous instance receive a reload event before they disconnect. The bump itself is best-effort; the SSE-close fallback on the browser handles the reconnect.
func (d *devProcess) signalReload() error {
	now := time.Now()
	if err := os.Chtimes(d.sigFile, now, now); err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "[dev] sigfile bump:", err)
	}
	d.stop()
	return d.start()
}
