package discord

import (
	"fmt"
	"strings"
	"time"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/models"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type DiscordClient struct {
	client     bot.Client
	conf       *config.Config
	guild      snowflake.ID
	memberList map[string]snowflake.ID
}

func NewDiscordClient(conf *config.Config) (*DiscordClient, error) {
	client, err := disgo.New(conf.DiscordToken)
	if err != nil {
		return nil, err
	}

	return &DiscordClient{
		conf:   conf,
		client: client,
	}, nil
}

func (d *DiscordClient) BuildPenaltyMessage(penalties *models.Penalties) (discord.MessageCreate, error) {
	message := fmt.Sprintf(`
🚓 **Penalties from Round %d** 🚓 

Stewarding is in from Round %d. The following penalties are to be served next week at %s:
`, d.conf.PreviousRound.Number, d.conf.PreviousRound.Number, d.conf.NextRound.Track)

	penaltyMessage, err := d.generatePenaltyMessage(penalties)
	if err != nil {
		return discord.MessageCreate{}, err
	}
	message += penaltyMessage

	return buildMessage(message), nil

}

func (d *DiscordClient) BuildBriefingMessage(penalties *models.Penalties, briefingUrl string) (discord.MessageCreate, error) {
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
`, role.ID, briefingTime.Unix(), briefingUrl, d.conf.NextRound.Number)

	penaltyMessage, err := d.generatePenaltyMessage(penalties)
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

func (d *DiscordClient) CreateBriefingEvent() error {
	guildId, err := d.getGuild()
	if err != nil {
		return err
	}
	briefingTime, err := d.briefingTime()
	if err != nil {
		return err
	}
	event := discord.GuildScheduledEventCreate{
		Name:               fmt.Sprintf("Rookies Briefing Round %d - %s", d.conf.NextRound.Number, d.conf.NextRound.Track),
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
		return 0, fmt.Errorf("could not find user %s in guild. check for special characters or league abandonment", handle)
	}
	return driver, nil
}

func (d *DiscordClient) generatePenaltyMessage(penalties *models.Penalties) (string, error) {
	message := `
**Quali Bans R1**
`
	if len(penalties.QualiBansR1CarriedOver)+len(penalties.QualiBansR1) == 0 {
		message += "- None!\n"
	} else {
		for _, driver := range penalties.QualiBansR1CarriedOver {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				return "", err
			}
			message += fmt.Sprintf("- <@%s> (carried over)\n", driverId)
		}
		for _, driver := range penalties.QualiBansR1 {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				return "", err
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
				return "", err
			}
			message += fmt.Sprintf("- <@%s> (carried over)\n", driverId)
		}
		for _, driver := range penalties.PitStartsR1 {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				return "", err
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
				return "", err
			}
			message += fmt.Sprintf("- <@%s> (carried over)\n", driverId)
		}
		for _, driver := range penalties.QualiBansR2 {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				return "", err
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
				return "", err
			}
			message += fmt.Sprintf("- <@%s> (carried over)\n", driverId)
		}
		for _, driver := range penalties.PitStartsR2 {
			driverId, err := d.getDriverId(driver.DiscordHandle)
			if err != nil {
				return "", err
			}
			message += fmt.Sprintf("- <@%s>\n", driverId)
		}
	}
	message += fmt.Sprintf(`
[Explanations of penalties can be found here.](%s)
`, d.conf.PreviousRound.PenaltyTrackerLink)

	return message, nil
}
