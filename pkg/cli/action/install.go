package action

import (
	"log"

	"github.com/suse/elemental/pkg/cli/cmd"
	"github.com/urfave/cli/v2"
)

func Install(*cli.Context) error {
	args := &cmd.InstallArgs

	log.Printf("args: %+v", args)

	// Perform args & input validation, initial setup and branch off to the actual business logic

	return nil
}
