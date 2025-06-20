// Package ui provides the web UI for Discobox
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:static
var staticFiles embed.FS

// Handler returns an HTTP handler for serving the UI
func Handler() http.Handler {
	// Create a sub filesystem from the embedded files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		// If static directory doesn't exist, serve from the embedded root
		staticFS = staticFiles
	}
	
	fileServer := http.FileServer(http.FS(staticFS))
	
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path
		upath := r.URL.Path
		if !strings.HasPrefix(upath, "/") {
			upath = "/" + upath
			r.URL.Path = upath
		}
		
		// Remove /ui prefix if present
		if strings.HasPrefix(upath, "/ui") {
			upath = strings.TrimPrefix(upath, "/ui")
			if upath == "" {
				upath = "/"
			}
			r.URL.Path = upath
		}
		
		// Try to serve the file
		// If it's a directory or doesn't exist, serve index.html
		if upath == "/" || !hasExtension(upath) {
			upath = "/index.html"
			r.URL.Path = upath
		}
		
		// Set cache headers for static assets
		if hasExtension(upath) && upath != "/index.html" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		
		// Set security headers
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		
		// Serve the file
		fileServer.ServeHTTP(w, r)
	})
}

// hasExtension checks if a path has a file extension
func hasExtension(p string) bool {
	return path.Ext(p) != ""
}

// NotFoundHandler returns a handler that serves the index.html for SPA routing
func NotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For SPA, always serve index.html for non-asset paths
		if !hasExtension(r.URL.Path) {
			r.URL.Path = "/index.html"
			Handler().ServeHTTP(w, r)
			return
		}
		
		// Otherwise, return 404
		http.NotFound(w, r)
	})
}
