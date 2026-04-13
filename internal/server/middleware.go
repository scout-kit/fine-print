package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/scout-kit/fine-print/internal/db"
)

type contextKey string

const adminContextKey contextKey = "admin"

// IsAdmin returns true if the request is from an authenticated admin.
func IsAdmin(r *http.Request) bool {
	v, ok := r.Context().Value(adminContextKey).(bool)
	return ok && v
}

// RecoveryMiddleware catches panics and returns a 500.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %v", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware logs each request.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.statusCode, time.Since(start))
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher for SSE support.
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// CaptiveMiddleware intercepts captive portal detection requests and redirects
// to the application. Only active when not in dev mode.
func CaptiveMiddleware(gatewayIP string, enabled bool) func(http.Handler) http.Handler {
	captivePaths := map[string]bool{
		"/hotspot-detect.html":          true,
		"/library/test/success.html":    true,
		"/generate_204":                 true,
		"/gen_204":                      true,
		"/connectivitycheck/gstatic/":   true,
		"/chrome/test":                  true,
		"/connecttest.txt":              true,
		"/ncsi.txt":                     true,
		"/redirect":                     true,
		"/canonical.html":               true,
		"/success.txt":                  true,
		"/generate_204_mobile":          true,
		"/generate_204_huawei":          true,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			path := strings.ToLower(r.URL.Path)
			if captivePaths[path] {
				redirectURL := "http://" + gatewayIP + "/"
				http.Redirect(w, r, redirectURL, http.StatusFound)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AdminAuthMiddleware checks for a valid admin session cookie.
func AdminAuthMiddleware(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("fineprint_session")
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			session, err := queries.GetAdminSessionByToken(r.Context(), cookie.Value)
			if err != nil || session == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), adminContextKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GuestSessionMiddleware ensures each guest has a session cookie.
func GuestSessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := r.Cookie("fineprint_guest")
		if err != nil {
			// Will be set by the guest API handler on first interaction
		}
		next.ServeHTTP(w, r)
	})
}
