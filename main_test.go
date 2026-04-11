package main

import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/discord"
	"github.com/geofffranks/rookies-bot/discord/fakes"
	"github.com/geofffranks/rookies-bot/gcloud"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"
)

func newTestCLIContext() *cli.Context {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Value: "config.yml"},
		},
	}
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	_ = set.String("config", "config.yml", "")
	return cli.NewContext(app, set, nil)
}

var _ = Describe("Runner.before", func() {
	var (
		r        *Runner
		cCtx     *cli.Context
		fakeDC   *fakes.FakeBotDiscordClient
		testConf *config.Config
	)

	BeforeEach(func() {
		fakeDC = new(fakes.FakeBotDiscordClient)
		testConf = &config.Config{}
		r = &Runner{
			loadConfig: func(_, _ string) (*config.Config, error) {
				return testConf, nil
			},
			newGCloudClient: func(_ context.Context) (*gcloud.Client, error) {
				return nil, nil
			},
			newDiscordClient: func(_ *config.Config, _ *gcloud.Client) (discord.BotDiscordClient, error) {
				return fakeDC, nil
			},
		}
		cCtx = newTestCLIContext()
	})

	It("returns an error wrapping 'could not load configs' when config loading fails", func() {
		r.loadConfig = func(_, _ string) (*config.Config, error) {
			return nil, errors.New("disk full")
		}
		err := r.before(cCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("could not load configs"))
	})

	It("returns an error wrapping 'failed to connect to Google APIs' when gcloud fails", func() {
		r.newGCloudClient = func(_ context.Context) (*gcloud.Client, error) {
			return nil, errors.New("no credentials")
		}
		err := r.before(cCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to connect to Google APIs"))
	})

	It("returns an error wrapping 'failed to connect to discord' when discord creation fails", func() {
		r.newDiscordClient = func(_ *config.Config, _ *gcloud.Client) (discord.BotDiscordClient, error) {
			return nil, errors.New("bad token")
		}
		err := r.before(cCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to connect to discord"))
	})

	It("sets r.conf and r.dc on success", func() {
		err := r.before(cCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.conf).To(Equal(testConf))
		Expect(r.dc).To(Equal(fakeDC))
	})
})

var _ = Describe("Runner.bot", func() {
	var (
		r      *Runner
		fakeDC *fakes.FakeBotDiscordClient
		stop   chan os.Signal
	)

	BeforeEach(func() {
		fakeDC = new(fakes.FakeBotDiscordClient)
		stop = make(chan os.Signal, 1)
		r = &Runner{
			dc:       fakeDC,
			stopChan: stop,
		}
	})

	It("returns an error when OpenGateway fails", func() {
		fakeDC.OpenGatewayReturns(errors.New("gateway error"))
		err := r.bot(nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("gateway error"))
	})

	It("calls Close and returns nil after receiving the stop signal", func() {
		stop <- os.Interrupt
		err := r.bot(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeDC.CloseCallCount()).To(Equal(1))
	})
})
