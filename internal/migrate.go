package internal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Legacy day files look like <data-dir>/YYYY/YYYY-MM-DD.txt and may hold
// multiple entries per file. They are migrated to one file per entry:
// <data-dir>/YYYY/MM/DD/<unix epoch>.md.
var (
	legacyYearDirRe = regexp.MustCompile(`^\d{4}$`)
	legacyDayFileRe = regexp.MustCompile(`^(\d{4})-\d{2}-\d{2}\.txt$`)
)

// legacyFile is a discovered old-format day file.
type legacyFile struct {
	Path    string
	DayDate time.Time // Date parsed from the filename, midnight local time
}

// legacyEntry is one entry of a legacy day file.
type legacyEntry struct {
	HeaderTime     time.Time
	Location       string
	HasOwnLocation bool // A "Location: " line was present in this entry's own content
	Body           string
	Raw            bool // Headerless file: Body is the whole raw file content and no header is prepended
}

// PlannedEntry is one legacy entry with its migration plan resolved.
type PlannedEntry struct {
	SourceFile      string
	DestPath        string
	Content         string // Exact bytes to write to DestPath
	ExistsIdentical bool   // DestPath already contains exactly Content
	HeaderTime      time.Time
	Location        string
	LocationSource  string // "own", "carried", or "" for raw entries
	Raw             bool   // File is moved as-is, without splitting or a prepended header
}

// FileGroup pairs a legacy source file with its planned entries.
type FileGroup struct {
	Source  string
	Entries []*PlannedEntry
}

// MigrationPlan is the fully validated result of scanning the data dir.
// Writing anything is only allowed after PlanMigration succeeds for every
// discovered file.
type MigrationPlan struct {
	Groups  []FileGroup
	Skipped []string
	Total   int
}

// DiscoverLegacyFiles finds old-format day files directly under
// <data-dir>/YYYY/YYYY-MM-DD.txt. Unrecognized regular files in year
// directories are reported as skipped; the year directory name must match the
// date in the filename or the scan fails.
func DiscoverLegacyFiles(dataDir string) (files []legacyFile, skipped []string, err error) {
	yearEntries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, nil, err
	}
	for _, y := range yearEntries {
		if !y.IsDir() || !legacyYearDirRe.MatchString(y.Name()) {
			continue
		}
		dayEntries, err := os.ReadDir(filepath.Join(dataDir, y.Name()))
		if err != nil {
			return nil, nil, err
		}
		for _, f := range dayEntries {
			full := filepath.Join(dataDir, y.Name(), f.Name())
			if f.IsDir() {
				continue
			}
			m := legacyDayFileRe.FindStringSubmatch(f.Name())
			if m == nil {
				skipped = append(skipped, full)
				continue
			}
			if m[1] != y.Name() {
				return nil, nil, fmt.Errorf("%s: file date %s does not match directory %s", full, m[1], y.Name())
			}
			day, err := time.ParseInLocation("2006-01-02", f.Name()[:10], time.Local)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: unparsable date in filename: %w", full, err)
			}
			files = append(files, legacyFile{Path: full, DayDate: day})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, skipped, nil
}

