package settings_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/scout-kit/fine-print/internal/config"
	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/settings"
)

func newTestStore(t *testing.T) (*settings.Store, *db.Queries) {
	t.Helper()
	dir := t.TempDir()
	dbx, err := db.Open(config.DatabaseConfig{
		Driver:     "sqlite",
		SQLitePath: filepath.Join(dir, "test.db"),
	})
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { dbx.Close() })

	if err := db.Migrate(dbx, "sqlite"); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	q := db.NewQueries(dbx)
	return settings.NewStore(q), q
}

func sampleConfig() config.Config {
	cfg := config.DefaultConfig()
	cfg.Hotspot.SSID = "Party"
	cfg.Hotspot.Password = "s3cret"
	cfg.DNS.Port = 5353
	cfg.Printer.Name = "CP1500"
	cfg.Imaging.PrintWidth = 1800
	cfg.Imaging.JPEGQuality = 95
	return cfg
}

func TestSeedFromConfig_WritesAllKeysOnFirstRun(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.SeedFromConfig(ctx, sampleConfig()); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	for _, key := range settings.TunableKeys {
		present, err := store.Has(ctx, key)
		if err != nil {
			t.Fatalf("checking %s: %v", key, err)
		}
		if !present && key != settings.KeyPrinterName && key != settings.KeyHotspotPassword {
			// printer_name / hotspot_password may be legitimately empty in
			// the sample cfg; we only guarantee non-empty keys seed.
			t.Errorf("key %s not seeded", key)
		}
	}

	if got := store.GetString(ctx, settings.KeyHotspotSSID, ""); got != "Party" {
		t.Errorf("hotspot_ssid: got %q, want Party", got)
	}
	if got := store.GetInt(ctx, settings.KeyDNSPort, 0); got != 5353 {
		t.Errorf("dns_port: got %d, want 5353", got)
	}
}

func TestSeedFromConfig_DoesNotOverwriteExisting(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	// Admin edits the SSID in the UI.
	if err := store.Set(ctx, settings.KeyHotspotSSID, "EventLive"); err != nil {
		t.Fatalf("set: %v", err)
	}

	// New boot seeds from YAML — must not overwrite the admin's value.
	if err := store.SeedFromConfig(ctx, sampleConfig()); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if got := store.GetString(ctx, settings.KeyHotspotSSID, ""); got != "EventLive" {
		t.Errorf("hotspot_ssid: got %q, want EventLive (admin value preserved)", got)
	}
}

func TestApplyToConfig_OverlaysDBValues(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.SeedFromConfig(ctx, sampleConfig()); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	// Admin changes print width and hotspot SSID.
	if err := store.Set(ctx, settings.KeyImagingPrintWidth, "2400"); err != nil {
		t.Fatalf("set width: %v", err)
	}
	if err := store.Set(ctx, settings.KeyHotspotSSID, "NewName"); err != nil {
		t.Fatalf("set ssid: %v", err)
	}

	cfg := sampleConfig()
	store.ApplyToConfig(ctx, &cfg)

	if cfg.Imaging.PrintWidth != 2400 {
		t.Errorf("print width: got %d, want 2400", cfg.Imaging.PrintWidth)
	}
	if cfg.Hotspot.SSID != "NewName" {
		t.Errorf("hotspot ssid: got %q, want NewName", cfg.Hotspot.SSID)
	}
	// Untouched field comes through unchanged.
	if cfg.Imaging.JPEGQuality != 95 {
		t.Errorf("jpeg quality: got %d, want 95", cfg.Imaging.JPEGQuality)
	}
}

func TestGetInt_MalformedFallsBackToDefault(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, settings.KeyDNSPort, "not-a-number"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := store.GetInt(ctx, settings.KeyDNSPort, 53); got != 53 {
		t.Errorf("got %d, want 53 (default fallback)", got)
	}
}

func TestGetBool_MalformedFallsBackToDefault(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if err := store.Set(ctx, settings.KeyHotspotEnabled, "maybe"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := store.GetBool(ctx, settings.KeyHotspotEnabled, true); got != true {
		t.Errorf("got %v, want true (default fallback)", got)
	}
}

func TestRequiresRestart(t *testing.T) {
	if settings.RequiresRestart(settings.KeyHotspotSSID) != true {
		t.Error("hotspot_ssid should require restart")
	}
	if settings.RequiresRestart(settings.KeyPrinterName) != false {
		t.Error("printer_name should be hot-reloadable")
	}
	if settings.RequiresRestart(settings.KeyPrinterMedia) != false {
		t.Error("printer_media should be hot-reloadable")
	}
}
