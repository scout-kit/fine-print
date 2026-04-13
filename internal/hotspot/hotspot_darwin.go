//go:build darwin

package hotspot

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
)

// DarwinManager manages the WiFi hotspot on macOS.
// macOS does not expose a clean programmatic API for hotspot creation.
// This implementation detects if Internet Sharing is active and configures
// the DNS hijack accordingly. If not active, it returns ErrManualSetupRequired.
type DarwinManager struct {
	cfg Config
}

func NewManager() Manager {
	return &DarwinManager{}
}

func (m *DarwinManager) Start(cfg Config) error {
	m.cfg = cfg

	// Check if Internet Sharing is already active by looking for bridge interfaces
	status, err := m.Status()
	if err != nil {
		return fmt.Errorf("checking hotspot status: %w", err)
	}

	if status.Active {
		log.Printf("Hotspot: Internet Sharing detected on %s (gateway: %s)",
			status.Interface, status.Gateway)
		return nil
	}

	// Internet Sharing is not active — instruct user to enable it
	return &ErrManualSetupRequired{
		Message: fmt.Sprintf(
			"Internet Sharing is not active. Please enable it:\n"+
				"  System Settings > General > Sharing > Internet Sharing\n"+
				"  Share from: [any connection]\n"+
				"  To: Wi-Fi\n"+
				"  Wi-Fi Options: SSID=%q, Password=%q\n"+
				"Then restart the application.",
			cfg.SSID, cfg.Password),
	}
}

func (m *DarwinManager) Stop() error {
	// We don't manage Internet Sharing lifecycle — user controls it
	return nil
}

func (m *DarwinManager) Status() (Status, error) {
	// Look for bridge interfaces that Internet Sharing creates
	ifaces, err := net.Interfaces()
	if err != nil {
		return Status{}, err
	}

	for _, iface := range ifaces {
		name := iface.Name
		// Internet Sharing typically creates bridge100, bridge101, etc.
		if !strings.HasPrefix(name, "bridge") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}

		// Found a bridge interface with an address — Internet Sharing is likely active
		gateway := ""
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				gateway = ipnet.IP.String()
				break
			}
		}

		if gateway != "" {
			return Status{
				Active:    true,
				Interface: name,
				Gateway:   gateway,
			}, nil
		}
	}

	return Status{Active: false}, nil
}

// DetectGatewayIP tries to find the gateway IP used by Internet Sharing.
func DetectGatewayIP() (string, error) {
	out, err := exec.Command("ifconfig", "bridge100").Output()
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1], nil
			}
		}
	}

	return "", fmt.Errorf("no gateway IP found on bridge100")
}
