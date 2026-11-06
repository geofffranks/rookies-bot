package discord

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/disgoorg/disgo/bot"
	dgo "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/gcloud"
	"github.com/geofffranks/rookies-bot/simgrid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("parseSeasonCommand", func() {
	DescribeTable("splits the two season commands from their optional argument",
		func(content string, expectedApply bool, expectedArg string, expectedOK bool) {
			apply, arg, ok := parseSeasonCommand(content)
			Expect(ok).To(Equal(expectedOK))
			if expectedOK {
				Expect(apply).To(Equal(expectedApply))
				Expect(arg).To(Equal(expectedArg))
			}
		},
		Entry("bare preview command", "!new-season", false, "", true),
		Entry("preview with argument", "!new-season fall", false, " fall", true),
		Entry("preview with multiword argument", "!new-season GT4 Rookies Fall", false, " GT4 Rookies Fall", true),
		Entry("bare apply command", "!new-season-apply", true, "", true),
		Entry("apply with argument", "!new-season-apply Winter", true, " Winter", true),
		Entry("apply argument must be separated by whitespace", "!new-season-applyWinter", false, "", false),
		Entry("lookalike suffix is not a season command", "!new-season-extra", false, "", false),
		Entry("lookalike argument suffix is not a season command", "!new-season-fall", false, "", false),
		Entry("longer command token is not a season command", "!new-seasonXYZ", false, "", false),
		Entry("lookalike apply suffix is not a season command", "!new-season-applyx", false, "", false),
		Entry("unrelated command is not a season command", "!help", false, "", false),
		Entry("empty message is not a season command", "", false, "", false),
	)
})

// eventRestStub satisfies rest.Rest for gateway event tests. Only
// CreateMessage is expected to be called (by sendBotResponse); any other
// method panics on the nil embedded interface.
type eventRestStub struct {
	rest.Rest
	createMessageFn func(channelID snowflake.ID, mc dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error)
}

func (s *eventRestStub) CreateMessage(channelID snowflake.ID, mc dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error) {
	if s.createMessageFn != nil {
		return s.createMessageFn(channelID, mc, opts...)
	}
	return &dgo.Message{ID: snowflake.ID(1)}, nil
}

// newSeasonEvent builds a MessageCreate event dispatched by the given rest
// stub, as if authored by authorID in the configured channel.
func newSeasonEvent(content string, authorID snowflake.ID, erest *eventRestStub) *events.MessageCreate {
	return &events.MessageCreate{
		GenericMessage: &events.GenericMessage{
			GenericEvent: events.NewGenericEvent(&bot.Client{Rest: erest}, 0, 0),
			Message:      dgo.Message{Content: content, Author: dgo.User{ID: authorID}},
			ChannelID:    snowflakeID(111),
		},
	}
}

