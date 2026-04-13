package server

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/scout-kit/fine-print/internal/api"
	"github.com/scout-kit/fine-print/internal/config"
	"github.com/scout-kit/fine-print/internal/db"
)

// Server is the main HTTP server for Fine Print.
type Server struct {
	mux    *http.ServeMux
	cfg    config.Config
	sseHub *SSEHub
}

// New creates a new Server with all routes and middleware configured.
func New(cfg config.Config, handlers *api.Handlers, queries *db.Queries, sseHub *SSEHub, frontendFS fs.FS) *Server {
	mux := http.NewServeMux()

	RegisterRoutes(mux, handlers, queries, sseHub)

	// Serve frontend SPA. All non-API routes serve the frontend.
	if frontendFS != nil {
		mux.Handle("/", spaHandler(frontendFS))
	}

	return &Server{
		mux:    mux,
		cfg:    cfg,
		sseHub: sseHub,
	}
}

// Handler returns the fully wrapped HTTP handler with all middleware applied.
func (s *Server) Handler() http.Handler {
	var handler http.Handler = s.mux

	// Apply middleware in reverse order (outermost first)
	captiveEnabled := !s.cfg.Dev.Mode && s.cfg.Hotspot.Enabled
	handler = CaptiveMiddleware(s.cfg.Hotspot.Gateway, captiveEnabled)(handler)
	handler = LoggingMiddleware(handler)
	handler = RecoveryMiddleware(handler)

	return handler
}

// ListenAddr returns the address to listen on.
func (s *Server) ListenAddr() string {
	return fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
}

// SSEHub returns the SSE hub for broadcasting events.
func (s *Server) SSEHub() *SSEHub {
	return s.sseHub
}

// spaHandler serves an embedded SPA filesystem. It serves static files when
// they exist, and falls back to index.html for client-side routing.
func spaHandler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Determine which file to serve
		servePath := strings.TrimPrefix(r.URL.Path, "/")
		if servePath == "" {
			servePath = "index.html"
		}

		// Check if the file exists and is not a directory
		f, err := fsys.Open(servePath)
		if err != nil {
			// Fall back to index.html for SPA routing
			servePath = "index.html"
		} else {
			stat, _ := f.Stat()
			f.Close()
			if stat != nil && stat.IsDir() {
				servePath = "index.html"
			}
		}

		// Read and serve the file
		data, err := fs.ReadFile(fsys, servePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Set content type based on extension
		contentType := "application/octet-stream"
		switch {
		case strings.HasSuffix(servePath, ".html"):
			contentType = "text/html; charset=utf-8"
		case strings.HasSuffix(servePath, ".js"):
			contentType = "application/javascript"
		case strings.HasSuffix(servePath, ".css"):
			contentType = "text/css"
		case strings.HasSuffix(servePath, ".json"):
			contentType = "application/json"
		case strings.HasSuffix(servePath, ".svg"):
			contentType = "image/svg+xml"
		case strings.HasSuffix(servePath, ".png"):
			contentType = "image/png"
		case strings.HasSuffix(servePath, ".jpg"), strings.HasSuffix(servePath, ".jpeg"):
			contentType = "image/jpeg"
		case strings.HasSuffix(servePath, ".woff2"):
			contentType = "font/woff2"
		case strings.HasSuffix(servePath, ".woff"):
			contentType = "font/woff"
		case strings.HasSuffix(servePath, ".txt"):
			contentType = "text/plain"
		}

		w.Header().Set("Content-Type", contentType)
		if strings.Contains(servePath, "/immutable/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		w.Write(data)
	})
}

// StartDev logs development mode information.
func StartDev(addr string) {
	log.Printf("Development mode active")
	log.Printf("Hotspot and DNS are disabled")
	log.Printf("App available at http://localhost:%s", strings.Split(addr, ":")[len(strings.Split(addr, ":"))-1])
}
