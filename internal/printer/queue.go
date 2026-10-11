package printer

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/scout-kit/fine-print/internal/db"
	"github.com/scout-kit/fine-print/internal/storage"
)

// QueueManager processes print jobs, one per printer concurrently.
// Supports round-robin (auto-assign to enabled printers) and manual mode.
type QueueManager struct {
	queries   *db.Queries
	store     storage.Store
	printer   Printer
	broadcast func(eventType string, data any)

	mu       sync.Mutex
	paused   bool
	active   map[string]*activeJob // printer name -> active job
	resumeCh chan struct{}

	// Round-robin state
	rrIndex int
}

// activeJob is a job being worked on by a processJob goroutine.
type activeJob struct {
	job    *db.PrintJob
	cancel context.CancelFunc // halts the job; see StopJob
	done   chan struct{}      // closed once processJob has returned
}

func NewQueueManager(queries *db.Queries, store storage.Store, printer Printer, broadcast func(eventType string, data any)) *QueueManager {
	return &QueueManager{
		queries:   queries,
		store:     store,
		printer:   printer,
		broadcast: broadcast,
		active:    make(map[string]*activeJob),
		resumeCh:  make(chan struct{}, 1),
	}
}

// Run starts the queue processing loop. Blocks until ctx is canceled.
func (q *QueueManager) Run(ctx context.Context) {
	log.Println("Print queue manager started")
	defer log.Println("Print queue manager stopped")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		q.mu.Lock()
		paused := q.paused
		q.mu.Unlock()

		if paused {
			select {
			case <-ctx.Done():
				return
			case <-q.resumeCh:
				continue
			}
		}

		// Find an available printer and a job to assign
		assigned := q.tryAssignJob(ctx)
		if !assigned {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				continue
			}
		}
	}
}

// tryAssignJob finds the next queued job and an available printer, then processes it.
func (q *QueueManager) tryAssignJob(ctx context.Context) bool {
	// Get enabled printers
	enabledPrinters, err := q.queries.GetEnabledPrinters(ctx)
	if err != nil || len(enabledPrinters) == 0 {
		// Fall back to legacy single-printer mode
		return q.tryAssignJobLegacy(ctx)
	}

	// Check printer mode
	mode, _ := q.queries.GetSetting(ctx, "printer_mode")
	if mode == "" {
		mode = "round_robin"
	}

	// Find an available printer (not currently printing)
	q.mu.Lock()
	var availablePrinters []string
	for _, p := range enabledPrinters {
		if _, busy := q.active[p.Name]; !busy {
			availablePrinters = append(availablePrinters, p.Name)
		}
	}
	q.mu.Unlock()

	if len(availablePrinters) == 0 {
		return false // All printers busy
	}

	// Get next queued job
	job, err := q.queries.GetNextQueuedJob(ctx)
	if err != nil || job == nil {
		return false
	}

	// Pick a printer
	var printerName string
	if mode == "manual" && job.PrinterName.Valid && job.PrinterName.String != "" {
		// Manual mode: use the assigned printer if available
		printerName = job.PrinterName.String
		available := false
		for _, p := range availablePrinters {
			if p == printerName {
				available = true
				break
			}
		}
		if !available {
			return false // Assigned printer is busy
		}
	} else {
		// Round-robin: pick the next available printer
		q.mu.Lock()
		printerName = availablePrinters[q.rrIndex%len(availablePrinters)]
		q.rrIndex++
		q.mu.Unlock()
	}

	// Assign printer and mark as printing before spawning goroutine
	// to prevent the main loop from picking up the same job again
	q.queries.UpdatePrintJobPrinter(ctx, job.ID, printerName)
	q.queries.UpdatePrintJobStatus(ctx, job.ID, db.PrintJobStatusPrinting, "", "")

	// Process in a goroutine (allows concurrent printing on multiple printers)
	q.start(ctx, job, printerName)
	return true
}