var _ = Describe("newSeason command dispatch", func() {
	var (
		d        *DiscordClient
		erest    *eventRestStub
		sgServer *httptest.Server
		sgReqs   int32
		sent     []dgo.MessageCreate
	)

	BeforeEach(func() {
		sent = nil
		sgReqs = 0
		erest = &eventRestStub{}
		erest.createMessageFn = func(channelID snowflake.ID, mc dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error) {
			sent = append(sent, mc)
			return &dgo.Message{ID: snowflake.ID(1)}, nil
		}
		d = NewTestDiscordClient(&stubRest{}, snowflakeID(1), &config.Config{
			BotConfig: config.BotConfig{
				Season:           "Fall",
				BriefingFolderID: "briefing-current",
				TrackerFolderID:  "tracker-current",
			},
		}, &gcloud.Client{})

		// The API hosts the current Fall championship alongside the automatic
		// next Winter one; only an explicit Fall selection may resolve to Fall.
		sgServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&sgReqs, 1)
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/championships":
				_, _ = w.Write([]byte(`{"data":[{"id":555,"name":"GT4 Rookies - Winter"},{"id":556,"name":"GT4 Rookies - Fall"}],"pagination":{"limit":100,"offset":0}}`))
			case "/championships/556":
				_, _ = w.Write([]byte(`{"id":556,"name":"GT4 Rookies - Fall","host_name":"TRACKILICIOUS","start_date":"2027-09-01T00:00:00.000Z","races":[{"track":{"name":"Monza"}}]}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		origNewSimGridClient := newSimGridClient
		newSimGridClient = func(token string) *simgrid.SimGridClient {
			c := simgrid.NewClient(token)
			c.BaseURL = sgServer.URL
			return c
		}
		DeferCleanup(func() {
			newSimGridClient = origNewSimGridClient
			sgServer.Close()
		})
	})

	lastResponse := func() string {
		if len(sent) == 0 {
			return ""
		}
		return sent[len(sent)-1].Content
	}

	It("routes an explicit season argument through normalization to the Fall preview", func() {
		d.onMessageCreate(newSeasonEvent("!new-season   gt4   rookies   fall  ", adminUsers[0], erest))
		Expect(lastResponse()).To(ContainSubstring("New Season Preview"))
		Expect(lastResponse()).To(ContainSubstring("GT4 Rookies - Fall"))
		Expect(lastResponse()).To(ContainSubstring("2027 Fall"))
		Expect(lastResponse()).To(ContainSubstring("GT4 Rookies Fall"))
		Expect(lastResponse()).To(ContainSubstring("!new-season-apply Fall"))
	})

	It("answers an invalid season argument with usage before any SimGrid call", func() {
		d.onMessageCreate(newSeasonEvent("!new-season bogus season", adminUsers[0], erest))
		Expect(lastResponse()).To(ContainSubstring("invalid season"))
		Expect(lastResponse()).To(ContainSubstring("New Year"))
		Expect(atomic.LoadInt32(&sgReqs)).To(Equal(int32(0)))
	})

	It("rejects an invalid argument on the apply command before any side effect", func() {
		d.onMessageCreate(newSeasonEvent("!new-season-apply nonsense", adminUsers[0], erest))
		Expect(lastResponse()).To(ContainSubstring("invalid season"))
		Expect(atomic.LoadInt32(&sgReqs)).To(Equal(int32(0)))
	})

	It("rejects a year-only apply argument before SimGrid or writes", func() {
		d.onMessageCreate(newSeasonEvent("!new-season-apply 2026 ", adminUsers[0], erest))
		Expect(lastResponse()).To(ContainSubstring("invalid season"))
		Expect(atomic.LoadInt32(&sgReqs)).To(Equal(int32(0)))
		Expect(d.snapshotConfig().Season).To(Equal("Fall"))
	})

	It("ignores command lookalikes without setup or response", func() {
		d.onMessageCreate(newSeasonEvent("!new-season-extra", adminUsers[0], erest))
		Expect(sent).To(BeEmpty())
		Expect(atomic.LoadInt32(&sgReqs)).To(Equal(int32(0)))
	})

	It("ignores messages from non-admin users", func() {
		d.onMessageCreate(newSeasonEvent("!new-season fall", snowflake.ID(999999), erest))
		Expect(sent).To(BeEmpty())
	})
})

var _ = Describe("runNewSeason season selection", func() {
	var (
		client   *DiscordClient
		stub     *stubRest
		sgServer *httptest.Server
		sgClient *simgrid.SimGridClient
	)

	BeforeEach(func() {
		stub = &stubRest{}
		client = NewTestDiscordClient(stub, snowflakeID(1), &config.Config{
			BotConfig: config.BotConfig{
				Season:           "Fall",
				BriefingFolderID: "briefing-current",
				TrackerFolderID:  "tracker-current",
			},
		}, &gcloud.Client{})

		sgServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/championships":
				_, _ = w.Write([]byte(`{"data":[{"id":555,"name":"GT4 Rookies - Winter"},{"id":556,"name":"GT4 Rookies - Fall"}],"pagination":{"limit":100,"offset":0}}`))
			case "/championships/555":
				_, _ = w.Write([]byte(`{"id":555,"name":"GT4 Rookies - Winter","host_name":"TRACKILICIOUS","start_date":"2026-12-01T00:00:00.000Z","races":[{"track":{"name":"Bathurst"}}]}`))
			case "/championships/556":
				_, _ = w.Write([]byte(`{"id":556,"name":"GT4 Rookies - Fall","host_name":"TRACKILICIOUS","start_date":"2027-09-01T00:00:00.000Z","races":[{"track":{"name":"Monza"}}]}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		sgClient = simgrid.NewClient("test-token")
		sgClient.BaseURL = sgServer.URL
	})

	AfterEach(func() {
		sgServer.Close()
	})

	It("selects the explicitly chosen Fall championship instead of automatic Winter", func() {
		msg, attachment, err := client.runNewSeason(false, "Fall", sgClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(attachment).To(BeEmpty())
		Expect(msg).To(ContainSubstring("GT4 Rookies - Fall"))
		Expect(msg).To(ContainSubstring("#556"))
		Expect(msg).To(ContainSubstring("2027 Fall"))
		Expect(msg).To(ContainSubstring("GT4 Rookies Fall"))
		Expect(msg).To(ContainSubstring("Monza"))
	})

	It("shows an apply command that retains the explicit selection", func() {
		msg, _, err := client.runNewSeason(false, "Fall", sgClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg).To(ContainSubstring("!new-season-apply Fall"))
	})

	It("resolves the selection when the current season is unparseable", func() {
		client.conf.Season = "unparseable-season"
		msg, _, err := client.runNewSeason(false, "Winter", sgClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg).To(ContainSubstring("GT4 Rookies - Winter"))
		Expect(msg).To(ContainSubstring("2026 Winter"))
	})

	It("requires the selected championship to exist instead of falling back to progression", func() {
		_, _, err := client.runNewSeason(false, "Spring", sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no upcoming"))
	})

	It("keeps automatic progression when no selection is supplied", func() {
		msg, _, err := client.runNewSeason(false, "", sgClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg).To(ContainSubstring("GT4 Rookies - Winter"))
		Expect(msg).To(ContainSubstring("2026 Winter"))
		Expect(msg).To(ContainSubstring("!new-season-apply"))
	})
})

var _ = Describe("helpMessage season argument documentation", func() {
	It("documents the optional season argument on both commands", func() {
		msg := helpMessage()
		Expect(msg).To(ContainSubstring("!new-season [season]"))
		Expect(msg).To(ContainSubstring("!new-season-apply [season]"))
		Expect(msg).To(ContainSubstring("GT4 Rookies Fall"))
	})

	It("explains the no-argument automatic behavior", func() {
		Expect(helpMessage()).To(ContainSubstring("no argument"))
	})
})
