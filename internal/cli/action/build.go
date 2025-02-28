package action

import (
	"log"

	"github.com/suse/elemental/internal/cli/cmd"
	"github.com/urfave/cli/v2"
)

func Build(*cli.Context) error {
	args := &cmd.BuildArgs

	log.Printf("args: %+v", args)

	// Perform args & input validation, initial setup and branch off to the actual business logic

	return nil
}
