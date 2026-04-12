# Discord Test Coverage Expansion — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand unit test coverage of `discord`, `gcloud`, `simgrid`, and `runner` packages by refactoring event handlers into testable inner methods and adding tests for all previously untested paths.

**Architecture:** Outer event handlers (`announcePenalties`, `raceSetup`) become thin wiring shells; inner methods (`runAnnouncePenalties`, `runRaceSetup`) take `*config.RoundConfig` and `*simgrid.SimGridClient`, return `(string, string, error)`, and hold all testable business logic. Internal tests (package `discord`) use a hand-rolled `stubRest` struct to avoid the `discord/fakes` circular import. External tests (package `discord_test`) use counterfeiter fakes.

**Tech Stack:** Go, Ginkgo v2, Gomega, counterfeiter fakes (`discord/fakes`, `gcloud/fakes`), `net/http/httptest`

---

## File Map

| File | Action | Purpose |
|---|---|---|
| `discord/discord.go` | **Modify** | Add `runAnnouncePenalties` and `runRaceSetup` inner methods; thin outer handlers |
| `discord/discord_internal_test.go` | **Modify** | Add `stubRest`, tests for `runAnnouncePenalties`, `runRaceSetup`, `getRoundConfig`, `generateNextRoundConfig` |
| `discord/discord_test.go` | **Modify** | Add 7 `generatePenaltyMessage` error-propagation tests |
| `gcloud/gcloud_test.go` | **Modify** | Add 9 `generateUpdates` tests (carried-over, non-carried-over, no-Stream-heading) |
| `simgrid/client_test.go` | **Modify** | Add `UsersForChampionship` non-JSON unmarshal error test |
| `main_test.go` | **Modify** | Add `newRunner` smoke test |

---

### Task 1: Refactor `announcePenalties` — extract `runAnnouncePenalties`

