package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/discord"
	"github.com/geofffranks/rookies-bot/gcloud"
	"github.com/geofffranks/rookies-bot/models"
	"github.com/geofffranks/rookies-bot/simgrid"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		conf         *config.Config
		sgClient     *simgrid.SimGridClient
		penalties    *models.Penalties
		driverLookup models.DriverLookup
		dc           *discord.DiscordClient
	)

	announcePenalties := func(cCtx *cli.Context) error {
		penaltyMessage, err := dc.BuildPenaltyMessage(penalties)
		if err != nil {
			return fmt.Errorf("failed to generate penalty message: %s", err)
		}
		msg, err := dc.SendMessage(penaltyMessage)
		if err != nil {
			return fmt.Errorf("failed to send penalty announcement: %s", err)
		}
		err = dc.Repin(msg)
		if err != nil {
			log.Fatalf("failed to pin penalty announcement: %s", err)
		}
		return nil
	}

	raceSetup := func(cCtx *cli.Context) error {
		briefingDoc, err := gcloud.GenerateBriefing(conf, penalties)
		if err != nil {
			return fmt.Errorf("failed to generate briefing doc: %s", err)
		}

		briefingMessage, err := dc.BuildBriefingMessage(penalties, briefingDoc)
		if err != nil {
			return fmt.Errorf("failed to generate briefingmessage: %s", err)
		}
		msg, err := dc.SendMessage(briefingMessage)
		if err != nil {
			return fmt.Errorf("failed to send briefing announcement: %s", err)
		}
		err = dc.Repin(msg)
		if err != nil {
			return fmt.Errorf("failed to pin briefing announcement: %s", err)
		}

		err = dc.CreateBriefingEvent()
		if err != nil {
			return fmt.Errorf("failed to create briefing event: %s", err)
		}

		if conf.NextRound.Track != "" {
			nextRoundConfig, err := generateNextRoundConfig(sgClient, conf, penalties)
			if err != nil {
				return fmt.Errorf("failed to generate config for next round: %s", err)
			}
			data, err := yaml.Marshal(nextRoundConfig)
			if err != nil {
				return fmt.Errorf("failed to convert next round config to yaml: %s", err)
			}

			file := strings.ToLower(fmt.Sprintf("%s-round-%d-%s.yml", conf.Season, conf.NextRound.Number, strings.ReplaceAll(conf.NextRound.Track, " ", "-")))
			err = os.WriteFile(file, data, 0644)
			if err != nil {
				return fmt.Errorf("failed to write out next round config to %s: %s", file, err)
			}

		}

		fmt.Printf("DQ List:\n")
		for _, carNum := range penalties.UniqueDriverNumbers() {
			fmt.Printf("/dq %d\n", carNum)
		}
		return nil
	}

	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:        "announce-penalties",
				Usage:       "announce-penalties <roundX.yml>",
				Description: "Announces penalties to Discord",
				Args:        true,
				Action:      announcePenalties,
			},
			{
				Name:        "race-setup",
				Usage:       "race-setup <roundX.yml>",
				Description: "Generates the race briefing doc, schedules the event, announces it in discord, and sets up the next round's penalty file",
				Args:        true,
				Action:      raceSetup,
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Value: "config.yml"},
		},
		Before: func(cCtx *cli.Context) error {
			var err error
			set := flag.NewFlagSet("github.com/geofffranks/rookies-bot", 1)
			nc := cli.NewContext(cCtx.App, set, cCtx)

			roundConfig := cCtx.Args().Get(1)
			if roundConfig == "" {
				return fmt.Errorf("bad args")
			}
			conf, err = config.Load(nc.String("config"), roundConfig)
			if err != nil {
				return fmt.Errorf("could not load configs: %s", err)
			}
			sgClient = simgrid.NewClient(conf.SimGridApiToken)

			driverLookup, err = sgClient.BuildDriverLookup(conf.ChampionshipId)
			if err != nil {
				log.Fatalf("Failed building driver list: %s\n", err)
			}

			penalties, err = buildPenaltyList(driverLookup, conf)
			if err != nil {
				return fmt.Errorf("failed penalty summary: %s", err)
			}
			dc, err = discord.NewDiscordClient(conf)
			if err != nil {
				return fmt.Errorf("failed to connect to discord: %s", err)
			}
			return nil
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
	}
}

func buildPenaltyList(driverLookup models.DriverLookup, conf *config.Config) (*models.Penalties, error) {
	penalties := models.Penalties{}

	var err error
	penalties.QualiBans, err = buildPenalizedDriverList(driverLookup, conf.Penalties.QualiBans)
	if err != nil {
		return nil, err
	}
	penalties.QualiBansCarriedOver, err = buildPenalizedDriverList(driverLookup, conf.CarriedOverPenalties.QualiBans)
	if err != nil {
		return nil, err
	}
	penalties.PitStartsR1, err = buildPenalizedDriverList(driverLookup, conf.Penalties.PitStartsR1)
	if err != nil {
		return nil, err
	}
	penalties.PitStartsR1CarriedOver, err = buildPenalizedDriverList(driverLookup, conf.CarriedOverPenalties.PitStartsR1)
	if err != nil {
		return nil, err
	}
	penalties.PitStartsR2, err = buildPenalizedDriverList(driverLookup, conf.Penalties.PitStartsR2)
	if err != nil {
		return nil, err
	}
	penalties.PitStartsR2CarriedOver, err = buildPenalizedDriverList(driverLookup, conf.CarriedOverPenalties.PitStartsR2)
	if err != nil {
		return nil, err
	}

	// FIXME: throw an error if a driver is serving both a carried over and new penalty
	return &penalties, nil
}

func buildPenalizedDriverList(driverLookup models.DriverLookup, carNumbers []int) ([]models.Driver, error) {
	var driverList []models.Driver
	for _, carNumber := range carNumbers {
		if driver, ok := driverLookup[carNumber]; ok {
			driverList = append(driverList, driver)
		} else {
			return nil, fmt.Errorf("could not find driver %d in registered SimGrid drivers. Please double check the car number and try again. Drivers may have changed their number, or withdrawn since the last race.", carNumber)
		}
	}
	return driverList, nil
}

func generateNextRoundConfig(sgc *simgrid.SimGridClient, conf *config.Config, penalties *models.Penalties) (*config.RoundConfig, error) {
	nextRound, err := sgc.GetNextRound(conf.ChampionshipId, conf.NextRound)
	if err != nil {
		return nil, fmt.Errorf("failed getting details for next round: %s", err)
	}

	nextRoundTracker, err := gcloud.GeneratePenaltyTracker(conf)
	if err != nil {
		return nil, fmt.Errorf("failed generating penalty tracker for next round: %s", err)
	}

	conf.NextRound.PenaltyTrackerLink = nextRoundTracker

	return &config.RoundConfig{
		PreviousRound:        conf.NextRound,
		NextRound:            *nextRound,
		CarriedOverPenalties: penalties.Consolidate(),
	}, nil
}
