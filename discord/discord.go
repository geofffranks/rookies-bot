package discord

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/gcloud"
	"github.com/geofffranks/rookies-bot/models"
	"github.com/geofffranks/rookies-bot/simgrid"
	"gopkg.in/yaml.v3"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"
)

var adminUsers = []snowflake.ID{
	208972532068515840, // porkchop
	371787234187280385, // ralli
	418087017448996864, // kallil
	942149076873543721, // geoff
}

type DiscordHandleNotFoundError struct {
	Handle string
}

func (e DiscordHandleNotFoundError) Error() string {
	return e.String()
}

func (e DiscordHandleNotFoundError) Is(err error) bool {
	_, ok := err.(DiscordHandleNotFoundError)
	return ok
}

func (e DiscordHandleNotFoundError) String() string {
	return fmt.Sprintf("could not find user %s in guild. check for special characters or league abandonment", e.Handle)
}

type DiscordClient struct {
	client     bot.Client
	conf       *config.Config
	guild      snowflake.ID
	memberList map[string]snowflake.ID
}

func downloadAttachment(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return []byte{}, err
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	return content, err
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

func getRoundConfig(event *events.MessageCreate) (*config.RoundConfig, error) {
	attachments := event.Message.Attachments

	if len(attachments) == 0 {
		return nil, fmt.Errorf("no race penalty YAML file was attached to this request")
	}

	if len(attachments) > 1 {
		return nil, fmt.Errorf("too many attachments were included on the request, please only submit one race penalty YAML file")
	}
	fileContent, err := downloadAttachment(attachments[0].URL)
	if err != nil {
		return nil, fmt.Errorf("unexpected error downloading the attached file: %s", err)
	}

	roundConfig, err := config.LoadRoundConfig(fileContent)
	if err != nil {
		return nil, fmt.Errorf("unable to parse race penalty YAML file: %s", err)
	}

	return roundConfig, nil
}
func buildPenaltyList(driverLookup models.DriverLookup, conf *config.RoundConfig) (*models.Penalties, error) {
	penalties := models.Penalties{}

	var err error
	penalties.QualiBansR1, err = buildPenalizedDriverList(driverLookup, conf.Penalties.QualiBansR1)
	if err != nil {
		return nil, err
	}
	penalties.QualiBansR1CarriedOver, err = buildPenalizedDriverList(driverLookup, conf.CarriedOverPenalties.QualiBansR1)
	if err != nil {
		return nil, err
	}
	penalties.QualiBansR2, err = buildPenalizedDriverList(driverLookup, conf.Penalties.QualiBansR2)
	if err != nil {
		return nil, err
	}
	penalties.QualiBansR2CarriedOver, err = buildPenalizedDriverList(driverLookup, conf.CarriedOverPenalties.QualiBansR2)
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

func isAllowedUser(userId snowflake.ID) bool {
	for _, id := range adminUsers {
		if userId == id {
			return true
		}
	}
	return false
}
func (d *DiscordClient) onMessageCreate(event *events.MessageCreate) {
	if !isAllowedUser(event.Message.Author.ID) {
		return
	}

	switch event.Message.Content {
	case "!announce-penalties":
		d.announcePenalties(event)
	case "!race-setup":
		d.raceSetup(event)
	}
}

func sendBotResponse(event *events.MessageCreate, msg, attachment string) {
	if msg != "" {
		dm := discord.MessageCreate{
			Content: msg,
			// Reply to the original message by using MessageReference
			MessageReference: &discord.MessageReference{
				MessageID: &event.Message.ID,
			},
		}
		if attachment != "" {
			file, err := os.Open(attachment)
			if err != nil {
				fmt.Printf("Error attaching file %s: %s\n", attachment, err)
			}
			dm.Files = []*discord.File{{
				Name:   attachment,
				Reader: file,
			}}
		}
		_, err := event.Client().Rest().CreateMessage(event.ChannelID, dm)
		if err != nil {
			fmt.Println("Error sending message:", err)
		}
	} else {
		fmt.Printf("No response message content provided\n")
	}
}
func (d *DiscordClient) announcePenalties(event *events.MessageCreate) {
	var msg, attachment string
	// reply back to the sender before returning
	defer func() {
		sendBotResponse(event, msg, attachment)
	}()
	roundConfig, err := getRoundConfig(event)
	if err != nil {
		msg = fmt.Sprintf("Failed getting race config: %s", err)
		return
	}

	sgClient := simgrid.NewClient(d.conf.SimGridApiToken)

	driverLookup, err := sgClient.BuildDriverLookup(d.conf.ChampionshipId)
	if err != nil {
		msg = fmt.Sprintf("Failed building driver list: %s\n", err)
		return
	}

	penalties, err := buildPenaltyList(driverLookup, roundConfig)
	if err != nil {
		msg = fmt.Sprintf("Failed generating penalty summary: %s", err)
		return
	}

	penaltyMessage, err := d.BuildPenaltyMessage(penalties, roundConfig)
	if err != nil {
		msg = fmt.Sprintf("Failed to generate penalty message: %s", err)
		return
	}
	announcementMsg, err := d.SendMessage(penaltyMessage)
	if err != nil {
		msg = fmt.Sprintf("Failed to send penalty announcement: %s", err)
		return
	}
	err = d.Repin(announcementMsg)
	if err != nil {
		msg = fmt.Sprintf("Failed to pin penalty announcement: %s", err)
		return
	}

	msg = fmt.Sprintf("Ok, I have announced penalties from %s", roundConfig.PreviousRound)
}

func (d *DiscordClient) raceSetup(event *events.MessageCreate) {
	var msg, attachment string
	// reply back to the sender before returning
	defer func() {
		sendBotResponse(event, msg, attachment)
	}()

	roundConfig, err := getRoundConfig(event)
	if err != nil {
		msg = fmt.Sprintf("Failed getting race config: %s", err)
		return
	}

	sgClient := simgrid.NewClient(d.conf.SimGridApiToken)

	driverLookup, err := sgClient.BuildDriverLookup(d.conf.ChampionshipId)
	if err != nil {
		msg = fmt.Sprintf("Failed building driver list: %s\n", err)
		return
	}

	penalties, err := buildPenaltyList(driverLookup, roundConfig)
	if err != nil {
		msg = fmt.Sprintf("Failed generating penalty summary: %s", err)
		return
	}
	bigConfig := &config.Config{
		RoundConfig: *roundConfig,
		BotConfig:   d.conf.BotConfig,
	}
	briefingDoc, err := gcloud.GenerateBriefing(bigConfig, penalties)
	if err != nil {
		msg = fmt.Sprintf("failed to generate briefing doc: %s", err)
		return
	}

	var nextConfigFileName string
	var nextRoundConfig *config.RoundConfig
	if roundConfig.NextRound.Track != "" {
		nextRoundConfig, err = generateNextRoundConfig(sgClient, bigConfig, penalties)
		if err != nil {
			msg = fmt.Sprintf("failed to generate config for next round: %s", err)
			return
		}
		data, err := yaml.Marshal(nextRoundConfig)
		if err != nil {
			msg = fmt.Sprintf("failed to convert next round config to yaml: %s", err)
			return
		}

		nextConfigFileName = strings.ToLower(fmt.Sprintf("%s-round-%d-%s.yml", d.conf.Season, roundConfig.NextRound.Number, strings.ReplaceAll(roundConfig.NextRound.Track, " ", "-")))
		err = os.WriteFile(nextConfigFileName, data, 0644)
		if err != nil {
			msg = fmt.Sprintf("failed to write out next round config to %s: %s", nextConfigFileName, err)
			return
		}

	}

	briefingMessage, err := d.BuildBriefingMessage(penalties, briefingDoc, roundConfig)
	if err != nil {
		msg = fmt.Sprintf("failed to generate briefingmessage: %s", err)
		return
	}
	announcementMsg, err := d.SendMessage(briefingMessage)
	if err != nil {
		msg = fmt.Sprintf("failed to send briefing announcement: %s", err)
		return
	}
	err = d.Repin(announcementMsg)
	if err != nil {
		msg = fmt.Sprintf("failed to pin briefing announcement: %s", err)
		return
	}

	err = d.CreateBriefingEvent(roundConfig)
	if err != nil {
		msg = fmt.Sprintf("failed to create briefing event: %s", err)
		return
	}

	msg = fmt.Sprintf("Race setup for %s complete!\n", roundConfig.NextRound)
	if nextRoundConfig != nil {
		msg = fmt.Sprintf("%s\n[Penalty Tracker](%s)\n", msg, nextRoundConfig.PreviousRound.PenaltyTrackerLink)
	}

	if len(penalties.UniqueDriverNumbers()) > 0 {
		msg = fmt.Sprintf("%s\nDQ List:\n```", msg)
		for _, carNum := range penalties.UniqueDriverNumbers() {
			msg = fmt.Sprintf("%s\n/dq %d\n", msg, carNum)
		}
		msg = fmt.Sprintf("%s\n```", msg)
	}
	fmt.Printf("%s", msg)
	attachment = nextConfigFileName
}

func NewDiscordClient(conf *config.Config) (*DiscordClient, error) {
	client, err := disgo.New(conf.DiscordToken, bot.WithGatewayConfigOpts(
		gateway.WithIntents(gateway.IntentMessageContent, gateway.IntentDirectMessages),
	))
	if err != nil {
		return nil, err
	}

	dc := &DiscordClient{
		conf:   conf,
		client: client,
	}

	client.AddEventListeners(bot.NewListenerFunc(dc.onMessageCreate))
	if err != nil {
		return nil, err
	}

	return dc, nil
}

func (d *DiscordClient) OpenGateway(ctx context.Context) error {
	return d.client.OpenGateway(ctx)
}
func (d *DiscordClient) Close(ctx context.Context) {
	d.client.Close(ctx)
}

func (d *DiscordClient) BuildPenaltyMessage(penalties *models.Penalties, config *config.RoundConfig) (discord.MessageCreate, error) {
	message := fmt.Sprintf(`
🚓 **Penalties from Round %d** 🚓 

Stewarding is in from Round %d. The following penalties are to be served next week at %s:
`, config.PreviousRound.Number, config.PreviousRound.Number, config.NextRound.Track)

	penaltyMessage, err := d.generatePenaltyMessage(penalties, config)
	if err != nil {
		return discord.MessageCreate{}, err
	}
	message += penaltyMessage

	return buildMessage(message), nil

}

func (d *DiscordClient) BuildBriefingMessage(penalties *models.Penalties, briefingUrl string, config *config.RoundConfig) (discord.MessageCreate, error) {
	role, err := d.lookupRole(d.conf.DiscordRoleName)
	if err != nil {
		return discord.MessageCreate{}, err
	}

	briefingTime, err := d.briefingTime()
	if err != nil {
		return discord.MessageCreate{}, err
	}

	message := fmt.Sprintf(`
🏎 **It's Race Day!!** 🏎

<@&%s> **Mandatory** drivers' briefing is at <t:%d>. Here's the [briefing doc](%s) for Round %d.

**Penalties to be Served This Week**
`, role.ID, briefingTime.Unix(), briefingUrl, config.NextRound.Number)

	penaltyMessage, err := d.generatePenaltyMessage(penalties, config)
	if err != nil {
		return discord.MessageCreate{}, err
	}
	message += penaltyMessage

	return buildMessage(message), nil
}

func (d *DiscordClient) SendMessage(message discord.MessageCreate) (*discord.Message, error) {
	return d.client.Rest().CreateMessage(d.conf.DiscordChannelId, message)
}

func (d *DiscordClient) Repin(message *discord.Message) error {
	pinnedMessages, err := d.client.Rest().GetPinnedMessages(d.conf.DiscordChannelId)
	if err != nil {
		return err
	}
	for _, msg := range pinnedMessages {
		if msg.Author.ID == d.client.ApplicationID() {
			err := d.client.Rest().UnpinMessage(d.conf.DiscordChannelId, msg.ID)
			if err != nil {
				return err
			}
		}
	}

	return d.client.Rest().PinMessage(d.conf.DiscordChannelId, message.ID)
}

func (d *DiscordClient) CreateBriefingEvent(config *config.RoundConfig) error {
	guildId, err := d.getGuild()
	if err != nil {
		return err
	}
	briefingTime, err := d.briefingTime()
	if err != nil {
		return err
	}
	event := discord.GuildScheduledEventCreate{
		Name:               fmt.Sprintf("Rookies Briefing Round %d - %s", config.NextRound.Number, config.NextRound.Track),
		ChannelID:          d.conf.DiscordBriefingChannelId,
		ScheduledStartTime: briefingTime,
		PrivacyLevel:       discord.ScheduledEventPrivacyLevelGuildOnly,
		EntityType:         discord.ScheduledEventEntityTypeStageInstance,
	}
	_, err = d.client.Rest().CreateGuildScheduledEvent(guildId, event)
	return err
}

func buildMessage(message string) discord.MessageCreate {
	return discord.NewMessageCreateBuilder().SetContent(message).SetSuppressEmbeds(true).Build()
}

func (d *DiscordClient) briefingTime() (time.Time, error) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.Time{}, err
	}

	now := time.Now().In(location)

	dayOffset := (time.Monday + 7 - now.Weekday()) % 7
	targetDate := now.AddDate(0, 0, int(dayOffset))
	return time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 19, 30, 00, 00, location), nil
}

