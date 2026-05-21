package main

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed output.js
var __sovaBundleJS []byte

//go:embed output.js.map
var __sovaBundleJSMap []byte

//go:embed output.html
var __sovaProdShell []byte

// __sovaDevServeMaybe registers the production asset routes on mux. In prod the JS bundle and HTML shell are embedded into the binary via go:embed and there is no live-reload channel. The HTML shell is either the user's `web/index.html` (copied into the output by `sova build`) or the compiler's built-in default.
func __sovaDevServeMaybe(mux *http.ServeMux) bool {
	mux.HandleFunc("/__sova/runtime.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(__sovaBundleJS)
	})
	mux.HandleFunc("/__sova/runtime.js.map", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(__sovaBundleJSMap)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(__sovaProdShell)
	})
	return true
}
