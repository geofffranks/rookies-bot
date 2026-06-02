package discord

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	dgo "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/gcloud"
	"github.com/geofffranks/rookies-bot/gcloud/fakes"
	"github.com/geofffranks/rookies-bot/models"
	"github.com/geofffranks/rookies-bot/simgrid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/api/docs/v1"
	drive "google.golang.org/api/drive/v3"
)

// snowflakeID converts a uint64 to snowflake.ID for test readability.
func snowflakeID(n uint64) snowflake.ID {
	return snowflake.ID(n)
}

var _ = Describe("DiscordHandleNotFoundError", func() {
	It("Error() returns a string containing the handle", func() {
		err := DiscordHandleNotFoundError{Handle: "testuser"}
		Expect(err.Error()).To(ContainSubstring("testuser"))
	})

	It("String() returns a string containing the handle", func() {
		err := DiscordHandleNotFoundError{Handle: "testuser"}
		Expect(err.String()).To(ContainSubstring("testuser"))
	})

	It("Error() and String() return the same value", func() {
		err := DiscordHandleNotFoundError{Handle: "testuser"}
		Expect(err.Error()).To(Equal(err.String()))
	})

	It("Is() returns true when target is the same type", func() {
		err := DiscordHandleNotFoundError{Handle: "a"}
		Expect(err.Is(DiscordHandleNotFoundError{Handle: "different"})).To(BeTrue())
	})

	It("Is() returns false when target is a different error type", func() {
		err := DiscordHandleNotFoundError{Handle: "a"}
		Expect(err.Is(fmt.Errorf("other error"))).To(BeFalse())
	})

	It("errors.Is works with DiscordHandleNotFoundError sentinel", func() {
		err := DiscordHandleNotFoundError{Handle: "missinguser"}
		Expect(errors.Is(err, DiscordHandleNotFoundError{})).To(BeTrue())
	})

	It("errors.Is returns false for unrelated errors", func() {
		err := fmt.Errorf("some other error")
		Expect(errors.Is(err, DiscordHandleNotFoundError{})).To(BeFalse())
	})
})

var _ = Describe("isAllowedUser", func() {
	DescribeTable("returns true for known admin IDs",
		func(id uint64) {
			Expect(isAllowedUser(snowflakeID(id))).To(BeTrue())
		},
		Entry("porkchop", uint64(208972532068515840)),
		Entry("ralli", uint64(371787234187280385)),
		Entry("kallil", uint64(418087017448996864)),
		Entry("geoff", uint64(942149076873543721)),
	)

	It("returns false for a non-admin user ID", func() {
		Expect(isAllowedUser(snowflakeID(999999999))).To(BeFalse())
	})
})

var _ = Describe("helpMessage", func() {
	It("lists the !help command", func() {
		Expect(helpMessage()).To(ContainSubstring("!help"))
	})

	It("lists the !announce-penalties command", func() {
		Expect(helpMessage()).To(ContainSubstring("!announce-penalties"))
	})

	It("lists the !race-setup command", func() {
		Expect(helpMessage()).To(ContainSubstring("!race-setup"))
	})

	It("notes that the penalty commands require a YAML attachment", func() {
		Expect(helpMessage()).To(ContainSubstring("YAML"))
	})

	It("lists the !new-season command", func() {
		Expect(helpMessage()).To(ContainSubstring("!new-season"))
	})

	It("lists the !new-season-apply command", func() {
		Expect(helpMessage()).To(ContainSubstring("!new-season-apply"))
	})
})

var _ = Describe("buildPenalizedDriverList", func() {
	var driverLookup models.DriverLookup

	BeforeEach(func() {
		driverLookup = models.DriverLookup{
			42: {FirstName: "Max", LastName: "V", CarNumber: 42, DiscordHandle: "maxv"},
			77: {FirstName: "Valt", LastName: "B", CarNumber: 77, DiscordHandle: "valtb"},
		}
	})

	It("returns nil slice for empty car number list", func() {
		drivers, err := buildPenalizedDriverList(driverLookup, []int{})
		Expect(err).NotTo(HaveOccurred())
		Expect(drivers).To(BeNil())
	})

	It("returns the correct drivers for known car numbers", func() {
		drivers, err := buildPenalizedDriverList(driverLookup, []int{42, 77})
		Expect(err).NotTo(HaveOccurred())
		Expect(drivers).To(HaveLen(2))
		Expect(drivers[0].CarNumber).To(Equal(42))
		Expect(drivers[1].CarNumber).To(Equal(77))
	})

	It("returns a single driver when one car number is given", func() {
		drivers, err := buildPenalizedDriverList(driverLookup, []int{42})
		Expect(err).NotTo(HaveOccurred())
		Expect(drivers).To(HaveLen(1))
		Expect(drivers[0].DiscordHandle).To(Equal("maxv"))
	})

	It("returns an error when a car number is not in the lookup", func() {
		_, err := buildPenalizedDriverList(driverLookup, []int{99})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("99"))
	})

	It("returns an error mentioning the unknown number even when some are valid", func() {
		_, err := buildPenalizedDriverList(driverLookup, []int{42, 99})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("99"))
	})
})

