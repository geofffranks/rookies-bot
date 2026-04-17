package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/disgoorg/snowflake/v2"
	"gopkg.in/yaml.v3"
)

type Penalty struct {
	QualiBansR1 []int `yaml:"quali_bans_r1"`
	QualiBansR2 []int `yaml:"quali_bans_r2"`
	PitStartsR1 []int `yaml:"pit_starts_r1"`
	PitStartsR2 []int `yaml:"pit_starts_r2"`
}

type Round struct {
	Number             int    `yaml:"number"`
	Track              string `yaml:"track"`
	PenaltyTrackerLink string `yaml:"penalty_tracker_link"`
}

func (r Round) String() string {
	return fmt.Sprintf("Round %d - %s", r.Number, r.Track)
}

type BotConfig struct {
	SimGridApiToken string `yaml:"simgrid_api_token"`
	ChampionshipId  string `yaml:"championship_id"`
	Season          string `yaml:"season"`

	GoogleServiceAccountToken string `yaml:"service_account_token_file"`
	BriefingTemplateDocID     string `yaml:"briefing_template_doc_id"`
	BriefingFolderID          string `yaml:"briefing_folder_id"`
	TrackerTemplateDocID      string `yaml:"tracker_template_doc_id"`
	TrackerFolderID           string `yaml:"tracker_folder_id"`

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
	if roundConfigPath != "" {
		err = loadFile(roundConfigPath, roundConfig)
		if err != nil {
			return nil, err
		}
	}

	//FIXME: add some validation to the config for empty
	config := &Config{
		BotConfig:   *botConfig,
		RoundConfig: *roundConfig,
	}
	if err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", config.GoogleServiceAccountToken); err != nil {
		return nil, fmt.Errorf("failed setting GOOGLE_APPLICATION_CREDENTIALS: %w", err)
	}
	return config, nil
}

func LoadRoundConfig(content []byte) (*RoundConfig, error) {
	roundConfig := &RoundConfig{}

	content = []byte(strings.Replace(string(content), "\t", "    ", -1))

	err := yaml.Unmarshal(content, roundConfig)
	if err != nil {
		return nil, fmt.Errorf("failed parsing YAML data: %s. Use something like https://yaml-online-parser.appspot.com to find the syntax error and try again", err)
	}
	return roundConfig, nil
}

func loadFile(file string, config interface{}) error {
	data, err := os.ReadFile(file) // #nosec G304 -- operator-supplied CLI path, no external caller
	if err != nil {
		return fmt.Errorf("failed reading %s: %s", file, err)
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return fmt.Errorf("failed parsing %s: %s", file, err)
	}
	return nil
}
