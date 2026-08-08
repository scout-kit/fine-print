package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/scout-kit/fine-print/internal/api"
	"github.com/scout-kit/fine-print/internal/config"
	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/printer"
	"github.com/scout-kit/fine-print/internal/settings"
	"golang.org/x/crypto/bcrypt"
)

type stubPrinter struct {
	printers []printer.PrinterInfo
}

func (s *stubPrinter) ListPrinters() ([]printer.PrinterInfo, error) { return s.printers, nil }
func (s *stubPrinter) Print(string, string, printer.PrintOptions) (string, error) {
	return "", nil
}
func (s *stubPrinter) JobStatus(string) (string, error) { return "", nil }
func (s *stubPrinter) CancelJob(string) error           { return nil }

func newTestHandlers(t *testing.T) (*api.Handlers, *db.Queries) {
	t.Helper()
	dbx, err := db.Open(config.DatabaseConfig{
		Driver:     "sqlite",
		SQLitePath: filepath.Join(t.TempDir(), "test.db"),
	})
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { dbx.Close() })
	if err := db.Migrate(dbx, "sqlite"); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	q := db.NewQueries(dbx)
	store := settings.NewStore(q)

	h := api.NewHandlers(
		config.DefaultConfig(),
		q,
		nil, // storage unused
		nil, // pipeline unused
		nil, // queue unused
		&stubPrinter{printers: []printer.PrinterInfo{{Name: "Selphy_CP1500", State: "idle", AcceptJobs: true}}},
		nil, // qr unused
		store,
		nil, // diskguard unused
		nil, // broadcast unused
	)
	return h, q
}

func doJSON(t *testing.T, h http.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encoding body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestSetupStatus_NeedsSetupOnFreshDB(t *testing.T) {
	h, _ := newTestHandlers(t)

	rec := doJSON(t, h.SetupStatus, "GET", "/api/setup/status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		NeedsSetup bool                  `json:"needs_setup"`
		Printers   []printer.PrinterInfo `json:"printers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !resp.NeedsSetup {
		t.Error("fresh DB should need setup")
	}
	if len(resp.Printers) == 0 {
		t.Error("should return detected printers")
	}
}

func TestCompleteSetup_WritesBcryptHashAndSettings(t *testing.T) {
	h, q := newTestHandlers(t)

	rec := doJSON(t, h.CompleteSetup, "POST", "/api/setup/complete", map[string]string{
		"admin_password":   "hunter2",
		"hotspot_ssid":     "EventWiFi",
		"hotspot_password": "",
		"printer_name":     "Selphy_CP1500",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Password is bcrypted, not stored plaintext.
	hash, _ := q.GetSetting(context.Background(),settings.KeyAdminPasswordHash)
	if hash == "hunter2" || hash == "" {
		t.Fatalf("admin password not hashed: got %q", hash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("hunter2")); err != nil {
		t.Errorf("bcrypt mismatch: %v", err)
	}

	// Non-empty settings persisted.
	if ssid, _ := q.GetSetting(context.Background(),settings.KeyHotspotSSID); ssid != "EventWiFi" {
		t.Errorf("hotspot_ssid: got %q, want EventWiFi", ssid)
	}
	if name, _ := q.GetSetting(context.Background(),settings.KeyPrinterName); name != "Selphy_CP1500" {
		t.Errorf("printer_name: got %q, want Selphy_CP1500", name)
	}

	// Status should now report setup done.
	rec2 := doJSON(t, h.SetupStatus, "GET", "/api/setup/status", nil)
	var resp struct {
		NeedsSetup bool `json:"needs_setup"`
	}
	json.Unmarshal(rec2.Body.Bytes(), &resp)
	if resp.NeedsSetup {
		t.Error("setup should be complete after submission")
	}
}

func TestCompleteSetup_RejectsSecondRun(t *testing.T) {
	h, _ := newTestHandlers(t)

	// First run succeeds.
	rec1 := doJSON(t, h.CompleteSetup, "POST", "/api/setup/complete", map[string]string{
		"admin_password": "first-pass",
	})
	if rec1.Code != http.StatusOK {
		t.Fatalf("first run: %d: %s", rec1.Code, rec1.Body.String())
	}

	// Second run must be refused — otherwise anyone on the LAN can reset
	// the password before the admin notices.
	rec2 := doJSON(t, h.CompleteSetup, "POST", "/api/setup/complete", map[string]string{
		"admin_password": "attacker-pass",
	})
	if rec2.Code != http.StatusForbidden {
		t.Errorf("expected 403 on repeat setup, got %d", rec2.Code)
	}
}

func TestCompleteSetup_RejectsShortPassword(t *testing.T) {
	h, _ := newTestHandlers(t)

	rec := doJSON(t, h.CompleteSetup, "POST", "/api/setup/complete", map[string]string{
		"admin_password": "ab",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}
