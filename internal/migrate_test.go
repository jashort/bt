package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Simulates exactly what the old writer produced for a two-entry day: each
// append wrote "\n" + header + [location] + body + "\n".
const twoEntryDay = `
## Monday 2024-01-01 9:19 AM CST
Location: Chicago, IL

First entry body

## Monday 2024-01-01 11:13 AM CST

Second entry body, location carried over.
`

func TestParseLegacyDayFileHeadered(t *testing.T) {
	day := time.Date(2024, 1, 1, 0, 0, 0, 0, time.FixedZone("CST", -6*60*60))
	entries, err := ParseLegacyDayFile("2024/2024-01-01.txt", day, twoEntryDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	first, second := entries[0], entries[1]

	if got := first.HeaderTime.Format("2006-01-02 3:04 PM"); got != "2024-01-01 9:19 AM" {
		t.Errorf("first header time = %q", got)
	}
	if !first.HasOwnLocation || first.Location != "Chicago, IL" {
		t.Errorf("first location = %q own=%v, want Chicago, IL own=true", first.Location, first.HasOwnLocation)
	}
	if first.Body != "First entry body" {
		t.Errorf("first body = %q", first.Body)
	}

	if second.HasOwnLocation {
		t.Error("second entry should not have its own location yet")
	}
	if second.Body != "Second entry body, location carried over." {
		t.Errorf("second body = %q", second.Body)
	}
}

func TestCarryDownLocations(t *testing.T) {
	entries, err := ParseLegacyDayFile("f", time.Time{}, twoEntryDay)
	if err != nil {
		t.Fatal(err)
	}
	carryDownLocations(entries)
	if entries[1].Location != "Chicago, IL" {
		t.Errorf("carried location = %q, want Chicago, IL", entries[1].Location)
	}

	// The most recent location wins for later entries.
	data := "## Monday 2024-01-01 9:00 AM CST\nLocation: Chicago, IL\n\nA\n\n## Monday 2024-01-01 10:00 AM CST\n\nB\n\n## Monday 2024-01-01 11:00 AM CST\nLocation: Omaha, NE\n\nC\n\n## Monday 2024-01-01 12:00 PM CST\n\nD\n"
	entries, err = ParseLegacyDayFile("f", time.Time{}, data)
	if err != nil {
		t.Fatal(err)
	}
	carryDownLocations(entries)
	want := []string{"Chicago, IL", "Chicago, IL", "Omaha, NE", "Omaha, NE"}
	for i, w := range want {
		if entries[i].Location != w {
			t.Errorf("entries[%d].Location = %q, want %q", i, entries[i].Location, w)
		}
	}
}

func TestParseLegacyDayFileMalformed(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"bad header format", "\n## Someday\n\nbody\n", "unparsable entry header"},
		{"bad header date", "\n## Monday 2024-13-45 9:00 AM CST\n\nbody\n", "unparsable entry header"},
		{"content before first header", "loose text\n\n## Monday 2024-01-01 9:00 AM CST\n\nbody\n", "content before first entry header"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseLegacyDayFile("2024/2024-01-01.txt", time.Time{}, tc.content)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), "2024/2024-01-01.txt:") {
				t.Errorf("error %q should name the file and line", err)
			}
		})
	}
}

func TestParseLegacyDayFileEmpty(t *testing.T) {
	entries, err := ParseLegacyDayFile("f", time.Time{}, "\n \n")
	if err != nil || entries != nil {
		t.Errorf("ParseLegacyDayFile() = %v, %v; want nil, nil for empty file", entries, err)
	}
}

func TestParseLegacyDayFileHeaderless(t *testing.T) {
	day := time.Date(2023, 3, 11, 0, 0, 0, 0, time.Local)
	content := "Location: Somewhere, CO\n\nText on a line\n\nMore text\n\nLater...\n\nNew entry\n"
	entries, err := ParseLegacyDayFile("2023/2023-03-11.txt", day, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (whole file)", len(entries))
	}
	e := entries[0]
	want := time.Date(2023, 3, 11, 12, 0, 0, 0, time.Local)
	if !e.HeaderTime.Equal(want) {
		t.Errorf("headerless timestamp = %v, want noon local %v", e.HeaderTime, want)
	}
	if e.Location != "Somewhere, CO" {
		t.Errorf("location = %q, want Somewhere, CO", e.Location)
	}
	if e.Body != "Text on a line\n\nMore text\n\nLater...\n\nNew entry" {
		t.Errorf("body = %q", e.Body)
	}
}

