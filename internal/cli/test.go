package cli

import (
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sova/internal/passes"
	"sova/internal/services/compiler"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/spf13/cobra"
)

func newTestCmd() *cobra.Command {
	var side string
	var parallel bool
	var filter string
	var jsonOut bool
	var tag string
	var only string
	var noColor bool
	var browser bool
	var headed bool
	var withBackend bool
	cmd := &cobra.Command{
		Use:   "test [file|dir]",
		Short: "Run Sova tests defined in .test.sova files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveConfig(args, "", "", cmd)
			if err != nil {
				return err
			}

			return runTestPipeline(cfg, side, parallel, filter, jsonOut, tag, only, noColor, browser, headed, withBackend)
		},
	}

	cmd.Flags().StringVar(&side, "side", "go", "test runtime side: go | js | both")
	cmd.Flags().BoolVar(&parallel, "parallel", false, "run JS-side tests concurrently when --side covers JS")
	cmd.Flags().StringVar(&filter, "filter", "", "only run tests whose full name contains this substring (case-sensitive)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit line-delimited JSON results to stdout for CI ingestion")
	cmd.Flags().StringVar(&tag, "tag", "", "only run tests carrying this tag (matches the test decl's `tag:` list or any enclosing group's)")
	cmd.Flags().StringVar(&only, "only", "", "only run tests declared in this file path (suffix-match against each test's source filename)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI colors in human-readable output (currently a no-op; output is already plain)")
	cmd.Flags().BoolVar(&browser, "browser", false, "run JS-side tests in a real headless Chromium via go-rod instead of goja (implies --side js); the test bundle is served from an in-process httptest.Server")
	cmd.Flags().BoolVar(&headed, "headed", false, "with --browser, launch Chromium with a visible window for debugging (default is headless)")
	cmd.Flags().BoolVar(&withBackend, "with-backend", false, "with --browser, also compile the project in regular (non-test) mode and spawn the resulting backend binary on a random port; the test bundle's WS client is rerouted to the spawned backend so real wire roundtrips work end-to-end")
	return cmd
}

func runTestPipeline(cfg BuildConfig, side string, jsParallel bool, filter string, jsonOut bool, tag string, only string, noColor bool, browser bool, headed bool, withBackend bool) error {
	_ = noColor
	if withBackend {
		browser = true
	}

	if browser && strings.ToLower(side) == "go" {
		side = "js"
	}

	root, files, err := collectSources(cfg)
	if err != nil {
		return err
	}

	cfg.SourceDir = root
	cfg.TestMode = true

	c := compiler.New()
	c.SetBuildConfig(CacheKey, cfg)
	c.Loader = makePackageLoader(root)
	for _, src := range files {
		c.AddSource(src.RelPath, src.Content)
	}

	if err := c.RunTestPipeline(); err != nil {
		c.Diag.Print()
		return err
	}

	c.Diag.Print()

	raw, ok := c.Cache[passes.TestRegistryCacheKey]
	if !ok {
		fmt.Println("no tests discovered")
		return nil
	}

	entries, ok := raw.([]passes.TestEntry)
	if !ok || len(entries) == 0 {
		fmt.Println("no tests discovered")
		return nil
	}

	if filter != "" || tag != "" || only != "" {
		filtered := entries[:0]
		for _, e := range entries {
			full := fullEntryName(e)
			if filter != "" && !strings.Contains(full, filter) {
				continue
			}

			if tag != "" {
				matched := false
				for _, t := range e.Tags {
					if t == tag {
						matched = true
						break
					}
				}

				if !matched {
					continue
				}
			}

			if only != "" {
				fname := ""
				if e.File != nil {
					fname = e.File.Filename
				}

				if !strings.HasSuffix(fname, only) {
					continue
				}
			}

			filtered = append(filtered, e)
		}

		entries = filtered
		if len(entries) == 0 {
			switch {
			case only != "":
				fmt.Printf("no tests match --only %q\n", only)
			case tag != "":
				fmt.Printf("no tests match --tag %q\n", tag)
			default:
				fmt.Printf("no tests match --filter %q\n", filter)
			}

			return nil
		}
	}

	if !jsonOut {
		switch strings.ToLower(side) {
		case "go", "":
			fmt.Printf("running %d test(s) [Go-side]\n\n", len(entries))
		case "js":
			if browser {
				fmt.Printf("running %d test(s) [JS-side via Chromium]\n\n", len(entries))
			} else {
				fmt.Printf("running %d test(s) [JS-side via goja]\n\n", len(entries))
			}

		case "both":
			fmt.Printf("running %d test(s) [Go + JS]\n\n", len(entries))
		}
	}

	allowedNames := map[string]bool{}

	for _, e := range entries {
		allowedNames[fullEntryName(e)] = true
	}

	switch strings.ToLower(side) {
	case "go", "":
		return executeTestBinary(cfg, filter, jsonOut, allowedNames)
	case "js":
		if browser {
			backendURL, backendStop, err := maybeStartBackend(cfg, withBackend)
			if err != nil {
				return err
			}

			defer backendStop()
			return runBrowserTests(cfg, entries, jsonOut, headed, backendURL)
		}

		return runJSTests(cfg, entries, jsParallel, jsonOut)
	case "both":
		var jsErr error
		if browser {
			backendURL, backendStop, err := maybeStartBackend(cfg, withBackend)
			if err != nil {
				return err
			}

			defer backendStop()
			jsErr = runBrowserTests(cfg, entries, jsonOut, headed, backendURL)
		} else {
			jsErr = runJSTests(cfg, entries, jsParallel, jsonOut)
		}

		if !jsonOut {
			fmt.Println()
		}

		goErr := executeTestBinary(cfg, filter, jsonOut, allowedNames)
		if jsErr != nil {
			return jsErr
		}

		return goErr
	default:
		return fmt.Errorf("unknown --side %q (expected go|js|both)", side)
	}
}