// SplitLegacyEntries splits a legacy day file into entries using two
// deterministic rules.
//
// If the file contains timestamp headers (any format in headerLayouts), it is
// split at those lines and each header is normalized later by
// buildEntryContent. Because the file is being split, the old "location only
// when it differs" semantics are restored: an entry's location is the last
// "Location: " line in its own content (or, before the first header, the most
// recent pre-header one), and an entry without any inherits the most recent
// earlier location in the file. The remaining content below the header is
// kept as it appeared; content before the first header is kept at the top of
// the first entry's body.
//
// If the file contains no timestamp headers, it cannot be split safely, so a
// single Raw entry is returned carrying the file content as-is; it is moved
// unmodified and named for noon on the file's date, with no location
// handling of any kind.
func SplitLegacyEntries(day time.Time, content string) ([]legacyEntry, error) {
	if strings.TrimSpace(content) == "" {
		return nil, nil // Empty file: nothing to migrate
	}
	lines := strings.Split(content, "\n")

	headerIdx := []int{}
	for i, line := range lines {
		if _, err := parseHeaderLine(line); err == nil {
			headerIdx = append(headerIdx, i)
		}
	}

	if len(headerIdx) == 0 {
		return []legacyEntry{{
			HeaderTime: noonLocal(day),
			Body:       content,
			Raw:        true,
		}}, nil
	}

	var entries []legacyEntry
	for h, start := range headerIdx {
		end := len(lines)
		if h+1 < len(headerIdx) {
			end = headerIdx[h+1]
		}
		segment := lines[start+1 : end]
		if h == 0 && headerIdx[0] > 0 {
			// Pre-header content (including any pre-header "Location: " line,
			// which then simply becomes this entry's own location) is part of
			// the first entry.
			pre := append([]string{}, lines[:headerIdx[0]]...)
			segment = append(pre, segment...)
		}
		location, hasOwn, bodyLines := extractLocation(segment)
		ts, _ := parseHeaderLine(lines[start])
		entries = append(entries, legacyEntry{
			HeaderTime:     ts,
			Location:       location,
			HasOwnLocation: hasOwn,
			Body:           strings.TrimSpace(strings.Join(bodyLines, "\n")),
		})
	}
	carryDownLocations(entries)
	return entries, nil
}

// locationPrefix is how location lines start, matching the historical writer.
const locationPrefix = "Location: "

// extractLocation pulls the last "Location: " line out of an entry's content
// (the old most-recent-wins semantics); any earlier location lines stay in
// the body.
func extractLocation(segment []string) (location string, hasOwn bool, body []string) {
	lastIdx := -1
	for i, line := range segment {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, locationPrefix) {
			location = strings.TrimSpace(t[len(locationPrefix):])
			hasOwn = true
			lastIdx = i
		}
	}
	body = make([]string, 0, len(segment))
	for i, line := range segment {
		if i == lastIdx {
			continue
		}
		body = append(body, line)
	}
	return location, hasOwn, body
}

// carryDownLocations fills in entries without their own "Location: " line
// with the most recent location seen earlier in the file.
func carryDownLocations(entries []legacyEntry) {
	var last string
	for i := range entries {
		if entries[i].HasOwnLocation {
			last = entries[i].Location
			continue
		}
		entries[i].Location = last
	}
}

// headerLayouts lists the header formats historical versions have written.
// Zone-less variants (entries recorded without a timezone) are interpreted in
// the local zone so the normalized header shows a plausible wall clock.
// Slash-date headers also appear with a zero-padded hour (e.g. "07:45"), which
// Go's parser accepts under the "3" layout.
var headerLayouts = []struct {
	layout  string
	inLocal bool // layout has no zone: parse the wall clock in time.Local
}{
	{timestampLayout, false},
	{"Monday 01/02/2006 3:04 PM MST", false},
	{"Monday 2006-01-02 3:04 PM", true},
	{"Monday 01/02/2006 3:04 PM", true},
}

// parseHeaderLine returns the timestamp of a "## <timestamp>" entry header
// line, or an error if the line is not a header (including "## " lines that
// are ordinary body text).
func parseHeaderLine(line string) (time.Time, error) {
	if !strings.HasPrefix(line, "## ") {
		return time.Time{}, errors.New("not an entry header")
	}
	text := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	for _, h := range headerLayouts {
		var ts time.Time
		var err error
		if h.inLocal {
			ts, err = time.ParseInLocation(h.layout, text, time.Local)
		} else {
			ts, err = time.Parse(h.layout, text)
		}
		if err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparsable timestamp %q", text)
}

// noonLocal returns noon in the local zone on day's date.
func noonLocal(day time.Time) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, 12, 0, 0, 0, time.Local)
}

