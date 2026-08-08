package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"

	"github.com/scout-kit/fine-print/internal/config"
	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/diskguard"
	"github.com/scout-kit/fine-print/internal/imaging"
	"github.com/scout-kit/fine-print/internal/printer"
	"github.com/scout-kit/fine-print/internal/qrcode"
	"github.com/scout-kit/fine-print/internal/settings"
	"github.com/scout-kit/fine-print/internal/storage"
)

// BroadcastFunc sends an admin-scoped SSE event. Passed as a closure so the
// api package doesn't need to import server (which would cycle).
type BroadcastFunc func(eventType string, data any)

// Handlers holds all API handler methods and their dependencies.
type Handlers struct {
	cfg            config.Config
	queries        *db.Queries
	store          storage.Store
	pipeline       *imaging.Pipeline
	queue          *printer.QueueManager
	printer        printer.Printer
	qr             *qrcode.Handler
	settings       *settings.Store
	diskGuard      *diskguard.Guard
	broadcastAdmin BroadcastFunc
}

func NewHandlers(
	cfg config.Config,
	queries *db.Queries,
	store storage.Store,
	pipeline *imaging.Pipeline,
	queue *printer.QueueManager,
	p printer.Printer,
	qr *qrcode.Handler,
	settingsStore *settings.Store,
	diskGuard *diskguard.Guard,
	broadcastAdmin BroadcastFunc,
) *Handlers {
	if broadcastAdmin == nil {
		broadcastAdmin = func(string, any) {}
	}
	return &Handlers{
		cfg:            cfg,
		queries:        queries,
		store:          store,
		pipeline:       pipeline,
		queue:          queue,
		printer:        p,
		qr:             qr,
		settings:       settingsStore,
		diskGuard:      diskGuard,
		broadcastAdmin: broadcastAdmin,
	}
}

// Health returns a simple health check response.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// parseID extracts and parses a uint64 path parameter.
func parseID(r *http.Request, param string) (uint64, error) {
	return strconv.ParseUint(r.PathValue(param), 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// Prevent nil slices from serializing as "null" — return "[]" instead
	if data != nil {
		v := reflect.ValueOf(data)
		if v.Kind() == reflect.Slice && v.IsNil() {
			w.Write([]byte("[]\n"))
			return
		}
	}

	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}
