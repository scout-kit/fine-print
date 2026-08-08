package printer

import (
	"context"
	"log"
	"sync"
	"time"
)

// MonitorConfig controls how often the printer heartbeat runs and how to
// look up the "which printer should be connected?" answer (which may
// change at runtime from the settings UI).
type MonitorConfig struct {
	Interval      time.Duration
	ExpectedName  func() string               // called every tick — empty string = no printer configured
	Broadcast     func(eventType string, data any)
	Queue         queuePauser
}

// queuePauser is the subset of QueueManager the monitor uses. Keeping it
// small makes the monitor trivial to unit-test without spinning up the
// real queue.
type queuePauser interface {
	Pause()
	IsPaused() bool
}

// Monitor polls the printer's availability and pauses the queue when the
// configured printer disappears. On reconnect it fires an SSE event but
// leaves the queue paused — the admin resumes manually once they've
// confirmed the paper/ink situation.
type Monitor struct {
	p   Printer
	cfg MonitorConfig

	mu        sync.Mutex
	connected bool
	known     bool // false until the first tick so we don't spam events at startup
}

func NewMonitor(p Printer, cfg MonitorConfig) *Monitor {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.ExpectedName == nil {
		cfg.ExpectedName = func() string { return "" }
	}
	if cfg.Broadcast == nil {
		cfg.Broadcast = func(string, any) {}
	}
	return &Monitor{p: p, cfg: cfg}
}

// Run polls until ctx is done. Safe to call as its own goroutine.
func (m *Monitor) Run(ctx context.Context) {
	t := time.NewTicker(m.cfg.Interval)
	defer t.Stop()

	// Fire once immediately so state is accurate without waiting a full interval.
	m.tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.tick()
		}
	}
}

func (m *Monitor) tick() {
	expected := m.cfg.ExpectedName()
	if expected == "" {
		// No printer configured — nothing to monitor.
		return
	}

	printers, err := m.p.ListPrinters()
	if err != nil {
		log.Printf("printer monitor: list failed: %v", err)
		return
	}

	nowConnected := false
	for _, p := range printers {
		if p.Name == expected {
			nowConnected = true
			break
		}
	}

	m.mu.Lock()
	prev := m.connected
	firstTick := !m.known
	m.connected = nowConnected
	m.known = true
	m.mu.Unlock()

	if firstTick {
		// Initial state — don't broadcast; if the printer is already down
		// we'll catch it via the next tick's transition or the admin can
		// look at /api/admin/printers directly.
		return
	}

	if prev && !nowConnected {
		log.Printf("printer monitor: %q disconnected — pausing queue", expected)
		if m.cfg.Queue != nil && !m.cfg.Queue.IsPaused() {
			m.cfg.Queue.Pause()
		}
		m.cfg.Broadcast("printer_disconnected", map[string]any{
			"printer": expected,
			"message": "Printer is no longer reachable. Queue paused.",
		})
	} else if !prev && nowConnected {
		log.Printf("printer monitor: %q reconnected", expected)
		m.cfg.Broadcast("printer_reconnected", map[string]any{
			"printer": expected,
			"message": "Printer is reachable again. Resume the queue when ready.",
		})
	}
}
