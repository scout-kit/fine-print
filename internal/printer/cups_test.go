package printer

import (
	"maps"
	"testing"
)

func TestParseAccepting(t *testing.T) {
	out := `Selphy accepting requests since Sat Oct 11 10:00:00 2026
Selphy_2 not accepting requests since Sat Oct 11 10:00:00 2026 -
	Rejecting Jobs
`
	// "not accepting" used to be read as accepting, since the line contains
	// the word, and Selphy_2's line was also credited to Selphy by prefix.
	want := map[string]bool{"Selphy": true, "Selphy_2": false}
	if got := parseAccepting(out); !maps.Equal(got, want) {
		t.Errorf("parseAccepting = %v, want %v", got, want)
	}
}

func TestParseStates(t *testing.T) {
	out := `printer Selphy is idle.  enabled since Sat Oct 11 10:00:00 2026
printer Busy now printing Busy-12.  enabled since Sat Oct 11 10:00:00 2026
printer Broken disabled since Sat Oct 11 10:00:00 2026 -
	Unable to locate printer "selphy.local".
`
	want := map[string]string{"Selphy": "idle", "Busy": "printing", "Broken": "stopped"}
	if got := parseStates(out); !maps.Equal(got, want) {
		t.Errorf("parseStates = %v, want %v", got, want)
	}
}
