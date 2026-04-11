package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/discord"
	"github.com/geofffranks/rookies-bot/gcloud"

	"github.com/urfave/cli/v2"
)

// Runner holds injectable factories used by the CLI before/bot actions.
type Runner struct {
	conf             *config.Config
	dc               discord.BotDiscordClient
	loadConfig       func(string, string) (*config.Config, error)
	newGCloudClient  func(context.Context) (*gcloud.Client, error)
	newDiscordClient func(*config.Config, *gcloud.Client) (discord.BotDiscordClient, error)
	// stopChan is used to unblock the bot; nil means use os.Interrupt.
	stopChan chan os.Signal
}

func newRunner() *Runner {
	return &Runner{
		loadConfig:      config.Load,
		newGCloudClient: gcloud.NewClient,
		newDiscordClient: func(conf *config.Config, gc *gcloud.Client) (discord.BotDiscordClient, error) {
			return discord.NewDiscordClient(conf, gc)
		},
	}
}

// before is the urfave/cli Before hook. It loads config and wires up clients.
func (r *Runner) before(cCtx *cli.Context) error {
	var err error
	r.conf, err = r.loadConfig(cCtx.String("config"), "")
	if err != nil {
		return fmt.Errorf("could not load configs: %s", err)
	}

	gc, err := r.newGCloudClient(cCtx.Context)
	if err != nil {
		return fmt.Errorf("failed to connect to Google APIs: %s", err)
	}

	r.dc, err = r.newDiscordClient(r.conf, gc)
	if err != nil {
		return fmt.Errorf("failed to connect to discord: %s", err)
	}
	return nil
}

// bot is the urfave/cli action for the "bot" subcommand.
func (r *Runner) bot(_ *cli.Context) error {
	ctx := context.TODO()
	if err := r.dc.OpenGateway(ctx); err != nil {
		return err
	}

	fmt.Printf("rookies-bot is now running. Press CTRL+C to exit.\n")

	stop := r.stopChan
	if stop == nil {
		stop = make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)
	}
	<-stop

	r.dc.Close(ctx)
	return nil
}
