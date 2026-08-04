package api

import (
	"strings"
	"testing"

	"github.com/scout-kit/fine-print/internal/settings"
)

// The hotspot keys are interpolated into generated hostapd/dnsmasq config,
// so these cases cover both "hostapd would refuse to start" values and
// newline injection into the generated files.
func TestValidateSettingValue_HotspotKeys(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		// SSID
		{"ssid ok", settings.KeyHotspotSSID, "Fine Print", false},
		{"ssid empty", settings.KeyHotspotSSID, "", true},
		{"ssid too long", settings.KeyHotspotSSID, strings.Repeat("a", 33), true},
		{"ssid max length", settings.KeyHotspotSSID, strings.Repeat("a", 32), false},
		{"ssid newline injection", settings.KeyHotspotSSID, "Party\nwpa=0", true},
		{"ssid carriage return", settings.KeyHotspotSSID, "Party\rwpa=0", true},
		{"ssid null byte", settings.KeyHotspotSSID, "Party\x00", true},

		// Password: empty (open network) or WPA2's 8-63 range.
		{"password empty is open network", settings.KeyHotspotPassword, "", false},
		{"password ok", settings.KeyHotspotPassword, "hunter2hunter2", false},
		{"password too short for wpa2", settings.KeyHotspotPassword, "short", true},
		{"password min length", settings.KeyHotspotPassword, "12345678", false},
		{"password max length", settings.KeyHotspotPassword, strings.Repeat("x", 63), false},
		{"password too long for wpa2", settings.KeyHotspotPassword, strings.Repeat("x", 64), true},
		{"password newline injection", settings.KeyHotspotPassword, "12345678\nssid=evil", true},

		// Interface
		{"interface ok linux", settings.KeyHotspotInterface, "wlan0", false},
		{"interface ok macos", settings.KeyHotspotInterface, "en0", false},
		{"interface ok vlan", settings.KeyHotspotInterface, "eth0.100", false},
		{"interface empty", settings.KeyHotspotInterface, "", true},
		{"interface too long", settings.KeyHotspotInterface, strings.Repeat("w", 16), true},
		{"interface shell metachars", settings.KeyHotspotInterface, "wlan0; rm -rf /", true},
		{"interface space", settings.KeyHotspotInterface, "wlan0 up", true},
		{"interface newline", settings.KeyHotspotInterface, "wlan0\ndriver=none", true},

		// Gateway must be a usable IPv4 literal — it feeds `ip addr add`
		// and dnsmasq's address=/#/ directive.
		{"gateway ok", settings.KeyHotspotGateway, "192.168.69.1", false},
		{"gateway empty", settings.KeyHotspotGateway, "", true},
		{"gateway not an ip", settings.KeyHotspotGateway, "not-an-ip", true},
		{"gateway ipv6 rejected", settings.KeyHotspotGateway, "fe80::1", true},
		{"gateway cidr rejected", settings.KeyHotspotGateway, "192.168.69.1/24", true},

		// Subnet
		{"subnet ok", settings.KeyHotspotSubnet, "192.168.69.0/24", false},
		{"subnet empty", settings.KeyHotspotSubnet, "", true},
		{"subnet missing mask", settings.KeyHotspotSubnet, "192.168.69.0", true},
		{"subnet garbage", settings.KeyHotspotSubnet, "192.168.69.0/99", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSettingValue(tc.key, tc.value)
			if tc.wantErr && err == nil {
				t.Fatalf("validateSettingValue(%q, %q) = nil, want error", tc.key, tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateSettingValue(%q, %q) = %v, want nil", tc.key, tc.value, err)
			}
		})
	}
}

// Guard against a future key being added to TunableKeys and silently
// bypassing validation of the values that reach generated config files.
func TestValidateSettingValue_AllHotspotKeysAreValidated(t *testing.T) {
	// A value that is invalid for every hotspot key: contains a newline,
	// is not an IP, is not a CIDR, and is not a legal interface name.
	const bogus = "bogus value\nwith newline"

	hotspotKeys := []string{
		settings.KeyHotspotSSID,
		settings.KeyHotspotPassword,
		settings.KeyHotspotInterface,
		settings.KeyHotspotGateway,
		settings.KeyHotspotSubnet,
	}
	for _, key := range hotspotKeys {
		if err := validateSettingValue(key, bogus); err == nil {
			t.Errorf("key %q accepted a value containing a newline — it would be written into generated hostapd/dnsmasq config", key)
		}
	}
}

// The pre-existing typed keys should keep behaving as before.
func TestValidateSettingValue_ExistingKeysUnchanged(t *testing.T) {
	tests := []struct {
		key     string
		value   string
		wantErr bool
	}{
		{settings.KeyHotspotEnabled, "true", false},
		{settings.KeyHotspotEnabled, "yes", true},
		{settings.KeyDNSPort, "53", false},
		{settings.KeyDNSPort, "0", true},
		{settings.KeyImagingJPEGQuality, "90", false},
		{settings.KeyImagingJPEGQuality, "101", true},
		{settings.KeyPrinterMedia, "4x6", false},
		{settings.KeyPrinterMedia, "A4", true},
		{settings.KeyPrinterMonitorIntervalSecs, "30", false},
		{settings.KeyPrinterMonitorIntervalSecs, "1", true},
		// Unconstrained key — any value is accepted.
		{settings.KeyPrinterName, "Canon_SELPHY_CP1500", false},
	}
	for _, tc := range tests {
		err := validateSettingValue(tc.key, tc.value)
		if tc.wantErr && err == nil {
			t.Errorf("validateSettingValue(%q, %q) = nil, want error", tc.key, tc.value)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateSettingValue(%q, %q) = %v, want nil", tc.key, tc.value, err)
		}
	}
}
