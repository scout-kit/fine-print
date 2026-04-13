package qrcode

import (
	"fmt"
	"net/http"

	qr "github.com/skip2/go-qrcode"
)

// Handler serves a QR code PNG for the gateway URL.
type Handler struct {
	baseURL string
}

func NewHandler(gatewayIP string, port int) *Handler {
	url := fmt.Sprintf("http://%s", gatewayIP)
	if port != 80 {
		url = fmt.Sprintf("http://%s:%d", gatewayIP, port)
	}
	return &Handler{baseURL: url}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	servePNG(w, h.baseURL)
}

// BaseURL returns the base URL for building project links.
func (h *Handler) BaseURL() string {
	return h.baseURL
}

// GeneratePNG creates a QR code PNG for any URL and writes it to the response.
func GeneratePNG(w http.ResponseWriter, url string) {
	servePNG(w, url)
}

func servePNG(w http.ResponseWriter, url string) {
	png, err := qr.Encode(url, qr.Medium, 256)
	if err != nil {
		http.Error(w, "failed to generate QR code", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(png)
}
