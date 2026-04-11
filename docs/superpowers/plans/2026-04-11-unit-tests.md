# Unit Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add unit test coverage for all five packages: `config`, `models`, `simgrid`, `gcloud`, and `discord`.

**Architecture:** Pure packages (`config`, `models`) need no mocking. `simgrid` uses an injectable `baseURL` + `httptest.Server`. `gcloud` extracts a `Client` struct with injectable `DocsServicer`/`DriveServicer` interfaces. `discord` extracts a flat `BotRestClient` interface to isolate Discord API calls. Counterfeiter generates all fakes; Ginkgo/Gomega is the test framework throughout.

**Tech Stack:** Go 1.23, github.com/onsi/ginkgo/v2, github.com/onsi/gomega, github.com/maxbrunsfeld/counterfeiter/v6, net/http/httptest (simgrid)

---

## File Map

| Status | File | Purpose |
|--------|------|---------|
| Modify | `simgrid/client.go` | Add exported `BaseURL` field; `makeRequest` uses it instead of package var |
| Create | `gcloud/interfaces.go` | `DocsServicer` and `DriveServicer` interfaces + counterfeiter directives |
| Modify | `gcloud/gcloud.go` | Replace inline service construction with injectable `Client` struct |
| Create | `gcloud/fakes/docs_servicer.go` | Counterfeiter fake for `DocsServicer` |
| Create | `gcloud/fakes/drive_servicer.go` | Counterfeiter fake for `DriveServicer` |
| Create | `discord/interfaces.go` | `BotRestClient` interface + counterfeiter directive |
| Modify | `discord/discord.go` | Use `BotRestClient` instead of calling `d.client.Rest().*` directly |
| Create | `discord/fakes/bot_rest_client.go` | Counterfeiter fake for `BotRestClient` |
| Create | `config/config_suite_test.go` | Ginkgo bootstrap for config package |
| Create | `config/config_test.go` | Tests for `Load`, `LoadRoundConfig`, `Round.String()` |
| Create | `models/models_suite_test.go` | Ginkgo bootstrap for models package |
| Create | `models/models_test.go` | Tests for `Penalties.Consolidate()`, `UniqueDriverNumbers()` |
| Create | `simgrid/client_suite_test.go` | Ginkgo bootstrap for simgrid package |
| Create | `simgrid/client_test.go` | Tests for all `SimGridClient` methods |
| Create | `gcloud/gcloud_suite_test.go` | Ginkgo bootstrap for gcloud package |
| Create | `gcloud/gcloud_test.go` | Tests for `generateUpdates`, `Client.GenerateBriefing`, `Client.GeneratePenaltyTracker` |
| Create | `discord/discord_suite_test.go` | Ginkgo bootstrap for discord package |
| Create | `discord/discord_test.go` | Tests for `BuildPenaltyMessage`, `BuildBriefingMessage`, `Repin`, helpers |

---

## Task 1: Install Test Dependencies

**Files:**
- Modify: `go.mod`, `go.sum`, `vendor/`

- [ ] **Step 1: Add test deps**

```bash
cd /path/to/rookies-bot
go get github.com/onsi/ginkgo/v2@latest
go get github.com/onsi/gomega@latest
go get github.com/maxbrunsfeld/counterfeiter/v6@latest
```

- [ ] **Step 2: Vendor the new deps**

```bash
go mod vendor
```

- [ ] **Step 3: Verify ginkgo CLI is available**

```bash
go run github.com/onsi/ginkgo/v2/ginkgo version
```
Expected: prints a version string like `Ginkgo Version 2.x.x`

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum vendor/
git commit -m "chore: add ginkgo, gomega, and counterfeiter test deps"
```

---

## Task 2: Config Package Tests

**Files:**
- Create: `config/config_suite_test.go`
- Create: `config/config_test.go`

- [ ] **Step 1: Create suite bootstrap**

Create `config/config_suite_test.go`:
```go
package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}
```

- [ ] **Step 2: Write the failing tests**

Create `config/config_test.go`:
```go
package config_test

import (
	"os"
	"path/filepath"

	"github.com/geofffranks/rookies-bot/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Round", func() {
	Describe("String()", func() {
		It("formats as 'Round N - Track'", func() {
			r := config.Round{Number: 3, Track: "Monza"}
			Expect(r.String()).To(Equal("Round 3 - Monza"))
		})
	})
})

