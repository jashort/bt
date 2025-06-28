package main

import (
	"bt/cmd"
	"fmt"
	"github.com/alecthomas/kong"
	"github.com/alecthomas/kong-toml"
	"github.com/allan-simon/go-singleinstance"
	"log"
	"os"
)

var CLI struct {
	DataDir string      `help:"Data directory for the application" default:"~/data/Blog" type:"path"`
	Debug   bool        `help:"Print debugging info to stderr"`
	Add     cmd.AddCmd  `cmd:"" help:"Add entry" default:"withargs"`
	View    cmd.ViewCmd `cmd:"" help:"View entries"`
	Edit    cmd.EditCmd `cmd:"" help:"Edit entries"`
}

func main() {
	// Check for existing instance
	homeDir, err := os.UserHomeDir()
	lockFile, err := singleinstance.CreateLockFile(homeDir + "/.bt.lock")
	if err != nil {
		fmt.Println(`Already running (file "~/.bt.lock" is locked)`)
		return
	}
	defer func(lockFile *os.File) {
		err := lockFile.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(lockFile)

	ctx := kong.Parse(&CLI, kong.Configuration(kongtoml.Loader, "~/data/bt/config.toml", "~/.config/bt/config.toml"))
	if CLI.Debug {
		_, _ = os.Stderr.WriteString("debug mode enabled\n")
		_, _ = os.Stderr.WriteString(fmt.Sprintf("DataDir: %s\n", CLI.DataDir))
	}
	err = ctx.Run(&cmd.Context{DataDir: CLI.DataDir, Debug: CLI.Debug})
	ctx.FatalIfErrorf(err)

}
