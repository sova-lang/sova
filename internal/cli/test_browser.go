package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sova/internal/passes"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// runBrowserTests serves the just-emitted JS test bundle from an in-process httptest.Server and drives a real Chromium via go-rod, calling `__sovaJSTestRun(name)` per discovered test through CDP. Result shape mirrors the goja path so the reporter is shared. When `backendWSURL` is non-empty, the host page injects `window.__sovaWSOverrideURL` before loading the test bundle so the WS client connects to the externally-spawned `sova-test-backend` instead of trying to reach `window.location.host`.
func runBrowserTests(cfg BuildConfig, entries []passes.TestEntry, jsonOut bool, headed bool, backendWSURL string) error {
	outDir := cfg.OutputDir
	if outDir == "" {
		outDir = ".output"
	}
	bundlePath := filepath.Join(outDir, cfg.OutputName+".js")
	if cfg.OutputName == "" {
		bundlePath = filepath.Join(outDir, "output.js")
	}
	srcBytes, err := os.ReadFile(bundlePath)
	if err != nil {
		return fmt.Errorf("read JS bundle %s: %w", bundlePath, err)
	}
	bundle := stripSourceMappingComment(string(srcBytes))

	mux := http.NewServeMux()
	mux.HandleFunc("/output.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(bundle))
	})
	mux.HandleFunc("/__sova/snapshot", browserSnapshotHandler)
	hostHTML := browserHostHTMLFor(backendWSURL)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(hostHTML))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	launchURL := launcher.New().Headless(!headed).MustLaunch()
	browser := rod.New().ControlURL(launchURL).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(server.URL).MustWaitLoad()
	page.MustWaitElementsMoreThan("script", 0)
	page.MustEval(`() => new Promise(r => setTimeout(r, 50))`)

	type browserResult struct {
		Name       string
		File       string
		DurationMS int64
		Panic      string
		Failures   []jsFailure
	}

	results := make([]browserResult, 0, len(entries))
	for _, entry := range entries {
		name := jsFullName(entry)
		fileLabel := ""
		if entry.File != nil {
			fileLabel = entry.File.Filename
		}
		start := time.Now()
		raw, err := page.Eval(`async (name) => { try { return JSON.stringify(await __sovaJSTestRun(name)); } catch (e) { return JSON.stringify({ failures: [], panic: String(e && e.stack || e) }); } }`, name)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			results = append(results, browserResult{Name: name, File: fileLabel, DurationMS: dur, Panic: err.Error()})
			continue
		}
		jsonStr := raw.Value.Str()
		var parsed struct {
			Failures []struct {
				Source      string         `json:"source"`
				Location    string         `json:"location"`
				HasOperands bool           `json:"hasOperands"`
				Lhs         any            `json:"lhs"`
				Rhs         any            `json:"rhs"`
				Vars        map[string]any `json:"vars"`
			} `json:"failures"`
			Panic string `json:"panic"`
		}
		if jerr := json.Unmarshal([]byte(jsonStr), &parsed); jerr != nil {
			results = append(results, browserResult{Name: name, File: fileLabel, DurationMS: dur, Panic: "bad JSON from browser: " + jerr.Error()})
			continue
		}
		var failures []jsFailure
		for _, f := range parsed.Failures {
			failures = append(failures, jsFailure{
				Source:      f.Source,
				Location:    f.Location,
				HasOperands: f.HasOperands,
				Lhs:         f.Lhs,
				Rhs:         f.Rhs,
				Vars:        f.Vars,
			})
		}
		results = append(results, browserResult{Name: name, File: fileLabel, DurationMS: dur, Panic: parsed.Panic, Failures: failures})
	}

	sort.SliceStable(results, func(i, j int) bool { return false })

	pass, fail := 0, 0
	for _, r := range results {
		passed := r.Panic == "" && len(r.Failures) == 0
		if passed {
			pass++
		} else {
			fail++
		}
		if jsonOut {
			status := "pass"
			if !passed {
				status = "fail"
			}
			rec := map[string]any{
				"name":       r.Name,
				"file":       r.File,
				"side":       "browser",
				"status":     status,
				"durationMs": r.DurationMS,
			}
			if r.Panic != "" {
				rec["panic"] = r.Panic
			}
			if len(r.Failures) > 0 {
				rec["failures"] = r.Failures
			}
			if buf, err := json.Marshal(rec); err == nil {
				fmt.Println(string(buf))
			}
			continue
		}
		if passed {
			fmt.Printf("PASS  %s  (%dms)\n", r.Name, r.DurationMS)
			continue
		}
		fmt.Printf("FAIL  %s  (%dms)\n", r.Name, r.DurationMS)
		if r.Panic != "" {
			fmt.Printf("      panic: %s\n", r.Panic)
		}
		for _, f := range r.Failures {
			if f.HasOperands {
				fmt.Printf("      assert failed at %s\n        assert %s\n        lhs = %v\n        rhs = %v\n", f.Location, f.Source, f.Lhs, f.Rhs)
			} else {
				fmt.Printf("      assert failed at %s\n        assert %s\n", f.Location, f.Source)
			}
			if len(f.Vars) > 0 {
				keys := make([]string, 0, len(f.Vars))
				for k := range f.Vars {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Printf("        %s = %v\n", k, f.Vars[k])
				}
			}
		}
	}
	if !jsonOut {
		fmt.Printf("\n%d passed, %d failed\n", pass, fail)
	}
	if fail > 0 {
		return fmt.Errorf("%d browser test(s) failed", fail)
	}
	return nil
}

