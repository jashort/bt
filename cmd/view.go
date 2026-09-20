package cmd

import (
	"bt/internal"
	"fmt"
	"github.com/bbrks/wrap"
	"golang.org/x/term"
	"os"
	"time"
)

type ViewCmd struct {
	At string `help:"Date/time. Ex: '2025-03-05' or 'yesterday 3:00 PM'. Default: now"`
}

func (l *ViewCmd) Run(ctx *Context) error {
	timestamp, err := internal.ParseTimestamp(l.At, time.Now())
	if err != nil {
		return err
	}

	files, err := internal.EntryFiles(ctx.DataDir, timestamp)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("No entries found for %s", timestamp.Format("2006-01-02"))
	}

	wrapper := wrap.NewWrapper()
	for _, f := range files {
		dat, err := os.ReadFile(f.Path)
		if err != nil {
			return err
		}
		fmt.Print(wrapper.Wrap(string(dat), getTerminalWidth()))
	}
	return nil
}

// getTerminalWidth returns the width of the terminal, or 80 if it can't be determined.
func getTerminalWidth() int {
	width, _, err := term.GetSize(0)
	if err != nil {
		return 80
	}
	return width
}
