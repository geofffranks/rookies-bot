package discord

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	dgo "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/gcloud"
	"github.com/geofffranks/rookies-bot/gcloud/fakes"
	"github.com/geofffranks/rookies-bot/models"
	"github.com/geofffranks/rookies-bot/simgrid"
	"google.golang.org/api/docs/v1"
	drive "google.golang.org/api/drive/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	getPinnedMessagesFn         func(channelID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Message, error)
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
func (s *stubRest) GetPinnedMessages(channelID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Message, error) {
	if s.getPinnedMessagesFn != nil {
		return s.getPinnedMessagesFn(channelID, opts...)
	}
	return nil, nil
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
		Expect(err.Error()).To(ContainSubstring("Failed building driver list"))
	})

	It("returns error when buildPenaltyList fails (unknown car)", func() {
		roundConfig.Penalties.QualiBansR1 = []int{999}
		_, _, err := client.runAnnouncePenalties(roundConfig, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Failed generating penalty summary"))
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
		Expect(err.Error()).To(ContainSubstring("Failed to generate penalty message"))
	})

	It("returns error when SendMessage fails", func() {
		stub.createMessageFn = func(channelID snowflake.ID, messageCreate dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error) {
			return nil, fmt.Errorf("send failed")
		}
		_, _, err := client.runAnnouncePenalties(roundConfig, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Failed to send penalty announcement"))
	})

	It("returns error when Repin fails", func() {
		stub.pinMessageFn = func(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error {
			return fmt.Errorf("pin failed")
		}
		_, _, err := client.runAnnouncePenalties(roundConfig, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Failed to pin penalty announcement"))
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
				DiscordChannelId:          snowflakeID(111),
				DiscordBriefingChannelId:  snowflakeID(222),
				DiscordRoleName:           "test-role",
				Season:                    "S1",
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
