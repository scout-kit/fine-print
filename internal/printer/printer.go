package printer

// Printer defines the interface for printing operations.
type Printer interface {
	// ListPrinters returns available printers.
	ListPrinters() ([]PrinterInfo, error)
	// Print sends a file to the printer and returns the CUPS job ID.
	Print(printerName, filePath string, opts PrintOptions) (string, error)
	// JobStatus returns the current status of a CUPS job.
	JobStatus(jobID string) (string, error)
	// CancelJob cancels a CUPS job.
	CancelJob(jobID string) error
}

type PrinterInfo struct {
	Name       string `json:"name"`
	Device     string `json:"device"`
	State      string `json:"state"`
	AcceptJobs bool   `json:"accept_jobs"`
}

type PrintOptions struct {
	MediaSize string `json:"media_size"` // e.g., "4x6"
	Copies    int    `json:"copies"`
}