func TestParseLegacyDayFileHeaderlessMultipleLocations(t *testing.T) {
	day := time.Date(2023, 3, 11, 0, 0, 0, 0, time.Local)
	content := "Location: First place\n\nEntry one stuff\n\nLocation: Second place\n\nEntry two stuff\n"
	entries, err := ParseLegacyDayFile("f", day, content)
	if err != nil {
		t.Fatal(err)
	}
	e := entries[0]
	if e.Location != "First place" {
		t.Errorf("location = %q, want First place", e.Location)
	}
	// The unattributable later location line must stay in the body, never dropped.
	if !strings.Contains(e.Body, "Location: Second place") {
		t.Errorf("body %q should retain the second location line", e.Body)
	}
	if len(e.Flags) == 0 {
		t.Error("expected a review flag for multiple location lines")
	}
}

func TestParseLegacyDayFileHeaderlessNoLocation(t *testing.T) {
	day := time.Date(2023, 3, 11, 0, 0, 0, 0, time.Local)
	entries, err := ParseLegacyDayFile("f", day, "Just some text\n")
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Location != "" || entries[0].HasOwnLocation {
		t.Errorf("location = %q own=%v, want empty", entries[0].Location, entries[0].HasOwnLocation)
	}
}

func TestPlanMigrationFullRun(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "2024", "2024-01-01.txt"), twoEntryDay)

	plan, err := PlanMigration(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Total != 2 || len(plan.Groups) != 1 {
		t.Fatalf("plan = %d entries in %d groups, want 2 in 1", plan.Total, len(plan.Groups))
	}

	// Expectations are derived through time.Parse because the epoch a header
	// resolves to depends on whether the local zone knows the abbreviation.
	ts1 := mustParseHeader(t, "Monday 2024-01-01 9:19 AM CST")
	ts2 := mustParseHeader(t, "Monday 2024-01-01 11:13 AM CST")
	wantDest1 := filepath.Join(root, "2024", "01", "01", fmtEpoch(ts1)+".md")
	wantDest2 := filepath.Join(root, "2024", "01", "01", fmtEpoch(ts2)+".md")
	g := plan.Groups[0]
	if g.Entries[0].DestPath != wantDest1 || g.Entries[1].DestPath != wantDest2 {
		t.Errorf("destinations = %s, %s", g.Entries[0].DestPath, g.Entries[1].DestPath)
	}

	want1 := "## Monday 2024-01-01 9:19 AM CST\nLocation: Chicago, IL\n\nFirst entry body\n"
	if g.Entries[0].Content != want1 {
		t.Errorf("content[0] = %q, want %q", g.Entries[0].Content, want1)
	}
	want2 := "## Monday 2024-01-01 11:13 AM CST\nLocation: Chicago, IL\n\nSecond entry body, location carried over.\n"
	if g.Entries[1].Content != want2 {
		t.Errorf("content[1] = %q, want %q", g.Entries[1].Content, want2)
	}
	if g.Entries[1].LocationSource != "carried" {
		t.Errorf("location source = %q, want carried", g.Entries[1].LocationSource)
	}

	// Plan must not write anything.
	if _, err := os.Stat(wantDest1); !os.IsNotExist(err) {
		t.Error("planning created destination files; dry run must write nothing")
	}
}

func TestPlanMigrationSkipsUnrecognized(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "2024", "2024-01-01.txt"), twoEntryDay)
	writeFile(t, filepath.Join(root, "2024", "notes.txt"), "keep me")

	plan, err := PlanMigration(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Skipped) != 1 || !strings.HasSuffix(plan.Skipped[0], "notes.txt") {
		t.Errorf("skipped = %v", plan.Skipped)
	}
}

func TestPlanMigrationRejectsConflictingDestination(t *testing.T) {
	root := t.TempDir()
	ts := mustParseHeader(t, "Monday 2024-01-01 9:19 AM CST")
	destDir := filepath.Join(root, "2024", "01", "01")
	writeFile(t, filepath.Join(destDir, fmtEpoch(ts)+".md"), "## Some other content\n")

	// Existing destination with different content aborts the run.
	writeFile(t, filepath.Join(root, "2024", "2024-01-01.txt"), twoEntryDay)
	if _, err := PlanMigration(root); err == nil || !strings.Contains(err.Error(), "different content") {
		t.Fatalf("err = %v, want conflicting destination error", err)
	}

	// Existing destination with identical content is recognized for skipping.
	writeFile(t, filepath.Join(destDir, fmtEpoch(ts)+".md"), "## Monday 2024-01-01 9:19 AM CST\nLocation: Chicago, IL\n\nFirst entry body\n")
	plan, err := PlanMigration(root)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Groups[0].Entries[0].ExistsIdentical {
		t.Error("entry should be marked ExistsIdentical")
	}
}

