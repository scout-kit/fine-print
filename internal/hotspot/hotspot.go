package hotspot

// Manager defines the interface for WiFi hotspot management.
// Implementations are OS-specific (darwin, linux, windows).
type Manager interface {
	// Start creates or verifies the WiFi hotspot.
	Start(cfg Config) error
	// Stop shuts down the hotspot.
	Stop() error
	// Status returns the current hotspot state.
	Status() (Status, error)
}

type Config struct {
	SSID      string
	Password  string
	Channel   int
	Interface string
	Subnet    string
	Gateway   string
}

type Status struct {
	Active    bool   `json:"active"`
	SSID      string `json:"ssid"`
	Interface string `json:"interface"`
	Gateway   string `json:"gateway"`
	Clients   int    `json:"clients"`
}

// ErrManualSetupRequired indicates the hotspot must be configured manually.
type ErrManualSetupRequired struct {
	Message string
}

func (e *ErrManualSetupRequired) Error() string {
	return e.Message
}
