package discord_test

import (
	"encoding/json"
	"time"

	"github.com/disgoorg/disgo/rest"
	dgo "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/discord"
	"github.com/geofffranks/rookies-bot/discord/fakes"
	"github.com/geofffranks/rookies-bot/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newGuildTextChannel is a helper to create a GuildTextChannel from JSON for testing.
func newGuildTextChannel(id, guildID snowflake.ID) dgo.GuildTextChannel {
	var ch dgo.GuildTextChannel
	data := map[string]interface{}{
		"id":       id,
		"type":     dgo.ChannelTypeGuildText,
		"guild_id": guildID,
		"name":     "test-channel",
	}
	jsonData, _ := json.Marshal(data)
	json.Unmarshal(jsonData, &ch)
	return ch
}

func newTestClient(restClient discord.BotRestClient, conf *config.Config) *discord.DiscordClient {
	return discord.NewTestDiscordClient(restClient, snowflake.ID(12345), conf, nil)
}

var _ = Describe("Repin", func() {
	var (
		fakeRest *fakes.FakeBotRestClient
		conf     *config.Config
		dc       *discord.DiscordClient
	)

	BeforeEach(func() {
		fakeRest = new(fakes.FakeBotRestClient)
		conf = &config.Config{
			BotConfig: config.BotConfig{
				DiscordChannelId:         snowflake.ID(111),
				DiscordBriefingChannelId: snowflake.ID(222),
				DiscordRoleName:          "Rookies",
			},
		}
		dc = newTestClient(fakeRest, conf)
	})

	It("unpins the bot's old messages and pins the new one", func() {
		botAppID := snowflake.ID(12345)
		oldMsgID := snowflake.ID(99)
		newMsgID := snowflake.ID(100)

		fakeRest.GetPinnedMessagesReturns([]dgo.Message{
			{ID: oldMsgID, Author: dgo.User{ID: botAppID}},
		}, nil)
		fakeRest.UnpinMessageReturns(nil)
		fakeRest.PinMessageReturns(nil)

		newMsg := &dgo.Message{ID: newMsgID}
		err := dc.Repin(newMsg)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRest.UnpinMessageCallCount()).To(Equal(1))
		chanID, msgID, _ := fakeRest.UnpinMessageArgsForCall(0)
		Expect(chanID).To(Equal(snowflake.ID(111)))
		Expect(msgID).To(Equal(oldMsgID))

		Expect(fakeRest.PinMessageCallCount()).To(Equal(1))
		chanID, msgID, _ = fakeRest.PinMessageArgsForCall(0)
		Expect(chanID).To(Equal(snowflake.ID(111)))
		Expect(msgID).To(Equal(newMsgID))
	})

	It("does not unpin messages from other bots", func() {
		fakeRest2 := new(fakes.FakeBotRestClient)
		dc2 := newTestClient(fakeRest2, conf)
		otherBotID := snowflake.ID(99999)
		fakeRest2.GetPinnedMessagesReturns([]dgo.Message{
			{ID: snowflake.ID(55), Author: dgo.User{ID: otherBotID}},
		}, nil)

		err := dc2.Repin(&dgo.Message{ID: snowflake.ID(100)})
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeRest2.UnpinMessageCallCount()).To(Equal(0))
	})

	It("returns an error when GetPinnedMessages fails", func() {
		fakeRest3 := new(fakes.FakeBotRestClient)
		dc3 := newTestClient(fakeRest3, conf)
		expectedErr := "test error"
		fakeRest3.GetPinnedMessagesReturns(nil, &errorMsg{msg: expectedErr})
		err := dc3.Repin(&dgo.Message{ID: snowflake.ID(1)})
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when UnpinMessage fails", func() {
		fakeRest4 := new(fakes.FakeBotRestClient)
		dc4 := newTestClient(fakeRest4, conf)
		fakeRest4.GetPinnedMessagesReturns([]dgo.Message{
			{ID: snowflake.ID(55), Author: dgo.User{ID: snowflake.ID(12345)}},
		}, nil)
		fakeRest4.UnpinMessageReturns(&errorMsg{msg: "unpin failed"})
		err := dc4.Repin(&dgo.Message{ID: snowflake.ID(1)})
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("BuildPenaltyMessage", func() {
	var (
		fakeRest  *fakes.FakeBotRestClient
		conf      *config.Config
		dc        *discord.DiscordClient
		callCount int
	)

	BeforeEach(func() {
		fakeRest = new(fakes.FakeBotRestClient)
		conf = &config.Config{
			BotConfig: config.BotConfig{
				DiscordChannelId:         snowflake.ID(111),
				DiscordBriefingChannelId: snowflake.ID(222),
				DiscordRoleName:          "Rookies",
			},
			RoundConfig: config.RoundConfig{
				NextRound:     config.Round{Number: 5, Track: "Monza"},
				PreviousRound: config.Round{Number: 4, Track: "Spa", PenaltyTrackerLink: "https://example.com/tracker"},
			},
		}
		dc = newTestClient(fakeRest, conf)

		// Stub GetChannel to return a GuildTextChannel (for guild ID lookup)
		fakeRest.GetChannelStub = func(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error) {
			return newGuildTextChannel(channelID, snowflake.ID(777)), nil
		}
		// Stub GetMembers with pagination handling
		callCount = 0
		fakeRest.GetMembersStub = func(guildID snowflake.ID, limit int, after snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error) {
			callCount++
			if callCount == 1 {
				// First call returns one member
				return []dgo.Member{
					{User: dgo.User{ID: snowflake.ID(1001), Username: "maxv"}},
				}, nil
			}
			// Subsequent calls return empty to end pagination
			return []dgo.Member{}, nil
		}
	})

	It("includes 'None!' for categories with no penalties", func() {
		penalties := &models.Penalties{}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.Content).To(ContainSubstring("None!"))
	})

	It("includes a mention for each penalized driver", func() {
		penalties := &models.Penalties{
			QualiBansR1: []models.Driver{
				{FirstName: "Max", LastName: "V", CarNumber: 42, DiscordHandle: "maxv"},
			},
		}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.Content).To(ContainSubstring("<@"))
	})

	It("uses car number fallback when driver handle not found in guild", func() {
		fakeRest2 := new(fakes.FakeBotRestClient)
		fakeRest2.GetChannelStub = func(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error) {
			return newGuildTextChannel(channelID, snowflake.ID(777)), nil
		}
		fakeRest2.GetMembersStub = func(guildID snowflake.ID, limit int, after snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error) {
			return []dgo.Member{}, nil
		}
		dc2 := newTestClient(fakeRest2, conf)
		penalties := &models.Penalties{
			QualiBansR1: []models.Driver{
				{FirstName: "Ghost", LastName: "Driver", CarNumber: 77, DiscordHandle: "notinguild"},
			},
		}
		msg, err := dc2.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.Content).To(ContainSubstring("#77"))
		Expect(msg.Content).To(ContainSubstring("Ghost Driver"))
	})

	It("marks carried-over penalties as '(carried over)'", func() {
		fakeRest3 := new(fakes.FakeBotRestClient)
		fakeRest3.GetChannelStub = func(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error) {
			return newGuildTextChannel(channelID, snowflake.ID(777)), nil
		}
		callCount3 := 0
		fakeRest3.GetMembersStub = func(guildID snowflake.ID, limit int, after snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error) {
			callCount3++
			if callCount3 == 1 {
				return []dgo.Member{
					{User: dgo.User{ID: snowflake.ID(1001), Username: "maxv"}},
				}, nil
			}
			return []dgo.Member{}, nil
		}
		dc3 := newTestClient(fakeRest3, conf)
		penalties := &models.Penalties{
			QualiBansR1CarriedOver: []models.Driver{
				{FirstName: "Max", LastName: "V", CarNumber: 42, DiscordHandle: "maxv"},
			},
		}
		msg, err := dc3.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.Content).To(ContainSubstring("carried over"))
	})

	It("includes the round number in the header", func() {
		penalties := &models.Penalties{}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.Content).To(ContainSubstring("Round 4"))
	})

	It("includes the next round track", func() {
		penalties := &models.Penalties{}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.Content).To(ContainSubstring("Monza"))
	})

	It("includes the penalty tracker link", func() {
		penalties := &models.Penalties{}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.Content).To(ContainSubstring("https://example.com/tracker"))
	})
})