**Files:**
- Modify: `discord/discord.go`
- Modify: `discord/discord_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `discord/discord_internal_test.go` (after the existing `buildPenaltyList` describe block):

```go
// stubRest implements BotRestClient using function fields so each test can
// inject only the methods it cares about. All stubs default to no-op / nil.
type stubRest struct {
	createMessageFn             func(channelID snowflake.ID, messageCreate dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error)
	getPinnedMessagesFn         func(channelID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Message, error)
	unpinMessageFn              func(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error
	pinMessageFn                func(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error
	getChannelFn                func(channelID snowflake.ID, opts ...rest.RequestOpt) (dgo.Channel, error)
	getRolesFn                  func(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Role, error)
	getMembersFn                func(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error)
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
func (s *stubRest) GetMembers(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error) {
	if s.getMembersFn != nil {
		return s.getMembersFn(guildID, opts...)
	}
	return nil, nil
}
func (s *stubRest) CreateGuildScheduledEvent(guildID snowflake.ID, e dgo.GuildScheduledEventCreate, opts ...rest.RequestOpt) (*dgo.GuildScheduledEvent, error) {
	if s.createGuildScheduledEventFn != nil {
		return s.createGuildScheduledEventFn(guildID, e, opts...)
	}
	return &dgo.GuildScheduledEvent{}, nil
}
```

Then add the imports needed at the top of the file (merge with existing imports):
```go
import (
    // existing imports...
    dgo "github.com/disgoorg/disgo/discord"
    "github.com/disgoorg/disgo/rest"
    "github.com/disgoorg/snowflake/v2"
    "net/http"
    "net/http/httptest"
    "rookies-bot/config"
    "rookies-bot/simgrid"
)
```

Then add this Describe block:

```go
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
        client = NewTestDiscordClient(stub)
        roundConfig = &config.RoundConfig{
            PreviousRound: config.Round{Name: "Round 1"},
            CurrentRound:  config.Round{Name: "Round 2"},
            NextRound:     config.Round{Name: "Round 3"},
            ChannelId:     "111",
            GuildId:       "222",
            Penalties:     config.PenaltyConfig{},
        }
        // default simgrid server: returns empty driver list
        sgServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write([]byte(`[]`))
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
        roundConfig.Penalties.QualiBansR1 = []config.Penalty{{Driver: "Driver A", Car: "UnknownCar999"}}
        _, _, err := client.runAnnouncePenalties(roundConfig, sgClient)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("Failed generating penalty summary"))
    })

    It("returns error when BuildPenaltyMessage fails (GetMembers error)", func() {
        roundConfig.Penalties.QualiBansR1 = []config.Penalty{{Driver: "Driver A", Car: "Ferrari 488 GT3"}}
        stub.getMembersFn = func(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Member, error) {
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
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./discord/... -run "runAnnouncePenalties" -v 2>&1 | tail -20
```

Expected: compilation error — `runAnnouncePenalties` undefined.

- [ ] **Step 3: Add `runAnnouncePenalties` to `discord/discord.go`**

Add this method and thin out `announcePenalties`:

```go
func (d *DiscordClient) runAnnouncePenalties(roundConfig *config.RoundConfig, sgClient *simgrid.SimGridClient) (string, string, error) {
    driverLookup, err := sgClient.BuildDriverLookup(roundConfig)
    if err != nil {
        return "", "", fmt.Errorf("Failed building driver list: %w", err)
    }

    penaltyList, err := buildPenaltyList(roundConfig.Penalties, driverLookup)
    if err != nil {
        return "", "", fmt.Errorf("Failed generating penalty summary: %w", err)
    }

    msg, err := d.BuildPenaltyMessage(roundConfig, penaltyList)
    if err != nil {
        return "", "", fmt.Errorf("Failed to generate penalty message: %w", err)
    }

    sentMsg, err := d.SendMessage(roundConfig.ChannelId, msg)
    if err != nil {
        return "", "", fmt.Errorf("Failed to send penalty announcement: %w", err)
    }

    err = d.Repin(roundConfig.ChannelId, sentMsg.ID.String())
    if err != nil {
        return "", "", fmt.Errorf("Failed to pin penalty announcement: %w", err)
    }

    return msg, "", nil
}

func (d *DiscordClient) announcePenalties(event *events.MessageCreate) {
    var msg, attachment string
    defer func() { sendBotResponse(event, msg, attachment) }()
    roundConfig, err := getRoundConfig(event)
    if err != nil {
        msg = err.Error()
        return
    }
    sgClient := simgrid.NewClient(d.conf.SimGridApiToken)
    msg, attachment, err = d.runAnnouncePenalties(roundConfig, sgClient)
    if err != nil {
        msg = err.Error()
    }
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./discord/... -run "runAnnouncePenalties" -v 2>&1 | tail -20
```

Expected: all 6 specs PASS.

- [ ] **Step 5: Commit**

```bash
git add discord/discord.go discord/discord_internal_test.go
git commit -m "test: add runAnnouncePenalties tests and extract from announcePenalties"
```

---

### Task 2: Refactor `raceSetup` — extract `runRaceSetup`

**Files:**
- Modify: `discord/discord.go`
- Modify: `discord/discord_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Add a `makeStreamDoc` helper and `runRaceSetup` describe block to `discord/discord_internal_test.go`:

```go
// makeStreamDoc returns a minimal *docs.Document with a Stream H3 heading
// at the body index required by generateUpdates.
func makeStreamDoc() *gdocs.Document {
    return &gdocs.Document{
        DocumentId: "doc-123",
        Body: &gdocs.Body{
            Content: []*gdocs.StructuralElement{
                {
                    Paragraph: &gdocs.Paragraph{
                        Elements: []*gdocs.ParagraphElement{
                            {TextRun: &gdocs.TextRun{Content: "Stream\n"}},
                        },
                        ParagraphStyle: &gdocs.ParagraphStyle{
                            NamedStyleType: "HEADING_3",
                        },
                    },
                    StartIndex: 1,
                    EndIndex:   8,
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
        fakeDrive   *gcfakes.FakeDriveServicer
        fakeDocs    *gcfakes.FakeDocsServicer
    )

    BeforeEach(func() {
        stub = &stubRest{}
        client = NewTestDiscordClient(stub)
        fakeDrive = &gcfakes.FakeDriveServicer{}
        fakeDocs = &gcfakes.FakeDocsServicer{}
        gcClient = &gcloud.Client{
            Drive: fakeDrive,
            Docs:  fakeDocs,
        }
        roundConfig = &config.RoundConfig{
            PreviousRound: config.Round{Name: "Round 1"},
            CurrentRound:  config.Round{Name: "Round 2"},
            NextRound:     config.Round{Name: "Round 3", Track: ""},
            ChannelId:     "111",
            GuildId:       "222",
            Penalties:     config.PenaltyConfig{},
        }
        sgServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write([]byte(`[]`))
        }))
        sgClient = simgrid.NewClient("test-token")
        sgClient.BaseURL = sgServer.URL

        // default: Docs returns a minimal doc with Stream heading
        fakeDocs.GetReturns(makeStreamDoc(), nil)
        // default: Drive copy returns a file with a webViewLink
        fakeDrive.CopyReturns(&gdrive.File{WebViewLink: "https://docs.google.com/tracker"}, nil)
        // default: Docs batch update succeeds
        fakeDocs.BatchUpdateReturns(&gdocs.BatchUpdateDocumentResponse{}, nil)
    })

    AfterEach(func() {
        sgServer.Close()
    })

    It("returns error when BuildDriverLookup fails", func() {
        sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusInternalServerError)
        })
        _, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
        Expect(err).To(HaveOccurred())
    })

    It("returns error when GenerateBriefing fails", func() {
        fakeDocs.GetReturns(nil, fmt.Errorf("docs api down"))
        _, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("failed to generate briefing doc"))
    })

    It("returns error when generateNextRoundConfig fails (Track != \"\")", func() {
        roundConfig.NextRound.Track = "Monza"
        sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusInternalServerError)
        })
        _, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("failed to generate config for next round"))
    })

    It("returns error when BuildBriefingMessage fails", func() {
        stub.getRolesFn = func(guildID snowflake.ID, opts ...rest.RequestOpt) ([]dgo.Role, error) {
            return nil, fmt.Errorf("roles fetch failed")
        }
        _, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("failed to generate briefingmessage"))
    })

    It("returns error when SendMessage fails", func() {
        stub.createMessageFn = func(channelID snowflake.ID, messageCreate dgo.MessageCreate, opts ...rest.RequestOpt) (*dgo.Message, error) {
            return nil, fmt.Errorf("send failed")
        }
        _, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("failed to send briefing announcement"))
    })

    It("returns error when Repin fails", func() {
        stub.pinMessageFn = func(channelID snowflake.ID, messageID snowflake.ID, opts ...rest.RequestOpt) error {
            return fmt.Errorf("pin failed")
        }
        _, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("failed to pin briefing announcement"))
    })

    It("returns error when CreateBriefingEvent fails", func() {
        stub.createGuildScheduledEventFn = func(guildID snowflake.ID, e dgo.GuildScheduledEventCreate, opts ...rest.RequestOpt) (*dgo.GuildScheduledEvent, error) {
            return nil, fmt.Errorf("event creation failed")
        }
        _, _, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("failed to create briefing event"))
    })

    Context("happy path, NextRound.Track == \"\"", func() {
        It("returns msg containing round name, empty attachment", func() {
            msg, attachment, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
            Expect(err).NotTo(HaveOccurred())
            Expect(msg).To(ContainSubstring("Round 2"))
            Expect(attachment).To(BeEmpty())
        })
    })

    Context("happy path, NextRound.Track != \"\"", func() {
        It("returns non-empty attachment path, msg contains penalty tracker link", func() {
            roundConfig.NextRound.Track = "Spa"
            // simgrid returns OK for GetNextRound
            sgServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if strings.Contains(r.URL.Path, "event") {
                    w.Header().Set("Content-Type", "application/json")
                    w.WriteHeader(http.StatusOK)
                    _, _ = w.Write([]byte(`{"id":99,"name":"Round 3","track":"Spa","date":"2026-05-01T12:00:00Z"}`))
                } else {
                    w.Header().Set("Content-Type", "application/json")
                    w.WriteHeader(http.StatusOK)
                    _, _ = w.Write([]byte(`[]`))
                }
            })
            msg, attachment, err := client.runRaceSetup(roundConfig, sgClient, gcClient)
            Expect(err).NotTo(HaveOccurred())
            Expect(attachment).NotTo(BeEmpty())
            Expect(msg).To(ContainSubstring("docs.google.com"))
            // cleanup written config file
            if attachment != "" {
                _ = os.Remove(attachment)
            }
        })
    })
})
```

Add `"strings"` and `"os"` to the import block. Also add:
```go
import (
    gcfakes "rookies-bot/gcloud/fakes"
    "rookies-bot/gcloud"
    gdocs "google.golang.org/api/docs/v1"
    gdrive "google.golang.org/api/drive/v3"
    // ...existing imports
)
```

- [ ] **Step 2: Run tests to confirm compilation failure**

```bash
go test ./discord/... -run "runRaceSetup" -v 2>&1 | tail -20
```

Expected: `runRaceSetup` undefined.

- [ ] **Step 3: Add `runRaceSetup` to `discord/discord.go`**

```go
func (d *DiscordClient) runRaceSetup(roundConfig *config.RoundConfig, sgClient *simgrid.SimGridClient, gcClient *gcloud.Client) (string, string, error) {
    driverLookup, err := sgClient.BuildDriverLookup(roundConfig)
    if err != nil {
        return "", "", err
    }

    briefingDoc, err := gcClient.GenerateBriefing(roundConfig, driverLookup)
    if err != nil {
        return "", "", fmt.Errorf("failed to generate briefing doc: %w", err)
    }

    var attachment string
    if roundConfig.NextRound.Track != "" {
        nextConfig, err := generateNextRoundConfig(sgClient, gcClient, roundConfig, roundConfig.Penalties)
        if err != nil {
            return "", "", fmt.Errorf("failed to generate config for next round: %w", err)
        }
        attachment, err = writeNextRoundConfig(nextConfig)
        if err != nil {
            return "", "", fmt.Errorf("failed to write config for next round: %w", err)
        }
    }

    msg, err := d.BuildBriefingMessage(roundConfig, briefingDoc)
    if err != nil {
        return "", "", fmt.Errorf("failed to generate briefingmessage: %w", err)
    }

    sentMsg, err := d.SendMessage(roundConfig.ChannelId, msg)
    if err != nil {
        return "", "", fmt.Errorf("failed to send briefing announcement: %w", err)
    }

    err = d.Repin(roundConfig.ChannelId, sentMsg.ID.String())
    if err != nil {
        return "", "", fmt.Errorf("failed to pin briefing announcement: %w", err)
    }

    _, err = d.CreateBriefingEvent(roundConfig)
    if err != nil {
        return "", "", fmt.Errorf("failed to create briefing event: %w", err)
    }

    return msg, attachment, nil
}

func (d *DiscordClient) raceSetup(event *events.MessageCreate) {
    var msg, attachment string
    defer func() { sendBotResponse(event, msg, attachment) }()
    roundConfig, err := getRoundConfig(event)
    if err != nil {
        msg = err.Error()
        return
    }
    sgClient := simgrid.NewClient(d.conf.SimGridApiToken)
    gcClient, err := gcloud.NewClient(context.Background())
    if err != nil {
        msg = err.Error()
        return
    }
    msg, attachment, err = d.runRaceSetup(roundConfig, sgClient, gcClient)
    if err != nil {
        msg = err.Error()
    }
}
```

Note: if `writeNextRoundConfig` doesn't exist yet, extract the YAML-writing logic from the old `raceSetup` body into a `writeNextRoundConfig(conf *config.RoundConfig) (string, error)` function in `discord.go`.

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./discord/... -run "runRaceSetup" -v 2>&1 | tail -30
```

Expected: all 9 specs PASS.

- [ ] **Step 5: Commit**

```bash
git add discord/discord.go discord/discord_internal_test.go
git commit -m "test: add runRaceSetup tests and extract from raceSetup"
```

---

### Task 3: `getRoundConfig` tests

**Files:**
- Modify: `discord/discord_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `discord/discord_internal_test.go`:

```go
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
previousRound:
  name: "Round 1"
currentRound:
  name: "Round 2"
nextRound:
  name: "Round 3"
channelId: "chan-999"
guildId: "guild-111"
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
        Expect(rc.PreviousRound.Name).To(Equal("Round 1"))
        Expect(rc.CurrentRound.Name).To(Equal("Round 2"))
        Expect(rc.ChannelId).To(Equal("chan-999"))
    })
})
```

Add to imports:
```go
"github.com/disgoorg/disgo/events"
```

- [ ] **Step 2: Run tests to confirm they fail (not yet pass)**

```bash
go test ./discord/... -run "getRoundConfig" -v 2>&1 | tail -20
```

Expected: FAIL — function signature mismatch or import error.

- [ ] **Step 3: Run tests to confirm they pass (no code change needed)**

`getRoundConfig` already exists in `discord.go`. If Step 2 shows compilation errors, fix import paths only. Then:

```bash
go test ./discord/... -run "getRoundConfig" -v 2>&1 | tail -20
```

Expected: all 5 specs PASS.

- [ ] **Step 4: Commit**

```bash
git add discord/discord_internal_test.go
git commit -m "test: add getRoundConfig tests"
```

---

### Task 4: `generateNextRoundConfig` tests

**Files:**
- Modify: `discord/discord_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `discord/discord_internal_test.go`:

```go
var _ = Describe("generateNextRoundConfig", func() {
    var (
        sgServer  *httptest.Server
        sgClient  *simgrid.SimGridClient
        gcClient  *gcloud.Client
        fakeDrive *gcfakes.FakeDriveServicer
        fakeDocs  *gcfakes.FakeDocsServicer
        conf      *config.RoundConfig
        penalties config.PenaltyConfig
    )

    BeforeEach(func() {
        fakeDrive = &gcfakes.FakeDriveServicer{}
        fakeDocs = &gcfakes.FakeDocsServicer{}
        gcClient = &gcloud.Client{
            Drive: fakeDrive,
            Docs:  fakeDocs,
        }
        conf = &config.RoundConfig{
            NextRound: config.Round{Name: "Round 3", Track: "Spa"},
        }
        penalties = config.PenaltyConfig{}

        sgServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write([]byte(`{"id":99,"name":"Round 3","track":"Spa","date":"2026-05-01T12:00:00Z"}`))
        }))
        sgClient = simgrid.NewClient("test-token")
        sgClient.BaseURL = sgServer.URL

        fakeDrive.CopyReturns(&gdrive.File{WebViewLink: "https://docs.google.com/penalty-tracker"}, nil)
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
        fakeDrive.CopyReturns(nil, fmt.Errorf("drive copy failed"))
        _, err := generateNextRoundConfig(sgClient, gcClient, conf, penalties)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("failed generating penalty tracker"))
    })

    It("returns *config.RoundConfig with PenaltyTrackerLink set on happy path", func() {
        result, err := generateNextRoundConfig(sgClient, gcClient, conf, penalties)
        Expect(err).NotTo(HaveOccurred())
        Expect(result.PenaltyTrackerLink).To(ContainSubstring("docs.google.com"))
    })
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./discord/... -run "generateNextRoundConfig" -v 2>&1 | tail -20
```

Expected: FAIL — `generateNextRoundConfig` not found in internal package scope, or import error.

- [ ] **Step 3: Run tests to confirm they pass (no production code change)**

`generateNextRoundConfig` is already a package-level function in `discord.go`. If there are import issues, fix them. Then:

```bash
go test ./discord/... -run "generateNextRoundConfig" -v 2>&1 | tail -20
```

Expected: all 3 specs PASS.

- [ ] **Step 4: Commit**

```bash
git add discord/discord_internal_test.go
git commit -m "test: add generateNextRoundConfig tests"
```

---

### Task 5: `generateUpdates` carried-over and non-carried-over penalty tests

**Files:**
- Modify: `gcloud/gcloud_test.go`

- [ ] **Step 1: Write the failing tests**

Add an `insertedTexts` helper and 8 new `It` blocks inside the existing `Describe("generateUpdates")` (or `Describe("GenerateBriefing")`) block in `gcloud/gcloud_test.go`:

```go
// insertedTexts extracts all InsertText content values from a BatchUpdateDocumentRequest.
func insertedTexts(req *gdocs.BatchUpdateDocumentRequest) []string {
    var texts []string
    for _, r := range req.Requests {
        if r.InsertText != nil {
            texts = append(texts, r.InsertText.Text)
        }
    }
    return texts
}
```

Then add these cases to the `generateUpdates` describe block (or create a new one if needed):

```go
Describe("generateUpdates carried-over penalties", func() {
    var (
        gc        *gcloud.Client
        fakeDocs  *fakes.FakeDocsServicer
        fakeDrive *fakes.FakeDriveServicer
        streamDoc *gdocs.Document
    )

    BeforeEach(func() {
        fakeDocs = &fakes.FakeDocsServicer{}
        fakeDrive = &fakes.FakeDriveServicer{}
        gc = &gcloud.Client{Docs: fakeDocs, Drive: fakeDrive}

        streamDoc = &gdocs.Document{
            DocumentId: "doc-abc",
            Body: &gdocs.Body{
                Content: []*gdocs.StructuralElement{
                    {
                        Paragraph: &gdocs.Paragraph{
                            Elements: []*gdocs.ParagraphElement{
                                {TextRun: &gdocs.TextRun{Content: "Stream\n"}},
                            },
                            ParagraphStyle: &gdocs.ParagraphStyle{NamedStyleType: "HEADING_3"},
                        },
                        StartIndex: 1,
                        EndIndex:   8,
                    },
                },
            },
        }
        fakeDocs.GetReturns(streamDoc, nil)
        fakeDocs.BatchUpdateReturns(&gdocs.BatchUpdateDocumentResponse{}, nil)
    })

    callGenerateUpdates := func(penalties config.PenaltyConfig) {
        rc := &config.RoundConfig{
            CurrentRound: config.Round{Name: "Round 2"},
            Penalties:    penalties,
        }
        err := gc.GenerateBriefing(rc, map[string]string{})
        Expect(err).NotTo(HaveOccurred())
    }

    getCapturedTexts := func() []string {
        Expect(fakeDocs.BatchUpdateCallCount()).To(BeNumerically(">", 0))
        _, req, _ := fakeDocs.BatchUpdateArgsForCall(0)
        return insertedTexts(req)
    }

    It("includes '(carried over)' for QualiBansR1CarriedOver driver", func() {
        callGenerateUpdates(config.PenaltyConfig{
            QualiBansR1CarriedOver: []config.Penalty{{Driver: "Alice", Car: "Ferrari 488 GT3"}},
        })
        Expect(getCapturedTexts()).To(ContainElement(ContainSubstring("Alice")))
        Expect(strings.Join(getCapturedTexts(), " ")).To(ContainSubstring("carried over"))
    })

    It("includes '(carried over)' for QualiBansR2CarriedOver driver", func() {
        callGenerateUpdates(config.PenaltyConfig{
            QualiBansR2CarriedOver: []config.Penalty{{Driver: "Bob", Car: "Ferrari 488 GT3"}},
        })
        Expect(strings.Join(getCapturedTexts(), " ")).To(ContainSubstring("Bob"))
        Expect(strings.Join(getCapturedTexts(), " ")).To(ContainSubstring("carried over"))
    })

    It("includes '(carried over)' for PitStartsR1CarriedOver driver", func() {
        callGenerateUpdates(config.PenaltyConfig{
            PitStartsR1CarriedOver: []config.Penalty{{Driver: "Carol", Car: "Ferrari 488 GT3"}},
        })
        Expect(strings.Join(getCapturedTexts(), " ")).To(ContainSubstring("Carol"))
        Expect(strings.Join(getCapturedTexts(), " ")).To(ContainSubstring("carried over"))
    })

    It("includes '(carried over)' for PitStartsR2CarriedOver driver", func() {
        callGenerateUpdates(config.PenaltyConfig{
            PitStartsR2CarriedOver: []config.Penalty{{Driver: "Dave", Car: "Ferrari 488 GT3"}},
        })
        Expect(strings.Join(getCapturedTexts(), " ")).To(ContainSubstring("Dave"))
        Expect(strings.Join(getCapturedTexts(), " ")).To(ContainSubstring("carried over"))
    })

    It("includes driver without '(carried over)' for QualiBansR1", func() {
        callGenerateUpdates(config.PenaltyConfig{
            QualiBansR1: []config.Penalty{{Driver: "Eve", Car: "Ferrari 488 GT3"}},
        })
        joined := strings.Join(getCapturedTexts(), " ")
        Expect(joined).To(ContainSubstring("Eve"))
        Expect(joined).NotTo(ContainSubstring("carried over"))
    })

    It("includes driver without '(carried over)' for QualiBansR2", func() {
        callGenerateUpdates(config.PenaltyConfig{
            QualiBansR2: []config.Penalty{{Driver: "Frank", Car: "Ferrari 488 GT3"}},
        })
        joined := strings.Join(getCapturedTexts(), " ")
        Expect(joined).To(ContainSubstring("Frank"))
        Expect(joined).NotTo(ContainSubstring("carried over"))
    })

    It("includes driver without '(carried over)' for PitStartsR1", func() {
        callGenerateUpdates(config.PenaltyConfig{
            PitStartsR1: []config.Penalty{{Driver: "Grace", Car: "Ferrari 488 GT3"}},
        })
        joined := strings.Join(getCapturedTexts(), " ")
        Expect(joined).To(ContainSubstring("Grace"))
        Expect(joined).NotTo(ContainSubstring("carried over"))
    })

    It("includes driver without '(carried over)' for PitStartsR2", func() {
        callGenerateUpdates(config.PenaltyConfig{
            PitStartsR2: []config.Penalty{{Driver: "Hank", Car: "Ferrari 488 GT3"}},
        })
        joined := strings.Join(getCapturedTexts(), " ")
        Expect(joined).To(ContainSubstring("Hank"))
        Expect(joined).NotTo(ContainSubstring("carried over"))
    })
})
```

Add `"strings"` to imports if not present.

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./gcloud/... -run "carried-over" -v 2>&1 | tail -30
```

Expected: FAIL — compilation or assertion failures.

- [ ] **Step 3: Run tests to confirm they pass (no production code change)**

`generateUpdates` already handles carried-over fields. If tests fail due to `GenerateBriefing` signature differences, adjust the call to match the actual signature.

```bash
go test ./gcloud/... -run "carried-over" -v 2>&1 | tail -30
```

Expected: all 8 specs PASS.

- [ ] **Step 4: Commit**

```bash
git add gcloud/gcloud_test.go
git commit -m "test: add generateUpdates carried-over and non-carried-over penalty tests"
```

---

### Task 6: `generateUpdates` no-Stream-heading bug documentation test

**Files:**
- Modify: `gcloud/gcloud_test.go`

- [ ] **Step 1: Write the test**

Add to `gcloud/gcloud_test.go` inside the `generateUpdates carried-over penalties` describe block (or a sibling block):

```go
// BUG DOCUMENTATION: penaltyStartIndex is int64, starts at 0, and the guard
// is `if penaltyStartIndex < 0`. Since 0 is never < 0, a doc with no Stream
// heading silently uses index 0 instead of returning an error. This test
// documents the current (buggy) behavior so any future fix is caught.
It("silently uses index 0 when doc has no Stream heading (documents bug)", func() {
    noStreamDoc := &gdocs.Document{
        DocumentId: "doc-no-stream",
        Body: &gdocs.Body{
            Content: []*gdocs.StructuralElement{
                {
                    Paragraph: &gdocs.Paragraph{
                        Elements: []*gdocs.ParagraphElement{
                            {TextRun: &gdocs.TextRun{Content: "Some other heading\n"}},
                        },
                        ParagraphStyle: &gdocs.ParagraphStyle{NamedStyleType: "HEADING_3"},
                    },
                    StartIndex: 1,
                    EndIndex:   20,
                },
            },
        },
    }
    fakeDocs.GetReturns(noStreamDoc, nil)
    fakeDocs.BatchUpdateReturns(&gdocs.BatchUpdateDocumentResponse{}, nil)

    rc := &config.RoundConfig{
        CurrentRound: config.Round{Name: "Round 2"},
        Penalties:    config.PenaltyConfig{},
    }
    // BUG: should return error "no Stream heading found", but currently succeeds
    err := gc.GenerateBriefing(rc, map[string]string{})
    Expect(err).NotTo(HaveOccurred()) // documents current buggy behavior
})
```

- [ ] **Step 2: Run test to confirm it passes (documenting existing behavior)**

```bash
go test ./gcloud/... -run "documents bug" -v 2>&1 | tail -10
```

Expected: PASS (the test passes because the bug makes the function succeed when it shouldn't).

- [ ] **Step 3: Commit**

```bash
git add gcloud/gcloud_test.go
git commit -m "test: document generateUpdates penaltyStartIndex < 0 bug (unreachable guard)"
```

---

### Task 7: `generatePenaltyMessage` error propagation for categories 2–8

**Files:**
- Modify: `discord/discord_test.go`

- [ ] **Step 1: Write the failing tests**

In `discord/discord_test.go`, find the existing `Describe("BuildPenaltyMessage")` block and add these 7 `It` blocks inside it:

```go
Context("GetMembers error propagates for each penalty category", func() {
    BeforeEach(func() {
        fakeRest.GetMembersReturns(nil, fmt.Errorf("members unavailable"))
    })

    It("propagates error for PitStartsR1", func() {
        roundConfig.Penalties = config.PenaltyConfig{
            PitStartsR1: []config.Penalty{{Driver: "Driver X", Car: "Ferrari 488 GT3"}},
        }
        _, err := client.BuildPenaltyMessage(roundConfig, penaltyList)
        Expect(err).To(HaveOccurred())
    })

    It("propagates error for QualiBansR2", func() {
        roundConfig.Penalties = config.PenaltyConfig{
            QualiBansR2: []config.Penalty{{Driver: "Driver X", Car: "Ferrari 488 GT3"}},
        }
        _, err := client.BuildPenaltyMessage(roundConfig, penaltyList)
        Expect(err).To(HaveOccurred())
    })

    It("propagates error for PitStartsR2", func() {
        roundConfig.Penalties = config.PenaltyConfig{
            PitStartsR2: []config.Penalty{{Driver: "Driver X", Car: "Ferrari 488 GT3"}},
        }
        _, err := client.BuildPenaltyMessage(roundConfig, penaltyList)
        Expect(err).To(HaveOccurred())
    })

    It("propagates error for PitStartsR1CarriedOver", func() {
        roundConfig.Penalties = config.PenaltyConfig{
            PitStartsR1CarriedOver: []config.Penalty{{Driver: "Driver X", Car: "Ferrari 488 GT3"}},
        }
        _, err := client.BuildPenaltyMessage(roundConfig, penaltyList)
        Expect(err).To(HaveOccurred())
    })

    It("propagates error for QualiBansR1CarriedOver", func() {
        roundConfig.Penalties = config.PenaltyConfig{
            QualiBansR1CarriedOver: []config.Penalty{{Driver: "Driver X", Car: "Ferrari 488 GT3"}},
        }
        _, err := client.BuildPenaltyMessage(roundConfig, penaltyList)
        Expect(err).To(HaveOccurred())
    })

    It("propagates error for QualiBansR2CarriedOver", func() {
        roundConfig.Penalties = config.PenaltyConfig{
            QualiBansR2CarriedOver: []config.Penalty{{Driver: "Driver X", Car: "Ferrari 488 GT3"}},
        }
        _, err := client.BuildPenaltyMessage(roundConfig, penaltyList)
        Expect(err).To(HaveOccurred())
    })

    It("propagates error for PitStartsR2CarriedOver", func() {
        roundConfig.Penalties = config.PenaltyConfig{
            PitStartsR2CarriedOver: []config.Penalty{{Driver: "Driver X", Car: "Ferrari 488 GT3"}},
        }
        _, err := client.BuildPenaltyMessage(roundConfig, penaltyList)
        Expect(err).To(HaveOccurred())
    })
})
```

Note: `fakeRest`, `roundConfig`, `client`, and `penaltyList` are already declared in the outer `BeforeEach` of the `BuildPenaltyMessage` describe block. Adjust variable names to match the existing test structure.

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./discord/... -run "GetMembers error propagates" -v 2>&1 | tail -30
```

Expected: FAIL — either compilation error or assertion failures.

- [ ] **Step 3: Run tests to confirm they pass (no production code change)**

`generatePenaltyMessage` in `discord.go` already has `return "", err` for non-`DiscordHandleNotFoundError` paths. If tests fail, verify the `GetMembers` fake is the one called by `getDriverId`.

```bash
go test ./discord/... -run "GetMembers error propagates" -v 2>&1 | tail -30
```

Expected: all 7 specs PASS.

- [ ] **Step 4: Commit**

```bash
git add discord/discord_test.go
git commit -m "test: add generatePenaltyMessage error propagation tests for all 8 penalty categories"
```

---

### Task 8: `UsersForChampionship` JSON error + `newRunner` smoke test

**Files:**
- Modify: `simgrid/client_test.go`
- Modify: `main_test.go`

- [ ] **Step 1: Write the failing simgrid test**

In `simgrid/client_test.go`, inside `Describe("SimGridClient")` (or create it if no outer describe exists), add:

```go
Describe("UsersForChampionship", func() {
    It("returns error when server returns 200 with non-JSON body", func() {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write([]byte(`}{not json at all`))
        }))
        defer server.Close()

        client := simgrid.NewClient("test-token")
        client.BaseURL = server.URL

        _, err := client.UsersForChampionship(99)
        Expect(err).To(HaveOccurred())
    })
})
```

- [ ] **Step 2: Write the failing runner smoke test**

In `main_test.go` (create if it doesn't exist — but check first):

```go
package main_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestMain(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Runner Suite")
}
```

Then add a `runner_test.go` (or append to `main_test.go`) with:

```go
var _ = Describe("newRunner", func() {
    It("returns a runner with non-nil factory functions", func() {
        r := newRunner()
        Expect(r.loadConfig).NotTo(BeNil())
        Expect(r.newGCloudClient).NotTo(BeNil())
        Expect(r.newDiscordClient).NotTo(BeNil())
    })
})
```

Note: `newRunner` is unexported. The test must be in `package main` (not `package main_test`) to access it:

```go
package main

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("newRunner", func() {
    It("returns a runner with non-nil factory functions", func() {
        r := newRunner()
        Expect(r.loadConfig).NotTo(BeNil())
        Expect(r.newGCloudClient).NotTo(BeNil())
        Expect(r.newDiscordClient).NotTo(BeNil())
    })
})
```

If a `*_suite_test.go` doesn't exist for the root package, check — Ginkgo specs silently don't run without `RunSpecs`. Add one if missing:

```go
// main_suite_test.go
package main

