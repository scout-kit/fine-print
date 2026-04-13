//go:build linux

package hotspot

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"text/template"
)

// LinuxManager manages the WiFi hotspot on Linux using hostapd and dnsmasq.
type LinuxManager struct {
	cfg         Config
	hostapdCmd  *exec.Cmd
	dnsmasqCmd  *exec.Cmd
}

func NewManager() Manager {
	return &LinuxManager{}
}

const hostapdConfTemplate = `interface={{.Interface}}
driver=nl80211
ssid={{.SSID}}
hw_mode=g
channel={{.Channel}}
wmm_enabled=0
macaddr_acl=0
auth_algs=1
ignore_broadcast_ssid=0
{{if .Password}}wpa=2
wpa_passphrase={{.Password}}
wpa_key_mgmt=WPA-PSK
wpa_pairwise=TKIP
rsn_pairwise=CCMP{{end}}
`

const dnsmasqConfTemplate = `interface={{.Interface}}
dhcp-range={{.DHCPStart}},{{.DHCPEnd}},255.255.255.0,24h
address=/#/{{.Gateway}}
`

type dnsmasqData struct {
	Interface string
	DHCPStart string
	DHCPEnd   string
	Gateway   string
}

func (m *LinuxManager) Start(cfg Config) error {
	m.cfg = cfg

	if cfg.Channel == 0 {
		cfg.Channel = 6
	}

	// Write hostapd config
	hostapdConf := "/tmp/fine-print-hostapd.conf"
	if err := writeTemplate(hostapdConf, hostapdConfTemplate, cfg); err != nil {
		return fmt.Errorf("writing hostapd config: %w", err)
	}

	// Write dnsmasq config
	dnsmasqConf := "/tmp/fine-print-dnsmasq.conf"
	dnsData := dnsmasqData{
		Interface: cfg.Interface,
		DHCPStart: "192.168.69.10",
		DHCPEnd:   "192.168.69.250",
		Gateway:   cfg.Gateway,
	}
	if err := writeTemplate(dnsmasqConf, dnsmasqConfTemplate, dnsData); err != nil {
		return fmt.Errorf("writing dnsmasq config: %w", err)
	}

	// Set interface to static IP
	if err := exec.Command("ip", "addr", "flush", "dev", cfg.Interface).Run(); err != nil {
		log.Printf("Warning: failed to flush interface: %v", err)
	}
	if err := exec.Command("ip", "addr", "add", cfg.Gateway+"/24", "dev", cfg.Interface).Run(); err != nil {
		return fmt.Errorf("setting interface IP: %w", err)
	}
	if err := exec.Command("ip", "link", "set", cfg.Interface, "up").Run(); err != nil {
		return fmt.Errorf("bringing up interface: %w", err)
	}

	// Start hostapd
	m.hostapdCmd = exec.Command("hostapd", hostapdConf)
	if err := m.hostapdCmd.Start(); err != nil {
		return fmt.Errorf("starting hostapd: %w", err)
	}

	// Start dnsmasq
	m.dnsmasqCmd = exec.Command("dnsmasq", "-C", dnsmasqConf, "--no-daemon")
	if err := m.dnsmasqCmd.Start(); err != nil {
		m.hostapdCmd.Process.Kill()
		return fmt.Errorf("starting dnsmasq: %w", err)
	}

	log.Printf("Hotspot: started on %s (SSID: %s, Gateway: %s)", cfg.Interface, cfg.SSID, cfg.Gateway)
	return nil
}

func (m *LinuxManager) Stop() error {
	if m.hostapdCmd != nil && m.hostapdCmd.Process != nil {
		m.hostapdCmd.Process.Kill()
	}
	if m.dnsmasqCmd != nil && m.dnsmasqCmd.Process != nil {
		m.dnsmasqCmd.Process.Kill()
	}
	return nil
}

func (m *LinuxManager) Status() (Status, error) {
	active := m.hostapdCmd != nil && m.hostapdCmd.Process != nil
	return Status{
		Active:    active,
		SSID:      m.cfg.SSID,
		Interface: m.cfg.Interface,
		Gateway:   m.cfg.Gateway,
	}, nil
}

func writeTemplate(path, tmpl string, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return err
	}
	return t.Execute(f, data)
}