func (d *DiscordClient) getGuild() (snowflake.ID, error) {
	if d.guild != 0 {
		return d.guild, nil
	}

	channel, err := d.client.Rest().GetChannel(d.conf.DiscordChannelId)
	if err != nil {
		return 0, err
	}
	if channel.Type() == discord.ChannelTypeGuildText {
		d.guild = channel.(discord.GuildChannel).GuildID()
		return d.guild, nil
	}
	return 0, fmt.Errorf("provided DiscordChannelId was not a guild channel: %d", channel.Type())

}

func (d *DiscordClient) lookupRole(roleName string) (discord.Role, error) {
	guildId, err := d.getGuild()
	if err != nil {
		return discord.Role{}, err
	}

	roles, err := d.client.Rest().GetRoles(guildId)
	if err != nil {
		return discord.Role{}, err
	}

	for _, role := range roles {
		if role.Name == roleName {
			return role, nil
		}
	}

	return discord.Role{}, fmt.Errorf("role %s not found", roleName)
}

func (d *DiscordClient) getDriverId(handle string) (snowflake.ID, error) {
	if d.memberList == nil {
		d.memberList = map[string]snowflake.ID{}

		guildId, err := d.getGuild()
		if err != nil {
			return 0, err
		}

		var lastUser snowflake.ID
		for {
			members, err := d.client.Rest().GetMembers(guildId, 1000, lastUser)
			if err != nil {
				return 0, err
			}
			if len(members) == 0 {
				break
			}
			for _, member := range members {
				normalizedUsername := strings.Replace(strings.ToLower(member.User.Username), ".", "", -1)
				d.memberList[normalizedUsername] = member.User.ID
			}
			lastUser = members[len(members)-1].User.ID
		}
	}

	driver, ok := d.memberList[handle]
	if !ok {
		return 0, DiscordHandleNotFoundError{Handle: handle}
	}
	return driver, nil
}

