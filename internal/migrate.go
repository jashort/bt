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
	locationPrefix  = "Location: "
)

// legacyFile is a discovered old-format day file.
type legacyFile struct {
	Path    string
	DayDate time.Time // Date parsed from the filename, midnight local time
}

// legacyEntry is a single entry extracted from a legacy day file, before a
// destination is assigned.
type legacyEntry struct {
	HeaderTime     time.Time
	Location       string
	HasOwnLocation bool
	Body           string
	Flags          []string
}

// PlannedEntry is one legacy entry with its migration plan resolved.
type PlannedEntry struct {
	SourceFile      string
	DestPath        string
	Content         string // Exact bytes to write to DestPath
	ExistsIdentical bool   // DestPath already contains exactly Content
	HeaderTime      time.Time
	Location        string
	LocationSource  string // "own", "carried", or "none"
	Flags           []string
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

// ParseLegacyDayFile splits a legacy day file into entries.
//
// Only "## <timestamp>" lines that parse with one of headerLayouts open a
// new entry; other lines starting with "## " are body text and pass through
// unchanged. Before the first entry header only blank lines and
// "Location: " lines are allowed: the last such location line becomes the
// first entry's location, and any other pre-header lines are preserved at
// the top of the first entry's body with a review flag. Anything else before
// the first header is malformed and returns an error naming the file and
// line. Each entry's location is the last "Location: " line in its segment;
// any earlier ones stay in the body.
//
// Files with no entry headers at all contain no machine-readable entry
// boundaries, so the whole file is treated as a single entry stamped at noon
// on the file's date. The first "Location: " line becomes the entry's
// location; later ones stay in the body and produce a review flag.
func ParseLegacyDayFile(path string, day time.Time, content string) ([]legacyEntry, error) {
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
		entry := parseHeaderlessEntry(day, content)
		return []legacyEntry{entry}, nil
	}

	// Before the first header, only blank lines and "Location: " lines are
	// tolerated; a pre-header location belongs to the first entry.
	for i, line := range lines[:headerIdx[0]] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, locationPrefix) {
			continue
		}
		return nil, fmt.Errorf("%s:%d: content before first entry header", path, i+1)
	}

	var entries []legacyEntry
	for h, start := range headerIdx {
		end := len(lines)
		if h+1 < len(headerIdx) {
			end = headerIdx[h+1]
		}
		entry := extractEntryBody(lines[start+1 : end])
		entry.HeaderTime, _ = parseHeaderLine(lines[start])
		entries = append(entries, entry)
	}
	applyPreHeader(lines[:headerIdx[0]], entries)
	return entries, nil
}

// applyPreHeader folds the pre-header region of a file into the first entry:
// the last pre-header "Location: " line becomes the first entry's location
// (unless the entry has its own, which wins), and every other pre-header
// line is preserved at the top of the first entry's body. Pre-header content
// other than blank and location lines is rejected by the caller.
func applyPreHeader(pre []string, entries []legacyEntry) {
	if len(entries) == 0 {
		return
	}
	lastLocIdx, locCount := -1, 0
	for i, line := range pre {
		if t := strings.TrimSpace(line); t != "" && strings.HasPrefix(t, locationPrefix) {
			lastLocIdx, locCount = i, locCount+1
		}
	}
	if lastLocIdx < 0 {
		return
	}
	first := &entries[0]
	promoted := strings.TrimSpace(strings.TrimSpace(pre[lastLocIdx])[len(locationPrefix):])
	keepAll := false
	if first.HasOwnLocation {
		// The entry's own location is more specific; keep the pre-header
		// line visible in the body rather than dropping it.
		keepAll = true
		first.Flags = append(first.Flags, "review: pre-header location line kept in body; entry has its own")
	}
	var keep []string
	for i, line := range pre {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || (i == lastLocIdx && !keepAll) {
			continue
		}
		keep = append(keep, trimmed)
	}
	if !keepAll {
		first.Location = promoted
		first.HasOwnLocation = true
	}
	if locCount > 1 {
		first.Flags = append(first.Flags, "review: multiple location lines before the first entry header; used the last")
	}
	if len(keep) > 0 {
		first.Body = strings.TrimSpace(strings.Join(keep, "\n") + "\n" + first.Body)
	}
}

