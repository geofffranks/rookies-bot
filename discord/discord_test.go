package discord_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/rest"
	dgo "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/discord"
	"github.com/geofffranks/rookies-bot/discord/fakes"
	"github.com/geofffranks/rookies-bot/models"
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

func TestRepin(t *testing.T) {
	fakeRest := new(fakes.FakeBotRestClient)
	conf := &config.Config{
		BotConfig: config.BotConfig{
			DiscordChannelId:         snowflake.ID(111),
			DiscordBriefingChannelId: snowflake.ID(222),
			DiscordRoleName:          "Rookies",
		},
	}
	dc := newTestClient(fakeRest, conf)

	t.Run("unpins the bot's old messages and pins the new one", func(t *testing.T) {
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if fakeRest.UnpinMessageCallCount() != 1 {
			t.Errorf("expected UnpinMessage to be called once, got %d", fakeRest.UnpinMessageCallCount())
		}
		chanID, msgID, _ := fakeRest.UnpinMessageArgsForCall(0)
		if chanID != snowflake.ID(111) {
			t.Errorf("expected channel ID 111, got %d", chanID)
		}
		if msgID != oldMsgID {
			t.Errorf("expected message ID %d, got %d", oldMsgID, msgID)
		}

		if fakeRest.PinMessageCallCount() != 1 {
			t.Errorf("expected PinMessage to be called once, got %d", fakeRest.PinMessageCallCount())
		}
		chanID, msgID, _ = fakeRest.PinMessageArgsForCall(0)
		if chanID != snowflake.ID(111) {
			t.Errorf("expected channel ID 111, got %d", chanID)
		}
		if msgID != newMsgID {
			t.Errorf("expected message ID %d, got %d", newMsgID, msgID)
		}
	})

	t.Run("does not unpin messages from other bots", func(t *testing.T) {
		fakeRest2 := new(fakes.FakeBotRestClient)
		dc2 := newTestClient(fakeRest2, conf)
		otherBotID := snowflake.ID(99999)
		fakeRest2.GetPinnedMessagesReturns([]dgo.Message{
			{ID: snowflake.ID(55), Author: dgo.User{ID: otherBotID}},
		}, nil)

		err := dc2.Repin(&dgo.Message{ID: snowflake.ID(100)})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fakeRest2.UnpinMessageCallCount() != 0 {
			t.Errorf("expected UnpinMessage not to be called, got %d calls", fakeRest2.UnpinMessageCallCount())
		}
	})

	t.Run("returns an error when GetPinnedMessages fails", func(t *testing.T) {
		fakeRest3 := new(fakes.FakeBotRestClient)
		dc3 := newTestClient(fakeRest3, conf)
		expectedErr := "test error"
		fakeRest3.GetPinnedMessagesReturns(nil, &errorMsg{msg: expectedErr})
		err := dc3.Repin(&dgo.Message{ID: snowflake.ID(1)})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("returns an error when UnpinMessage fails", func(t *testing.T) {
		fakeRest4 := new(fakes.FakeBotRestClient)
		dc4 := newTestClient(fakeRest4, conf)
		fakeRest4.GetPinnedMessagesReturns([]dgo.Message{
			{ID: snowflake.ID(55), Author: dgo.User{ID: snowflake.ID(12345)}},
		}, nil)
		fakeRest4.UnpinMessageReturns(&errorMsg{msg: "unpin failed"})
		err := dc4.Repin(&dgo.Message{ID: snowflake.ID(1)})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestBuildPenaltyMessage(t *testing.T) {
	fakeRest := new(fakes.FakeBotRestClient)
	conf := &config.Config{
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
	dc := newTestClient(fakeRest, conf)

	// Stub GetChannel to return a GuildTextChannel (for guild ID lookup)
	fakeRest.GetChannelStub = func(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error) {
		return newGuildTextChannel(channelID, snowflake.ID(777)), nil
	}
	// Stub GetMembers with pagination handling
	callCount := 0
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

	t.Run("includes 'None!' for categories with no penalties", func(t *testing.T) {
		penalties := &models.Penalties{}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg.Content, "None!") {
			t.Error("expected 'None!' in message content")
		}
	})

	t.Run("includes a mention for each penalized driver", func(t *testing.T) {
		penalties := &models.Penalties{
			QualiBansR1: []models.Driver{
				{FirstName: "Max", LastName: "V", CarNumber: 42, DiscordHandle: "maxv"},
			},
		}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg.Content, "<@") {
			t.Error("expected mention in message content")
		}
	})

	t.Run("uses car number fallback when driver handle not found in guild", func(t *testing.T) {
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg.Content, "#77") {
			t.Error("expected '#77' in message content")
		}
		if !strings.Contains(msg.Content, "Ghost Driver") {
			t.Error("expected 'Ghost Driver' in message content")
		}
	})

	t.Run("marks carried-over penalties as '(carried over)'", func(t *testing.T) {
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg.Content, "carried over") {
			t.Error("expected 'carried over' in message content")
		}
	})

	t.Run("includes the round number in the header", func(t *testing.T) {
		penalties := &models.Penalties{}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg.Content, "Round 4") {
			t.Error("expected 'Round 4' in message content")
		}
	})

	t.Run("includes the next round track", func(t *testing.T) {
		penalties := &models.Penalties{}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg.Content, "Monza") {
			t.Error("expected 'Monza' in message content")
		}
	})

	t.Run("includes the penalty tracker link", func(t *testing.T) {
		penalties := &models.Penalties{}
		msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(msg.Content, "https://example.com/tracker") {
			t.Error("expected penalty tracker link in message content")
		}
	})
}

func TestCreateBriefingEvent(t *testing.T) {
	fakeRest := new(fakes.FakeBotRestClient)
	conf := &config.Config{
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
	dc := newTestClient(fakeRest, conf)

	fakeRest.GetChannelStub = func(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error) {
		return newGuildTextChannel(channelID, snowflake.ID(777)), nil
	}
	fakeRest.GetRolesReturns([]dgo.Role{{Name: "Rookies", ID: snowflake.ID(500)}}, nil)
	fakeRest.CreateGuildScheduledEventReturns(&dgo.GuildScheduledEvent{}, nil)

	t.Run("schedules the event at 7:30 PM Eastern on the next Monday", func(t *testing.T) {
		err := dc.CreateBriefingEvent(&conf.RoundConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if fakeRest.CreateGuildScheduledEventCallCount() != 1 {
			t.Fatalf("expected CreateGuildScheduledEvent to be called once, got %d", fakeRest.CreateGuildScheduledEventCallCount())
		}
		_, eventCreate, _ := fakeRest.CreateGuildScheduledEventArgsForCall(0)
		loc, _ := time.LoadLocation("America/New_York")
		scheduledTime := eventCreate.ScheduledStartTime.In(loc)
		if scheduledTime.Weekday() != time.Monday {
			t.Errorf("expected Monday, got %v", scheduledTime.Weekday())
		}
		if scheduledTime.Hour() != 19 {
			t.Errorf("expected hour 19, got %d", scheduledTime.Hour())
		}
		if scheduledTime.Minute() != 30 {
			t.Errorf("expected minute 30, got %d", scheduledTime.Minute())
		}
	})
}

type errorMsg struct {
	msg string
}

func (e *errorMsg) Error() string {
	return e.msg
}