func TestPlanMigrationRejectsDuplicateDestination(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "2024", "2024-01-01.txt"),
		"## Monday 2024-01-01 9:00 AM CST\n\nA\n\n## Monday 2024-01-01 9:00 AM CST\n\nB\n")
	if _, err := PlanMigration(root); err == nil || !strings.Contains(err.Error(), "same destination") {
		t.Fatalf("err = %v, want duplicate destination error", err)
	}
}

func TestPlanMigrationFailsEntirelyOnMalformedFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "2024", "2024-01-01.txt"), twoEntryDay)
	writeFile(t, filepath.Join(root, "2024", "2024-01-02.txt"), "\n## Broken header\n\nbody\n")

	if _, err := PlanMigration(root); err == nil {
		t.Fatal("expected error for malformed file")
	}
	// The good file must not have been touched.
	if _, err := os.Stat(filepath.Join(root, "2024", "2024-01-01.txt")); err != nil {
		t.Errorf("source vanished during failed plan: %v", err)
	}
	dayDir := filepath.Join(root, "2024", "01")
	if _, err := os.Stat(dayDir); !os.IsNotExist(err) {
		t.Error("failed plan must not create destination directories")
	}
}

func TestApplyMigration(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "2024", "2024-01-01.txt")
	writeFile(t, src, twoEntryDay)

	plan, err := PlanMigration(root)
	if err != nil {
		t.Fatal(err)
	}
	var logged strings.Builder
	if err := ApplyMigration(plan, func(f string, a ...any) { fmt.Fprintf(&logged, f, a...) }); err != nil {
		t.Fatal(err)
	}

	for _, g := range plan.Groups {
		if _, err := os.Stat(g.Source); !os.IsNotExist(err) {
			t.Errorf("source %s not deleted", g.Source)
		}
		for _, e := range g.Entries {
			dat, err := os.ReadFile(e.DestPath)
			if err != nil {
				t.Fatalf("destination missing: %v", err)
			}
			if string(dat) != e.Content {
				t.Errorf("dest %s = %q, want %q", e.DestPath, dat, e.Content)
			}
		}
	}

	// Re-running after a simulated interruption: source is gone, so nothing happens.
	plan2, err := PlanMigration(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.Total != 0 {
		t.Errorf("re-plan after apply found %d entries, want 0", plan2.Total)
	}
}

func TestApplyMigrationCompletesAfterInterruption(t *testing.T) {
	root := t.TempDir()
	ts1 := mustParseHeader(t, "Monday 2024-01-01 9:19 AM CST")
	dest1 := filepath.Join(root, "2024", "01", "01", fmtEpoch(ts1)+".md")
	// Simulate a prior run that wrote the first entry but crashed before deleting the source.
	writeFile(t, dest1, "## Monday 2024-01-01 9:19 AM CST\nLocation: Chicago, IL\n\nFirst entry body\n")
	src := filepath.Join(root, "2024", "2024-01-01.txt")
	writeFile(t, src, twoEntryDay)

	plan, err := PlanMigration(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigration(plan, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source should be deleted once all entries are verified on disk")
	}
}

func TestBuildEntryFile(t *testing.T) {
	tz := time.FixedZone("CST", -6*60*60)
	ts := time.Date(2024, 1, 1, 9, 19, 0, 0, tz)
	got := BuildEntryFile(ts, "Chicago, IL", "Body text")
	want := "## Monday 2024-01-01 9:19 AM CST\nLocation: Chicago, IL\n\nBody text\n"
	if got != want {
		t.Errorf("BuildEntryFile() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func fmtEpoch(ts time.Time) string {
	return strconv.FormatInt(ts.Unix(), 10)
}

// mustParseHeader parses a header timestamp exactly the way the migrator does.
func mustParseHeader(t *testing.T, header string) time.Time {
	t.Helper()
	ts, err := time.Parse(timestampLayout, header)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}
