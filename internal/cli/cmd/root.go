package cmd

import (
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

func appName() string {
	return filepath.Base(os.Args[0])
}

func NewApp() *cli.App {
	app := cli.NewApp()

	app.Name = appName()
	app.Usage = "Build, install and upgrade infrastructure platforms"
	app.Suggest = true

	return app
}
