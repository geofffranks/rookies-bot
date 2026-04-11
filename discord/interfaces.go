package discord

import (
	dgo "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate . BotRestClient
type BotRestClient interface {
	CreateMessage(channelID snowflake.ID, messageCreate dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error)
	GetPinnedMessages(channelID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Message, error)
	UnpinMessage(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error
	PinMessage(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error
	GetChannel(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error)
	GetRoles(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Role, error)
	GetMembers(guildID snowflake.ID, limit int, after snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error)
	CreateGuildScheduledEvent(guildID snowflake.ID, guildScheduledEventCreate dgo.GuildScheduledEventCreate, opts ...rest.RequestOpt) (*dgo.GuildScheduledEvent, error)
}