// tryAssignJobLegacy handles the case where no printers are configured in the DB.
func (q *QueueManager) tryAssignJobLegacy(ctx context.Context) bool {
	q.mu.Lock()
	if len(q.active) > 0 {
		q.mu.Unlock()
		return false // Already printing
	}
	q.mu.Unlock()

	job, err := q.queries.GetNextQueuedJob(ctx)
	if err != nil || job == nil {
		return false
	}

	printerName, _ := q.queries.GetSetting(ctx, "printer_name")
	if printerName == "" {
		// No printer configured — admin must set one up
		return false
	}

	q.queries.UpdatePrintJobStatus(ctx, job.ID, db.PrintJobStatusPrinting, "", "")

	q.start(ctx, job, printerName)
	return true
}

// start marks printerName busy with job and spawns its processJob goroutine.
// The active entry is registered here, before the goroutine runs, so the main
// loop can't hand the same printer a second job in the meantime.
func (q *QueueManager) start(ctx context.Context, job *db.PrintJob, printerName string) {
	jobCtx, cancel := context.WithCancel(ctx)
	aj := &activeJob{job: job, cancel: cancel, done: make(chan struct{})}

	q.mu.Lock()
	q.active[printerName] = aj
	q.mu.Unlock()

	go func() {
		defer close(aj.done)
		defer cancel()
		defer func() {
			q.mu.Lock()
			delete(q.active, printerName)
			q.mu.Unlock()
		}()
		q.processJob(ctx, jobCtx, job, printerName)
	}()
}

// processJob prints job on printerName. jobCtx is canceled when the job is
// halted — by StopJob or by ctx (server shutdown) ending. A halted job's
// status is left alone: whoever halted it decides what it becomes.
func (q *QueueManager) processJob(ctx, jobCtx context.Context, job *db.PrintJob, printerName string) {
	photo, err := q.queries.GetPhoto(ctx, job.PhotoID)
	if err != nil || photo == nil {
		log.Printf("Error getting photo for print job %d: %v", job.ID, err)
		q.failJob(ctx, job, "photo not found")
		return
	}

	if !photo.RenderedKey.Valid {
		q.failJob(ctx, job, "photo has no rendered image")
		return
	}

	q.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusPrinting)

	q.broadcast("print_status", map[string]any{
		"job_id":       job.ID,
		"photo_id":     job.PhotoID,
		"status":       "printing",
		"printer_name": printerName,
	})

	mediaSize, _ := q.queries.GetSetting(ctx, "printer_media")
	if mediaSize == "" {
		mediaSize = "4x6"
	}

	filePath := q.store.Path(storage.BucketRendered, photo.RenderedKey.String)
	cupsJobID, err := q.printer.Print(printerName, filePath, PrintOptions{
		MediaSize: mediaSize,
		Copies:    1,
	})
	if jobCtx.Err() != nil {
		// Halted while lp was submitting. A job that reached CUPS would
		// otherwise print with nobody tracking it — unless the server is
		// shutting down, in which case it should finish on its own.
		if err == nil && ctx.Err() == nil {
			q.cancelCUPSJob(job.ID, cupsJobID)
		}
		return
	}
	if err != nil {
		// Printer unreachable/offline — requeue the job, don't fail it
		log.Printf("Print job %d: printer %s unavailable, requeueing: %v", job.ID, printerName, err)
		q.queries.UpdatePrintJobStatus(ctx, job.ID, db.PrintJobStatusQueued, "", "")
		q.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusQueued)
		return
	}

	q.queries.UpdatePrintJobStatus(ctx, job.ID, db.PrintJobStatusPrinting, cupsJobID, "")

	if err := q.pollJob(jobCtx, job.ID, cupsJobID); err != nil {
		if jobCtx.Err() != nil {
			if ctx.Err() == nil {
				q.cancelCUPSJob(job.ID, cupsJobID)
			}
			return
		}
		// CUPS accepted the job but it failed during printing — this is a real failure
		q.failJob(ctx, job, err.Error())
		return
	}

	q.queries.UpdatePrintJobStatus(ctx, job.ID, db.PrintJobStatusPrinted, cupsJobID, "")
	q.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusPrinted)

	q.broadcast("print_status", map[string]any{
		"job_id":       job.ID,
		"photo_id":     job.PhotoID,
		"status":       "printed",
		"printer_name": printerName,
	})

	log.Printf("Print job %d completed on %s", job.ID, printerName)
}

