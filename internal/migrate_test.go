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

Second entry body.
`

func TestSplitLegacyEntriesHeadered(t *testing.T) {
	day := time.Date(2024, 1, 1, 0, 0, 0, 0, time.FixedZone("CST", -6*60*60))
	entries, err := SplitLegacyEntries(day, twoEntryDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	first, second := entries[0], entries[1]

	if first.HeaderTime.Format("2006-01-02 3:04 PM") != "2024-01-01 9:19 AM" {
		t.Errorf("first header time = %q", first.HeaderTime)
	}
	if !first.HasOwnLocation || first.Location != "Chicago, IL" {
		t.Errorf("first location = %q own=%v, want Chicago, IL own=true", first.Location, first.HasOwnLocation)
	}
	if first.Body != "First entry body" {
		t.Errorf("first body = %q", first.Body)
	}
	// Split files carry the location down to entries without their own.
	if second.HasOwnLocation {
		t.Error("second entry should not have its own location")
	}
	if second.Location != "Chicago, IL" {
		t.Errorf("second location = %q, want Chicago, IL carried down", second.Location)
	}
	if second.Body != "Second entry body." {
		t.Errorf("second body = %q", second.Body)
	}
	if second.Raw || first.Raw {
		t.Error("headered entries must not be raw")
	}
}

// The most recent location wins for later entries.
func TestSplitLegacyEntriesCarryDownChain(t *testing.T) {
	data := "## Monday 2024-01-01 9:00 AM CST\nLocation: Chicago, IL\n\nA\n\n## Monday 2024-01-01 10:00 AM CST\n\nB\n\n## Monday 2024-01-01 11:00 AM CST\nLocation: Omaha, NE\n\nC\n\n## Monday 2024-01-01 12:00 PM CST\n\nD\n"
	entries, err := SplitLegacyEntries(time.Time{}, data)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Chicago, IL", "Chicago, IL", "Omaha, NE", "Omaha, NE"}
	for i, w := range want {
		if entries[i].Location != w {
			t.Errorf("entries[%d].Location = %q, want %q", i, entries[i].Location, w)
		}
	}
}

// If one entry's content contains several location lines, the last one wins
// and earlier ones stay in the body.
func TestSplitLegacyEntriesLastLocationWins(t *testing.T) {
	data := "## Monday 2024-01-01 9:00 AM CST\nLocation: First place\n\nText\nLocation: Second place\n\nMore text\n"
	entries, err := SplitLegacyEntries(time.Time{}, data)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Location != "Second place" {
		t.Errorf("location = %q, want Second place", entries[0].Location)
	}
	if !strings.Contains(entries[0].Body, "Location: First place") {
		t.Errorf("body %q should retain the earlier location line", entries[0].Body)
	}
	if strings.Contains(entries[0].Body, "Second place") {
		t.Errorf("promoted location should be removed from body %q", entries[0].Body)
	}
}

func TestSplitLegacyEntriesSlashDateHeaders(t *testing.T) {
	content := "\n## Saturday 06/28/2025 07:45 AM CDT\nLocation: Bull Shoals, AR\nSlept... not bad I think? So that's kinda cool.\n"
	entries, err := SplitLegacyEntries(time.Time{}, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].HeaderTime.Format("2006-01-02 3:04 PM") != "2025-06-28 7:45 AM" {
		t.Errorf("header time = %q, want 2025-06-28 7:45 AM", entries[0].HeaderTime)
	}
	if entries[0].Location != "Bull Shoals, AR" {
		t.Errorf("location = %q, want Bull Shoals, AR", entries[0].Location)
	}
	if entries[0].Body != "Slept... not bad I think? So that's kinda cool." {
		t.Errorf("body = %q", entries[0].Body)
	}
}

func TestSplitLegacyEntriesMixedHeaderFormats(t *testing.T) {
	content := "\n## Saturday 06/28/2025 7:45 AM CDT\n\nFirst\n\n## Saturday 2025-06-28 11:13 AM CDT\n\nSecond\n"
	entries, err := SplitLegacyEntries(time.Time{}, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	for i, e := range entries {
		if e.HeaderTime.Format("2006-01-02") != "2025-06-28" {
			t.Errorf("entries[%d] header time = %q", i, e.HeaderTime)
		}
	}
}

// Headers without a timezone must parse too; this is the exact shape of a
// real file (pre-header location + zone-less slash-date header). The
// pre-header location is inside the first entry's content, so it simply
// becomes that entry's own location.
func TestSplitLegacyEntriesZonelessHeader(t *testing.T) {
	content := "Location: Flight DL2014, RDU > MSP\n\n## Tuesday 03/14/2023 01:25 PM\nGot up a touch early today\n"
	entries, err := SplitLegacyEntries(time.Date(2023, 3, 14, 0, 0, 0, 0, time.Local), content)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.HeaderTime.Format("2006-01-02 3:04 PM") != "2023-03-14 1:25 PM" {
		t.Errorf("header time = %q, want 2023-03-14 1:25 PM", e.HeaderTime)
	}
	if e.HeaderTime.Location() != time.Local {
		t.Errorf("zone-less header should resolve to the local zone, got %v", e.HeaderTime.Location())
	}
	if e.Location != "Flight DL2014, RDU > MSP" || !e.HasOwnLocation {
		t.Errorf("location = %q own=%v, want Flight DL2014, RDU > MSP own=true", e.Location, e.HasOwnLocation)
	}
	if e.Body != "Got up a touch early today" {
		t.Errorf("body = %q", e.Body)
	}
}

func TestSplitLegacyEntriesZonelessISOHeader(t *testing.T) {
	entries, err := SplitLegacyEntries(time.Time{}, "\n## Tuesday 2023-03-14 1:25 PM\nBody only\n")
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].HeaderTime.Format("2006-01-02 3:04 PM") != "2023-03-14 1:25 PM" {
		t.Errorf("header time = %q", entries[0].HeaderTime)
	}
}

// Lines starting with "## " that are not timestamp headers must pass through
// as body text, not abort the migration.
func TestSplitLegacyEntriesPassesThroughNonHeaderLines(t *testing.T) {
	content := `
