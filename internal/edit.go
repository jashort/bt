package internal

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

func TimestampHeader(timestamp time.Time) string {
	return fmt.Sprintf("## %s\n", timestamp.Format("Monday 2006-01-02 3:04 PM MST"))
}

func ShouldAddLocationToFile(filename string, location string) (bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return false, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(file)
	scanner := bufio.NewScanner(file)
	return ShouldAddLocation(scanner, location)
}

func ShouldAddLocation(scanner *bufio.Scanner, location string) (bool, error) {
	var lastLocation string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "Location: ") {
			lastLocation = strings.TrimSpace(line)
		}
	}
	lastLocation = strings.TrimPrefix(lastLocation, "Location: ")
	if strings.EqualFold(lastLocation, location) {
		return false, nil
	}
	return true, nil
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

func DestinationFile(baseDir string, at time.Time) string {
	return path.Join(baseDir, at.Format("2006"), at.Format("2006-01-02")+".txt")
}