func (q *QueueManager) pollJob(ctx context.Context, jobID uint64, cupsJobID string) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			status, err := q.printer.JobStatus(cupsJobID)
			if err != nil {
				return err
			}
			switch status {
			case "completed":
				return nil
			case "stopped", "aborted", "canceled":
				return &PrintError{Status: status, JobID: cupsJobID}
			case "pending", "processing":
				continue
			}
		}
	}
}

func (q *QueueManager) failJob(ctx context.Context, job *db.PrintJob, errMsg string) {
	log.Printf("Print job %d failed: %s", job.ID, errMsg)

	q.queries.UpdatePrintJobStatus(ctx, job.ID, db.PrintJobStatusFailed, "", errMsg)
	q.queries.UpdatePhotoStatus(ctx, job.PhotoID, db.PhotoStatusFailed)

	q.mu.Lock()
	q.paused = true
	q.mu.Unlock()

	q.broadcast("print_error", map[string]any{
		"job_id":   job.ID,
		"photo_id": job.PhotoID,
		"error":    errMsg,
	})

	q.broadcast("queue_paused", map[string]any{
		"reason": "Print failure: " + errMsg,
	})
}

func (q *QueueManager) cancelCUPSJob(jobID uint64, cupsJobID string) {
	if err := q.printer.CancelJob(cupsJobID); err != nil {
		log.Printf("Print job %d: canceling CUPS job %s: %v", jobID, cupsJobID, err)
	}
}

// stopTimeout bounds how long StopJob waits for a job's goroutine to exit.
// The goroutine notices a halt immediately unless it is inside an lp call.
const stopTimeout = 15 * time.Second

// StopJob halts a job that is currently printing, canceling its CUPS job if
// one was submitted. It reports whether the job was running here; a job left
// marked printing by an earlier run of the server has no goroutine, and false
// is returned for it.
//
// StopJob returns once the job's goroutine has exited, so the caller can set
// the job's status without it being overwritten. The job's status itself is
// not changed — that's the caller's decision (cancel, requeue, ...).
func (q *QueueManager) StopJob(jobID uint64) bool {
	q.mu.Lock()
	var aj *activeJob
	for _, a := range q.active {
		if a.job.ID == jobID {
			aj = a
			break
		}
	}
	q.mu.Unlock()

	if aj == nil {
		return false
	}

	aj.cancel()
	select {
	case <-aj.done:
	case <-time.After(stopTimeout):
		log.Printf("Print job %d: still stopping after %s", jobID, stopTimeout)
	}
	return true
}

// Pause pauses the print queue.
func (q *QueueManager) Pause() {
	q.mu.Lock()
	q.paused = true
	q.mu.Unlock()

	q.broadcast("queue_paused", map[string]any{
		"reason": "Manually paused",
	})
}

// Resume resumes the print queue.
func (q *QueueManager) Resume() {
	q.mu.Lock()
	q.paused = false
	q.mu.Unlock()

	select {
	case q.resumeCh <- struct{}{}:
	default:
	}

	q.broadcast("queue_resumed", nil)
}

// IsPaused returns whether the queue is paused.
func (q *QueueManager) IsPaused() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.paused
}

// ActiveJobs returns all currently printing jobs by printer name.
func (q *QueueManager) ActiveJobs() map[string]*db.PrintJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make(map[string]*db.PrintJob, len(q.active))
	for k, v := range q.active {
		result[k] = v.job
	}
	return result
}

// PrintError represents a print failure with CUPS status details.
type PrintError struct {
	Status string
	JobID  string
}

func (e *PrintError) Error() string {
	return "print job " + e.JobID + " failed with status: " + e.Status
}
