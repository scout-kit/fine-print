<p align="center">
  <img src="docs/app_logo.png" alt="Fine Print" width="300" />
</p>

<p align="center">
  Self-hosted photo printing kiosk. Turn any machine into a WiFi hotspot that lets guests upload, edit, and print 4x6 photos — no internet required.
</p>

<p align="center">
  <a href="https://scout-kit.github.io/fine-print/">Website</a> · <a href="#quick-start">Quick Start</a> · <a href="#features">Features</a>
</p>

---

## Features

- **WiFi Hotspot + Captive Portal** — Guests connect and are redirected to the app
- **Photo Upload** — Single or multi-file upload with drag-and-drop
- **Image Editor** — Crop, rotate, brightness/contrast/saturation adjustments
- **Template System** — Per-project overlays (PNG) and text overlays with landscape/portrait support
- **Print Queue** — CUPS integration, multiple printer support, round-robin or manual assignment
- **Photo Booth Mode** — Live camera viewfinder, countdown timer, instant print
- **Project Management** — Public/hidden/private visibility, QR codes, copy projects
- **Gallery** — Guest-accessible photo gallery with download options
- **Offline-First** — Runs entirely on local network, no internet needed
- **Cross-Platform** — macOS (primary), Linux, Windows (build tags)
- **HTTPS** — Auto-generated self-signed certificates for camera access on LAN

## Quick Start

### Prerequisites

- Go 1.22+
- Node.js 20+
- CUPS (for printing)
- Linux only: `hostapd` + `dnsmasq` (for hotspot / captive portal)

`make deps` verifies all of the above and prints a copy-paste install command for your OS (Homebrew on macOS, apt on Debian/Raspberry Pi OS) if anything is missing. It does not auto-install toolchains.

### Development

```bash
# Verify + install dependencies
make deps

# Run in development mode (port 8080, no hotspot/DNS)
make dev

# With HTTPS (required for camera on non-localhost)
FINEPRINT_TLS=1 FINEPRINT_PORT=8443 make dev
```

Open `http://localhost:8080` (or `https://localhost:8443` with TLS).

### Build

```bash
# Build frontend + Go binary
make all

# Binary is at bin/fine-print
./bin/fine-print -dev -port 8080
```

### Cross-Compile

```bash
# Raspberry Pi
make build-pi

# Linux x86_64
make build-linux
```

### Production Install (single command)

```bash
sudo make install
```

That runs `scripts/install.sh`, which in turn:

1. Calls `bootstrap.sh` to fail fast on missing deps.
2. Builds the frontend + a native backend binary.
3. Installs the binary, service file, and config to system paths via `install-service.sh`.

On first boot, visit the kiosk URL in a browser — you'll be redirected to `/setup`, a one-time wizard that captures the admin password, hotspot SSID/password (both optional), and printer choice. The wizard refuses to run a second time once submitted.

## Operations

### Health checks

- `GET /healthz` — liveness. Always 200 when the process is running.
- `GET /readyz` — readiness. Returns 503 when the DB is unreachable or disk free space is below the hard guard threshold.

### systemd watchdog

`configs/fine-print.service` uses `Type=notify` with `WatchdogSec=60`. The app sends `READY=1` at startup and pings `WATCHDOG=1` every 30 s. If the process hangs past the watchdog interval, systemd kills and restarts it (`Restart=always`).

### Disk-space guard

A minimum-free threshold (default 2 GiB, admin-editable in Settings) protects the data volume:

- Uploads are refused with **507 Insufficient Storage** once free space falls below the threshold.
- A persistent banner appears in the admin UI when used space crosses 90%, and escalates to a critical style when uploads are blocked.
- `/readyz` fails while the guard is tripped so external monitoring can react.

### Backup & restore

From **Admin → Settings → Backup & Restore**:

- **Download** produces a `.tar.gz` containing a consistent SQLite snapshot (via `VACUUM INTO`), original uploads, overlays, and fonts. Rendered/preview images are excluded — they regenerate on demand.
- **Restore** accepts a previously downloaded backup. The existing DB and directories are moved aside as `.bak-STAMP` files before the new ones are swapped in; a service restart is required afterwards.

### Printer monitoring

A background poller (default every 30 s, admin-editable) verifies the configured printer is still listed by CUPS. On disconnect it **pauses the queue** and emits an SSE alert; on reconnect it emits a reconnect event but leaves the queue paused so the admin can confirm paper/ink before resuming.

## Configuration

Config is layered: **defaults → YAML file → DB (admin UI) → env vars → CLI flags**. The YAML
seeds the DB on first boot; after that, runtime-tunable settings live in the DB and can be
edited from the admin UI without touching files on disk.

- **YAML-only (bootstrap)**: `server.*`, `data_dir`, `database.*`, `tls.*`, `dev.*`
- **DB-backed (editable in admin UI)**: `hotspot.*`, `dns.*`, `printer.*`, `admin.password`, `imaging.*`

Copy `configs/fine-print.example.yml` to `config.yml` and edit:

```yaml
server:
  port: 80
  host: "0.0.0.0"

database:
  driver: "sqlite"  # or "mysql"
  sqlite_path: "data/fine-print.db"

admin:
  password: "changeme"  # seeds the DB on first boot; change it in the admin UI after that

tls:
  enabled: false  # Enable for camera access on LAN

printer:
  name: ""  # Must be configured by admin in settings
  media: "4x6"
```

Environment variables override config: `FINEPRINT_DEV=1`, `FINEPRINT_PORT=8080`, `FINEPRINT_TLS=1`, etc.

Most DB-backed settings require a service restart to take effect (the admin UI flags these and
exposes a **Restart Now** button that exits cleanly so systemd/launchd respawns the process).
`printer.name`, `printer.media`, and `printer.auto_queue` are hot-reloadable.

## Architecture

Single Go binary with embedded Svelte frontend.

- **Backend**: Go, SQLite/MySQL, CUPS printing, miekg/dns for captive portal
- **Frontend**: SvelteKit (SPA mode), Fabric.js for canvas editing
- **Image Processing**: Pure Go (disintegration/imaging, fogleman/gg)
- **Networking**: OS-specific hotspot management (macOS Internet Sharing, Linux hostapd)

## Project Structure

```
cmd/fine-print/       # Entry point
internal/
  api/                # REST API handlers
  config/             # Configuration
  db/                 # Database, migrations, models
  imaging/            # Image processing pipeline
  printer/            # CUPS integration, print queue
  server/             # HTTP server, SSE, middleware, TLS
  hotspot/            # WiFi hotspot (darwin/linux)
  captive/            # Captive portal + DNS hijack
  storage/            # File storage
web/                  # Svelte frontend source
configs/              # Example config, service files
scripts/              # Install scripts
```

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