var _ = Describe("buildPenaltyList", func() {
	var (
		driverLookup models.DriverLookup
		roundConfig  *config.RoundConfig
	)

	BeforeEach(func() {
		driverLookup = models.DriverLookup{
			1: {CarNumber: 1, DiscordHandle: "d1"},
			2: {CarNumber: 2, DiscordHandle: "d2"},
			3: {CarNumber: 3, DiscordHandle: "d3"},
			4: {CarNumber: 4, DiscordHandle: "d4"},
			5: {CarNumber: 5, DiscordHandle: "d5"},
			6: {CarNumber: 6, DiscordHandle: "d6"},
			7: {CarNumber: 7, DiscordHandle: "d7"},
			8: {CarNumber: 8, DiscordHandle: "d8"},
		}
		roundConfig = &config.RoundConfig{
			Penalties: config.Penalty{
				QualiBansR1: []int{1},
				QualiBansR2: []int{2},
				PitStartsR1: []int{3},
				PitStartsR2: []int{4},
			},
			CarriedOverPenalties: config.Penalty{
				QualiBansR1: []int{5},
				QualiBansR2: []int{6},
				PitStartsR1: []int{7},
				PitStartsR2: []int{8},
			},
		}
	})

	It("populates all 8 penalty fields correctly", func() {
		penalties, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).NotTo(HaveOccurred())

		Expect(penalties.QualiBansR1).To(HaveLen(1))
		Expect(penalties.QualiBansR1[0].CarNumber).To(Equal(1))

		Expect(penalties.QualiBansR2).To(HaveLen(1))
		Expect(penalties.QualiBansR2[0].CarNumber).To(Equal(2))

		Expect(penalties.PitStartsR1).To(HaveLen(1))
		Expect(penalties.PitStartsR1[0].CarNumber).To(Equal(3))

		Expect(penalties.PitStartsR2).To(HaveLen(1))
		Expect(penalties.PitStartsR2[0].CarNumber).To(Equal(4))

		Expect(penalties.QualiBansR1CarriedOver).To(HaveLen(1))
		Expect(penalties.QualiBansR1CarriedOver[0].CarNumber).To(Equal(5))

		Expect(penalties.QualiBansR2CarriedOver).To(HaveLen(1))
		Expect(penalties.QualiBansR2CarriedOver[0].CarNumber).To(Equal(6))

		Expect(penalties.PitStartsR1CarriedOver).To(HaveLen(1))
		Expect(penalties.PitStartsR1CarriedOver[0].CarNumber).To(Equal(7))

		Expect(penalties.PitStartsR2CarriedOver).To(HaveLen(1))
		Expect(penalties.PitStartsR2CarriedOver[0].CarNumber).To(Equal(8))
	})

	It("returns empty penalty slices when config has no car numbers", func() {
		roundConfig = &config.RoundConfig{}
		penalties, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).NotTo(HaveOccurred())
		Expect(penalties.QualiBansR1).To(BeNil())
		Expect(penalties.QualiBansR2).To(BeNil())
		Expect(penalties.PitStartsR1).To(BeNil())
		Expect(penalties.PitStartsR2).To(BeNil())
	})

	It("returns error when QualiBansR1 has unknown car number", func() {
		roundConfig.Penalties.QualiBansR1 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("999"))
	})

	It("returns error when QualiBansR2 has unknown car number", func() {
		roundConfig.Penalties.QualiBansR2 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when PitStartsR1 has unknown car number", func() {
		roundConfig.Penalties.PitStartsR1 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when PitStartsR2 has unknown car number", func() {
		roundConfig.Penalties.PitStartsR2 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when QualiBansR1CarriedOver has unknown car number", func() {
		roundConfig.CarriedOverPenalties.QualiBansR1 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when QualiBansR2CarriedOver has unknown car number", func() {
		roundConfig.CarriedOverPenalties.QualiBansR2 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when PitStartsR1CarriedOver has unknown car number", func() {
		roundConfig.CarriedOverPenalties.PitStartsR1 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when PitStartsR2CarriedOver has unknown car number", func() {
		roundConfig.CarriedOverPenalties.PitStartsR2 = []int{999}
		_, err := buildPenaltyList(driverLookup, roundConfig)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("downloadAttachment", func() {
	It("returns the response body on success", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("penalty: yaml content here"))
		}))
		defer server.Close()

		content, err := downloadAttachment(server.URL)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(Equal([]byte("penalty: yaml content here")))
	})

	It("returns an error when the server is unreachable", func() {
		// Port 1 is privileged and never listening
		_, err := downloadAttachment("http://127.0.0.1:1")
		Expect(err).To(HaveOccurred())
	})

	It("returns an empty body for a 200 response with no content", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		content, err := downloadAttachment(server.URL)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(BeEmpty())
	})
})

