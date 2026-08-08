// Package systemd is a minimal sd_notify client that lets the app signal
// readiness and liveness to a systemd service supervisor configured with
// Type=notify and WatchdogSec=N. No external dependencies; a no-op when
// NOTIFY_SOCKET is unset (e.g. running locally).
package systemd

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"time"
)

// NotifyReady tells systemd the service has finished starting up. Safe
// to call when not under systemd (returns false silently).
func NotifyReady() bool {
	return send("READY=1")
}

// NotifyStopping tells systemd the service is shutting down gracefully.
// Stops the watchdog from restarting us while we clean up.
func NotifyStopping() bool {
	return send("STOPPING=1")
}

// NotifyWatchdog pings the systemd watchdog. Must be called more
// frequently than WatchdogSec or systemd will kill the process.
func NotifyWatchdog() bool {
	return send("WATCHDOG=1")
}

// WatchdogInterval returns the configured watchdog interval and whether
// the watchdog is active. systemd exposes WATCHDOG_USEC (microseconds)
// when WatchdogSec is set on the service unit.
func WatchdogInterval() (time.Duration, bool) {
	v := os.Getenv("WATCHDOG_USEC")
	if v == "" {
		return 0, false
	}
	usec, err := strconv.ParseInt(v, 10, 64)
	if err != nil || usec <= 0 {
		return 0, false
	}
	// Ping at half the interval — systemd's documented guidance. Gives
	// the process room to miss one heartbeat without being killed.
	return time.Duration(usec) * time.Microsecond / 2, true
}

// RunWatchdog sends WATCHDOG=1 at the systemd-configured cadence until
// ctx is canceled. No-op when the watchdog isn't active. Intended to run
// as its own goroutine from main().
func RunWatchdog(ctx context.Context) {
	interval, ok := WatchdogInterval()
	if !ok {
		return
	}
	log.Printf("systemd watchdog: pinging every %s", interval)

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			NotifyWatchdog()
		}
	}
}

// send writes a single sd_notify state message to the socket named by
// NOTIFY_SOCKET. Returns false when the env var is unset (normal in
// development) or the send fails (which we treat as non-fatal).
func send(state string) bool {
	addr := os.Getenv("NOTIFY_SOCKET")
	if addr == "" {
		return false
	}
	// Abstract socket (prefix '@') — systemd's default on Linux.
	if addr[0] == '@' {
		addr = "\x00" + addr[1:]
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: addr, Net: "unixgram"})
	if err != nil {
		return false
	}
	defer conn.Close()
	_, err = conn.Write([]byte(state))
	return err == nil
}