import (
    "testing"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestRunner(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Runner Suite")
}
```

- [ ] **Step 3: Run tests to confirm they fail**

```bash
go test ./simgrid/... -run "UsersForChampionship" -v 2>&1 | tail -15
go test . -run "newRunner" -v 2>&1 | tail -15
```

Expected: FAIL.

- [ ] **Step 4: Run tests to confirm they pass (no production code change)**

```bash
go test ./simgrid/... -run "UsersForChampionship" -v 2>&1 | tail -15
go test . -run "newRunner" -v 2>&1 | tail -15
```

Expected: PASS for both.

- [ ] **Step 5: Commit**

```bash
git add simgrid/client_test.go main_test.go
git commit -m "test: add UsersForChampionship JSON error test and newRunner smoke test"
```

---

### Task 9: Full coverage verification

**Files:** None — verification only.

- [ ] **Step 1: Run all tests**

```bash
go test ./... -v 2>&1 | tail -40
```

Expected: all specs PASS, no FAILs.

- [ ] **Step 2: Generate coverage report**

```bash
go test ./... -coverprofile=coverage.out 2>&1 | tail -20
go tool cover -func=coverage.out 2>&1 | grep -E "(discord|gcloud|simgrid|runner|total)"
```

Expected: `discord` package ≥ 75%, `gcloud` ≥ 80%, `simgrid` ≥ 80%, total ≥ 70%.

- [ ] **Step 3: Commit coverage profile (optional)**

If you want coverage artifact in git:

```bash
git add coverage.out
git commit -m "chore: add coverage profile after test expansion"
```

Otherwise skip this step.
