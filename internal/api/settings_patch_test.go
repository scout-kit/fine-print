package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/scout-kit/fine-print/internal/settings"
)

// A PATCH containing any invalid value must apply none of it. Previously
// validation and writes shared one pass over the request map, so valid keys
// ahead of the bad one were already committed when the 400 was returned —
// and Go's randomized map order made the surviving set differ per request.
func TestUpdateSettings_RejectsWholeRequestOnInvalidValue(t *testing.T) {
	h, q := newTestHandlers(t)

	// hotspot_ssid sorts before printer_media, so under the old
	// write-as-you-go loop it stood a good chance of landing before the
	// invalid key aborted the request.
	rec := doJSON(t, h.UpdateSettings, "PUT", "/api/admin/settings", map[string]string{
		settings.KeyHotspotSSID:  "Should Not Persist",
		settings.KeyPrinterMedia: "A4", // invalid — only 4x6 / Postcard
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}

	for _, key := range []string{settings.KeyHotspotSSID, settings.KeyPrinterMedia} {
		got, _ := q.GetSetting(context.Background(), key)
		if got != "" {
			t.Errorf("%s = %q after a rejected PATCH, want it unwritten", key, got)
		}
	}
}

// An unknown key is likewise all-or-nothing.
func TestUpdateSettings_RejectsWholeRequestOnUnknownKey(t *testing.T) {
	h, q := newTestHandlers(t)

	rec := doJSON(t, h.UpdateSettings, "PUT", "/api/admin/settings", map[string]string{
		settings.KeyHotspotSSID: "Should Not Persist",
		"definitely_not_a_key":  "whatever",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	if got, _ := q.GetSetting(context.Background(), settings.KeyHotspotSSID); got != "" {
		t.Errorf("%s = %q after a rejected PATCH, want it unwritten", settings.KeyHotspotSSID, got)
	}
}

// The happy path still writes every key and reports them in sorted order.
func TestUpdateSettings_AppliesAllValidKeys(t *testing.T) {
	h, q := newTestHandlers(t)

	want := map[string]string{
		settings.KeyHotspotSSID:     "Fine Print",
		settings.KeyHotspotPassword: "hunter2hunter2",
		settings.KeyPrinterMedia:    "4x6",
	}
	rec := doJSON(t, h.UpdateSettings, "PUT", "/api/admin/settings", want)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	for key, value := range want {
		got, _ := q.GetSetting(context.Background(), key)
		if got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}

	var resp struct {
		Changed         []string `json:"changed"`
		RequiresRestart bool     `json:"requires_restart"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	// Sorted, so the list is stable across identical requests.
	wantOrder := []string{
		settings.KeyHotspotPassword,
		settings.KeyHotspotSSID,
		settings.KeyPrinterMedia,
	}
	if len(resp.Changed) != len(wantOrder) {
		t.Fatalf("changed = %v, want %v", resp.Changed, wantOrder)
	}
	for i, key := range wantOrder {
		if resp.Changed[i] != key {
			t.Errorf("changed[%d] = %q, want %q (full: %v)", i, resp.Changed[i], key, resp.Changed)
		}
	}
	// Hotspot keys are restart-gated, so this must be reported.
	if !resp.RequiresRestart {
		t.Error("requires_restart = false, want true (hotspot keys were changed)")
	}
}