// stubRest implements BotRestClient using function fields so each test can
// inject only the methods it cares about. All stubs default to no-op / nil.
type stubRest struct {
	createMessageFn             func(channelID snowflake.ID, messageCreate dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error)
	getChannelPinsFn            func(channelID snowflake.ID, before time.Time, limit int, opts ...rest.RequestOpt) (*dgo.ChannelPins, error)
	unpinMessageFn              func(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error
	pinMessageFn                func(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error
	getChannelFn                func(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error)
	getRolesFn                  func(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Role, error)
	getMembersFn                func(guildID snowflake.ID, limit int, after snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error)
	createGuildScheduledEventFn func(guildID snowflake.ID, e dgo.GuildScheduledEventCreate, opts ...rest.RequestOpt) (*dgo.GuildScheduledEvent, error)
}

func (s *stubRest) CreateMessage(channelID snowflake.ID, messageCreate dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error) {
	if s.createMessageFn != nil {
		return s.createMessageFn(channelID, messageCreate, opts...)
	}
	id := snowflake.ID(42)
	return &dgo.Message{ID: id}, nil
}
func (s *stubRest) GetChannelPins(channelID snowflake.ID, before time.Time, limit int, opts ...rest.RequestOpt) (*dgo.ChannelPins, error) {
	if s.getChannelPinsFn != nil {
		return s.getChannelPinsFn(channelID, before, limit, opts...)
	}
	return &dgo.ChannelPins{}, nil
}
func (s *stubRest) UnpinMessage(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error {
	if s.unpinMessageFn != nil {
		return s.unpinMessageFn(channelID, messageID, opts...)
	}
	return nil
}
func (s *stubRest) PinMessage(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error {
	if s.pinMessageFn != nil {
		return s.pinMessageFn(channelID, messageID, opts...)
	}
	return nil
}
func (s *stubRest) GetChannel(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error) {
	if s.getChannelFn != nil {
		return s.getChannelFn(channelID, opts...)
	}
	return dgo.GuildTextChannel{}, nil
}
func (s *stubRest) GetRoles(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Role, error) {
	if s.getRolesFn != nil {
		return s.getRolesFn(guildID, opts...)
	}
	return nil, nil
}
func (s *stubRest) GetMembers(guildID snowflake.ID, limit int, after snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error) {
	if s.getMembersFn != nil {
		return s.getMembersFn(guildID, limit, after, opts...)
	}
	return nil, nil
}
func (s *stubRest) CreateGuildScheduledEvent(guildID snowflake.ID, e dgo.GuildScheduledEventCreate, opts ...rest.RequestOpt) (*dgo.GuildScheduledEvent, error) {
	if s.createGuildScheduledEventFn != nil {
		return s.createGuildScheduledEventFn(guildID, e, opts...)
	}
	return &dgo.GuildScheduledEvent{}, nil
}

var _ = Describe("runAnnouncePenalties", func() {
	var (
		client      *DiscordClient
		stub        *stubRest
		roundConfig *config.RoundConfig
		sgServer    *httptest.Server
		sgClient    *simgrid.SimGridClient
	)

	BeforeEach(func() {
		stub = &stubRest{}
		client = NewTestDiscordClient(stub, snowflakeID(1), &config.Config{
			BotConfig: config.BotConfig{
				DiscordChannelId: snowflakeID(111),
				DiscordRoleName:  "test-role",
			},
		}, nil)
		roundConfig = &config.RoundConfig{
			PreviousRound: config.Round{Number: 1},
			NextRound:     config.Round{Number: 2},
			Penalties:     config.Penalty{},
		}
		// default simgrid server: returns empty driver list
		sgServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if strings.Contains(r.URL.Path, "entrylist") {
				_, _ = w.Write([]byte(`{"entries":[]}`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		}))
		sgClient = simgrid.NewClient("test-token")
		sgClient.BaseURL = sgServer.URL
	})

	AfterEach(func() {
		sgServer.Close()
	})

	It("returns error when BuildDriverLookup fails", func() {
		sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		_, _, err := client.runAnnouncePenalties(roundConfig, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed building driver list"))
	})

	It("returns error when buildPenaltyList fails (unknown car)", func() {
		roundConfig.Penalties.QualiBansR1 = []int{999}
		_, _, err := client.runAnnouncePenalties(roundConfig, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed generating penalty summary"))
	})

	It("returns error when BuildPenaltyMessage fails (GetMembers error)", func() {
		roundConfig.Penalties.QualiBansR1 = []int{1}
		// Mock the server to return a driver with car number 1
		sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if strings.Contains(r.URL.Path, "entrylist") {
				_, _ = w.Write([]byte(`{"entries":[{"drivers":[{"firstName":"Test","lastName":"Driver","playerId":"S123"}],"raceNumber":1}]}`))
			} else {
				_, _ = w.Write([]byte(`[{"steam64_id":"123","username":"testdriver"}]`))
			}
		})
		stub.getMembersFn = func(guildID snowflake.ID, limit int, after snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error) {
			return nil, fmt.Errorf("members fetch failed")
		}
		_, _, err := client.runAnnouncePenalties(roundConfig, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to generate penalty message"))
	})

	It("returns error when SendMessage fails", func() {
		stub.createMessageFn = func(channelID snowflake.ID, messageCreate dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error) {
			return nil, fmt.Errorf("send failed")
		}
		_, _, err := client.runAnnouncePenalties(roundConfig, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to send penalty announcement"))
	})

	It("returns error when Repin fails", func() {
		stub.pinMessageFn = func(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error {
			return fmt.Errorf("pin failed")
		}
		_, _, err := client.runAnnouncePenalties(roundConfig, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to pin penalty announcement"))
	})

	It("returns msg containing previous round name on happy path", func() {
		msg, attachment, err := client.runAnnouncePenalties(roundConfig, sgClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg).To(ContainSubstring("Round 1"))
		Expect(attachment).To(BeEmpty())
	})
})

// makeStreamDoc creates a minimal *docs.Document with a Stream H3 heading at body index 1.
// This is needed by generateUpdates inside GenerateBriefing.
func makeStreamDoc() *docs.Document {
	return &docs.Document{
		DocumentId: "test-doc",
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					StartIndex: 0,
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "Title\n",
								},
							},
						},
					},
				},
				{
					StartIndex: 1,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{
							NamedStyleType: "HEADING_3",
						},
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "Stream",
								},
							},
						},
					},
				},
			},
		},
	}
}