// headerLayouts lists the header formats historical versions have written.
// Zone-less variants (entries recorded without a timezone) are interpreted in
// the local zone so the regenerated header shows a plausible wall clock.
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

// parseHeaderlessEntry treats the entire content as one entry stamped at noon
// on the file's date.
func parseHeaderlessEntry(day time.Time, content string) legacyEntry {
	entry := legacyEntry{
		HeaderTime:     time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.Local),
		HasOwnLocation: true,
	}
	var body []string
	found := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), locationPrefix) {
			if found == 0 {
				entry.Location = strings.TrimSpace(strings.TrimSpace(line)[len(locationPrefix):])
			} else {
				// Not attributable to any entry boundary; keep it visible in the body.
				entry.Flags = append(entry.Flags, "review: multiple location lines; only the first was promoted")
				body = append(body, line)
			}
			found++
			continue
		}
		body = append(body, line)
	}
	if found == 0 {
		entry.HasOwnLocation = false
	}
	entry.Body = strings.TrimSpace(strings.Join(body, "\n"))
	return entry
}

// extractEntryBody pulls the last "Location: " line out of a headered entry's
// segment and returns the trimmed body. Earlier location lines stay in the
// body.
func extractEntryBody(segment []string) legacyEntry {
	entry := legacyEntry{HasOwnLocation: true}
	var body []string
	found := 0
	for _, line := range segment {
		if strings.HasPrefix(strings.TrimSpace(line), locationPrefix) {
			entry.Location = strings.TrimSpace(strings.TrimSpace(line)[len(locationPrefix):])
			found++
			continue
		}
		body = append(body, line)
	}
	if found == 0 {
		entry.HasOwnLocation = false
	} else if found > 1 {
		entry.Flags = append(entry.Flags, fmt.Sprintf("review: %d location lines in one entry; kept the last", found))
	}
	entry.Body = strings.TrimSpace(strings.Join(body, "\n"))
	return entry
}

// carryDownLocations fills in entries that have no "Location: " line with the
// most recent location seen earlier in the file, reproducing the old
// "only when it differs" writing behavior.
func carryDownLocations(entries []legacyEntry) {
	var last string
	seen := false
	for i := range entries {
		if entries[i].HasOwnLocation {
			last = entries[i].Location
			seen = true
		} else if seen {
			entries[i].Location = last
		}
	}
}

// BuildEntryFile renders an entry exactly the way `bt add` writes it.
func BuildEntryFile(ts time.Time, location string, body string) string {
	var b strings.Builder
	b.WriteString(TimestampHeader(ts))
	b.WriteString(fmt.Sprintf("Location: %s\n\n", location))
	b.WriteString(strings.TrimSpace(body) + "\n")
	return b.String()
}

// PlanMigration scans the data dir, parses every legacy file, and validates
// the whole migration up front. Any parse error, duplicate destination, or
// conflicting existing file aborts with an error before anything is written.
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
		entries, err := ParseLegacyDayFile(f.Path, f.DayDate, string(dat))
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			continue
		}
		carryDownLocations(entries)

		group := FileGroup{Source: f.Path}
		for _, e := range entries {
			source := "carried"
			switch {
			case e.HasOwnLocation:
				source = "own"
			case e.Location == "":
				source = "none"
			}
			content := BuildEntryFile(e.HeaderTime, e.Location, e.Body)
			dest := DestinationFile(dataDir, e.HeaderTime)
			if prev, ok := claimed[dest]; ok {
				return nil, fmt.Errorf("%s: %s maps to the same destination as %s", f.Path, dest, prev)
			}
			claimed[dest] = f.Path
			pe := &PlannedEntry{
				SourceFile:     f.Path,
				DestPath:       dest,
				Content:        content,
				HeaderTime:     e.HeaderTime,
				Location:       e.Location,
				LocationSource: source,
				Flags:          e.Flags,
			}
			if existing, err := os.ReadFile(dest); err == nil {
				if !bytes.Equal(existing, []byte(content)) {
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
