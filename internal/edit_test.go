package internal

import (
	"bufio"
	"strings"
	"testing"
	"time"
)

// If the last "Location:" line in the file is the same as the location, don't add it
func TestShouldAddLocation(t *testing.T) {
	data := `
Location: Chicago, IL
Location: Omaha, NE
`
	got, _ := ShouldAddLocation(bufio.NewScanner(strings.NewReader(data)), "Chicago, IL")
	if !got {
		t.Errorf("ShouldAddLocation() = %v, want %v", got, true)
	}
	got, _ = ShouldAddLocation(bufio.NewScanner(strings.NewReader(data)), "Omaha, NE")
	if got {
		t.Errorf("ShouldAddLocation() = %v, want %v", got, false)
	}
}

// Make sure header is in the expected format.
func TestTimestampHeader(t *testing.T) {
	tz := time.FixedZone("CST", -6*60*60)
	ts := time.Date(2025, 5, 13, 15, 33, 0, 0, tz)
	got := TimestampHeader(ts)
	if got != "## Tuesday 2025-05-13 3:33 PM CST\n" {
		t.Errorf("TimestampHeader() = %v, want %v", got, "## Tuesday 2025-05-13 3:33 PM CST\n")
	}
}