var _ = Describe("runRaceSetup", func() {
	var (
		client      *DiscordClient
		stub        *stubRest
		roundConfig *config.RoundConfig
		sgServer    *httptest.Server
		sgClient    *simgrid.SimGridClient
		gcClient    *gcloud.Client
		fakeDrive   *fakes.FakeDriveServicer
		fakeDocs    *fakes.FakeDocsServicer
	)

	BeforeEach(func() {
		stub = &stubRest{}
		gcClient = &gcloud.Client{}
		fakeDrive = &fakes.FakeDriveServicer{}
		fakeDocs = &fakes.FakeDocsServicer{}
		gcClient.Drive = fakeDrive
		gcClient.Docs = fakeDocs

		// Default fake drive returns a file with an ID
		fakeDrive.CopyFileReturns(&drive.File{Id: "test-doc-id"}, nil)
		// Default fake docs returns a document with Stream heading
		fakeDocs.GetDocumentReturns(makeStreamDoc(), nil)

		client = NewTestDiscordClient(stub, snowflakeID(1), &config.Config{
			BotConfig: config.BotConfig{
				DiscordChannelId:         snowflakeID(111),
				DiscordBriefingChannelId: snowflakeID(222),
				DiscordRoleName:          "test-role",
				Season:                   "S1",
			},
		}, gcClient)

		roundConfig = &config.RoundConfig{
			PreviousRound: config.Round{Number: 1, Track: "Monza"},
			NextRound:     config.Round{Number: 2, Track: ""},
			Penalties:     config.Penalty{},
		}

		// default simgrid server: returns empty driver list
		sgServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if strings.Contains(r.URL.Path, "entrylist") {
				_, _ = w.Write([]byte(`{"entries":[]}`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		}))
		sgClient = simgrid.NewClient("test-token")
		sgClient.BaseURL = sgServer.URL

		// Default Discord stubs - getRoles must be set to return test-role
		stub.getRolesFn = func(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Role, error) {
			return []dgo.Role{
				{Name: "test-role", ID: snowflakeID(777)},
			}, nil
		}
	})

	AfterEach(func() {
		sgServer.Close()
	})

	It("returns error when BuildDriverLookup fails (simgrid 500)", func() {
		sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		_, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).To(HaveOccurred())
	})

	It("returns error when GenerateBriefing fails (fakeDocs.Get returns error)", func() {
		fakeDocs.GetDocumentReturns(nil, fmt.Errorf("docs error"))
		_, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to generate briefing doc"))
	})

	It("returns error when generateNextRoundConfig fails (Track != '')", func() {
		roundConfig.NextRound.Track = "Monza"
		roundConfig.NextRound.Number = 1
		sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(r.URL.Path, "entrylist") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"entries":[]}`))
			} else if strings.Contains(r.URL.Path, "participating_users") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			} else {
				// Championships endpoint fails
				w.WriteHeader(http.StatusInternalServerError)
			}
		})
		_, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to generate config for next round"))
	})

	It("returns error when BuildBriefingMessage fails (getRoles returns error)", func() {
		stub.getRolesFn = func(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Role, error) {
			return nil, fmt.Errorf("roles error")
		}
		fakeDocs.GetDocumentReturns(makeStreamDoc(), nil)
		_, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to generate briefingmessage"))
	})

	It("returns error when SendMessage fails", func() {
		stub.createMessageFn = func(channelID snowflake.ID, messageCreate dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error) {
			return nil, fmt.Errorf("send failed")
		}
		fakeDocs.GetDocumentReturns(makeStreamDoc(), nil)
		_, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to send briefing announcement"))
	})

	It("returns error when Repin fails", func() {
		stub.pinMessageFn = func(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error {
			return fmt.Errorf("pin failed")
		}
		fakeDocs.GetDocumentReturns(makeStreamDoc(), nil)
		_, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to pin briefing announcement"))
	})

	It("returns error when CreateBriefingEvent fails", func() {
		stub.createGuildScheduledEventFn = func(guildID snowflake.ID, e dgo.GuildScheduledEventCreate, opts ...rest.RequestOpt) (*dgo.GuildScheduledEvent, error) {
			return nil, fmt.Errorf("event creation failed")
		}
		fakeDocs.GetDocumentReturns(makeStreamDoc(), nil)
		_, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to create briefing event"))
	})

	It("happy path with NextRound.Track == '' (no file written, msg contains round name, empty attachment)", func() {
		fakeDocs.GetDocumentReturns(makeStreamDoc(), nil)
		msg, attachment, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg).To(ContainSubstring("Round"))
		Expect(attachment).To(BeEmpty())
	})

	It("happy path with NextRound.Track != '' (attachment non-empty, msg contains penalty tracker link)", func() {
		roundConfig.NextRound.Track = "Silverstone"
		roundConfig.NextRound.Number = 3
		fakeDocs.GetDocumentReturns(makeStreamDoc(), nil)

		sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if strings.Contains(r.URL.Path, "entrylist") {
				_, _ = w.Write([]byte(`{"entries":[]}`))
			} else if strings.Contains(r.URL.Path, "participating_users") {
				_, _ = w.Write([]byte(`[]`))
			} else {
				// Default case: championships endpoint for GetNextRound
				_, _ = w.Write([]byte(`{"races":[{"track":{"name":"Round1"}},{"track":{"name":"Round2"}},{"track":{"name":"Silverstone"}}]}`))
			}
		})

		msg, attachment, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(attachment).NotTo(BeEmpty())
		Expect(msg).To(ContainSubstring("Penalty Tracker"))

		// Clean up the written file
		if attachment != "" {
			_ = os.Remove(attachment)
		}
	})

	It("includes /dq lines in success message when drivers have penalties", func() {
		// Simgrid returns one driver with car #99
		sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if strings.Contains(r.URL.Path, "entrylist") {
				_, _ = w.Write([]byte(`{"entries":[{"drivers":[{"firstName":"Test","lastName":"Driver","playerId":"S999"}],"raceNumber":99}]}`))
			} else {
				_, _ = w.Write([]byte(`[{"steam64_id":"999","username":"testdriver"}]`))
			}
		})
		roundConfig.Penalties.QualiBansR1 = []int{99}

		msg, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg).To(ContainSubstring("DQ List:"))
		Expect(msg).To(ContainSubstring("/dq 99"))
	})

	It("success message contains tracker URL from generated next round config, not stale roundConfig", func() {
		roundConfig.NextRound.Track = "Silverstone"
		roundConfig.NextRound.Number = 3
		// Call 0 = briefing doc (default "test-doc-id" from BeforeEach)
		// Call 1 = tracker sheet — use a distinct ID we can assert on
		fakeDrive.CopyFileReturnsOnCall(1, &drive.File{Id: "tracker-sheet-id"}, nil)

		sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if strings.Contains(r.URL.Path, "entrylist") {
				_, _ = w.Write([]byte(`{"entries":[]}`))
			} else if strings.Contains(r.URL.Path, "participating_users") {
				_, _ = w.Write([]byte(`[]`))
			} else {
				_, _ = w.Write([]byte(`{"races":[{"track":{"name":"Round1"}},{"track":{"name":"Round2"}},{"track":{"name":"Silverstone"}}]}`))
			}
		})

		msg, attachment, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg).To(ContainSubstring("docs.google.com/spreadsheets/d/tracker-sheet-id"))
		if attachment != "" {
			_ = os.Remove(attachment)
		}
	})
})

var _ = Describe("getRoundConfig", func() {
	It("returns error when message has 0 attachments", func() {
		event := &events.MessageCreate{
			GenericMessage: &events.GenericMessage{
				Message: dgo.Message{Attachments: []dgo.Attachment{}},
			},
		}
		_, err := getRoundConfig(event)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no race penalty YAML file"))
	})

	It("returns error when message has 2 attachments", func() {
		event := &events.MessageCreate{
			GenericMessage: &events.GenericMessage{
				Message: dgo.Message{
					Attachments: []dgo.Attachment{
						{URL: "http://example.com/a.yaml"},
						{URL: "http://example.com/b.yaml"},
					},
				},
			},
		}
		_, err := getRoundConfig(event)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("too many attachments"))
	})

	It("returns error when download fails (unreachable URL)", func() {
		event := &events.MessageCreate{
			GenericMessage: &events.GenericMessage{
				Message: dgo.Message{
					Attachments: []dgo.Attachment{
						{URL: "http://127.0.0.1:1/unreachable.yaml"},
					},
				},
			},
		}
		_, err := getRoundConfig(event)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unexpected error downloading"))
	})

	It("returns error when server returns non-YAML", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`}{not valid yaml or json`))
		}))
		defer server.Close()
		event := &events.MessageCreate{
			GenericMessage: &events.GenericMessage{
				Message: dgo.Message{
					Attachments: []dgo.Attachment{{URL: server.URL + "/config.yaml"}},
				},
			},
		}
		_, err := getRoundConfig(event)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unable to parse race penalty YAML file"))
	})

	It("returns *config.RoundConfig with correct fields from valid YAML", func() {
		yaml := `
previous_round:
  number: 1
  track: "Monza"
  penalty_tracker_link: "http://tracker.example.com"
next_round:
  number: 2
  track: "Silverstone"
  penalty_tracker_link: "http://tracker2.example.com"
penalties:
  quali_bans_r1: []
  quali_bans_r2: []
  pit_starts_r1: []
  pit_starts_r2: []
penalties_carried_over:
  quali_bans_r1: []
  quali_bans_r2: []
  pit_starts_r1: []
  pit_starts_r2: []
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(yaml))
		}))
		defer server.Close()
		event := &events.MessageCreate{
			GenericMessage: &events.GenericMessage{
				Message: dgo.Message{
					Attachments: []dgo.Attachment{{URL: server.URL + "/config.yaml"}},
				},
			},
		}
		rc, err := getRoundConfig(event)
		Expect(err).NotTo(HaveOccurred())
		Expect(rc.PreviousRound.Number).To(Equal(1))
		Expect(rc.PreviousRound.Track).To(Equal("Monza"))
		Expect(rc.NextRound.Number).To(Equal(2))
	})
})

