package cmd

import (
	"bt/internal"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AddCmd struct {
	At       string `help:"Date/time. Ex: '2025-03-05' or 'yesterday 3:00 PM'. Default: now"`
	Location string `help:"Location string to add." type:"string"`
}

func (l *AddCmd) Run(ctx *Context) error {
	timestamp, err := internal.ParseTimestamp(l.At, time.Now())
	if err != nil {
		return err
	}

	entry, err := internal.RunEditor(internal.TimestampHeader(timestamp))
	if err != nil {
		return err
	}
	if strings.TrimSpace(entry) == "" {
		return fmt.Errorf("%s", "No entry provided")
	}

	targetFile := internal.DestinationFile(ctx.DataDir, timestamp)
	dir := filepath.Dir(targetFile)
	err = os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}

	output := "\n" + internal.TimestampHeader(timestamp)
	if _, err := os.Stat(targetFile); errors.Is(err, os.ErrNotExist) {
		output += fmt.Sprintf("Location: %s\n\n", l.Location)
	} else {
		shouldAdd, err := internal.ShouldAddLocationToFile(targetFile, l.Location)
		if err != nil {
			return err
		}
		if shouldAdd {
			output += fmt.Sprintf("Location: %s\n\n", l.Location)
		}
	}
	output += strings.TrimSpace(entry) + "\n"

	f, err := os.OpenFile(targetFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(f)

	size, err := f.WriteString(output)
	if err != nil {
		return err
	}
	fmt.Printf("Appended %d bytes to %s\n", size, targetFile)

	return nil
}
