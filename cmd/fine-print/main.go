package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scout-kit/fine-print/internal/api"
	"github.com/scout-kit/fine-print/internal/captive"
	"github.com/scout-kit/fine-print/internal/config"
	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/frontend"
	"github.com/scout-kit/fine-print/internal/hotspot"
	"github.com/scout-kit/fine-print/internal/imaging"
	"github.com/scout-kit/fine-print/internal/printer"
	"github.com/scout-kit/fine-print/internal/qrcode"
	"github.com/scout-kit/fine-print/internal/server"
	"github.com/scout-kit/fine-print/internal/storage"
)

func main() {
	var (
		configPath = flag.String("config", "", "Path to config file")
		devMode    = flag.Bool("dev", false, "Enable development mode")
		port       = flag.Int("port", 0, "Override server port")
	)
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Apply flag overrides
	if *devMode {
		cfg.Dev.Mode = true
		cfg.Hotspot.Enabled = false
		cfg.DNS.Enabled = false
	}
	if *port > 0 {
		cfg.Server.Port = *port
	}

	// Initialize data directory
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialize storage
	store, err := storage.NewDiskStore(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Initialize database
	database, err := db.Open(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Run migrations
	if err := db.Migrate(database, cfg.Database.Driver); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	queries := db.NewQueries(database)

	// Initialize imaging pipeline
	pipeline := imaging.NewPipeline(
		cfg.Imaging.PrintWidth,
		cfg.Imaging.PrintHeight,
		cfg.Imaging.PreviewMaxWidth,
		cfg.Imaging.JPEGQuality,
		cfg.Imaging.MaxUploadPixels,
	)

	// Initialize printer
	cupsPrinter := printer.NewCUPSPrinter()

	// Initialize SSE hub
	sseHub := server.NewSSEHub()

	// Initialize print queue manager
	broadcastFn := func(eventType string, data any) {
		sseHub.Broadcast(server.NewEvent(eventType, data))
	}
	queueMgr := printer.NewQueueManager(queries, cupsPrinter, broadcastFn)

	// Initialize QR code handler
	qrHandler := qrcode.NewHandler(cfg.Hotspot.Gateway, cfg.Server.Port)

	// Initialize API handlers
	handlers := api.NewHandlers(cfg, queries, store, pipeline, queueMgr, cupsPrinter, qrHandler)

	// Get frontend filesystem
	frontendFSys, err := frontend.FS()
	if err != nil {
		log.Printf("Frontend not available: %v (API-only mode)", err)
		frontendFSys = nil
	}

	// Initialize HTTP server
	srv := server.New(cfg, handlers, queries, sseHub, frontendFSys)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start hotspot (if enabled)
	if cfg.Hotspot.Enabled && !cfg.Dev.Mode {
		hotspotMgr := hotspot.NewManager()
		hotspotCfg := hotspot.Config{
			SSID:      cfg.Hotspot.SSID,
			Password:  cfg.Hotspot.Password,
			Interface: cfg.Hotspot.Interface,
			Subnet:    cfg.Hotspot.Subnet,
			Gateway:   cfg.Hotspot.Gateway,
		}
		if err := hotspotMgr.Start(hotspotCfg); err != nil {
			if _, ok := err.(*hotspot.ErrManualSetupRequired); ok {
				log.Printf("WARNING: %v", err)
				log.Println("Continuing without hotspot management...")
			} else {
				log.Fatalf("Failed to start hotspot: %v", err)
			}
		}
		defer hotspotMgr.Stop()
	}

	// Start DNS server (if enabled)
	if cfg.DNS.Enabled && !cfg.Dev.Mode {
		dnsServer, err := captive.NewDNSServer(cfg.Hotspot.Gateway, cfg.DNS.Port)
		if err != nil {
			log.Printf("WARNING: Failed to create DNS server: %v", err)
		} else {
			go func() {
				if err := dnsServer.Start(); err != nil {
					log.Printf("DNS server error: %v", err)
				}
			}()
			defer dnsServer.Stop()
		}
	}

	// Start print queue manager
	go queueMgr.Run(ctx)

	// Start HTTP/HTTPS server
	httpServer := &http.Server{
		Addr:         srv.ListenAddr(),
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if cfg.TLS.Enabled {
		tlsCfg, err := server.GenerateOrLoadTLS(cfg.DataDir)
		if err != nil {
			log.Fatalf("Failed to setup TLS: %v", err)
		}
		httpServer.TLSConfig = tlsCfg
	}

	// If TLS enabled, run an HTTP→HTTPS redirect server on port 80
	if cfg.TLS.Enabled {
		go func() {
			redirectAddr := fmt.Sprintf("%s:80", cfg.Server.Host)
			log.Printf("HTTP→HTTPS redirect on %s", redirectAddr)
			redirectServer := &http.Server{
				Addr: redirectAddr,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					target := "https://" + r.Host + r.URL.RequestURI()
					http.Redirect(w, r, target, http.StatusMovedPermanently)
				}),
			}
			redirectServer.ListenAndServe() // ignore error (port 80 may not be available)
		}()
	}

	go func() {
		if cfg.Dev.Mode {
			server.StartDev(httpServer.Addr)
		}
		if cfg.TLS.Enabled {
			log.Printf("Fine Print starting on https://%s", httpServer.Addr)
			if err := httpServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTPS server error: %v", err)
			}
		} else {
			log.Printf("Fine Print starting on http://%s", httpServer.Addr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("HTTP server error: %v", err)
			}
		}
	}()

	// Log startup summary
	logStartupSummary(cfg)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	cancel() // Stop queue manager

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Fine Print stopped")
}

func logStartupSummary(cfg config.Config) {
	log.Println("=== Fine Print ===")
	log.Printf("  Mode:     %s", modeStr(cfg.Dev.Mode))
	log.Printf("  Database: %s", cfg.Database.Driver)
	scheme := "http"
	if cfg.TLS.Enabled {
		scheme = "https"
	}
	log.Printf("  Server:   %s://%s:%d", scheme, cfg.Server.Host, cfg.Server.Port)

	if cfg.Hotspot.Enabled && !cfg.Dev.Mode {
		log.Printf("  Hotspot:  %s (gateway: %s)", cfg.Hotspot.SSID, cfg.Hotspot.Gateway)
		log.Printf("  DNS:      port %d", cfg.DNS.Port)
	} else {
		log.Printf("  Hotspot:  disabled")
	}

	if cfg.Printer.Name != "" {
		log.Printf("  Printer:  %s (%s)", cfg.Printer.Name, cfg.Printer.Media)
	} else {
		log.Printf("  Printer:  auto-detect")
	}

	log.Printf("  Data:     %s", cfg.DataDir)
	fmt.Println()
}

func modeStr(dev bool) string {
	if dev {
		return "development"
	}
	return "production"
}
