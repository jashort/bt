package cmd

import (
	"bt/internal"
	"os"
	"os/exec"
	"time"
)

type EditCmd struct {
	At string `help:"Date/time. Ex: '2025-03-05' or 'yesterday 3:00 PM'. Default: now"`
}

func (l *EditCmd) Run(ctx *Context) error {
	timestamp, err := internal.ParseTimestamp(l.At, time.Now())
	if err != nil {
		return err
	}

	targetFile := internal.DestinationFile(ctx.DataDir, timestamp)
	cmd := exec.Command("nvim", targetFile, "+star", "+")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