func fullEntryName(e passes.TestEntry) string {
	if len(e.GroupPath) == 0 {
		return e.Decl.Name
	}

	return strings.Join(e.GroupPath, " > ") + " > " + e.Decl.Name
}

func runJSTests(cfg BuildConfig, entries []passes.TestEntry, parallel bool, jsonOut bool) error {
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

	src := stripSourceMappingComment(string(srcBytes))

	type jsResult struct {
		Name       string
		File       string
		DurationMS int64
		Panic      string
		Failures   []jsFailure
	}

	run := func(entry passes.TestEntry) jsResult {
		name := jsFullName(entry)
		fileLabel := ""
		if entry.File != nil {
			fileLabel = entry.File.Filename
		}

		rt := goja.New()
		installJSSnapshotIO(rt)
		installJSTimers(rt)
		installJSWebAPIs(rt)
		start := time.Now()
		if _, err := rt.RunString(string(src)); err != nil {
			return jsResult{Name: name, File: fileLabel, Panic: fmt.Sprintf("bundle eval: %v", err), DurationMS: time.Since(start).Milliseconds()}
		}

		fn, ok := goja.AssertFunction(rt.Get("__sovaJSTestRun"))
		if !ok {
			return jsResult{Name: name, File: fileLabel, Panic: "__sovaJSTestRun not exported from JS bundle", DurationMS: time.Since(start).Milliseconds()}
		}

		out, err := fn(goja.Undefined(), rt.ToValue(name))
		if err != nil {
			return jsResult{Name: name, File: fileLabel, Panic: err.Error(), DurationMS: time.Since(start).Milliseconds()}
		}

		if promise, ok := out.Export().(*goja.Promise); ok {
			switch promise.State() {
			case goja.PromiseStateFulfilled:
				out = promise.Result()
			case goja.PromiseStateRejected:
				return jsResult{Name: name, File: fileLabel, Panic: promise.Result().String(), DurationMS: time.Since(start).Milliseconds()}

			default:
				return jsResult{Name: name, File: fileLabel, Panic: "test promise did not resolve synchronously (goja has no event loop; use --browser for async wire roundtrips)", DurationMS: time.Since(start).Milliseconds()}
			}
		}

		obj := out.ToObject(rt)
		failuresRaw := obj.Get("failures")
		panicMsg := ""
		if v := obj.Get("panic"); v != nil {
			panicMsg = v.String()
		}

		var failures []jsFailure
		if failuresRaw != nil {
			arr := failuresRaw.ToObject(rt)
			for i := 0; i < int(arr.Get("length").ToInteger()); i++ {
				f := arr.Get(fmt.Sprintf("%d", i)).ToObject(rt)
				vars := map[string]any{}

				if vRaw := f.Get("vars"); vRaw != nil && !goja.IsUndefined(vRaw) && !goja.IsNull(vRaw) {
					vObj := vRaw.ToObject(rt)
					for _, key := range vObj.Keys() {
						vars[key] = exportVal(vObj.Get(key))
					}
				}

				failures = append(failures, jsFailure{
					Source:      strVal(f.Get("source")),
					Location:    strVal(f.Get("location")),
					HasOperands: boolVal(f.Get("hasOperands")),
					Lhs:         exportVal(f.Get("lhs")),
					Rhs:         exportVal(f.Get("rhs")),
					Vars:        vars,
				})
			}
		}

		return jsResult{Name: name, File: fileLabel, DurationMS: time.Since(start).Milliseconds(), Panic: panicMsg, Failures: failures}
	}

	results := make([]jsResult, len(entries))
	if parallel {
		var wg sync.WaitGroup
		for i, entry := range entries {
			i, entry := i, entry
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = run(entry)
			}()
		}

		wg.Wait()
	} else {
		for i, entry := range entries {
			results[i] = run(entry)
		}
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
				"side":       "js",
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
		return fmt.Errorf("%d JS test(s) failed", fail)
	}

	return nil
}