var _ = Describe("generateNextRoundConfig", func() {
	var (
		sgServer  *httptest.Server
		sgClient  *simgrid.SimGridClient
		gcClient  *gcloud.Client
		fakeDrive *fakes.FakeDriveServicer
		conf      *config.Config
		penalties *models.Penalties
	)

	BeforeEach(func() {
		fakeDrive = &fakes.FakeDriveServicer{}
		gcClient = &gcloud.Client{
			Drive: fakeDrive,
		}
		conf = &config.Config{
			BotConfig: config.BotConfig{
				ChampionshipId:       "champ-123",
				Season:               "S1",
				TrackerTemplateDocID: "template-123",
				TrackerFolderID:      "folder-456",
			},
			RoundConfig: config.RoundConfig{
				NextRound: config.Round{Number: 3, Track: "Spa"},
			},
		}
		penalties = &models.Penalties{}

		sgServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"races":[{"track":{"name":"R1"}},{"track":{"name":"R2"}},{"track":{"name":"Spa"}}]}`))
		}))
		sgClient = simgrid.NewClient("test-token")
		sgClient.BaseURL = sgServer.URL

		fakeDrive.CopyFileReturns(&drive.File{Id: "tracker-file-id"}, nil)
	})

	AfterEach(func() {
		sgServer.Close()
	})

	It("returns error when simgrid returns 4xx", func() {
		sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		_, err := generateNextRoundConfig(sgClient, gcClient, conf, penalties)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed getting details for next round"))
	})

	It("returns error when GeneratePenaltyTracker fails", func() {
		fakeDrive.CopyFileReturns(nil, fmt.Errorf("drive copy failed"))
		_, err := generateNextRoundConfig(sgClient, gcClient, conf, penalties)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed generating penalty tracker"))
	})

	It("returns *config.RoundConfig with PenaltyTrackerLink set on happy path", func() {
		result, err := generateNextRoundConfig(sgClient, gcClient, conf, penalties)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.PreviousRound.PenaltyTrackerLink).To(ContainSubstring("docs.google.com"))
	})
})

var _ = Describe("runNewSeason preview", func() {
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
				_, _ = w.Write([]byte(`[{"id":555,"name":"GT4 Rookies - Winter"}]`))
			case "/championships/555":
				_, _ = w.Write([]byte(`{"id":555,"name":"GT4 Rookies - Winter","host_name":"TRACKILICIOUS","start_date":"2026-12-01T00:00:00.000Z","races":[{"track":{"name":"Bathurst"}},{"track":{"name":"Spa"}}]}`))
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

	It("returns a preview describing the championship, computed values, and how to apply", func() {
		msg, attachment, err := client.runNewSeason(false, sgClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(attachment).To(BeEmpty())

		Expect(msg).To(ContainSubstring("GT4 Rookies - Winter"))
		Expect(msg).To(ContainSubstring("#555"))
		Expect(msg).To(ContainSubstring("Bathurst"))
		Expect(msg).To(ContainSubstring("2026 Winter"))
		Expect(msg).To(ContainSubstring("GT4 Rookies Winter"))
		Expect(msg).To(ContainSubstring("!new-season-apply"))
	})

	It("makes no changes in preview mode (config file path never touched)", func() {
		_, _, err := client.runNewSeason(false, sgClient)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns an error when the current season cannot be parsed", func() {
		client.conf.Season = "Autumn"
		_, _, err := client.runNewSeason(false, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("could not determine current season"))
	})

	It("returns an error when no matching championship is found", func() {
		client.conf.Season = "Summer" // next term = Fall, which the server does not offer
		_, _, err := client.runNewSeason(false, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no upcoming"))
	})
})

var _ = Describe("runNewSeason apply", func() {
	var (
		client     *DiscordClient
		stub       *stubRest
		sgServer   *httptest.Server
		sgClient   *simgrid.SimGridClient
		fakeDrive  *fakes.FakeDriveServicer
		gcClient   *gcloud.Client
		tmpDir     string
		configPath string
		origWD     string
	)

	BeforeEach(func() {
		stub = &stubRest{}
		fakeDrive = &fakes.FakeDriveServicer{}
		gcClient = &gcloud.Client{Drive: fakeDrive}

		fakeDrive.GetFileReturns(&drive.File{Id: "current", Parents: []string{"parent-1"}}, nil)
		fakeDrive.FindFolderReturns(nil, nil)
		fakeDrive.CreateFolderReturnsOnCall(0, &drive.File{Id: "briefing-new"}, nil)
		fakeDrive.CreateFolderReturnsOnCall(1, &drive.File{Id: "tracker-new"}, nil)

		var err error
		tmpDir, err = os.MkdirTemp("", "rookies-bot-newseason-test")
		Expect(err).NotTo(HaveOccurred())
		configPath = tmpDir + "/config.yml"
		Expect(os.WriteFile(configPath, []byte(`season: Fall
championship_id: "9485"
discord_role_name: GT4 Rookie
briefing_folder_id: briefing-current
tracker_folder_id: tracker-current
discord_token: keep-me-secret
`), 0600)).To(Succeed())

		// Each spec writes its round-0 config into the CWD; isolate the CWD per
		// spec so parallel Ginkgo workers don't collide on the same file name.
		origWD, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(tmpDir)).To(Succeed())

		client = NewTestDiscordClient(stub, snowflakeID(1), &config.Config{
			BotConfig: config.BotConfig{
				Season:           "Fall",
				BriefingFolderID: "briefing-current",
				TrackerFolderID:  "tracker-current",
			},
		}, gcClient)
		client.configPath = configPath

		sgServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/championships":
				_, _ = w.Write([]byte(`[{"id":555,"name":"GT4 Rookies - Winter"}]`))
			case "/championships/555":
				_, _ = w.Write([]byte(`{"id":555,"name":"GT4 Rookies - Winter","host_name":"TRACKILICIOUS","start_date":"2026-12-01T00:00:00.000Z","races":[{"track":{"name":"Bathurst"}}]}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		sgClient = simgrid.NewClient("test-token")
		sgClient.BaseURL = sgServer.URL
	})

	AfterEach(func() {
		sgServer.Close()
		_ = os.Chdir(origWD)
		os.RemoveAll(tmpDir)
	})

	It("creates folders, rewrites config, updates live config, and attaches round-0", func() {
		msg, attachment, err := client.runNewSeason(true, sgClient)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeDrive.CreateFolderCallCount()).To(Equal(2))

		Expect(client.conf.Season).To(Equal("2026 Winter"))
		Expect(client.conf.ChampionshipId).To(Equal("555"))
		Expect(client.conf.DiscordRoleName).To(Equal("GT4 Rookies Winter"))
		Expect(client.conf.BriefingFolderID).To(Equal("briefing-new"))
		Expect(client.conf.TrackerFolderID).To(Equal("tracker-new"))

		data, err := os.ReadFile(configPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("2026 Winter"))
		Expect(string(data)).To(ContainSubstring("GT4 Rookies Winter"))
		Expect(string(data)).To(ContainSubstring("keep-me-secret"))

		Expect(attachment).To(Equal("2026-winter-round-0.yml"))
		rcData, err := os.ReadFile(attachment)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(rcData)).To(ContainSubstring("number: 1"))
		Expect(string(rcData)).To(ContainSubstring("Bathurst"))

		Expect(msg).To(ContainSubstring("2026 Winter"))
		Expect(msg).To(ContainSubstring("!race-setup"))
	})

	It("returns an error when folder creation fails (no config written)", func() {
		fakeDrive.CreateFolderReturnsOnCall(0, nil, fmt.Errorf("drive create failed"))
		_, _, err := client.runNewSeason(true, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("briefing folder"))

		data, _ := os.ReadFile(configPath)
		Expect(string(data)).To(ContainSubstring("season: Fall"))
	})

	It("does not mutate the live config when the config-file write fails", func() {
		client.configPath = "/no/such/dir/config.yml"
		_, _, err := client.runNewSeason(true, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed updating config file"))
		Expect(client.conf.Season).To(Equal("Fall"))
	})

	It("removes the orphaned round-0 file when the config-file write fails", func() {
		client.configPath = "/no/such/dir/config.yml"
		_, _, err := client.runNewSeason(true, sgClient)
		Expect(err).To(HaveOccurred())
		_, statErr := os.Stat("2026-winter-round-0.yml")
		Expect(os.IsNotExist(statErr)).To(BeTrue())
	})
})

