package printer

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/scout-kit/fine-print/internal/db"
)

// QueueManager processes print jobs, one per printer concurrently.
// Supports round-robin (auto-assign to enabled printers) and manual mode.
type QueueManager struct {
	queries   *db.Queries
	printer   Printer
	broadcast func(eventType string, data any)

	mu       sync.Mutex
	paused   bool
	active   map[string]*db.PrintJob // printer name -> active job
	resumeCh chan struct{}

	// Round-robin state
	rrIndex int
}

func NewQueueManager(queries *db.Queries, printer Printer, broadcast func(eventType string, data any)) *QueueManager {
	return &QueueManager{
		queries:   queries,
		printer:   printer,
		broadcast: broadcast,
		active:    make(map[string]*db.PrintJob),
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

	// Assign printer to job
	q.queries.UpdatePrintJobPrinter(ctx, job.ID, printerName)

	// Process in a goroutine (allows concurrent printing on multiple printers)
	go q.processJob(ctx, job, printerName)
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

	go q.processJob(ctx, job, printerName)
	return true
}

func (q *QueueManager) processJob(ctx context.Context, job *db.PrintJob, printerName string) {
	q.mu.Lock()
	q.active[printerName] = job
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		delete(q.active, printerName)
		q.mu.Unlock()
	}()

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

	// Update status to printing
	if err := q.queries.UpdatePrintJobStatus(ctx, job.ID, db.PrintJobStatusPrinting, "", ""); err != nil {
		log.Printf("Error updating job status: %v", err)
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

	cupsJobID, err := q.printer.Print(printerName, photo.RenderedKey.String, PrintOptions{
		MediaSize: mediaSize,
		Copies:    1,
	})
	if err != nil {
		// Printer unreachable/offline — requeue the job, don't fail it
		log.Printf("Print job %d: printer %s unavailable, requeueing: %v", job.ID, printerName, err)
		q.queries.UpdatePrintJobStatus(ctx, job.ID, db.PrintJobStatusQueued, "", "")
		q.queries.UpdatePhotoStatus(ctx, photo.ID, db.PhotoStatusQueued)
		return
	}

	q.queries.UpdatePrintJobStatus(ctx, job.ID, db.PrintJobStatusPrinting, cupsJobID, "")

	if err := q.pollJob(ctx, job.ID, cupsJobID); err != nil {
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
		result[k] = v
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
