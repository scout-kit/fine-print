package printer

import (
	"fmt"
	"os/exec"
	"strings"
)

// CUPSPrinter implements the Printer interface using CUPS command-line tools.
type CUPSPrinter struct{}

func NewCUPSPrinter() *CUPSPrinter {
	return &CUPSPrinter{}
}

func (c *CUPSPrinter) ListPrinters() ([]PrinterInfo, error) {
	out, err := exec.Command("lpstat", "-v").Output()
	if err != nil {
		return nil, fmt.Errorf("lpstat -v: %w", err)
	}

	var printers []PrinterInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "device for PrinterName: usb://..."
		if !strings.HasPrefix(line, "device for ") {
			continue
		}
		line = strings.TrimPrefix(line, "device for ")
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		device := strings.TrimSpace(parts[1])
		printers = append(printers, PrinterInfo{
			Name:   name,
			Device: device,
		})
	}

	// Get accepting status
	stateOut, err := exec.Command("lpstat", "-a").Output()
	if err == nil {
		for _, line := range strings.Split(string(stateOut), "\n") {
			for i := range printers {
				if strings.HasPrefix(line, printers[i].Name) {
					printers[i].AcceptJobs = strings.Contains(line, "accepting")
				}
			}
		}
	}

	return printers, nil
}

func (c *CUPSPrinter) Print(printerName, filePath string, opts PrintOptions) (string, error) {
	args := []string{"-d", printerName}

	if opts.MediaSize != "" {
		args = append(args, "-o", "media="+opts.MediaSize)
	}
	// fill: scale to cover entire page (crop excess rather than letterbox)
	// StpFullBleed=True: borderless printing (Canon Selphy / Gutenprint PPD)
	args = append(args, "-o", "fill", "-o", "StpFullBleed=True")

	if opts.Copies > 1 {
		args = append(args, "-n", fmt.Sprintf("%d", opts.Copies))
	}

	args = append(args, filePath)

	out, err := exec.Command("lp", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("lp command failed: %w, output: %s", err, out)
	}

	// Parse output: "request id is PrinterName-42 (1 file(s))"
	outStr := strings.TrimSpace(string(out))
	if strings.HasPrefix(outStr, "request id is ") {
		parts := strings.Fields(outStr)
		if len(parts) >= 4 {
			return parts[3], nil
		}
	}

	return outStr, nil
}

func (c *CUPSPrinter) JobStatus(jobID string) (string, error) {
	// List only active (not-completed) jobs. Once a job finishes,
	// it disappears from this list — no need to parse status keywords.
	out, err := exec.Command("lpstat", "-W", "not-completed").CombinedOutput()
	if err != nil {
		// lpstat failing likely means no active jobs
		return "completed", nil
	}

	outStr := strings.TrimSpace(string(out))
	if outStr == "" {
		// No active jobs at all
		return "completed", nil
	}

	// Check if our job is still in the active list
	if strings.Contains(outStr, jobID) {
		return "processing", nil
	}

	// Job not in active list — it's done
	return "completed", nil
}

func (c *CUPSPrinter) CancelJob(jobID string) error {
	out, err := exec.Command("cancel", jobID).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cancel failed: %w, output: %s", err, out)
	}
	return nil
}
