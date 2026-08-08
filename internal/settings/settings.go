// Package settings provides typed, DB-backed access to runtime-tunable
// configuration. YAML values seed the DB on first boot; subsequent reads
// come from the DB so admins can change settings without redeploying.
package settings

import (
	"context"
	"fmt"
	"strconv"

	"github.com/scout-kit/fine-print/internal/config"
	"github.com/scout-kit/fine-print/internal/db"
)

// Canonical setting keys. Names match the pre-existing convention (flat
// snake_case) so the printer queue and admin login continue to work
// unchanged against the settings table.
const (
	KeyAdminPasswordHash = "admin_password_hash"

	KeyHotspotEnabled   = "hotspot_enabled"
	KeyHotspotSSID      = "hotspot_ssid"
	KeyHotspotPassword  = "hotspot_password"
	KeyHotspotInterface = "hotspot_interface"
	KeyHotspotSubnet    = "hotspot_subnet"
	KeyHotspotGateway   = "gateway_ip" // pre-existing, used by UI

	KeyDNSEnabled = "dns_enabled"
	KeyDNSPort    = "dns_port"

	KeyPrinterName      = "printer_name"
	KeyPrinterMedia     = "printer_media"
	KeyPrinterAutoQueue = "printer_auto_queue"

	KeyImagingMaxUpload    = "imaging_max_upload_pixels"
	KeyImagingPreviewWidth = "imaging_preview_max_width"
	KeyImagingPrintWidth   = "imaging_print_width"
	KeyImagingPrintHeight  = "imaging_print_height"
	KeyImagingJPEGQuality  = "imaging_jpeg_quality"
)

// TunableKeys is the canonical set of keys exposed through the admin API.
// Order is UI-display order.
var TunableKeys = []string{
	KeyHotspotEnabled, KeyHotspotSSID, KeyHotspotPassword,
	KeyHotspotInterface, KeyHotspotSubnet, KeyHotspotGateway,
	KeyDNSEnabled, KeyDNSPort,
	KeyPrinterName, KeyPrinterMedia, KeyPrinterAutoQueue,
	KeyImagingMaxUpload, KeyImagingPreviewWidth,
	KeyImagingPrintWidth, KeyImagingPrintHeight, KeyImagingJPEGQuality,
}

// HotReloadKeys are read per-request/per-job rather than at boot, so
// changes take effect without a restart.
var HotReloadKeys = map[string]bool{
	KeyPrinterName:      true,
	KeyPrinterMedia:     true,
	KeyPrinterAutoQueue: true,
}

// RequiresRestart reports whether changing the given key takes effect only
// after the service is restarted.
func RequiresRestart(key string) bool {
	return !HotReloadKeys[key]
}

type Store struct {
	q *db.Queries
}

func NewStore(q *db.Queries) *Store {
	return &Store{q: q}
}

func (s *Store) GetString(ctx context.Context, key, def string) string {
	v, err := s.q.GetSetting(ctx, key)
	if err != nil || v == "" {
		return def
	}
	return v
}

