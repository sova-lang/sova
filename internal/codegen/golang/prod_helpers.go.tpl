package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

//go:embed assets
var __sovaAssets embed.FS

// __sovaManifest is the bundler's `assets/manifest.json`, loaded once at startup. Maps logical names (`entry`, `entry.map`) to the hashed on-disk filenames esbuild produced. The HTTP handlers read this to know which file to serve for `/__sova/runtime.js`, so a new build with a new hash works without any directive changes - only the `assets/` directory contents differ.
type __sovaBundleManifest struct {
	Entry    string `json:"entry"`
	EntryMap string `json:"entry.map"`
}

var __sovaManifest = func() __sovaBundleManifest {
	var m __sovaBundleManifest
	data, err := fs.ReadFile(__sovaAssets, "assets/manifest.json")
	if err != nil {
		return m
	}
	_ = json.Unmarshal(data, &m)
	return m
}()

// __sovaDevServeMaybe registers the production asset routes on mux. In prod the bundler's hashed JS bundle + every other generated asset is embedded into the binary via `//go:embed assets`; `/__sova/<filename>` reads each file out of the embedded FS. The HTML shell at `assets/index.html` has its `<script src>` already rewritten by `sova build` to point at the hashed entry, so the browser bootstraps with no extra wiring at runtime.
func __sovaDevServeMaybe(mux *http.ServeMux) bool {
	mux.HandleFunc("/__sova/runtime.js", func(w http.ResponseWriter, r *http.Request) {
		__sovaServeAsset(w, r, __sovaManifest.Entry, "application/javascript")
	})
	mux.HandleFunc("/__sova/runtime.js.map", func(w http.ResponseWriter, r *http.Request) {
		__sovaServeAsset(w, r, __sovaManifest.EntryMap, "application/json")
	})
	mux.HandleFunc("/__sova/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/__sova/")
		if name == "" || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		__sovaServeAsset(w, r, name, __sovaContentTypeFor(name))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(__sovaAssets, "assets/index.html")
		if err != nil {
			http.Error(w, "shell unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	return true
}

func __sovaServeAsset(w http.ResponseWriter, r *http.Request, name, contentType string) {
	if name == "" {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(__sovaAssets, path.Join("assets", name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

func __sovaContentTypeFor(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".map", ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".html":
		return "text/html; charset=utf-8"
	}
	return "application/octet-stream"
}