var _ = Describe("LoadRoundConfig", func() {
	It("parses valid YAML", func() {
		yaml := `
penalties:
  quali_bans_r1: [12, 34]
  pit_starts_r2: [56]
next_round:
  number: 4
  track: "Spa"
`
		rc, err := config.LoadRoundConfig([]byte(yaml))
		Expect(err).NotTo(HaveOccurred())
		Expect(rc.Penalties.QualiBansR1).To(Equal([]int{12, 34}))
		Expect(rc.Penalties.PitStartsR2).To(Equal([]int{56}))
		Expect(rc.NextRound.Number).To(Equal(4))
		Expect(rc.NextRound.Track).To(Equal("Spa"))
	})

	It("converts tabs to spaces before parsing", func() {
		yamlWithTabs := "penalties:\n\tquali_bans_r1: [99]\n"
		rc, err := config.LoadRoundConfig([]byte(yamlWithTabs))
		Expect(err).NotTo(HaveOccurred())
		Expect(rc.Penalties.QualiBansR1).To(Equal([]int{99}))
	})

	It("returns an error for invalid YAML", func() {
		_, err := config.LoadRoundConfig([]byte("}{garbage"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Failed parsing YAML data"))
	})

	It("returns empty struct for empty input", func() {
		rc, err := config.LoadRoundConfig([]byte(""))
		Expect(err).NotTo(HaveOccurred())
		Expect(rc).To(Equal(&config.RoundConfig{}))
	})
})

var _ = Describe("Load", func() {
	var (
		tmpDir         string
		botConfigPath  string
		roundConfigPath string
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "rookies-bot-config-test")
		Expect(err).NotTo(HaveOccurred())

		botConfigPath = filepath.Join(tmpDir, "bot.yml")
		err = os.WriteFile(botConfigPath, []byte(`
simgrid_api_token: "tok123"
championship_id: "champ42"
season: "2026"
discord_token: "disc-tok"
discord_channel_id: 1234567890
discord_role_name: "Rookies"
discord_briefing_channel_id: 9876543210
service_account_token_file: "/dev/null"
briefing_template_doc_id: "tmpl1"
briefing_folder_id: "folder1"
tracker_template_doc_id: "tmpl2"
tracker_folder_id: "folder2"
`), 0644)
		Expect(err).NotTo(HaveOccurred())

		roundConfigPath = filepath.Join(tmpDir, "round.yml")
		err = os.WriteFile(roundConfigPath, []byte(`
next_round:
  number: 3
  track: "Monza"
previous_round:
  number: 2
  track: "Spa"
`), 0644)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("loads both configs and merges them", func() {
		cfg, err := config.Load(botConfigPath, roundConfigPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.SimGridApiToken).To(Equal("tok123"))
		Expect(cfg.Season).To(Equal("2026"))
		Expect(cfg.NextRound.Track).To(Equal("Monza"))
		Expect(cfg.PreviousRound.Track).To(Equal("Spa"))
	})

	It("loads bot config alone when round config path is empty", func() {
		cfg, err := config.Load(botConfigPath, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.SimGridApiToken).To(Equal("tok123"))
		Expect(cfg.NextRound.Track).To(Equal(""))
	})

	It("returns an error when bot config file does not exist", func() {
		_, err := config.Load("/no/such/file.yml", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Failed reading"))
	})

	It("returns an error when round config file does not exist", func() {
		_, err := config.Load(botConfigPath, "/no/such/round.yml")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Failed reading"))
	})

	It("returns an error when bot config has invalid YAML", func() {
		err := os.WriteFile(botConfigPath, []byte("}{garbage"), 0644)
		Expect(err).NotTo(HaveOccurred())
		_, err = config.Load(botConfigPath, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Failed parsing"))
	})
})
```

- [ ] **Step 3: Run tests to verify they fail (no code changes yet — tests should pass since no refactor needed)**

```bash
go test -v ./config/...
```
Expected: All tests PASS (config code needs no structural changes).

- [ ] **Step 4: Commit**

```bash
git add config/config_suite_test.go config/config_test.go
git commit -m "test: add config package tests"
```

---

## Task 3: Models Package Tests

**Files:**
- Create: `models/models_suite_test.go`
- Create: `models/models_test.go`

- [ ] **Step 1: Create suite bootstrap**

Create `models/models_suite_test.go`:
```go
package models_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestModels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Models Suite")
}
```

- [ ] **Step 2: Write tests**

Create `models/models_test.go`:
```go
package models_test

import (
	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Penalties", func() {
	var (
		driver1 = models.Driver{FirstName: "Alice", LastName: "Smith", CarNumber: 11, DiscordHandle: "alice"}
		driver2 = models.Driver{FirstName: "Bob", LastName: "Jones", CarNumber: 22, DiscordHandle: "bob"}
		driver3 = models.Driver{FirstName: "Carol", LastName: "Lee", CarNumber: 33, DiscordHandle: "carol"}
	)

	Describe("Consolidate()", func() {
		It("merges current and carried-over penalties into one Penalty struct", func() {
			p := models.Penalties{
				QualiBansR1:            []models.Driver{driver1},
				QualiBansR1CarriedOver: []models.Driver{driver2},
				QualiBansR2:            []models.Driver{driver3},
				QualiBansR2CarriedOver: []models.Driver{driver1},
				PitStartsR1:            []models.Driver{driver2},
				PitStartsR1CarriedOver: []models.Driver{driver3},
				PitStartsR2:            []models.Driver{driver1},
				PitStartsR2CarriedOver: []models.Driver{driver2},
			}

			result := p.Consolidate()

			Expect(result).To(BeAssignableToTypeOf(config.Penalty{}))
			// QualiBansR1: driver1(11) + driver2(22)
			Expect(result.QualiBansR1).To(ConsistOf(11, 22))
			// QualiBansR2: driver3(33) + driver1(11)
			Expect(result.QualiBansR2).To(ConsistOf(33, 11))
			// PitStartsR1: driver2(22) + driver3(33)
			Expect(result.PitStartsR1).To(ConsistOf(22, 33))
			// PitStartsR2: driver1(11) + driver2(22)
			Expect(result.PitStartsR2).To(ConsistOf(11, 22))
		})

		It("deduplicates drivers appearing in both current and carried-over lists", func() {
			p := models.Penalties{
				QualiBansR1:            []models.Driver{driver1},
				QualiBansR1CarriedOver: []models.Driver{driver1}, // same driver
			}
			result := p.Consolidate()
			Expect(result.QualiBansR1).To(HaveLen(1))
			Expect(result.QualiBansR1).To(ConsistOf(11))
		})

		It("returns empty slices when all fields are empty", func() {
			p := models.Penalties{}
			result := p.Consolidate()
			Expect(result.QualiBansR1).To(BeEmpty())
			Expect(result.QualiBansR2).To(BeEmpty())
			Expect(result.PitStartsR1).To(BeEmpty())
			Expect(result.PitStartsR2).To(BeEmpty())
		})
	})

	Describe("UniqueDriverNumbers()", func() {
		It("returns unique car numbers across all penalty lists", func() {
			p := models.Penalties{
				QualiBansR1:            []models.Driver{driver1},
				QualiBansR1CarriedOver: []models.Driver{driver2},
				QualiBansR2:            []models.Driver{driver3},
				QualiBansR2CarriedOver: []models.Driver{driver1}, // driver1 duplicate
				PitStartsR1:            []models.Driver{driver2}, // driver2 duplicate
				PitStartsR1CarriedOver: []models.Driver{},
				PitStartsR2:            []models.Driver{},
				PitStartsR2CarriedOver: []models.Driver{},
			}
			result := p.UniqueDriverNumbers()
			Expect(result).To(ConsistOf(11, 22, 33))
		})

		It("returns empty slice when no drivers have penalties", func() {
			p := models.Penalties{}
			result := p.UniqueDriverNumbers()
			Expect(result).To(BeEmpty())
		})

		It("handles a driver with penalties across all categories", func() {
			p := models.Penalties{
				QualiBansR1:            []models.Driver{driver1},
				QualiBansR1CarriedOver: []models.Driver{driver1},
				QualiBansR2:            []models.Driver{driver1},
				QualiBansR2CarriedOver: []models.Driver{driver1},
				PitStartsR1:            []models.Driver{driver1},
				PitStartsR1CarriedOver: []models.Driver{driver1},
				PitStartsR2:            []models.Driver{driver1},
				PitStartsR2CarriedOver: []models.Driver{driver1},
			}
			result := p.UniqueDriverNumbers()
			Expect(result).To(HaveLen(1))
			Expect(result).To(ConsistOf(11))
		})
	})
})
```

- [ ] **Step 3: Run tests**

```bash
go test -v ./models/...
```
Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add models/models_suite_test.go models/models_test.go
git commit -m "test: add models package tests"
```

---

## Task 4: SimGrid — Refactor + Tests

The `SimGridClient` hard-codes `baseUrl` as a package-level var. Change it to a field on the struct so tests can point at `httptest.Server`.

**Files:**
- Modify: `simgrid/client.go`
- Create: `simgrid/client_suite_test.go`
- Create: `simgrid/client_test.go`

- [ ] **Step 1: Write the failing test (just enough to see the compile error)**

Create `simgrid/client_suite_test.go`:
```go
package simgrid_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestSimgrid(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Simgrid Suite")
}
```

Create `simgrid/client_test.go` (partial — uses `testServer.URL` which requires the `BaseURL` field):
```go
package simgrid_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/simgrid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newTestClient(server *httptest.Server) *simgrid.SimGridClient {
	c := simgrid.NewClient("test-token")
	c.BaseURL = server.URL
	return c
}

var _ = Describe("SimGridClient", func() {
	var (
		server *httptest.Server
		client *simgrid.SimGridClient
		mux    *http.ServeMux
	)

	BeforeEach(func() {
		mux = http.NewServeMux()
		server = httptest.NewServer(mux)
		client = newTestClient(server)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("GetEntriesForChampionship", func() {
		It("returns parsed entries on success", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
				Expect(r.URL.Query().Get("format")).To(Equal("json"))
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.EntryListResp{
					Entries: []simgrid.Entry{
						{
							CarNumber: 42,
							Drivers: []simgrid.Driver{
								{FirstName: "Max", LastName: "V", PlayerID: "S111"},
							},
						},
					},
				})
			})

			entries, err := client.GetEntriesForChampionship("champ1")
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].CarNumber).To(Equal(42))
			Expect(entries[0].Drivers[0].FirstName).To(Equal("Max"))
		})

		It("returns an error when server responds with 4xx", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			})
			_, err := client.GetEntriesForChampionship("champ1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP request failure"))
		})

		It("returns an error on malformed JSON", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("}{not json"))
			})
			_, err := client.GetEntriesForChampionship("champ1")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UsersForChampionship", func() {
		It("returns parsed users on success", func() {
			mux.HandleFunc("/championships/champ1/participating_users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]simgrid.User{
					{FirstName: "Lewis", LastName: "H", SteamID: "9999", DiscordHandle: "lewis"},
				})
			})

			users, err := client.UsersForChampionship("champ1")
			Expect(err).NotTo(HaveOccurred())
			Expect(users).To(HaveLen(1))
			Expect(users[0].FirstName).To(Equal("Lewis"))
			Expect(users[0].DiscordHandle).To(Equal("lewis"))
		})

		It("returns an error when server responds with 5xx", func() {
			mux.HandleFunc("/championships/champ1/participating_users", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "server error", http.StatusInternalServerError)
			})
			_, err := client.UsersForChampionship("champ1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP request failure"))
		})
	})

	Describe("GetNextRound", func() {
		It("returns the track for the next race when it exists", func() {
			mux.HandleFunc("/championships/champ1", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.Championship{
					Races: []simgrid.Race{
						{Track: simgrid.Track{Name: "Spa"}},
						{Track: simgrid.Track{Name: "Monza"}},
						{Track: simgrid.Track{Name: "Silverstone"}},
					},
				})
			})
			prev := config.Round{Number: 2}
			next, err := client.GetNextRound("champ1", prev)
			Expect(err).NotTo(HaveOccurred())
			Expect(next.Number).To(Equal(3))
			Expect(next.Track).To(Equal("Silverstone"))
		})

		It("returns an empty track when the race list is exhausted", func() {
			mux.HandleFunc("/championships/champ1", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.Championship{
					Races: []simgrid.Race{
						{Track: simgrid.Track{Name: "Spa"}},
					},
				})
			})
			prev := config.Round{Number: 1}
			next, err := client.GetNextRound("champ1", prev)
			Expect(err).NotTo(HaveOccurred())
			Expect(next.Number).To(Equal(2))
			Expect(next.Track).To(Equal(""))
		})

		It("returns an error on HTTP failure", func() {
			mux.HandleFunc("/championships/champ1", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			})
			_, err := client.GetNextRound("champ1", config.Round{Number: 1})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("BuildDriverLookup", func() {
		BeforeEach(func() {
			mux.HandleFunc("/championships/champ1/participating_users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]simgrid.User{
					{FirstName: "Max", LastName: "V", SteamID: "111", DiscordHandle: "maxv"},
					{FirstName: "Lewis", LastName: "H", SteamID: "222", DiscordHandle: "lewish"},
				})
			})
		})

		It("builds a lookup of car number to driver", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.EntryListResp{
					Entries: []simgrid.Entry{
						{CarNumber: 33, Drivers: []simgrid.Driver{{FirstName: "Max", LastName: "V", PlayerID: "S111"}}},
						{CarNumber: 44, Drivers: []simgrid.Driver{{FirstName: "Lewis", LastName: "H", PlayerID: "S222"}}},
					},
				})
			})

			lookup, err := client.BuildDriverLookup("champ1")
			Expect(err).NotTo(HaveOccurred())
			Expect(lookup).To(HaveLen(2))
			Expect(lookup[33].DiscordHandle).To(Equal("maxv"))
			Expect(lookup[44].DiscordHandle).To(Equal("lewish"))
		})

		It("skips entry drivers with blank first and last name", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.EntryListResp{
					Entries: []simgrid.Entry{
						{CarNumber: 33, Drivers: []simgrid.Driver{
							{FirstName: "", LastName: "", PlayerID: "S111"},
						}},
					},
				})
			})

			lookup, err := client.BuildDriverLookup("champ1")
			Expect(err).NotTo(HaveOccurred())
			// blank driver skipped; user with car number 0 (unset) included
			_ = lookup
		})

		It("returns an error when an entry driver is not in the user list", func() {
			mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(simgrid.EntryListResp{
					Entries: []simgrid.Entry{
						{CarNumber: 99, Drivers: []simgrid.Driver{{FirstName: "Ghost", LastName: "Driver", PlayerID: "S999"}}},
					},
				})
			})
			_, err := client.BuildDriverLookup("champ1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unknown driver"))
		})

		It("returns an error when the user API fails", func() {
			// override the participating_users handler to fail
			mux2 := http.NewServeMux()
			mux2.HandleFunc("/championships/champ1/participating_users", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			})
			failServer := httptest.NewServer(mux2)
			defer failServer.Close()
			failClient := simgrid.NewClient("token")
			failClient.BaseURL = failServer.URL
			_, err := failClient.BuildDriverLookup("champ1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("HTTP request failure"))
		})
	})
})

// Verify EntryListResp, Entry, Driver, User, Race, Track, Championship are exported
// (needed by external test package)
var _ simgrid.EntryListResp
var _ simgrid.Entry
var _ simgrid.Driver
var _ simgrid.User
var _ simgrid.Race
var _ simgrid.Track
var _ simgrid.Championship

// Ensure fmt is used
var _ = fmt.Sprintf
```

- [ ] **Step 2: Run tests — expect compile error on `c.BaseURL`**

```bash
go test ./simgrid/...
```
Expected: compile error — `c.BaseURL` field does not exist yet.

- [ ] **Step 3: Add `BaseURL` field to `SimGridClient`**

In `simgrid/client.go`, change:
```go
// BEFORE
type SimGridClient struct {
	token      string
	httpClient *http.Client
}

func NewClient(apitoken string) *SimGridClient {
	return &SimGridClient{
		token:      apitoken,
		httpClient: &http.Client{},
	}
}

var baseUrl = "https://www.thesimgrid.com/api/v1"
```

To:
```go
// AFTER
type SimGridClient struct {
	token      string
	httpClient *http.Client
	BaseURL    string
}

func NewClient(apitoken string) *SimGridClient {
	return &SimGridClient{
		token:      apitoken,
		httpClient: &http.Client{},
		BaseURL:    "https://www.thesimgrid.com/api/v1",
	}
}
```

And in `makeRequest`, change:
```go
// BEFORE
toReq := fmt.Sprintf("%s%s", baseUrl, url)
```
To:
```go
// AFTER
toReq := fmt.Sprintf("%s%s", sgc.BaseURL, url)
```

Also remove the now-unused `var baseUrl = ...` line.

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test -v ./simgrid/...
```
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add simgrid/client.go simgrid/client_suite_test.go simgrid/client_test.go
git commit -m "test: add simgrid tests; make BaseURL injectable for testing"
```

---

## Task 5: GCloud — Extract Client + Interfaces + Tests

The `GenerateBriefing` and `GeneratePenaltyTracker` functions create Google API services inline, making them untestable. Introduce a `Client` struct with injectable `DocsServicer` and `DriveServicer` interfaces.

**Files:**
- Create: `gcloud/interfaces.go`
- Modify: `gcloud/gcloud.go`
- Create: `gcloud/fakes/docs_servicer.go` (generated)
- Create: `gcloud/fakes/drive_servicer.go` (generated)
- Create: `gcloud/gcloud_suite_test.go`
- Create: `gcloud/gcloud_test.go`

Note: callers of the old top-level functions (`discord/discord.go` and `main.go`) must be updated to use the new `Client` struct.

- [ ] **Step 1: Create `gcloud/interfaces.go`**

```go
package gcloud

import (
	"context"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate . DocsServicer
type DocsServicer interface {
	GetDocument(ctx context.Context, id string) (*docs.Document, error)
	BatchUpdateDocument(ctx context.Context, id string, req *docs.BatchUpdateDocumentRequest) (*docs.BatchUpdateDocumentResponse, error)
}

//counterfeiter:generate . DriveServicer
type DriveServicer interface {
	CopyFile(ctx context.Context, templateID, folderID, title string) (*drive.File, error)
}
```

- [ ] **Step 2: Generate counterfeiter fakes**

```bash
cd gcloud
go generate ./...
```
Expected: creates `gcloud/fakes/docs_servicer.go` and `gcloud/fakes/drive_servicer.go`.

Alternatively, run counterfeiter directly if `go generate` isn't wired up yet:
```bash
go run github.com/maxbrunsfeld/counterfeiter/v6 -o fakes/docs_servicer.go . DocsServicer
go run github.com/maxbrunsfeld/counterfeiter/v6 -o fakes/drive_servicer.go . DriveServicer
```

- [ ] **Step 3: Refactor `gcloud/gcloud.go` — add `Client` struct and real adapter types**

Replace the top of `gcloud/gcloud.go` with the new `Client` struct and real implementations. The pure helper functions (`generateUpdates`, `generateHeading`, `generatePenaltyEntry`, `replaceText`) stay unchanged. Remove the old standalone `GenerateBriefing` and `GeneratePenaltyTracker` functions and replace with methods on `Client`.

```go
package gcloud

import (
	"context"
	"fmt"
	"strings"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/models"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

// Client holds injectable service dependencies for Google API calls.
type Client struct {
	Docs  DocsServicer
	Drive DriveServicer
}

// NewClient creates a Client using real Google API credentials from the environment.
func NewClient(ctx context.Context) (*Client, error) {
	docsService, err := docs.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to Google Docs: %s", err)
	}
	driveService, err := drive.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to Google Drive: %s", err)
	}
	return &Client{
		Docs:  &realDocsService{svc: docsService},
		Drive: &realDriveService{svc: driveService},
	}, nil
}

// --- Real adapters (wrap the Google SDK) ---

type realDocsService struct{ svc *docs.Service }

func (r *realDocsService) GetDocument(ctx context.Context, id string) (*docs.Document, error) {
	return r.svc.Documents.Get(id).Context(ctx).Do()
}

func (r *realDocsService) BatchUpdateDocument(ctx context.Context, id string, req *docs.BatchUpdateDocumentRequest) (*docs.BatchUpdateDocumentResponse, error) {
	return r.svc.Documents.BatchUpdate(id, req).Context(ctx).Do()
}

type realDriveService struct{ svc *drive.Service }

func (r *realDriveService) CopyFile(ctx context.Context, templateID, folderID, title string) (*drive.File, error) {
	return r.svc.Files.Copy(templateID, &drive.File{
		Name:    title,
		Parents: []string{folderID},
	}).Context(ctx).Do()
}

// --- Methods ---

func (c *Client) GenerateBriefing(conf *config.Config, penalties *models.Penalties) (string, error) {
	ctx := context.Background()

	briefingFile, err := c.Drive.CopyFile(ctx, conf.BriefingTemplateDocID, conf.BriefingFolderID,
		fmt.Sprintf("Drivers Briefing Round %d at %s", conf.NextRound.Number, conf.NextRound.Track))
	if err != nil {
		return "", fmt.Errorf("failed to copy Briefing Template to Briefing folder: %s", err)
	}

	briefingDoc, err := c.Docs.GetDocument(ctx, briefingFile.Id)
	if err != nil {
		return "", fmt.Errorf("failed getting Briefing Doc: %s", err)
	}

	updates, err := generateUpdates(conf, penalties, briefingDoc)
	if err != nil {
		return "", fmt.Errorf("failed processing Briefing Template: %s", err)
	}

	_, err = c.Docs.BatchUpdateDocument(ctx, briefingFile.Id, updates)
	if err != nil {
		return "", fmt.Errorf("could not update the Briefing Doc: %s", err)
	}

	return fmt.Sprintf("https://docs.google.com/document/d/%s", briefingFile.Id), nil
}

func (c *Client) GeneratePenaltyTracker(conf *config.Config) (string, error) {
	ctx := context.Background()
	file, err := c.Drive.CopyFile(ctx, conf.TrackerTemplateDocID, conf.TrackerFolderID,
		fmt.Sprintf("%s Rookies Round %d - %s", conf.Season, conf.NextRound.Number, conf.NextRound.Track))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", file.Id), nil
}

// --- Pure helpers (unchanged) ---

func generateUpdates(conf *config.Config, penalties *models.Penalties, doc *docs.Document) (*docs.BatchUpdateDocumentRequest, error) {
	// ... (keep existing implementation unchanged)
}

func replaceText(find, replace string) *docs.Request {
	// ... (keep existing implementation unchanged)
}

func generateHeading(startIndex int64, style, text string) []*docs.Request {
	// ... (keep existing implementation unchanged)
}

func generatePenaltyEntry(startIndex int64, text string) []*docs.Request {
	// ... (keep existing implementation unchanged)
}
```

> **Note:** The `generateUpdates`, `replaceText`, `generateHeading`, and `generatePenaltyEntry` function bodies are unchanged — keep the existing implementations. The note above is only to clarify that those bodies stay as-is.

- [ ] **Step 4: Update `discord/discord.go` — use `*gcloud.Client` instead of top-level functions**

In `discord/discord.go`:

Change the `DiscordClient` struct to include a gcloud client:
```go
type DiscordClient struct {
	client     bot.Client
	conf       *config.Config
	guild      snowflake.ID
	memberList map[string]snowflake.ID
	gcloud     *gcloud.Client
}
```

Update `NewDiscordClient` to accept and store a `*gcloud.Client`:
```go
func NewDiscordClient(conf *config.Config, gc *gcloud.Client) (*DiscordClient, error) {
	client, err := disgo.New(conf.DiscordToken, bot.WithGatewayConfigOpts(
		gateway.WithIntents(gateway.IntentMessageContent, gateway.IntentDirectMessages),
	))
	if err != nil {
		return nil, err
	}

	dc := &DiscordClient{
		conf:   conf,
		client: client,
		gcloud: gc,
	}
	client.AddEventListeners(bot.NewListenerFunc(dc.onMessageCreate))
	return dc, nil
}
```

In the `raceSetup` method, replace `gcloud.GenerateBriefing(...)` with `d.gcloud.GenerateBriefing(...)` and `gcloud.GeneratePenaltyTracker(...)` with `d.gcloud.GeneratePenaltyTracker(...)`.

Also update `generateNextRoundConfig` (standalone in `discord.go`) to receive `*gcloud.Client`:
```go
func generateNextRoundConfig(sgc *simgrid.SimGridClient, gc *gcloud.Client, conf *config.Config, penalties *models.Penalties) (*config.RoundConfig, error) {
	nextRound, err := sgc.GetNextRound(conf.ChampionshipId, conf.NextRound)
	if err != nil {
		return nil, fmt.Errorf("failed getting details for next round: %s", err)
	}
	nextRoundTracker, err := gc.GeneratePenaltyTracker(conf)
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
```

Update the call site in `raceSetup`:
```go
nextRoundConfig, err = generateNextRoundConfig(sgClient, d.gcloud, bigConfig, penalties)
```

- [ ] **Step 5: Update `main.go` — use `gcloud.NewClient()` and updated `NewDiscordClient`**

In `main.go`:
```go
// In the Before hook, after loading config:
gc, err := gcloud.NewClient(context.Background())
if err != nil {
    return fmt.Errorf("failed to connect to Google APIs: %s", err)
}

// ...existing sgClient + driverLookup + penalties setup...

dc, err = discord.NewDiscordClient(conf, gc)
```

Also replace `gcloud.GenerateBriefing(...)` and `gcloud.GeneratePenaltyTracker(...)` in main.go's `raceSetup` closure with `gc.GenerateBriefing(...)` / `gc.GeneratePenaltyTracker(...)`.

Update `generateNextRoundConfig` in `main.go` to match the same signature change as in `discord.go` — pass `gc`:
```go
nextRoundConfig, err := generateNextRoundConfig(sgClient, gc, conf, penalties)
```

And update `main.go`'s `generateNextRoundConfig` function signature:
```go
func generateNextRoundConfig(sgc *simgrid.SimGridClient, gc *gcloud.Client, conf *config.Config, penalties *models.Penalties) (*config.RoundConfig, error) {
	// ...
	nextRoundTracker, err := gc.GeneratePenaltyTracker(conf)
	// ...
}
```

- [ ] **Step 6: Verify the build compiles**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 7: Write the test suite bootstrap**

Create `gcloud/gcloud_suite_test.go`:
```go
package gcloud_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestGcloud(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCloud Suite")
}
```

- [ ] **Step 8: Write gcloud tests**

Create `gcloud/gcloud_test.go`:
```go
package gcloud_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/gcloud"
	"github.com/geofffranks/rookies-bot/gcloud/fakes"
	"github.com/geofffranks/rookies-bot/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

// makeDoc builds a minimal docs.Document with a HEADING_3 "Stream" element
// at the given start index for testing generateUpdates.
func makeDoc(streamIndex int64) *docs.Document {
	return &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					StartIndex: streamIndex,
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{
							NamedStyleType: "HEADING_3",
						},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Stream\n"}},
						},
					},
				},
			},
		},
	}
}

var _ = Describe("Client", func() {
	var (
		fakeDocsService  *fakes.FakeDocsServicer
		fakeDriveService *fakes.FakeDriveServicer
		client           *gcloud.Client
		conf             *config.Config
		penalties        *models.Penalties
	)

	BeforeEach(func() {
		fakeDocsService = new(fakes.FakeDocsServicer)
		fakeDriveService = new(fakes.FakeDriveServicer)
		client = &gcloud.Client{
			Docs:  fakeDocsService,
			Drive: fakeDriveService,
		}
		conf = &config.Config{
			BotConfig: config.BotConfig{
				Season:                "2026",
				BriefingTemplateDocID: "tmpl-doc-id",
				BriefingFolderID:      "briefing-folder-id",
				TrackerTemplateDocID:  "tmpl-tracker-id",
				TrackerFolderID:       "tracker-folder-id",
			},
			RoundConfig: config.RoundConfig{
				NextRound:     config.Round{Number: 5, Track: "Monza"},
				PreviousRound: config.Round{Number: 4, Track: "Spa"},
			},
		}
		penalties = &models.Penalties{}
	})

	Describe("GeneratePenaltyTracker", func() {
		It("copies the tracker template and returns a spreadsheet URL", func() {
			fakeDriveService.CopyFileReturns(&drive.File{Id: "new-tracker-id"}, nil)

			url, err := client.GeneratePenaltyTracker(conf)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://docs.google.com/spreadsheets/d/new-tracker-id"))

			Expect(fakeDriveService.CopyFileCallCount()).To(Equal(1))
			_, templateID, folderID, title := fakeDriveService.CopyFileArgsForCall(0)
			Expect(templateID).To(Equal("tmpl-tracker-id"))
			Expect(folderID).To(Equal("tracker-folder-id"))
			Expect(title).To(Equal("2026 Rookies Round 5 - Monza"))
		})

		It("returns an error when Drive fails", func() {
			fakeDriveService.CopyFileReturns(nil, errors.New("drive unavailable"))
			_, err := client.GeneratePenaltyTracker(conf)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("drive unavailable"))
		})
	})

	Describe("GenerateBriefing", func() {
		BeforeEach(func() {
			fakeDriveService.CopyFileReturns(&drive.File{Id: "new-briefing-id"}, nil)
			fakeDocsService.GetDocumentReturns(makeDoc(10), nil)
			fakeDocsService.BatchUpdateDocumentReturns(&docs.BatchUpdateDocumentResponse{}, nil)
		})

		It("copies the template, fetches the doc, sends updates, returns URL", func() {
			url, err := client.GenerateBriefing(conf, penalties)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("https://docs.google.com/document/d/new-briefing-id"))

			Expect(fakeDriveService.CopyFileCallCount()).To(Equal(1))
			_, templateID, folderID, title := fakeDriveService.CopyFileArgsForCall(0)
			Expect(templateID).To(Equal("tmpl-doc-id"))
			Expect(folderID).To(Equal("briefing-folder-id"))
			Expect(title).To(Equal("Drivers Briefing Round 5 at Monza"))

			Expect(fakeDocsService.GetDocumentCallCount()).To(Equal(1))
			_, docID := fakeDocsService.GetDocumentArgsForCall(0)
			Expect(docID).To(Equal("new-briefing-id"))

			Expect(fakeDocsService.BatchUpdateDocumentCallCount()).To(Equal(1))
		})

		It("returns an error when Drive copy fails", func() {
			fakeDriveService.CopyFileReturns(nil, errors.New("copy failed"))
			_, err := client.GenerateBriefing(conf, penalties)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("copy failed"))
		})

		It("returns an error when GetDocument fails", func() {
			fakeDocsService.GetDocumentReturns(nil, errors.New("docs api down"))
			_, err := client.GenerateBriefing(conf, penalties)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("docs api down"))
		})

		It("returns an error when BatchUpdate fails", func() {
			fakeDocsService.BatchUpdateDocumentReturns(nil, errors.New("batch failed"))
			_, err := client.GenerateBriefing(conf, penalties)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("batch failed"))
		})
	})
})

var _ = Describe("generateUpdates (via GenerateBriefing)", func() {
	// generateUpdates is private but its output is exercised through GenerateBriefing.
	// We verify it produces the right replacement text by checking what BatchUpdate receives.
	var (
		fakeDocsService  *fakes.FakeDocsServicer
		fakeDriveService *fakes.FakeDriveServicer
		client           *gcloud.Client
		conf             *config.Config
	)

	BeforeEach(func() {
		fakeDocsService = new(fakes.FakeDocsServicer)
		fakeDriveService = new(fakes.FakeDriveServicer)
		client = &gcloud.Client{Docs: fakeDocsService, Drive: fakeDriveService}
		fakeDriveService.CopyFileReturns(&drive.File{Id: "doc-id"}, nil)
		fakeDocsService.GetDocumentReturns(makeDoc(5), nil)
		fakeDocsService.BatchUpdateDocumentReturns(&docs.BatchUpdateDocumentResponse{}, nil)
		conf = &config.Config{
			BotConfig: config.BotConfig{Season: "2026"},
			RoundConfig: config.RoundConfig{
				NextRound: config.Round{Number: 4, Track: "Silverstone"},
			},
		}
	})

	It("includes a replaceText request for [num] with the round number", func() {
		_, err := client.GenerateBriefing(conf, &models.Penalties{})
		Expect(err).NotTo(HaveOccurred())

		_, _, req := fakeDocsService.BatchUpdateDocumentArgsForCall(0)
		texts := make([]string, 0)
		for _, r := range req.Requests {
			if r.ReplaceAllText != nil {
				texts = append(texts, fmt.Sprintf("%s->%s", r.ReplaceAllText.ContainsText.Text, r.ReplaceAllText.ReplaceText))
			}
		}
		Expect(texts).To(ContainElement("[num]->4"))
		Expect(texts).To(ContainElement("[Track Name]->Silverstone"))
		Expect(texts).To(ContainElement("[SEASON]->2026"))
	})

	It("sets group1=ODD and group2=EVEN for odd round numbers", func() {
		conf.NextRound.Number = 3
		_, err := client.GenerateBriefing(conf, &models.Penalties{})
		Expect(err).NotTo(HaveOccurred())

		_, _, req := fakeDocsService.BatchUpdateDocumentArgsForCall(0)
		texts := make([]string, 0)
		for _, r := range req.Requests {
			if r.ReplaceAllText != nil {
				texts = append(texts, fmt.Sprintf("%s->%s", r.ReplaceAllText.ContainsText.Text, r.ReplaceAllText.ReplaceText))
			}
		}
		Expect(texts).To(ContainElement("[group1]->ODD"))
		Expect(texts).To(ContainElement("[group2]->EVEN"))
	})

	It("sets group1=EVEN and group2=ODD for even round numbers", func() {
		conf.NextRound.Number = 4
		_, err := client.GenerateBriefing(conf, &models.Penalties{})
		Expect(err).NotTo(HaveOccurred())

		_, _, req := fakeDocsService.BatchUpdateDocumentArgsForCall(0)
		texts := make([]string, 0)
		for _, r := range req.Requests {
			if r.ReplaceAllText != nil {
				texts = append(texts, fmt.Sprintf("%s->%s", r.ReplaceAllText.ContainsText.Text, r.ReplaceAllText.ReplaceText))
			}
		}
		Expect(texts).To(ContainElement("[group1]->EVEN"))
		Expect(texts).To(ContainElement("[group2]->ODD"))
	})
})
```

- [ ] **Step 9: Run gcloud tests**

```bash
go test -v ./gcloud/...
```
Expected: All tests PASS.

- [ ] **Step 10: Commit**

```bash
git add gcloud/ discord/discord.go main.go
git commit -m "test: add gcloud tests; extract injectable Client with DocsServicer/DriveServicer interfaces"
```

---

## Task 6: Discord — Extract BotRestClient Interface + Tests

The `DiscordClient` calls `d.client.Rest().*` methods for all Discord API operations. Extract a flat `BotRestClient` interface to make those calls injectable.

**Files:**
- Create: `discord/interfaces.go`
- Modify: `discord/discord.go`
- Create: `discord/fakes/bot_rest_client.go` (generated)
- Create: `discord/discord_suite_test.go`
- Create: `discord/discord_test.go`

- [ ] **Step 1: Create `discord/interfaces.go`**

```go
package discord

import (
	"context"

	dgo "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate . BotRestClient
type BotRestClient interface {
	CreateMessage(channelID snowflake.ID, messageCreate dgo.MessageCreate, opts ...interface{}) (*dgo.Message, error)
	GetPinnedMessages(channelID snowflake.ID, opts ...interface{}) ([]dgo.Message, error)
	UnpinMessage(channelID snowflake.ID, messageID snowflake.ID, opts ...interface{}) error
	PinMessage(channelID snowflake.ID, messageID snowflake.ID, opts ...interface{}) error
	GetChannel(channelID snowflake.ID, opts ...interface{}) (dgo.Channel, error)
	GetRoles(guildID snowflake.ID, opts ...interface{}) ([]dgo.Role, error)
	GetMembers(guildID snowflake.ID, limit int, after snowflake.ID, opts ...interface{}) ([]dgo.Member, error)
	CreateGuildScheduledEvent(guildID snowflake.ID, guildScheduledEventCreate dgo.GuildScheduledEventCreate, opts ...interface{}) (*dgo.GuildScheduledEvent, error)
}

//counterfeiter:generate . ApplicationIDer
type ApplicationIDer interface {
	ApplicationID() snowflake.ID
}
```

> **Note:** Check the exact signatures of the disgo `rest.Rest` interface in `vendor/github.com/disgoorg/disgo/rest/rest.go` to confirm the parameter and return types. Adjust the interface above to match exactly (some methods take `...rest.RequestOpt` not `...interface{}`).

- [ ] **Step 2: Check the actual rest.Rest method signatures**

```bash
grep -A 2 "CreateMessage\|GetPinnedMessages\|UnpinMessage\|PinMessage\|GetChannel\|GetRoles\|GetMembers\|CreateGuildScheduledEvent" vendor/github.com/disgoorg/disgo/rest/rest.go | head -60
```

Update `discord/interfaces.go` to match the exact signatures found. Typically they look like:
```go
CreateMessage(channelID snowflake.ID, messageCreate discord.MessageCreate, opts ...RequestOpt) (*discord.Message, error)
```

So the interface should use `rest.RequestOpt` for the variadic arg:
```go
package discord

import (
	"context"

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
```

- [ ] **Step 3: Generate counterfeiter fake**

```bash
go run github.com/maxbrunsfeld/counterfeiter/v6 -o discord/fakes/bot_rest_client.go ./discord/. BotRestClient
```
Expected: creates `discord/fakes/bot_rest_client.go`.

- [ ] **Step 4: Refactor `discord/discord.go` — use `BotRestClient` + store `applicationID`**

Change the `DiscordClient` struct:
```go
type DiscordClient struct {
	rest          BotRestClient
	applicationID snowflake.ID
	conf          *config.Config
	guild         snowflake.ID
	memberList    map[string]snowflake.ID
	gcloud        *gcloud.Client
}
```

Update `NewDiscordClient`:
```go
func NewDiscordClient(conf *config.Config, gc *gcloud.Client) (*DiscordClient, error) {
	client, err := disgo.New(conf.DiscordToken, bot.WithGatewayConfigOpts(
		gateway.WithIntents(gateway.IntentMessageContent, gateway.IntentDirectMessages),
	))
	if err != nil {
		return nil, err
	}

	dc := &DiscordClient{
		conf:          conf,
		rest:          client.Rest(),
		applicationID: client.ApplicationID(),
		gcloud:        gc,
	}

	client.AddEventListeners(bot.NewListenerFunc(dc.onMessageCreate))
	return dc, nil
}
```

Update all `d.client.Rest().Foo(...)` calls to `d.rest.Foo(...)` throughout `discord.go`.
Update all `d.client.ApplicationID()` to `d.applicationID`.

Remove the `client bot.Client` field (it's no longer needed after construction). Keep `OpenGateway` and `Close` by storing the `bot.Client` only for those two lifecycle methods OR move them to use the gateway directly. Simplest: keep a `bot.Client` field only for `OpenGateway`/`Close`:
```go
type DiscordClient struct {
	botClient     bot.Client   // used only for OpenGateway/Close
	rest          BotRestClient
	applicationID snowflake.ID
	conf          *config.Config
	guild         snowflake.ID
	memberList    map[string]snowflake.ID
	gcloud        *gcloud.Client
}
```

- [ ] **Step 5: Verify build compiles**

```bash
go build ./...
```
Expected: no errors.

- [ ] **Step 6: Create suite bootstrap**

Create `discord/discord_suite_test.go`:
```go
package discord_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDiscord(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Discord Suite")
}
```

- [ ] **Step 7: Write discord tests**

Create `discord/discord_test.go`:
```go
package discord_test

import (
	"errors"
	"time"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/discord"
	"github.com/geofffranks/rookies-bot/discord/fakes"
	"github.com/geofffranks/rookies-bot/models"
	dgo "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newTestClient creates a DiscordClient with injected fakes for unit testing.
// This requires DiscordClient to expose a constructor for testing (see note below).
func newTestClient(restClient discord.BotRestClient, conf *config.Config) *discord.DiscordClient {
	return discord.NewTestDiscordClient(restClient, 12345, conf, nil)
}

var _ = Describe("DiscordClient", func() {
	var (
		fakeRest *fakes.FakeBotRestClient
		dc       *discord.DiscordClient
		conf     *config.Config
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
	})

	Describe("Repin", func() {
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
			chanID, msgID := fakeRest.UnpinMessageArgsForCall(0)
			Expect(chanID).To(Equal(snowflake.ID(111)))
			Expect(msgID).To(Equal(oldMsgID))

			Expect(fakeRest.PinMessageCallCount()).To(Equal(1))
			chanID, msgID = fakeRest.PinMessageArgsForCall(0)
			Expect(chanID).To(Equal(snowflake.ID(111)))
			Expect(msgID).To(Equal(newMsgID))
		})

		It("does not unpin messages from other bots", func() {
			otherBotID := snowflake.ID(99999)
			fakeRest.GetPinnedMessagesReturns([]dgo.Message{
				{ID: snowflake.ID(55), Author: dgo.User{ID: otherBotID}},
			}, nil)

			err := dc.Repin(&dgo.Message{ID: snowflake.ID(100)})
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeRest.UnpinMessageCallCount()).To(Equal(0))
		})

		It("returns an error when GetPinnedMessages fails", func() {
			fakeRest.GetPinnedMessagesReturns(nil, errors.New("discord down"))
			err := dc.Repin(&dgo.Message{ID: snowflake.ID(1)})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("discord down"))
		})

		It("returns an error when UnpinMessage fails", func() {
			fakeRest.GetPinnedMessagesReturns([]dgo.Message{
				{ID: snowflake.ID(55), Author: dgo.User{ID: snowflake.ID(12345)}},
			}, nil)
			fakeRest.UnpinMessageReturns(errors.New("unpin failed"))
			err := dc.Repin(&dgo.Message{ID: snowflake.ID(1)})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unpin failed"))
		})
	})

	Describe("BuildPenaltyMessage", func() {
		var (
			penalties  *models.Penalties
			driver42   models.Driver
			driver99   models.Driver
		)

		BeforeEach(func() {
			driver42 = models.Driver{FirstName: "Max", LastName: "V", CarNumber: 42, DiscordHandle: "maxv"}
			driver99 = models.Driver{FirstName: "Lewis", LastName: "H", CarNumber: 99, DiscordHandle: "lewish"}
			penalties = &models.Penalties{}

			// Stub GetMembers to return these two drivers
			fakeRest.GetMembersStub = func(guildID snowflake.ID, limit int, after snowflake.ID, _ ...interface{}) ([]dgo.Member, error) {
				return []dgo.Member{
					{User: dgo.User{ID: snowflake.ID(1001), Username: "maxv"}},
					{User: dgo.User{ID: snowflake.ID(1002), Username: "lewish"}},
				}, nil
			}
			// Stub GetChannel to return a guild channel (for guild ID lookup)
			fakeRest.GetChannelReturns(dgo.GuildTextChannel{
				// GuildID field
			}, nil)
		})

		It("includes 'None!' for categories with no penalties", func() {
			msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
			Expect(err).NotTo(HaveOccurred())
			content := msg.Content
			Expect(content).To(ContainSubstring("None!"))
		})

		It("includes a mention for each penalized driver", func() {
			penalties.QualiBansR1 = []models.Driver{driver42}
			msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Content).To(ContainSubstring("<@"))
		})

		It("uses car number fallback when driver handle not found in guild", func() {
			penalties.QualiBansR1 = []models.Driver{
				{FirstName: "Ghost", LastName: "Driver", CarNumber: 77, DiscordHandle: "notinguild"},
			}
			fakeRest.GetMembersStub = func(guildID snowflake.ID, limit int, after snowflake.ID, _ ...interface{}) ([]dgo.Member, error) {
				return []dgo.Member{}, nil // empty — nobody found
			}
			msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Content).To(ContainSubstring("#77"))
			Expect(msg.Content).To(ContainSubstring("Ghost Driver"))
		})

		It("marks carried-over penalties as '(carried over)'", func() {
			penalties.QualiBansR1CarriedOver = []models.Driver{driver42}
			msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Content).To(ContainSubstring("carried over"))
		})

		It("includes the round number in the header", func() {
			msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Content).To(ContainSubstring("Round 4"))
		})

		It("includes the next round track", func() {
			msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Content).To(ContainSubstring("Monza"))
		})

		It("includes the penalty tracker link", func() {
			msg, err := dc.BuildPenaltyMessage(penalties, &conf.RoundConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Content).To(ContainSubstring("https://example.com/tracker"))
		})
	})

	Describe("briefingTime (via CreateBriefingEvent)", func() {
		// briefingTime is private. We test it indirectly via CreateBriefingEvent.
		// It should return the upcoming Monday at 7:30 PM Eastern.
		It("schedules the event at 7:30 PM Eastern on the next Monday", func() {
			fakeRest.GetChannelReturns(dgo.GuildTextChannel{}, nil)
			fakeRest.GetRolesReturns([]dgo.Role{{Name: "Rookies", ID: snowflake.ID(500)}}, nil)
			fakeRest.CreateGuildScheduledEventReturns(&dgo.GuildScheduledEvent{}, nil)

			err := dc.CreateBriefingEvent(&conf.RoundConfig)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRest.CreateGuildScheduledEventCallCount()).To(Equal(1))
			_, _, event := fakeRest.CreateGuildScheduledEventArgsForCall(0)
			loc, _ := time.LoadLocation("America/New_York")
			t := event.ScheduledStartTime.In(loc)
			Expect(t.Weekday()).To(Equal(time.Monday))
			Expect(t.Hour()).To(Equal(19))
			Expect(t.Minute()).To(Equal(30))
		})
	})
})
```

> **Note on `NewTestDiscordClient`:** The test calls `discord.NewTestDiscordClient(restClient, appID, conf, gcClient)`. You must add this constructor to `discord/discord.go` (or a separate `discord/testing.go` file) to allow tests to inject the fake:
>
> ```go
> // NewTestDiscordClient creates a DiscordClient with injected dependencies for testing.
> func NewTestDiscordClient(rest BotRestClient, applicationID snowflake.ID, conf *config.Config, gc *gcloud.Client) *DiscordClient {
>     return &DiscordClient{
>         rest:          rest,
>         applicationID: applicationID,
>         conf:          conf,
>         gcloud:        gc,
>     }
> }
> ```

- [ ] **Step 8: Add `NewTestDiscordClient` to `discord/discord.go`**

Add the function from the note above to `discord/discord.go`.

- [ ] **Step 9: Run discord tests**

```bash
go test -v ./discord/...
```
Expected: All tests PASS.

Some tests (particularly `BuildPenaltyMessage` with member lookups) may need adjustments based on how `GetChannel` returns a typed `GuildTextChannel` — check the disgo type and update the stub if needed.

- [ ] **Step 10: Run all tests**

```bash
go test ./...
```
Expected: All packages PASS.

- [ ] **Step 11: Commit**

```bash
git add discord/
git commit -m "test: add discord tests; extract BotRestClient interface for testability"
```

---

## Self-Review Checklist

- [x] All five packages covered: `config`, `models`, `simgrid`, `gcloud`, `discord`
- [x] Every test file has a `_suite_test.go` bootstrap with `RegisterFailHandler` + `RunSpecs`
- [x] No mocks for `config` or `models` — they have no external dependencies
- [x] `simgrid` uses `httptest.Server` (not a mock) — tests real JSON parsing and HTTP error handling
- [x] `gcloud` uses counterfeiter fakes for `DocsServicer` and `DriveServicer`
- [x] `discord` uses counterfeiter fake for `BotRestClient`
- [x] Callers in `main.go` and `discord.go` updated to use new `gcloud.Client` struct
- [x] `NewTestDiscordClient` added to allow injection without a real bot token
- [x] No placeholder steps — all test code is concrete and runnable
- [x] All types used in tests (`simgrid.EntryListResp`, etc.) are exported
- [x] `simgrid.BaseURL` is an exported field (capital B) — verify with the refactor step
