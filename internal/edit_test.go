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

// EntryFiles lists a day's entries sorted by the timestamp in the header row.
func TestEntryFiles(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "09", "20")
	if err := os.MkdirAll(dayDir, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"1789863000.md": "## Sunday 2026-09-20 10:00 AM PDT\n\nLater entry\n",
		"1789900000.md": "## Sunday 2026-09-20 8:12 AM PDT\n\nEarlier entry\n",
		"1789999999.md": "no header here\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dayDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := EntryFiles(t.TempDir(), time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC))
	if err != nil || len(got) != 0 {
		t.Errorf("EntryFiles() = %v, %v; want none for empty dir", got, err)
	}

	got, err = EntryFiles(root, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{"1789900000.md", "1789863000.md", "1789999999.md"}
	if len(got) != len(wantOrder) {
		t.Fatalf("EntryFiles() returned %d files, want %d", len(got), len(wantOrder))
	}
	for i, want := range wantOrder {
		if base := filepath.Base(got[i].Path); base != want {
			t.Errorf("EntryFiles()[%d] = %v, want %v", i, base, want)
		}
	}
}

// parseHeaderTimestamp reads the timestamp out of the header row.
func TestParseHeaderTimestamp(t *testing.T) {
	got, err := parseHeaderTimestamp("## Sunday 2026-09-20 8:12 AM PDT\n\nSome things happened...\n")
	if err != nil {
		t.Fatal(err)
	}
	if formatted := got.Format("2006-01-02 3:04 PM"); formatted != "2026-09-20 8:12 AM" {
		t.Errorf("parseHeaderTimestamp() = %v, want 2026-09-20 8:12 AM", formatted)
	}
}