var _ = Describe("CreateBriefingEvent", func() {
	var (
		fakeRest *fakes.FakeBotRestClient
		conf     *config.Config
		dc       *discord.DiscordClient
	)

	BeforeEach(func() {
		fakeRest = new(fakes.FakeBotRestClient)
		conf = &config.Config{
			BotConfig: config.BotConfig{
				DiscordChannelId:         snowflake.ID(111),
				DiscordBriefingChannelId: snowflake.ID(222),
				DiscordRoleName:          "Rookies",
			},
			RoundConfig: config.RoundConfig{
				NextRound:     config.Round{Number: 5, Track: "Monza"},
				PreviousRound: config.Round{Number: 4, Track: "Spa", PenaltyTrackerLink: "https://example.com/tracker"},
			},
		}
		dc = newTestClient(fakeRest, conf)

		fakeRest.GetChannelStub = func(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error) {
			return newGuildTextChannel(channelID, snowflake.ID(777)), nil
		}
		fakeRest.GetRolesReturns([]dgo.Role{{Name: "Rookies", ID: snowflake.ID(500)}}, nil)
		fakeRest.CreateGuildScheduledEventReturns(&dgo.GuildScheduledEvent{}, nil)
	})

	It("schedules the event at 7:30 PM Eastern on the next Monday", func() {
		err := dc.CreateBriefingEvent(&conf.RoundConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeRest.CreateGuildScheduledEventCallCount()).To(Equal(1))
		_, eventCreate, _ := fakeRest.CreateGuildScheduledEventArgsForCall(0)
		loc, _ := time.LoadLocation("America/New_York")
		scheduledTime := eventCreate.ScheduledStartTime.In(loc)
		Expect(scheduledTime.Weekday()).To(Equal(time.Monday))
		Expect(scheduledTime.Hour()).To(Equal(19))
		Expect(scheduledTime.Minute()).To(Equal(30))
	})
})

type errorMsg struct {
	msg string
}

func (e *errorMsg) Error() string {
	return e.msg
}