func (s *Store) GetInt(ctx context.Context, key string, def int) int {
	v, err := s.q.GetSetting(ctx, key)
	if err != nil || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func (s *Store) GetBool(ctx context.Context, key string, def bool) bool {
	v, err := s.q.GetSetting(ctx, key)
	if err != nil || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func (s *Store) Set(ctx context.Context, key, value string) error {
	return s.q.SetSetting(ctx, key, value)
}

// Has reports whether a key has been explicitly set in the DB.
func (s *Store) Has(ctx context.Context, key string) (bool, error) {
	v, err := s.q.GetSetting(ctx, key)
	if err != nil {
		return false, err
	}
	return v != "", nil
}

// SeedFromConfig writes YAML-derived values into the DB for any tunable key
// that is not yet present. Runs once per boot; never overwrites existing DB values.
func (s *Store) SeedFromConfig(ctx context.Context, cfg config.Config) error {
	seeds := map[string]string{
		KeyHotspotEnabled:   strconv.FormatBool(cfg.Hotspot.Enabled),
		KeyHotspotSSID:      cfg.Hotspot.SSID,
		KeyHotspotPassword:  cfg.Hotspot.Password,
		KeyHotspotInterface: cfg.Hotspot.Interface,
		KeyHotspotSubnet:    cfg.Hotspot.Subnet,
		KeyHotspotGateway:   cfg.Hotspot.Gateway,

		KeyDNSEnabled: strconv.FormatBool(cfg.DNS.Enabled),
		KeyDNSPort:    strconv.Itoa(cfg.DNS.Port),

		KeyPrinterName:      cfg.Printer.Name,
		KeyPrinterMedia:     cfg.Printer.Media,
		KeyPrinterAutoQueue: strconv.FormatBool(cfg.Printer.AutoQueue),

		KeyImagingMaxUpload:    strconv.Itoa(cfg.Imaging.MaxUploadPixels),
		KeyImagingPreviewWidth: strconv.Itoa(cfg.Imaging.PreviewMaxWidth),
		KeyImagingPrintWidth:   strconv.Itoa(cfg.Imaging.PrintWidth),
		KeyImagingPrintHeight:  strconv.Itoa(cfg.Imaging.PrintHeight),
		KeyImagingJPEGQuality:  strconv.Itoa(cfg.Imaging.JPEGQuality),
	}
	for key, value := range seeds {
		present, err := s.Has(ctx, key)
		if err != nil {
			return fmt.Errorf("checking seed for %s: %w", key, err)
		}
		if present {
			continue
		}
		if err := s.Set(ctx, key, value); err != nil {
			return fmt.Errorf("seeding %s: %w", key, err)
		}
	}
	return nil
}

// ApplyToConfig overlays current DB values onto cfg for tunable sections.
// Missing or malformed values fall back to the existing cfg value.
func (s *Store) ApplyToConfig(ctx context.Context, cfg *config.Config) {
	cfg.Hotspot.Enabled = s.GetBool(ctx, KeyHotspotEnabled, cfg.Hotspot.Enabled)
	cfg.Hotspot.SSID = s.GetString(ctx, KeyHotspotSSID, cfg.Hotspot.SSID)
	cfg.Hotspot.Password = s.GetString(ctx, KeyHotspotPassword, cfg.Hotspot.Password)
	cfg.Hotspot.Interface = s.GetString(ctx, KeyHotspotInterface, cfg.Hotspot.Interface)
	cfg.Hotspot.Subnet = s.GetString(ctx, KeyHotspotSubnet, cfg.Hotspot.Subnet)
	cfg.Hotspot.Gateway = s.GetString(ctx, KeyHotspotGateway, cfg.Hotspot.Gateway)

	cfg.DNS.Enabled = s.GetBool(ctx, KeyDNSEnabled, cfg.DNS.Enabled)
	cfg.DNS.Port = s.GetInt(ctx, KeyDNSPort, cfg.DNS.Port)

	cfg.Printer.Name = s.GetString(ctx, KeyPrinterName, cfg.Printer.Name)
	cfg.Printer.Media = s.GetString(ctx, KeyPrinterMedia, cfg.Printer.Media)
	cfg.Printer.AutoQueue = s.GetBool(ctx, KeyPrinterAutoQueue, cfg.Printer.AutoQueue)

	cfg.Imaging.MaxUploadPixels = s.GetInt(ctx, KeyImagingMaxUpload, cfg.Imaging.MaxUploadPixels)
	cfg.Imaging.PreviewMaxWidth = s.GetInt(ctx, KeyImagingPreviewWidth, cfg.Imaging.PreviewMaxWidth)
	cfg.Imaging.PrintWidth = s.GetInt(ctx, KeyImagingPrintWidth, cfg.Imaging.PrintWidth)
	cfg.Imaging.PrintHeight = s.GetInt(ctx, KeyImagingPrintHeight, cfg.Imaging.PrintHeight)
	cfg.Imaging.JPEGQuality = s.GetInt(ctx, KeyImagingJPEGQuality, cfg.Imaging.JPEGQuality)
}