// browserHostHTMLFor builds the host page that loads the test bundle inside Chromium. When `backendWSURL` is non-empty (set by --with-backend), the page first installs `window.__sovaWSOverrideURL` so the WS client in the bundle connects to the externally-spawned backend on its random port. With an empty URL the page just hosts the bundle and the WS client falls back to its default same-origin URL - which never resolves, but that is fine for JS-logic-only tests.
func browserHostHTMLFor(backendWSURL string) string {
	override := ""
	if backendWSURL != "" {
		override = fmt.Sprintf(`<script>window.__sovaWSOverrideURL = %q;</script>`, backendWSURL)
	}
	return `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Sova Test Host</title></head>
<body>
<script>window.__sovaTestHost = true;</script>
` + override + `
<script src="/output.js"></script>
</body></html>
`
}

// browserSnapshotHandler bridges `testing.expectSnapshot` from the browser side to the same on-disk snapshot store the goja and Go paths use. The browser sends a synchronous XHR POST with the JSON-serialised value as body and the snapshot name as query param; this handler resolves the storage path identically to the goja host fn (`SOVA_TEST_SNAPSHOT_DIR` env override, else `<cwd>/.sova/snapshots/<safe>.snap.json`), honors `SOVA_TEST_SNAPSHOT_CI=1` for CI mode, and returns a small JSON verdict (`{ok, created?, expected?, missing?, error?}`) the JS body parses.
func browserSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		_, _ = w.Write([]byte(`{"ok":false,"error":"POST required"}`))
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		_, _ = w.Write([]byte(`{"ok":false,"error":"name query required"}`))
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error())))
		return
	}
	dir := os.Getenv("SOVA_TEST_SNAPSHOT_DIR")
	if dir == "" {
		cwd, _ := os.Getwd()
		dir = filepath.Join(cwd, ".sova", "snapshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error())))
		return
	}
	path := filepath.Join(dir, snapshotSafeName(name)+".snap.json")
	existing, rerr := os.ReadFile(path)
	if rerr != nil {
		if os.IsNotExist(rerr) {
			if os.Getenv("SOVA_TEST_SNAPSHOT_CI") != "" {
				_, _ = w.Write([]byte(`{"ok":false,"missing":true}`))
				return
			}
			if werr := os.WriteFile(path, body, 0o644); werr != nil {
				_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":false,"error":%q}`, werr.Error())))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"created":true}`))
			return
		}
		_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":false,"error":%q}`, rerr.Error())))
		return
	}
	if string(existing) == string(body) {
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}
	buf, _ := json.Marshal(map[string]any{"ok": false, "expected": string(existing)})
	_, _ = w.Write(buf)
}

// silence unused import warnings during partial builds.
var _ = strings.Contains
