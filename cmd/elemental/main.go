package main

import (
	"log"
	"os"

	"github.com/suse/elemental/pkg/cli/cmd"
	"github.com/urfave/cli/v2"
)

func main() {
	app := cmd.NewApp()
	app.Commands = []*cli.Command{}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
