package printer_test

import (
	"sync"
	"testing"
	"time"

	"github.com/scout-kit/fine-print/internal/printer"
)

type stubQueue struct {
	mu     sync.Mutex
	paused bool
}

func (q *stubQueue) Pause() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = true
}

func (q *stubQueue) IsPaused() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.paused
}

type stubPrinter struct {
	mu       sync.Mutex
	printers []printer.PrinterInfo
	listErr  error
}

func (s *stubPrinter) set(printers []printer.PrinterInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.printers = printers
}

func (s *stubPrinter) ListPrinters() ([]printer.PrinterInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]printer.PrinterInfo(nil), s.printers...), s.listErr
}
func (s *stubPrinter) Print(string, string, printer.PrintOptions) (string, error) {
	return "", nil
}
func (s *stubPrinter) JobStatus(string) (string, error) { return "", nil }
func (s *stubPrinter) CancelJob(string) error           { return nil }

type event struct {
	typ  string
	data any
}

// runOneTick constructs a monitor, seeds its state, then exposes an
// unexported tick via the short interval trick. We only need the public
// behavior though — a single call to Run with cancel after two ticks
// captures the transitions.
func TestMonitor_PausesAndAlertsOnDisconnect(t *testing.T) {
	stub := &stubPrinter{printers: []printer.PrinterInfo{{Name: "Selphy"}}}
	queue := &stubQueue{}
	var events []event
	var eventMu sync.Mutex

	m := printer.NewMonitor(stub, printer.MonitorConfig{
		Interval:     10 * time.Millisecond,
		ExpectedName: func() string { return "Selphy" },
		Broadcast: func(typ string, data any) {
			eventMu.Lock()
			defer eventMu.Unlock()
			events = append(events, event{typ, data})
		},
		Queue: queue,
	})

	ctx, cancel := timeoutCtx(t, 400*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()

	// First tick: present. Wait then remove the printer.
	time.Sleep(40 * time.Millisecond)
	stub.set(nil)
	// Let a few more ticks run so the disconnect is observed.
	time.Sleep(80 * time.Millisecond)

	// Bring it back before shutdown so we can see the reconnect event too.
	stub.set([]printer.PrinterInfo{{Name: "Selphy"}})
	time.Sleep(80 * time.Millisecond)

	cancel()
	<-done

	eventMu.Lock()
	defer eventMu.Unlock()

	if !queue.IsPaused() {
		t.Error("queue should be paused after disconnect")
	}

	foundDisc := false
	foundReconn := false
	for _, e := range events {
		if e.typ == "printer_disconnected" {
			foundDisc = true
		}
		if e.typ == "printer_reconnected" {
			foundReconn = true
		}
	}
	if !foundDisc {
		t.Error("expected a printer_disconnected event")
	}
	if !foundReconn {
		t.Error("expected a printer_reconnected event")
	}
}

func TestMonitor_NoExpectedNameIsNoop(t *testing.T) {
	stub := &stubPrinter{printers: nil}
	queue := &stubQueue{}
	m := printer.NewMonitor(stub, printer.MonitorConfig{
		Interval:     5 * time.Millisecond,
		ExpectedName: func() string { return "" },
		Queue:        queue,
	})

	ctx, cancel := timeoutCtx(t, 60*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	<-done

	if queue.IsPaused() {
		t.Error("queue should not be paused when no printer is configured")
	}
}
