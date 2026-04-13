package imaging

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ConvertHEIC converts a HEIC/HEIF file to JPEG.
// On macOS, uses `sips`. On Linux, uses `heif-convert` from libheif-examples.
// Returns the path to the converted JPEG file.
func ConvertHEIC(inputPath string) (string, error) {
	outputPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".jpg"

	switch runtime.GOOS {
	case "darwin":
		return convertHEICDarwin(inputPath, outputPath)
	case "linux":
		return convertHEICLinux(inputPath, outputPath)
	default:
		return "", fmt.Errorf("HEIC conversion not supported on %s", runtime.GOOS)
	}
}

func convertHEICDarwin(input, output string) (string, error) {
	cmd := exec.Command("sips", "-s", "format", "jpeg", input, "--out", output)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("sips conversion failed: %w, output: %s", err, out)
	}
	return output, nil
}

func convertHEICLinux(input, output string) (string, error) {
	cmd := exec.Command("heif-convert", input, output)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("heif-convert failed: %w, output: %s", err, out)
	}
	return output, nil
}

// IsHEIC returns true if the filename has a HEIC/HEIF extension.
func IsHEIC(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".heic" || ext == ".heif"
}

// HEICSupported checks if HEIC conversion tools are available on this system.
func HEICSupported() bool {
	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("sips")
		return err == nil
	case "linux":
		_, err := exec.LookPath("heif-convert")
		return err == nil
	default:
		return false
	}
}

// ConvertHEICToTemp converts a HEIC file to JPEG in a temp directory.
// The caller is responsible for cleaning up the temp file.
func ConvertHEICToTemp(inputPath string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "fine-print-heic-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir: %w", err)
	}

	cleanup := func() { os.RemoveAll(tmpDir) }

	outputPath := filepath.Join(tmpDir, "converted.jpg")

	switch runtime.GOOS {
	case "darwin":
		_, err = convertHEICDarwin(inputPath, outputPath)
	case "linux":
		_, err = convertHEICLinux(inputPath, outputPath)
	default:
		cleanup()
		return "", nil, fmt.Errorf("HEIC conversion not supported on %s", runtime.GOOS)
	}

	if err != nil {
		cleanup()
		return "", nil, err
	}

	return outputPath, cleanup, nil
}
