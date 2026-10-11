package printer_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/scout-kit/fine-print/internal/config"
	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/printer"
	"github.com/scout-kit/fine-print/internal/storage"
)

// hangingPrinter accepts every job and never finishes one — a printer that
// has gone away while CUPS still holds its jobs.
type hangingPrinter struct {
	mu       sync.Mutex
	canceled []string
}

func (p *hangingPrinter) ListPrinters() ([]printer.PrinterInfo, error) { return nil, nil }
func (p *hangingPrinter) Print(name, _ string, _ printer.PrintOptions) (string, error) {
	return name + "-1", nil
}
func (p *hangingPrinter) JobStatus(string) (string, error) { return "processing", nil }
func (p *hangingPrinter) CancelJob(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.canceled = append(p.canceled, id)
	return nil
}

func (p *hangingPrinter) canceledJobs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.canceled...)
}

func newQueueTestDB(t *testing.T) *db.Queries {
	t.Helper()
	dbx, err := db.Open(config.DatabaseConfig{
		Driver:     "sqlite",
		SQLitePath: filepath.Join(t.TempDir(), "test.db"),
	})
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { dbx.Close() })
	if err := db.Migrate(dbx, "sqlite"); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	return db.NewQueries(dbx)
}

// newQueuedJob creates a rendered photo with one queued print job.
func newQueuedJob(t *testing.T, q *db.Queries) *db.PrintJob {
	t.Helper()
	ctx := context.Background()
	project := &db.Project{Name: "Event", VisibilityID: db.VisibilityPublic, ProjectTypeID: db.ProjectTypeStandard}
	if err := q.CreateProject(ctx, project); err != nil {
		t.Fatalf("creating project: %v", err)
	}
	photo := &db.Photo{ProjectID: project.ID, SessionID: "sess", StatusID: db.PhotoStatusQueued}
	if err := q.CreatePhoto(ctx, photo); err != nil {
		t.Fatalf("creating photo: %v", err)
	}
	if err := q.UpdatePhotoRendered(ctx, photo.ID, "rendered.jpg"); err != nil {
		t.Fatalf("rendering photo: %v", err)
	}
	job := &db.PrintJob{PhotoID: photo.ID, Position: 1, StatusID: db.PrintJobStatusQueued}
	if err := q.CreatePrintJob(ctx, job); err != nil {
		t.Fatalf("creating job: %v", err)
	}
	return job
}

// waitForCUPSJob waits until the queue has handed job id to CUPS.
func waitForCUPSJob(t *testing.T, q *db.Queries, id uint64) *db.PrintJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := q.GetPrintJob(context.Background(), id)
		if err != nil {
			t.Fatalf("getting job: %v", err)
		}
		if job.CUPSJobID.Valid {
			return job
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %d never reached CUPS", id)
	return nil
}

func startQueue(t *testing.T, q *db.Queries, p printer.Printer) (*printer.QueueManager, context.CancelFunc) {
	t.Helper()
	store, err := storage.NewDiskStore(t.TempDir())
	if err != nil {
		t.Fatalf("creating store: %v", err)
	}
	if err := q.UpsertPrinterAssignment(context.Background(), "Selphy", true); err != nil {
		t.Fatalf("enabling printer: %v", err)
	}
	qm := printer.NewQueueManager(q, store, p, func(string, any) {})
	ctx, cancel := context.WithCancel(context.Background())
	go qm.Run(ctx)
	t.Cleanup(cancel)
	return qm, cancel
}

// A job stuck printing used to have no way out: nothing could halt the
// goroutine polling CUPS, and it held the printer until the server restarted.
func TestStopJob_HaltsPrintingJobAndCancelsItInCUPS(t *testing.T) {
	q := newQueueTestDB(t)
	p := &hangingPrinter{}
	job := newQueuedJob(t, q)
	qm, _ := startQueue(t, q, p)

	waitForCUPSJob(t, q, job.ID)

	if !qm.StopJob(job.ID) {
		t.Fatal("StopJob reported the job wasn't running")
	}
	if got := p.canceledJobs(); len(got) != 1 || got[0] != "Selphy-1" {
		t.Errorf("canceled CUPS jobs = %v, want [Selphy-1]", got)
	}
	if active := qm.ActiveJobs(); len(active) != 0 {
		t.Errorf("printer still busy after stop: %v", active)
	}
	if qm.IsPaused() {
		t.Error("stopping a job paused the queue as if it had failed")
	}

	// The caller decides what a stopped job becomes; the queue must not
	// have marked it failed on its way out.
	got, _ := q.GetPrintJob(context.Background(), job.ID)
	if got.StatusID != db.PrintJobStatusPrinting {
		t.Errorf("status = %s, want printing", db.PrintJobStatusName(got.StatusID))
	}
}

// Shutting down mustn't cancel what CUPS is printing — the print should
// still come out.
func TestShutdown_LeavesCUPSJobAlone(t *testing.T) {
	q := newQueueTestDB(t)
	p := &hangingPrinter{}
	job := newQueuedJob(t, q)
	qm, stop := startQueue(t, q, p)

	waitForCUPSJob(t, q, job.ID)
	stop()

	deadline := time.Now().Add(5 * time.Second)
	for len(qm.ActiveJobs()) > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := p.canceledJobs(); len(got) != 0 {
		t.Errorf("shutdown canceled CUPS jobs %v", got)
	}
}

func TestStopJob_UnknownJob(t *testing.T) {
	q := newQueueTestDB(t)
	qm := printer.NewQueueManager(q, nil, &hangingPrinter{}, func(string, any) {})
	if qm.StopJob(42) {
		t.Error("StopJob reported a job that was never started as running")
	}
}
