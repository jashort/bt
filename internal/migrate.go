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
	HeaderTime time.Time
	Body       string
	Raw        bool // Headerless file: Body is the whole raw file content and no header is prepended
}

// PlannedEntry is one legacy entry with its migration plan resolved.
type PlannedEntry struct {
	SourceFile      string
	DestPath        string
	Content         string // Exact bytes to write to DestPath
	ExistsIdentical bool   // DestPath already contains exactly Content
	HeaderTime      time.Time
	Raw             bool // File is moved as-is, without splitting or a prepended header
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
// deterministic rules; nothing is inferred and no data is synthesized.
//
// If the file contains timestamp headers (any format in headerLayouts), it is
// split at those lines. Each entry's body is the content below its header as
// it appears in the file — Location lines stay exactly where they are, and
// nothing is carried down between entries. Content before the first header is
// kept at the top of the first entry's body.
//
// If the file contains no timestamp headers, it cannot be split safely, so a
// single Raw entry is returned carrying the file content as-is; it is moved
// unmodified and named for noon on the file's date.
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
			// Keep pre-header content at the top of the first entry, as-is.
			pre := append([]string{}, lines[:headerIdx[0]]...)
			segment = append(pre, segment...)
		}
		ts, _ := parseHeaderLine(lines[start])
		entries = append(entries, legacyEntry{
			HeaderTime: ts,
			Body:       strings.TrimSpace(strings.Join(segment, "\n")),
		})
	}
	return entries, nil
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
// normalized header followed by the content as it appeared below the original
// header. Raw (headerless) entries are moved as-is, byte-for-byte, with no
// header prepended.
func buildEntryContent(e legacyEntry) string {
	if e.Raw {
		return e.Body
	}
	return TimestampHeader(e.HeaderTime) + e.Body + "\n"
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
			pe := &PlannedEntry{
				SourceFile: f.Path,
				DestPath:   dest,
				Content:    buildEntryContent(e),
				HeaderTime: e.HeaderTime,
				Raw:        e.Raw,
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
