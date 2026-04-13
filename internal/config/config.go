package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DataDir  string         `yaml:"data_dir"`
	Database DatabaseConfig `yaml:"database"`
	Hotspot  HotspotConfig  `yaml:"hotspot"`
	DNS      DNSConfig      `yaml:"dns"`
	Printer  PrinterConfig  `yaml:"printer"`
	Admin    AdminConfig    `yaml:"admin"`
	Imaging  ImagingConfig  `yaml:"imaging"`
	TLS      TLSConfig      `yaml:"tls"`
	Dev      DevConfig      `yaml:"dev"`
}

type TLSConfig struct {
	Enabled bool `yaml:"enabled"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type DatabaseConfig struct {
	Driver     string `yaml:"driver"`
	SQLitePath string `yaml:"sqlite_path"`
	MySQLDSN   string `yaml:"mysql_dsn"`
}

type HotspotConfig struct {
	Enabled   bool   `yaml:"enabled"`
	SSID      string `yaml:"ssid"`
	Password  string `yaml:"password"`
	Interface string `yaml:"interface"`
	Subnet    string `yaml:"subnet"`
	Gateway   string `yaml:"gateway"`
}

type DNSConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

type PrinterConfig struct {
	Name      string `yaml:"name"`
	Media     string `yaml:"media"`
	AutoQueue bool   `yaml:"auto_queue"`
}

type AdminConfig struct {
	Password string `yaml:"password"`
}

type ImagingConfig struct {
	MaxUploadPixels int `yaml:"max_upload_pixels"`
	PreviewMaxWidth int `yaml:"preview_max_width"`
	PrintWidth      int `yaml:"print_width"`
	PrintHeight     int `yaml:"print_height"`
	JPEGQuality     int `yaml:"jpeg_quality"`
}

type DevConfig struct {
	Mode          bool   `yaml:"mode"`
	FrontendProxy string `yaml:"frontend_proxy"`
}

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Port: 80,
			Host: "0.0.0.0",
		},
		DataDir: "./data",
		Database: DatabaseConfig{
			Driver:     "sqlite",
			SQLitePath: "data/fine-print.db",
		},
		Hotspot: HotspotConfig{
			Enabled:   true,
			SSID:      "Fine Print",
			Interface: "en0",
			Subnet:    "192.168.69.0/24",
			Gateway:   "192.168.69.1",
		},
		DNS: DNSConfig{
			Enabled: true,
			Port:    53,
		},
		Printer: PrinterConfig{
			Media:     "4x6",
			AutoQueue: true,
		},
		Admin: AdminConfig{
			Password: "changeme",
		},
		Imaging: ImagingConfig{
			MaxUploadPixels: 6000,
			PreviewMaxWidth: 1200,
			PrintWidth:      1800,
			PrintHeight:     1200,
			JPEGQuality:     95,
		},
		TLS: TLSConfig{
			Enabled: true,
		},
		Dev: DevConfig{
			Mode: false,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("reading config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parsing config file: %w", err)
		}
	}

	applyEnvOverrides(&cfg)

	if err := validate(cfg); err != nil {
		return cfg, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("FINEPRINT_DEV"); v == "1" || strings.EqualFold(v, "true") {
		cfg.Dev.Mode = true
		cfg.Hotspot.Enabled = false
		cfg.DNS.Enabled = false
	}
	if v := os.Getenv("FINEPRINT_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("FINEPRINT_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("FINEPRINT_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("FINEPRINT_DB_DRIVER"); v != "" {
		cfg.Database.Driver = v
	}
	if v := os.Getenv("FINEPRINT_DB_SQLITE_PATH"); v != "" {
		cfg.Database.SQLitePath = v
	}
	if v := os.Getenv("FINEPRINT_DB_MYSQL_DSN"); v != "" {
		cfg.Database.MySQLDSN = v
	}
	if v := os.Getenv("FINEPRINT_ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	if v := os.Getenv("FINEPRINT_FRONTEND_PROXY"); v != "" {
		cfg.Dev.FrontendProxy = v
	}
	if v := os.Getenv("FINEPRINT_HOTSPOT_SSID"); v != "" {
		cfg.Hotspot.SSID = v
	}
	if v := os.Getenv("FINEPRINT_HOTSPOT_PASSWORD"); v != "" {
		cfg.Hotspot.Password = v
	}
	if v := os.Getenv("FINEPRINT_PRINTER_NAME"); v != "" {
		cfg.Printer.Name = v
	}
	if v := os.Getenv("FINEPRINT_TLS"); v == "1" || strings.EqualFold(v, "true") {
		cfg.TLS.Enabled = true
	}
}

func validate(cfg Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", cfg.Server.Port)
	}
	switch cfg.Database.Driver {
	case "sqlite", "mysql":
	default:
		return fmt.Errorf("database.driver must be 'sqlite' or 'mysql', got %q", cfg.Database.Driver)
	}
	if cfg.Database.Driver == "sqlite" && cfg.Database.SQLitePath == "" {
		return fmt.Errorf("database.sqlite_path is required when driver is 'sqlite'")
	}
	if cfg.Database.Driver == "mysql" && cfg.Database.MySQLDSN == "" {
		return fmt.Errorf("database.mysql_dsn is required when driver is 'mysql'")
	}
	if cfg.Imaging.PrintWidth <= 0 || cfg.Imaging.PrintHeight <= 0 {
		return fmt.Errorf("imaging.print_width and print_height must be positive")
	}
	if cfg.Imaging.JPEGQuality < 1 || cfg.Imaging.JPEGQuality > 100 {
		return fmt.Errorf("imaging.jpeg_quality must be between 1 and 100")
	}
	return nil
}
