package internal

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const timestampLayout = "Monday 2006-01-02 3:04 PM MST"

func TimestampHeader(timestamp time.Time) string {
	return fmt.Sprintf("## %s\n", timestamp.Format(timestampLayout))
}

// RunEditor creates a temporary file containing header and runs nvim to edit it.
// On close, it strips the header and returns the content as a string,
// or an error if nothing was changed.
func RunEditor(header string) (string, error) {
	tempFile, err := os.CreateTemp("", "blog*.md")
	if err != nil {
		return "", err
	}
	defer func(file *os.File) {
		err := os.Remove(file.Name())
		if err != nil {
			log.Fatal(err)
		}
	}(tempFile)
	_, err = tempFile.WriteString(header + "\n")
	if err != nil {
		return "", err
	}
	err = tempFile.Close()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("nvim", tempFile.Name(), "+star", "+")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	dat, err := os.ReadFile(tempFile.Name())
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(strings.TrimPrefix(string(dat), header)), nil
}

// DestinationFile returns the path a new entry made at the given time should be saved to.
func DestinationFile(baseDir string, at time.Time) string {
	dayDir := path.Join(baseDir, at.Format("2006"), at.Format("01"), at.Format("02"))
	return path.Join(dayDir, fmt.Sprintf("%d.md", at.Unix()))
}

// LatestEntryFile returns the path of the most recent entry file saved on the
// day of the given timestamp, or "" if there is no entry that day.
func LatestEntryFile(baseDir string, at time.Time) (string, error) {
	files, err := EntryFiles(baseDir, at)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	return files[len(files)-1].Path, nil
}

// EntryFile is a single entry file saved on a given day.
type EntryFile struct {
	Path   string
	Epoch  int64     // Seconds since unix epoch, from the filename
	Header time.Time // Timestamp from the header row; zero if it could not be parsed
}

// EntryFiles returns all entry files saved on the day of the given timestamp,
// sorted by the timestamp in their header row. Files without a parseable
// header fall back to their filename epoch.
func EntryFiles(baseDir string, at time.Time) ([]EntryFile, error) {
	dayDir := path.Join(baseDir, at.Format("2006"), at.Format("01"), at.Format("02"))
	dirEntries, err := os.ReadDir(dayDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]EntryFile, 0, len(dirEntries))
	for _, e := range dirEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		epoch, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".md"), 10, 64)
		if err != nil {
			continue
		}
		f := EntryFile{Path: path.Join(dayDir, e.Name()), Epoch: epoch}
		if dat, err := os.ReadFile(f.Path); err == nil {
			if ts, err := parseHeaderTimestamp(string(dat)); err == nil {
				f.Header = ts
			}
		}
		files = append(files, f)
	}
	key := func(f EntryFile) time.Time {
		if !f.Header.IsZero() {
			return f.Header
		}
		return time.Unix(f.Epoch, 0)
	}
	sort.SliceStable(files, func(i, j int) bool {
		return key(files[i]).Before(key(files[j]))
	})
	return files, nil
}

// parseHeaderTimestamp parses the "## ..." header row of an entry file.
func parseHeaderTimestamp(content string) (time.Time, error) {
	header := content
	if i := strings.IndexByte(content, '\n'); i >= 0 {
		header = content[:i]
	}
	header = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(header), "## "))
	return time.Parse(timestampLayout, header)
}