## Monday 2024-01-01 9:00 AM CST
Intro text

## A plain markdown heading

More text

## Monday 2024-13-45 9:00 AM CST

This looks like a header but the date is bogus.
`
	entries, err := SplitLegacyEntries(time.Time{}, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	want := "Intro text\n\n## A plain markdown heading\n\nMore text\n\n## Monday 2024-13-45 9:00 AM CST\n\nThis looks like a header but the date is bogus."
	if entries[0].Body != want {
		t.Errorf("body = %q, want %q", entries[0].Body, want)
	}
}

// A file whose only "## " lines are not headers has no timestamp headers and
// is moved as-is.
func TestSplitLegacyEntriesOnlyNonHeaderLines(t *testing.T) {
	day := time.Date(2023, 3, 11, 0, 0, 0, 0, time.Local)
	content := "## Some heading\n\nJust some text\n"
	entries, err := SplitLegacyEntries(day, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].Raw {
		t.Fatalf("entries = %+v, want one raw entry", entries)
	}
	if entries[0].Body != content {
		t.Errorf("raw body = %q, want the file content as-is", entries[0].Body)
	}
	if entries[0].HeaderTime.Format("2006-01-02 3:04 PM") != "2023-03-11 12:00 PM" {
		t.Errorf("raw timestamp = %q, want noon local", entries[0].HeaderTime)
	}
}

func TestSplitLegacyEntriesHeaderless(t *testing.T) {
	day := time.Date(2023, 3, 11, 0, 0, 0, 0, time.Local)
	content := "Location: Somewhere, CO\n\nText on a line\n\nLater...\n\nNew entry\n"
	entries, err := SplitLegacyEntries(day, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (whole file)", len(entries))
	}
	e := entries[0]
	if !e.Raw {
		t.Error("headerless file should be marked Raw")
	}
	if e.Body != content {
		t.Errorf("raw body = %q, want file content byte-for-byte", e.Body)
	}
	want := time.Date(2023, 3, 11, 12, 0, 0, 0, time.Local)
	if !e.HeaderTime.Equal(want) {
		t.Errorf("timestamp = %v, want noon local %v", e.HeaderTime, want)
	}
}

func TestSplitLegacyEntriesEmpty(t *testing.T) {
	entries, err := SplitLegacyEntries(time.Time{}, "\n \n")
	if err != nil || entries != nil {
		t.Errorf("SplitLegacyEntries() = %v, %v; want nil, nil for empty file", entries, err)
	}
}

func TestBuildEntryContent(t *testing.T) {
	tz := time.FixedZone("CST", -6*60*60)
	ts := time.Date(2024, 1, 1, 9, 19, 0, 0, tz)
	got := buildEntryContent(legacyEntry{HeaderTime: ts, Location: "Chicago, IL", Body: "Body text"})
	want := "## Monday 2024-01-01 9:19 AM CST\nLocation: Chicago, IL\n\nBody text\n"
	if got != want {
		t.Errorf("buildEntryContent() = %q, want %q", got, want)
	}

	raw := "Whatever the file contained\n\nas-is.\n"
	if buildEntryContent(legacyEntry{Raw: true, Body: raw}) != raw {
		t.Error("raw entries must be moved as-is with no header or location line added")
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
	// The second entry inherits the carried-down location.
	want2 := "## Monday 2024-01-01 11:13 AM CST\nLocation: Chicago, IL\n\nSecond entry body.\n"
	if g.Entries[1].Content != want2 {
		t.Errorf("content[1] = %q, want %q", g.Entries[1].Content, want2)
	}
	if g.Entries[0].LocationSource != "own" || g.Entries[1].LocationSource != "carried" {
		t.Errorf("location sources = %q, %q; want own, carried", g.Entries[0].LocationSource, g.Entries[1].LocationSource)
	}

	// Plan must not write anything.
	if _, err := os.Stat(wantDest1); !os.IsNotExist(err) {
		t.Error("planning created destination files; dry run must write nothing")
	}
}

func TestPlanMigrationHeaderlessRaw(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "2023", "2023-03-11.txt")
	content := "Location: Somewhere, CO\n\nText on a line\n\nLater...\n\nNew entry\n"
	writeFile(t, src, content)

	plan, err := PlanMigration(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Total != 1 || len(plan.Groups[0].Entries) != 1 {
		t.Fatalf("plan = %+v, want 1 entry", plan)
	}
	e := plan.Groups[0].Entries[0]
	if !e.Raw {
		t.Error("headerless file should be planned as raw")
	}
	noon := time.Date(2023, 3, 11, 12, 0, 0, 0, time.Local)
	wantDest := filepath.Join(root, "2023", "03", "11", fmtEpoch(noon)+".md")
	if e.DestPath != wantDest {
		t.Errorf("dest = %q, want %q", e.DestPath, wantDest)
	}
	if e.Content != content {
		t.Errorf("content = %q, want file content byte-for-byte", e.Content)
	}

	if err := ApplyMigration(plan, nil); err != nil {
		t.Fatal(err)
	}
	dat, err := os.ReadFile(wantDest)
	if err != nil {
		t.Fatal(err)
	}
	if string(dat) != content {
		t.Error("migrated raw file does not match source bytes")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source not deleted")
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
	ts, err := parseHeaderLine("## " + header)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}
