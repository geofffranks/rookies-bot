package config

import (
	"fmt"
	"os"

	"github.com/disgoorg/snowflake/v2"
	"gopkg.in/yaml.v3"
)

type Penalty struct {
	QualiBans   []int `yaml:"quali_bans"`
	PitStartsR1 []int `yaml:"pit_starts_r1"`
	PitStartsR2 []int `yaml:"pit_starts_r2"`
}

type Round struct {
	Number             int    `yaml:"number"`
	Track              string `yaml:"track"`
	PenaltyTrackerLink string `yaml:"penalty_tracker_link"`
}

type BotConfig struct {
	SimGridApiToken       string `yaml:"simgrid_api_token"`
	ChampionshipId        string `yaml:"championship_id"`
	Season                string `yaml:"season"`
	BriefingTemplateDocID string `yaml:"briefing_template_doc_id"`
	BriefingFolderID      string `yaml:"briefing_folder_id"`
	TrackerTemplateDocID  string `yaml:"tracker_template_doc_id"`
	TrackerFolderID       string `yaml:"tracker_folder_id"`

	DiscordToken             string       `yaml:"discord_token"`
	DiscordChannelId         snowflake.ID `yaml:"discord_channel_id"`
	DiscordRoleName          string       `yaml:"discord_role_name"`
	DiscordBriefingChannelId snowflake.ID `yaml:"discord_briefing_channel_id"`
}

type RoundConfig struct {
	Penalties            Penalty `yaml:"penalties"`
	CarriedOverPenalties Penalty `yaml:"penalties_carried_over"`
	NextRound            Round   `yaml:"next_round"`
	PreviousRound        Round   `yaml:"previous_round"`
}

type Config struct {
	BotConfig
	RoundConfig
}

func Load(botConfigPath, roundConfigPath string) (*Config, error) {
	botConfig := &BotConfig{}
	err := loadFile(botConfigPath, botConfig)
	if err != nil {
		return nil, err
	}

	roundConfig := &RoundConfig{}
	err = loadFile(roundConfigPath, roundConfig)
	if err != nil {
		return nil, err
	}

	//FIXME: add some validation to the config for empty
	config := &Config{
		BotConfig:   *botConfig,
		RoundConfig: *roundConfig,
	}
	return config, nil
}

func loadFile(file string, config interface{}) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("Failed reading %s: %s", file, err)
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return fmt.Errorf("Failed parsing %s: %s", file, err)
	}
	return nil
}