type jsFailure struct {
	Source      string
	Location    string
	HasOperands bool
	Lhs         any
	Rhs         any
	Vars        map[string]any
}

func jsFullName(e passes.TestEntry) string {
	if len(e.GroupPath) == 0 {
		return e.Decl.Name
	}

	return strings.Join(e.GroupPath, " > ") + " > " + e.Decl.Name
}

func strVal(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}

	return v.String()
}

func boolVal(v goja.Value) bool {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return false
	}

	return v.ToBoolean()
}

func exportVal(v goja.Value) any {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}

	return v.Export()
}

func installJSWebAPIs(rt *goja.Runtime) {
	rt.Set("__sovaJSEncodeBase64", func(s string) string {
		return base64.StdEncoding.EncodeToString([]byte(s))
	})
	rt.Set("__sovaJSDecodeBase64", func(s string) string {
		b, _ := base64.StdEncoding.DecodeString(s)
		return string(b)
	})
	rt.Set("__sovaJSHashHex", func(algo, data string) string {
		var sum []byte
		switch algo {
		case "SHA-1":
			h := sha1.Sum([]byte(data))
			sum = h[:]
		case "SHA-256":
			h := sha256.Sum256([]byte(data))
			sum = h[:]
		case "SHA-512":
			h := sha512.Sum512([]byte(data))
			sum = h[:]
		}

		out := make([]byte, len(sum)*2)
		const hexd = "0123456789abcdef"
		for i, b := range sum {
			out[i*2] = hexd[b>>4]
			out[i*2+1] = hexd[b&0x0f]
		}

		return string(out)
	})
	rt.Set("__sovaJSHmacHex", func(algo, key, data string) string {
		var mac hash.Hash
		switch algo {
		case "SHA-1":
			mac = hmac.New(sha1.New, []byte(key))
		case "SHA-256":
			mac = hmac.New(sha256.New, []byte(key))
		case "SHA-512":
			mac = hmac.New(sha512.New, []byte(key))
		}

		if mac == nil {
			return ""
		}

		mac.Write([]byte(data))
		sum := mac.Sum(nil)
		out := make([]byte, len(sum)*2)
		const hexd = "0123456789abcdef"
		for i, b := range sum {
			out[i*2] = hexd[b>>4]
			out[i*2+1] = hexd[b&0x0f]
		}

		return string(out)
	})
	rt.Set("__sovaJSRandomHex", func(n int) string {
		b := make([]byte, n)
		_, _ = cryptorand.Read(b)
		out := make([]byte, n*2)
		const hexd = "0123456789abcdef"
		for i, v := range b {
			out[i*2] = hexd[v>>4]
			out[i*2+1] = hexd[v&0x0f]
		}

		return string(out)
	})
	rt.Set("__sovaJSRandomUUID", func() string {
		b := make([]byte, 16)
		_, _ = cryptorand.Read(b)
		b[6] = (b[6] & 0x0f) | 0x40
		b[8] = (b[8] & 0x3f) | 0x80
		const hexd = "0123456789abcdef"
		out := make([]byte, 36)
		j := 0
		for i, v := range b {
			if i == 4 || i == 6 || i == 8 || i == 10 {
				out[j] = '-'
				j++
			}

			out[j] = hexd[v>>4]
			j++
			out[j] = hexd[v&0x0f]
			j++
		}

		return string(out)
	})

	polyfillJS := `(function () {
  if (typeof globalThis.TextEncoder === 'undefined') {
    globalThis.TextEncoder = function TextEncoder() {};
    globalThis.TextEncoder.prototype.encode = function (s) {
      s = String(s == null ? '' : s);
      const out = [];
      for (let i = 0; i < s.length; i++) {
        let c = s.charCodeAt(i);
        if (c < 0x80) {
          out.push(c);
        } else if (c < 0x800) {
          out.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f));
        } else if (c < 0xd800 || c >= 0xe000) {
          out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
        } else {
          i++;
          const c2 = s.charCodeAt(i);
          const u = 0x10000 + (((c & 0x3ff) << 10) | (c2 & 0x3ff));
          out.push(0xf0 | (u >> 18), 0x80 | ((u >> 12) & 0x3f), 0x80 | ((u >> 6) & 0x3f), 0x80 | (u & 0x3f));
        }
      }
      return new Uint8Array(out);
    };
  }
  if (typeof globalThis.TextDecoder === 'undefined') {
    globalThis.TextDecoder = function TextDecoder() {};
    globalThis.TextDecoder.prototype.decode = function (bytes) {
      const b = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes || []);
      let s = '';
      let i = 0;
      while (i < b.length) {
        const c = b[i++];
        if (c < 0x80) { s += String.fromCharCode(c); continue; }
        if (c < 0xc0) { continue; }
        if (c < 0xe0) { s += String.fromCharCode(((c & 0x1f) << 6) | (b[i++] & 0x3f)); continue; }
        if (c < 0xf0) { s += String.fromCharCode(((c & 0x0f) << 12) | ((b[i++] & 0x3f) << 6) | (b[i++] & 0x3f)); continue; }
        const u = ((c & 0x07) << 18) | ((b[i++] & 0x3f) << 12) | ((b[i++] & 0x3f) << 6) | (b[i++] & 0x3f);
        const off = u - 0x10000;
        s += String.fromCharCode(0xd800 | (off >> 10), 0xdc00 | (off & 0x3ff));
      }
      return s;
    };
  }
  if (typeof globalThis.btoa === 'undefined') {
    globalThis.btoa = function (s) { return __sovaJSEncodeBase64(s); };
  }
  if (typeof globalThis.atob === 'undefined') {
    globalThis.atob = function (s) { return __sovaJSDecodeBase64(s); };
  }
  if (typeof globalThis.crypto === 'undefined') {
    globalThis.crypto = {};
  }
  if (typeof globalThis.crypto.getRandomValues !== 'function') {
    globalThis.crypto.getRandomValues = function (buf) {
      const hex = __sovaJSRandomHex(buf.length);
      for (let i = 0; i < buf.length; i++) {
        buf[i] = parseInt(hex.substr(i*2, 2), 16);
      }
      return buf;
    };
  }
  if (typeof globalThis.crypto.randomUUID !== 'function') {
    globalThis.crypto.randomUUID = function () { return __sovaJSRandomUUID(); };
  }
  if (typeof globalThis.crypto.subtle === 'undefined') {
    function bytesFromBufOrView(d) {
      if (d instanceof Uint8Array) return d;
      if (d && d.buffer) return new Uint8Array(d.buffer, d.byteOffset || 0, d.byteLength);
      return new Uint8Array(d || []);
    }
    function bytesToString(b) {
      let s = '';
      for (let i = 0; i < b.length; i++) s += String.fromCharCode(b[i]);
      return s;
    }
    function hexToBytes(h) {
      const b = new Uint8Array(h.length / 2);
      for (let i = 0; i < b.length; i++) b[i] = parseInt(h.substr(i*2, 2), 16);
      return b;
    }
    globalThis.crypto.subtle = {
      digest: function (algo, data) {
        const b = bytesFromBufOrView(data);
        const hex = __sovaJSHashHex(algo, bytesToString(b));
        return Promise.resolve(hexToBytes(hex).buffer);
      },
      importKey: function (_format, keyData, algo, _extractable, _usages) {
        return Promise.resolve({ __sovaKey: bytesFromBufOrView(keyData), __sovaAlgo: (algo && algo.hash) || 'SHA-256' });
      },
      sign: function (_algo, key, data) {
        const k = bytesToString(key.__sovaKey);
        const d = bytesToString(bytesFromBufOrView(data));
        const hex = __sovaJSHmacHex(key.__sovaAlgo, k, d);
        return Promise.resolve(hexToBytes(hex).buffer);
      },
      verify: function (_algo, key, sigBuf, data) {
        const k = bytesToString(key.__sovaKey);
        const d = bytesToString(bytesFromBufOrView(data));
        const expected = __sovaJSHmacHex(key.__sovaAlgo, k, d);
        const actual = Array.from(bytesFromBufOrView(sigBuf)).map(x => x.toString(16).padStart(2, '0')).join('');
        return Promise.resolve(expected === actual);
      },
    };
  }
})();`
	_, _ = rt.RunString(polyfillJS)
}