// buildEntryContent renders a legacy entry in the per-entry file format: a
// normalized header, the entry's location line, then the content as it
// appeared below the original header. Raw (headerless) entries are moved
// as-is, byte-for-byte, with no header or location line added.
func buildEntryContent(e legacyEntry) string {
	if e.Raw {
		return e.Body
	}
	var b strings.Builder
	b.WriteString(TimestampHeader(e.HeaderTime))
	b.WriteString(fmt.Sprintf("Location: %s\n\n", e.Location))
	b.WriteString(strings.TrimSpace(e.Body) + "\n")
	return b.String()
}

// PlanMigration scans the data dir, splits every legacy file, and validates
// the whole migration up front. Any duplicate destination or conflicting
// existing file aborts with an error before anything is written.
func PlanMigration(dataDir string) (*MigrationPlan, error) {
	files, skipped, err := DiscoverLegacyFiles(dataDir)
	if err != nil {
		return nil, err
	}
	plan := &MigrationPlan{Skipped: skipped}
	claimed := map[string]string{} // DestPath -> source file that claimed it
	for _, f := range files {
		dat, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, err
		}
		entries, err := SplitLegacyEntries(f.DayDate, string(dat))
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			continue
		}

		group := FileGroup{Source: f.Path}
		for _, e := range entries {
			dest := DestinationFile(dataDir, e.HeaderTime)
			if prev, ok := claimed[dest]; ok {
				return nil, fmt.Errorf("%s: %s maps to the same destination as %s", f.Path, dest, prev)
			}
			claimed[dest] = f.Path
			source := "" // Raw entries get no location handling.
			if !e.Raw {
				source = "carried"
				if e.HasOwnLocation {
					source = "own"
				}
			}
			pe := &PlannedEntry{
				SourceFile:     f.Path,
				DestPath:       dest,
				Content:        buildEntryContent(e),
				HeaderTime:     e.HeaderTime,
				Location:       e.Location,
				LocationSource: source,
				Raw:            e.Raw,
			}
			if existing, err := os.ReadFile(dest); err == nil {
				if !bytes.Equal(existing, []byte(pe.Content)) {
					return nil, fmt.Errorf("%s: destination %s already exists with different content", f.Path, dest)
				}
				pe.ExistsIdentical = true
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			group.Entries = append(group.Entries, pe)
			plan.Total++
		}
		plan.Groups = append(plan.Groups, group)
	}
	return plan, nil
}

// ApplyMigration writes every planned entry atomically, verifies each file's
// contents on disk, and only then deletes the source file. A failure stops
// immediately; files not yet processed are untouched, and re-running is safe
// because identical destinations are skipped.
func ApplyMigration(plan *MigrationPlan, logf func(format string, args ...any)) error {
	for _, g := range plan.Groups {
		for _, e := range g.Entries {
			if err := writeEntryFile(e, logf); err != nil {
				return err
			}
		}
		if err := os.Remove(g.Source); err != nil {
			return fmt.Errorf("%s: entries written but source could not be removed: %w", g.Source, err)
		}
		if logf != nil {
			logf("migrated %s (%d entries, source removed)\n", g.Source, len(g.Entries))
		}
	}
	return nil
}

// writeEntryFile atomically creates e.DestPath unless it already exists with
// identical content, then verifies what is on disk.
func writeEntryFile(e *PlannedEntry, logf func(format string, args ...any)) error {
	if !e.ExistsIdentical {
		dir := filepath.Dir(e.DestPath)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(dir, ".bt-migrate-*")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		_, err = tmp.WriteString(e.Content)
		if err == nil {
			err = tmp.Chmod(0644)
		}
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(tmpName)
			return err
		}
		if err := os.Rename(tmpName, e.DestPath); err != nil {
			_ = os.Remove(tmpName)
			return err
		}
		if logf != nil {
			logf("wrote %s\n", e.DestPath)
		}
	}
	dat, err := os.ReadFile(e.DestPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(dat, []byte(e.Content)) {
		return fmt.Errorf("%s: verification failed after write", e.DestPath)
	}
	return nil
}
