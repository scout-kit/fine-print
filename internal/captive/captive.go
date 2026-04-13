package captive

import (
	"net/http"
	"strings"
)

// Handler intercepts captive portal detection requests and redirects to the app.
// This is used as part of the captive portal middleware chain.
type Handler struct {
	redirectURL string
}

func NewHandler(gatewayIP string) *Handler {
	return &Handler{
		redirectURL: "http://" + gatewayIP + "/",
	}
}

// captive portal detection paths by OS
var captivePaths = map[string]bool{
	// Apple (iOS, macOS)
	"/hotspot-detect.html":       true,
	"/library/test/success.html": true,

	// Android
	"/generate_204":            true,
	"/gen_204":                 true,
	"/connectivitycheck/gstatic/": true,

	// Google Chrome OS
	"/chrome/test": true,

	// Microsoft (Windows)
	"/connecttest.txt": true,
	"/ncsi.txt":        true,
	"/redirect":        true,

	// Firefox
	"/canonical.html": true,
	"/success.txt":    true,

	// Samsung
	"/generate_204_mobile": true,

	// Huawei
	"/generate_204_huawei": true,
}

// IsCaptiveRequest returns true if the request matches a known captive portal detection URL.
func IsCaptiveRequest(r *http.Request) bool {
	return captivePaths[strings.ToLower(r.URL.Path)]
}

// ServeHTTP redirects captive portal detection requests to the app.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, h.redirectURL, http.StatusFound)
}
