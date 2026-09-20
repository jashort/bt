package internal

import (
	"bufio"
	"os"
	"path/filepath"
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

// Entries are saved to <data-dir>/<YYYY>/<MM>/<DD>/<unix seconds>.md
func TestDestinationFile(t *testing.T) {
	tz := time.FixedZone("CST", -6*60*60)
	ts := time.Date(2025, 5, 13, 15, 33, 5, 0, tz)
	got := DestinationFile("~/data/Blog", ts)
	if got != "~/data/Blog/2025/05/13/1747171985.md" {
		t.Errorf("DestinationFile() = %v, want %v", got, "~/data/Blog/2025/05/13/1747171985.md")
	}
}

// LatestEntryFile returns the newest entry file of the day, or "" when there is none.
func TestLatestEntryFile(t *testing.T) {
	tz := time.FixedZone("CST", -6*60*60)
	ts := time.Date(2025, 5, 13, 15, 33, 5, 0, tz)
	root := t.TempDir()
	dayDir := filepath.Join(root, "2025", "05", "13")
	if err := os.MkdirAll(dayDir, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"1747171985.md", "1747179999.md", "notes.md", "1747171985.txt"} {
		if err := os.WriteFile(filepath.Join(dayDir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := LatestEntryFile(root, ts)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dayDir, "1747179999.md")
	if got != want {
		t.Errorf("LatestEntryFile() = %v, want %v", got, want)
	}

	got, err = LatestEntryFile(t.TempDir(), ts)
	if err != nil || got != "" {
		t.Errorf("LatestEntryFile() = %v, %v; want \"\", nil for empty dir", got, err)
	}
}
