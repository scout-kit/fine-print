package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scout-kit/fine-print/internal/api"
	"github.com/scout-kit/fine-print/internal/db"
)

// newPrintingJob creates a job marked printing with nothing working on it —
// what a restart leaves behind when it interrupts a print.
func newPrintingJob(t *testing.T, q *db.Queries) *db.PrintJob {
	t.Helper()
	ctx := context.Background()
	project := newProject(t, q)
	photo := &db.Photo{ProjectID: project.ID, SessionID: "sess", StatusID: db.PhotoStatusPrinting}
	if err := q.CreatePhoto(ctx, photo); err != nil {
		t.Fatalf("creating photo: %v", err)
	}
	job := &db.PrintJob{PhotoID: photo.ID, Position: 1, StatusID: db.PrintJobStatusPrinting}
	if err := q.CreatePrintJob(ctx, job); err != nil {
		t.Fatalf("creating job: %v", err)
	}
	return job
}

func jobStatus(t *testing.T, q *db.Queries, id uint64) string {
	t.Helper()
	job, err := q.GetPrintJob(context.Background(), id)
	if err != nil || job == nil {
		t.Fatalf("getting job %d: %v", id, err)
	}
	return db.PrintJobStatusName(job.StatusID)
}

// A job stuck printing could be neither retried nor canceled, so the only way
// to unstick it was editing the database.
func TestRetryJob_RequeuesPrintingJob(t *testing.T) {
	h, q := newTestHandlers(t)
	job := newPrintingJob(t, q)

	rec := doJSONWithID(t, h.RetryJob, "POST", "/api/admin/queue/"+itoa(job.ID)+"/retry", job.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := jobStatus(t, q, job.ID); got != "queued" {
		t.Errorf("job status = %s, want queued", got)
	}
}

func TestCancelJob_CancelsPrintingJob(t *testing.T) {
	h, q := newTestHandlers(t)
	job := newPrintingJob(t, q)

	rec := doJSONWithID(t, h.CancelJob, "POST", "/api/admin/queue/"+itoa(job.ID)+"/cancel", job.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := jobStatus(t, q, job.ID); got != "canceled" {
		t.Errorf("job status = %s, want canceled", got)
	}
	photo, _ := q.GetPhoto(context.Background(), job.PhotoID)
	if photo.StatusID != db.PhotoStatusUploaded {
		t.Errorf("photo status = %d, want uploaded so it can be re-approved", photo.StatusID)
	}
}

func TestCancelJob_RefusesPrintedJob(t *testing.T) {
	h, q := newTestHandlers(t)
	job := newPrintingJob(t, q)
	q.UpdatePrintJobStatus(context.Background(), job.ID, db.PrintJobStatusPrinted, "", "")

	rec := doJSONWithID(t, h.CancelJob, "POST", "/api/admin/queue/"+itoa(job.ID)+"/cancel", job.ID, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if got := jobStatus(t, q, job.ID); got != "printed" {
		t.Errorf("job status = %s, want printed", got)
	}
}

func setJobPrinter(t *testing.T, h *api.Handlers, id uint64, name string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSONWithID(t, h.SetJobPrinter, "PUT", "/api/admin/queue/"+itoa(id)+"/printer", id,
		map[string]string{"printer_name": name})
}

func manualModeWithPrinters(t *testing.T, q *db.Queries, enabled ...string) {
	t.Helper()
	ctx := context.Background()
	q.SetSetting(ctx, "printer_mode", "manual")
	for _, name := range enabled {
		q.UpsertPrinterAssignment(ctx, name, true)
	}
}

// A job stuck on a printer that has gone away can be moved to another one,
// which also puts it back in the queue.
func TestSetJobPrinter_MovesPrintingJob(t *testing.T) {
	h, q := newTestHandlers(t)
	manualModeWithPrinters(t, q, "Selphy", "Spare")
	job := newPrintingJob(t, q)
	q.UpdatePrintJobPrinter(context.Background(), job.ID, "Selphy")

	if rec := setJobPrinter(t, h, job.ID, "Spare"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	got, _ := q.GetPrintJob(context.Background(), job.ID)
	if got.PrinterName.String != "Spare" {
		t.Errorf("printer = %q, want Spare", got.PrinterName.String)
	}
	if name := db.PrintJobStatusName(got.StatusID); name != "queued" {
		t.Errorf("status = %s, want queued", name)
	}
}

func TestSetJobPrinter_EmptyNameClearsAssignment(t *testing.T) {
	h, q := newTestHandlers(t)
	manualModeWithPrinters(t, q, "Selphy")
	job := newPrintingJob(t, q)
	q.UpdatePrintJobPrinter(context.Background(), job.ID, "Selphy")

	if rec := setJobPrinter(t, h, job.ID, ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	got, _ := q.GetPrintJob(context.Background(), job.ID)
	if got.PrinterName.Valid {
		t.Errorf("printer = %q, want none", got.PrinterName.String)
	}
}

func TestSetJobPrinter_Rejects(t *testing.T) {
	for name, tc := range map[string]struct {
		mode    string
		printer string
	}{
		"printer not enabled": {"manual", "Elsewhere"},
		"round robin mode":    {"round_robin", "Selphy"},
	} {
		t.Run(name, func(t *testing.T) {
			h, q := newTestHandlers(t)
			manualModeWithPrinters(t, q, "Selphy")
			q.SetSetting(context.Background(), "printer_mode", tc.mode)
			job := newPrintingJob(t, q)

			if rec := setJobPrinter(t, h, job.ID, tc.printer); rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if got := jobStatus(t, q, job.ID); got != "printing" {
				t.Errorf("job status = %s, want it left printing", got)
			}
		})
	}
}
