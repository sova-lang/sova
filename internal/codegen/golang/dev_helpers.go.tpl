package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var __sovaDevSubscribers = struct {
	sync.Mutex
	chans map[chan struct{}]struct{}
}{chans: map[chan struct{}]struct{}{}}

// __sovaDevServeMaybe attaches the dev-server endpoints to mux when SOVA_DEV=1. It is a no-op outside dev mode so the same generated code can run in prod without changes.
func __sovaDevServeMaybe(mux *http.ServeMux) bool {
	if os.Getenv("SOVA_DEV") != "1" {
		return false
	}
	if sig := os.Getenv("SOVA_RELOAD_SIGFILE"); sig != "" {
		go __sovaWatchSigFile(sig)
	}
	webDir := os.Getenv("SOVA_WEB_DIR")
	if webDir == "" {
		webDir = "web"
	}
	bundlePath := __sovaFindBundlePath()

	mux.HandleFunc("/__sova/dev/sse", __sovaSSEHandler)
	mux.HandleFunc("/__sova/runtime.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, bundlePath)
	})
	mux.HandleFunc("/__sova/runtime.js.map", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, bundlePath+".map")
	})
	mux.HandleFunc("/__sova/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/__sova/")
		if name == "" || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		candidate := filepath.Join(filepath.Dir(bundlePath), "assets", name)
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, candidate)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		userIndex := filepath.Join(webDir, "index.html")
		if _, err := os.Stat(userIndex); err == nil {
			data, _ := os.ReadFile(userIndex)
			html := __sovaInjectLiveReload(string(data))
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(html))
			return
		}
		if r.URL.Path != "/" {
			candidate := filepath.Join(webDir, strings.TrimPrefix(r.URL.Path, "/"))
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				http.ServeFile(w, r, candidate)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(__sovaDefaultShell()))
	})
	fmt.Fprintln(os.Stderr, "[dev] dev endpoints registered (SSE, runtime.js, /)")
	return true
}

func __sovaFindBundlePath() string {
	candidates := []string{"output.js"}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "output.js"))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "output.js"))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "output.js"
}

func __sovaSSEHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan struct{}, 4)
	__sovaDevSubscribers.Lock()
	__sovaDevSubscribers.chans[ch] = struct{}{}
	__sovaDevSubscribers.Unlock()
	defer func() {
		__sovaDevSubscribers.Lock()
		delete(__sovaDevSubscribers.chans, ch)
		__sovaDevSubscribers.Unlock()
	}()
	fmt.Fprint(w, "event: hello\ndata: ready\n\n")
	flusher.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			fmt.Fprint(w, "event: reload\ndata: reload\n\n")
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func __sovaBroadcastReload() {
	__sovaDevSubscribers.Lock()
	defer __sovaDevSubscribers.Unlock()
	for ch := range __sovaDevSubscribers.chans {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func __sovaWatchSigFile(path string) {
	var lastMod time.Time
	if info, err := os.Stat(path); err == nil {
		lastMod = info.ModTime()
	}
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().After(lastMod) {
			lastMod = info.ModTime()
			__sovaBroadcastReload()
		}
	}
}

const __sovaLiveReloadSnippet = `<script>(function(){var es=new EventSource('/__sova/dev/sse');es.addEventListener('reload',function(){location.reload();});es.onerror=function(){es.close();setTimeout(function(){location.reload();},500);};})();</script>`

func __sovaInjectLiveReload(html string) string {
	if strings.Contains(html, "__sova/dev/sse") {
		return html
	}
	lower := strings.ToLower(html)
	idx := strings.LastIndex(lower, "</body>")
	if idx < 0 {
		return html + __sovaLiveReloadSnippet
	}
	return html[:idx] + __sovaLiveReloadSnippet + html[idx:]
}

func __sovaDefaultShell() string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sova App</title>
</head>
<body>
<div id="app"></div>
<script type="module" src="/__sova/runtime.js"></script>
` + __sovaLiveReloadSnippet + `
</body>
</html>`
}