func installJSTimers(rt *goja.Runtime) {
	timersSrc := `(function () {
  const handles = new Map();
  let nextId = 1;
  const intervalMaxTicks = 4;
  globalThis.setTimeout = function (fn, _ms) {
    const id = nextId++;
    handles.set(id, { cancelled: false });
    Promise.resolve().then(() => {
      const h = handles.get(id);
      if (!h || h.cancelled) { return; }
      handles.delete(id);
      try { fn(); } catch (_) {}
    });
    return id;
  };
  globalThis.clearTimeout = function (id) {
    const h = handles.get(id);
    if (h) { h.cancelled = true; }
  };
  globalThis.setInterval = function (fn, _ms) {
    const id = nextId++;
    const h = { cancelled: false, ticks: 0 };
    handles.set(id, h);
    const tick = () => {
      if (h.cancelled || h.ticks >= intervalMaxTicks) { handles.delete(id); return; }
      h.ticks++;
      try { fn(); } catch (_) {}
      Promise.resolve().then(tick);
    };
    Promise.resolve().then(tick);
    return id;
  };
  globalThis.clearInterval = function (id) {
    const h = handles.get(id);
    if (h) { h.cancelled = true; }
  };
})();`
	_, _ = rt.RunString(timersSrc)
}

func installJSSnapshotIO(rt *goja.Runtime) {
	rt.Set("__sovaJSSnapshotIO", func(name string, payload string) map[string]any {
		dir := os.Getenv("SOVA_TEST_SNAPSHOT_DIR")
		if dir == "" {
			cwd, _ := os.Getwd()
			dir = filepath.Join(cwd, ".sova", "snapshots")
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}

		safe := snapshotSafeName(name)
		path := filepath.Join(dir, safe+".snap.json")
		existing, rerr := os.ReadFile(path)
		if rerr != nil {
			if os.IsNotExist(rerr) {
				if os.Getenv("SOVA_TEST_SNAPSHOT_CI") != "" {
					return map[string]any{"ok": false, "missing": true}
				}

				if werr := os.WriteFile(path, []byte(payload), 0o644); werr != nil {
					return map[string]any{"ok": false, "error": werr.Error()}
				}

				return map[string]any{"ok": true, "created": true}
			}

			return map[string]any{"ok": false, "error": rerr.Error()}
		}

		if string(existing) == payload {
			return map[string]any{"ok": true}
		}

		return map[string]any{"ok": false, "expected": string(existing)}
	})
}

