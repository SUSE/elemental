package action

import (
	"log"

	"github.com/urfave/cli/v2"
)

func Version(*cli.Context) error {
	log.Print("devel")

	return nil
}