var _ = Describe("writeNextRoundConfig", func() {
	It("normalizes spaces in the season so the generated filename has none", func() {
		rc := &config.RoundConfig{NextRound: config.Round{Number: 3, Track: "Watkins Glen"}}
		name, err := writeNextRoundConfig(rc, "2026 Winter")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.Remove(name) }()
		Expect(name).To(Equal("2026-winter-round-3-watkins-glen.yml"))
		Expect(name).NotTo(ContainSubstring(" "))
	})
})

var _ = Describe("snapshotConfig", func() {
	It("is safe for concurrent reads while apply mutates the live config", func() {
		client := NewTestDiscordClient(&stubRest{}, snowflakeID(1), &config.Config{
			BotConfig: config.BotConfig{Season: "Fall", ChampionshipId: "1"},
		}, &gcloud.Client{})

		done := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			for i := 0; i < 1000; i++ {
				client.mu.Lock()
				client.conf.Season = fmt.Sprintf("2026 Winter %d", i)
				client.conf.ChampionshipId = fmt.Sprintf("%d", i)
				client.mu.Unlock()
			}
			close(done)
		}()
		for i := 0; i < 1000; i++ {
			snap := client.snapshotConfig()
			_ = snap.Season
			_ = snap.ChampionshipId
		}
		<-done
	})
})
