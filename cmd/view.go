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

	dat, err := os.ReadFile(internal.DestinationFile(ctx.DataDir, timestamp))
	if err != nil {
		return err
	}

	wrapper := wrap.NewWrapper()
	fmt.Print(wrapper.Wrap(string(dat), getTerminalWidth()))
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
