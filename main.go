package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	r := newRunner()

	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:        "bot",
				Usage:       "bot",
				Description: "Starts a long-running discord bot for rookies-bot",
				Action:      r.bot,
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Value: "config.yml"},
		},
		Before: r.before,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
	}
}