func snapshotSafeName(s string) string {
	buf := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			buf = append(buf, byte(r))
		default:
			buf = append(buf, '_')
		}
	}

	if len(buf) == 0 {
		return "snap"
	}

	return string(buf)
}

func stripSourceMappingComment(src string) string {
	lines := strings.Split(src, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "//# sourceMappingURL=") {
			lines = append(lines[:i], lines[i+1:]...)
		}

		break
	}

	return strings.Join(lines, "\n")
}

func executeTestBinary(cfg BuildConfig, filter string, jsonOut bool, allowedNames map[string]bool) error {
	outDir := cfg.OutputDir
	if outDir == "" {
		outDir = ".output"
	}

	buildCmd := exec.Command("go", "build", "-o", "sovatest", ".")
	buildCmd.Dir = outDir
	buildCmd.Env = append(os.Environ(), "GOWORK=off")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("go build (test driver) in %s: %w", outDir, err)
	}

	runCmd := exec.Command("./sovatest")
	runCmd.Dir = outDir
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	env := os.Environ()
	if filter != "" {
		env = append(env, "SOVA_TEST_FILTER="+filter)
	}

	if jsonOut {
		env = append(env, "SOVA_TEST_JSON=1")
	}

	if allowedNames != nil {
		names := make([]string, 0, len(allowedNames))
		for k := range allowedNames {
			names = append(names, k)
		}

		env = append(env, "SOVA_TEST_ALLOWED="+strings.Join(names, "\n"))
	}

	runCmd.Env = env
	if err := runCmd.Run(); err != nil {
		return fmt.Errorf("test run failed: %w", err)
	}

	return nil
}
