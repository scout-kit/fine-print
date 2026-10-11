package printer

import (
	"fmt"
	"os"
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

	// Whether each queue accepts jobs, and whether it's enabled. Either lookup
	// failing leaves the printers marked accepting with an unknown state, so
	// a flaky lpstat doesn't take every printer out of rotation.
	accepting := map[string]bool{}
	if out, err := lpstatC("-a"); err == nil {
		accepting = parseAccepting(string(out))
	}
	states := map[string]string{}
	if out, err := lpstatC("-p"); err == nil {
		states = parseStates(string(out))
	}
	for i := range printers {
		accept, ok := accepting[printers[i].Name]
		printers[i].AcceptJobs = accept || !ok
		printers[i].State = states[printers[i].Name]
	}

	return printers, nil
}

// lpstatC runs lpstat in the C locale: its status text is translated, and
// the parsers below match the English wording.
func lpstatC(args ...string) ([]byte, error) {
	cmd := exec.Command("lpstat", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd.Output()
}

// parseAccepting reads `lpstat -a` output:
//
//	Selphy accepting requests since Sat Oct 11 10:00:00 2026
//	Other not accepting requests since Sat Oct 11 10:00:00 2026 -
//		reason unknown
func parseAccepting(out string) map[string]bool {
	accepting := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		accepting[fields[0]] = fields[1] == "accepting"
	}
	return accepting
}

// parseStates reads `lpstat -p` output into idle, printing or stopped (CUPS's
// name for a disabled queue — what a printer that errored out becomes):
//
//	printer Selphy is idle.  enabled since Sat Oct 11 10:00:00 2026
//	printer Other now printing Other-12.  enabled since ...
//	printer Third disabled since Sat Oct 11 10:00:00 2026 -
func parseStates(out string) map[string]string {
	states := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != "printer" {
			continue
		}
		switch fields[2] {
		case "is":
			states[fields[1]] = "idle"
		case "now":
			states[fields[1]] = "printing"
		case "disabled":
			states[fields[1]] = "stopped"
		}
	}
	return states
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