func (d *DiscordClient) generatePenaltyMessage(penalties *models.Penalties, config *config.RoundConfig) (string, error) {
	message := `
**Quali Bans R1**
`
	if len(penalties.QualiBansR1CarriedOver)+len(penalties.QualiBansR1) == 0 {
		message += "- None!\n"
	} else {
		for _, driver := range penalties.QualiBansR1CarriedOver {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				if errors.Is(err, DiscordHandleNotFoundError{}) {
					message += fmt.Sprintf("- #%d %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName)
					continue
				} else {
					return "", err
				}
			}
			message += fmt.Sprintf("- <@%s> (carried over)\n", driverId)
		}
		for _, driver := range penalties.QualiBansR1 {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				if errors.Is(err, DiscordHandleNotFoundError{}) {
					message += fmt.Sprintf("- #%d %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName)
					continue
				} else {
					return "", err
				}
			}
			message += fmt.Sprintf("- <@%s>\n", driverId)
		}
	}

	message += `
**Pit Starts R1**
`
	if len(penalties.PitStartsR1CarriedOver)+len(penalties.PitStartsR1) == 0 {
		message += "- None!\n"
	} else {
		for _, driver := range penalties.PitStartsR1CarriedOver {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				if errors.Is(err, DiscordHandleNotFoundError{}) {
					message += fmt.Sprintf("- #%d %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName)
					continue
				} else {
					return "", err
				}
			}
			message += fmt.Sprintf("- <@%s> (carried over)\n", driverId)
		}
		for _, driver := range penalties.PitStartsR1 {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				if errors.Is(err, DiscordHandleNotFoundError{}) {
					message += fmt.Sprintf("- #%d %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName)
					continue
				} else {
					return "", err
				}
			}
			message += fmt.Sprintf("- <@%s>\n", driverId)
		}
	}
	message += `
**Quali Bans R2**
`
	if len(penalties.QualiBansR2CarriedOver)+len(penalties.QualiBansR2) == 0 {
		message += "- None!\n"
	} else {
		for _, driver := range penalties.QualiBansR2CarriedOver {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				if errors.Is(err, DiscordHandleNotFoundError{}) {
					message += fmt.Sprintf("- #%d %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName)
					continue
				} else {
					return "", err
				}
			}
			message += fmt.Sprintf("- <@%s> (carried over)\n", driverId)
		}
		for _, driver := range penalties.QualiBansR2 {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				if errors.Is(err, DiscordHandleNotFoundError{}) {
					message += fmt.Sprintf("- #%d %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName)
					continue
				} else {
					return "", err
				}
			}
			message += fmt.Sprintf("- <@%s>\n", driverId)
		}
	}

	message += `
**Pit Starts R2**
`
	if len(penalties.PitStartsR2CarriedOver)+len(penalties.PitStartsR2) == 0 {
		message += "- None!\n"
	} else {
		for _, driver := range penalties.PitStartsR2CarriedOver {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				if errors.Is(err, DiscordHandleNotFoundError{}) {
					message += fmt.Sprintf("- #%d %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName)
					continue
				} else {
					return "", err
				}
			}
			message += fmt.Sprintf("- <@%s> (carried over)\n", driverId)
		}
		for _, driver := range penalties.PitStartsR2 {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				if errors.Is(err, DiscordHandleNotFoundError{}) {
					message += fmt.Sprintf("- #%d %s %s\n", driver.CarNumber, driver.FirstName, driver.LastName)
					continue
				} else {
					return "", err
				}
			}
			message += fmt.Sprintf("- <@%s>\n", driverId)
		}
	}
	message += fmt.Sprintf(`
[Explanations of penalties can be found here.](%s)
`, config.PreviousRound.PenaltyTrackerLink)

	return message, nil
}
