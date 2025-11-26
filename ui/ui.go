package ui

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/upspeak/upspeak/app"
)

//go:embed web/build/*
var buildFS embed.FS

//go:embed web/static/*
var staticFS embed.FS

// Implements app.Module interface
type ModuleUI struct{}

func (m ModuleUI) Name() string {
	return "ui"
}

func (m ModuleUI) Init(config map[string]any) error {
	// Initialization logic for the UI module
	return nil
}

func (m ModuleUI) HTTPHandlers(pub app.Publisher) []app.HTTPHandler {
	// Create sub filesystem for build files
	buildFs, err := fs.Sub(buildFS, "web/build")
	if err != nil {
		slog.Error("Failed to create sub filesystem for build files", "error", err)
		// Return empty handlers if build files not found
		return []app.HTTPHandler{}
	}

	// Create file server handler for build assets
	buildFileServer := http.FileServer(http.FS(buildFs))

	// Return HTTP handlers for the UI module
	return []app.HTTPHandler{
		// Serve SvelteKit's _app directory (contains JS, CSS, and other assets)
		{
			Method:  "GET",
			Path:    "/_app/",
			Handler: buildFileServer.ServeHTTP,
		},
		// Serve favicon from static directory
		{
			Method: "GET",
			Path:   "/favicon.ico",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				data, err := staticFS.ReadFile("web/static/logo/logo-no-bg.png")
				if err != nil {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				w.Write(data)
			},
		},
		// Catch-all route for SPA - serves static files or index.html
		{
			Method: "GET",
			Path:   "/",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				// Special case: root path always serves index.html
				if r.URL.Path == "/" {
					data, err := buildFS.ReadFile("web/build/index.html")
					if err != nil {
						http.Error(w, "Frontend not found", http.StatusNotFound)
						slog.Error("Failed to read index.html", "error", err)
						return
					}
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.Write(data)
					return
				}

				// For other paths: check if it's a file, directory, or non-existent
				// Normalize path by removing trailing slash for directory detection
				// embed.FS doesn't recognize "path/" as a directory
				checkPath := "web/build" + strings.TrimSuffix(r.URL.Path, "/")

				file, err := buildFS.Open(checkPath)
				if err == nil {
					defer file.Close()
					stat, err := file.Stat()
					if err == nil && stat.IsDir() {
						// Don't allow directory listing, return 404
						http.NotFound(w, r)
						return
					}
					// It's a file, serve it using the file server
					buildFileServer.ServeHTTP(w, r)
					return
				}

				// File doesn't exist, serve index.html for SPA client-side routing
				data, err := buildFS.ReadFile("web/build/index.html")
				if err != nil {
					http.Error(w, "Frontend not found", http.StatusNotFound)
					slog.Error("Failed to read index.html", "error", err)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(data)
			},
		},
	}
}

func (m ModuleUI) MsgHandlers(pub app.Publisher) []app.MsgHandler {
	// Return message handlers for the UI module
	return []app.MsgHandler{}
}
