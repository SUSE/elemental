package main

import (
	"log"
	"os"

	"github.com/suse/elemental/internal/cli/action"
	"github.com/suse/elemental/internal/cli/cmd"
	"github.com/urfave/cli/v2"
)

func main() {
	app := cmd.NewApp()
	app.Commands = []*cli.Command{
		cmd.NewBuildCommand(action.Build),
		cmd.NewInstallCommand(action.Install),
		cmd.NewVersionCommand(action.Version),
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
