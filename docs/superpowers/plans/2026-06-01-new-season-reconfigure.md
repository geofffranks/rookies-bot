# New-Season Self-Reconfigure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a live Discord admin command pair (`!new-season` preview / `!new-season-apply` commit) that auto-rotates the racing season, looks up the upcoming trackilicious Rookies championship on SimGrid, find-or-creates the Drive briefing/tracker folders, rewrites `config.yml` while preserving comments, updates the running bot's in-memory config without a restart, and emits a round-0 config.

**Architecture:** New pure helpers in `config` (season rotation, role map, comment-preserving YAML patch); new SimGrid client methods (list upcoming, get detail, find-by-host/term); new Drive servicer methods + a `Client.EnsureSeasonFolder` helper; a new `newSeason` orchestrator in `discord` wired like the existing `raceSetup`. The config-file path is threaded from the CLI flag into `DiscordClient` so the command can rewrite it.

**Tech Stack:** Go, Ginkgo/Gomega, counterfeiter fakes, `gopkg.in/yaml.v3`, disgo, Google Drive/Docs API, SimGrid REST API.

**Spec:** [docs/superpowers/specs/2026-06-01-new-season-reconfigure-design.md](../specs/2026-06-01-new-season-reconfigure-design.md)

**Conventions (from repo + author preferences):**
- Tests are **Ginkgo** specs (`Describe`/`It`), never `testing.T`. Every package already has its `*_suite_test.go` bootstrap — do not add new ones.
- Run a single package's specs verbosely with: `rtk proxy go test -v ./<pkg>/` (plain `go test -v` output is swallowed).
- Counterfeiter fakes: always regenerate with an explicit `-o <path>` (default output dir is `<pkg>fakes/`, not `fakes/`).
- `discord/fakes` imports `discord`, so the **internal** `discord` test package (`package discord`) cannot import `discord/fakes`; it uses the local `stubRest` type and `NewTestDiscordClient`. The **external** `discord_test` package uses `discord/fakes`.
- TDD: write the failing test first (RED), watch it fail, then implement.
- One bash command per call; never chain with `&&`/`;`/pipes.

---

## Task 1: Season rotation + role-name map (config package)

**Files:**
- Create: `config/season.go`
- Test: `config/season_test.go`

- [ ] **Step 1: Write the failing test**

Create `config/season_test.go`:

```go
package config_test

import (
	"github.com/geofffranks/rookies-bot/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseSeasonTerm", func() {
	DescribeTable("extracts the term from a season string",
		func(season, expected string) {
			term, err := config.ParseSeasonTerm(season)
			Expect(err).NotTo(HaveOccurred())
			Expect(term).To(Equal(expected))
		},
		Entry("term only", "Fall", "Fall"),
		Entry("year and term", "2026 Summer", "Summer"),
		Entry("two-word term with year", "2027 New Year", "New Year"),
		Entry("two-word term only", "New Year", "New Year"),
		Entry("lowercase", "2026 spring", "Spring"),
		Entry("extra whitespace", "  2026   Winter  ", "Winter"),
	)

	It("returns an error for an unrecognized term", func() {
		_, err := config.ParseSeasonTerm("2026 Autumn")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Autumn"))
	})
})

var _ = Describe("NextTerm", func() {
	DescribeTable("advances through the season cycle",
		func(current, expected string) {
			next, err := config.NextTerm(current)
			Expect(err).NotTo(HaveOccurred())
			Expect(next).To(Equal(expected))
		},
		Entry("New Year -> Spring", "New Year", "Spring"),
		Entry("Spring -> Summer", "Spring", "Summer"),
		Entry("Summer -> Fall", "Summer", "Fall"),
		Entry("Fall -> Winter", "Fall", "Winter"),
		Entry("Winter wraps to New Year", "Winter", "New Year"),
	)

	It("returns an error for an unknown term", func() {
		_, err := config.NextTerm("Autumn")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("RoleNameForTerm", func() {
	DescribeTable("maps each term to its (inconsistent) role name",
		func(term, expected string) {
			role, err := config.RoleNameForTerm(term)
			Expect(err).NotTo(HaveOccurred())
			Expect(role).To(Equal(expected))
		},
		Entry("Summer", "Summer", "GT4 Rookies Summer"),
		Entry("Fall", "Fall", "GT4 Rookies Fall"),
		Entry("Winter", "Winter", "GT4 Rookies Winter"),
		Entry("New Year", "New Year", "GT4 Rookie New"),
		Entry("Spring", "Spring", "GT4 Rookies Springs"),
	)

	It("returns an error for an unknown term", func() {
		_, err := config.RoleNameForTerm("Autumn")
		Expect(err).To(HaveOccurred())
	})
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `rtk proxy go test ./config/`
Expected: FAIL — `undefined: config.ParseSeasonTerm` (and `NextTerm`, `RoleNameForTerm`).

- [ ] **Step 3: Write the implementation**

Create `config/season.go`:

```go
package config

import (
	"fmt"
	"regexp"
	"strings"
)

// SeasonCycle is the chronological order of racing seasons. The year increments
// only on rollover from Winter back to New Year (handled by the caller, which
// takes the year from the matched championship rather than from rotation).
var SeasonCycle = []string{"New Year", "Spring", "Summer", "Fall", "Winter"}

// roleNames maps a season term to the Discord role name for that season. The
// names are deliberately inconsistent ("Rookie" vs "Rookies", "Springs"); they
// match the names configured in Discord exactly.
var roleNames = map[string]string{
	"New Year": "GT4 Rookie New",
	"Spring":   "GT4 Rookies Springs",
	"Summer":   "GT4 Rookies Summer",
	"Fall":     "GT4 Rookies Fall",
	"Winter":   "GT4 Rookies Winter",
}

// leadingYear matches an optional 4-digit year prefix (e.g. "2026 ") so a
// season string of either "Fall" or "2026 Fall" parses to the same term.
var leadingYear = regexp.MustCompile(`^\s*\d{4}\s+`)

// ParseSeasonTerm extracts the season term from a season string, tolerating an
// optional leading year and surrounding whitespace.
func ParseSeasonTerm(season string) (string, error) {
	s := strings.TrimSpace(leadingYear.ReplaceAllString(season, ""))
	for _, term := range SeasonCycle {
		if strings.EqualFold(s, term) {
			return term, nil
		}
	}
	return "", fmt.Errorf("could not determine season term from %q (expected one of: %s)", season, strings.Join(SeasonCycle, ", "))
}

// NextTerm returns the season term that follows term in SeasonCycle, wrapping
// Winter back to New Year.
func NextTerm(term string) (string, error) {
	for i, t := range SeasonCycle {
		if t == term {
			return SeasonCycle[(i+1)%len(SeasonCycle)], nil
		}
	}
	return "", fmt.Errorf("unknown season term %q", term)
}

// RoleNameForTerm returns the Discord role name for the given season term.
func RoleNameForTerm(term string) (string, error) {
	name, ok := roleNames[term]
	if !ok {
		return "", fmt.Errorf("no role name configured for season term %q", term)
	}
	return name, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `rtk proxy go test ./config/`
Expected: PASS (existing config specs still pass too).

- [ ] **Step 5: Commit**

```bash
git add config/season.go config/season_test.go
git commit -m "feat(config): add season rotation and role-name map"
```

---

## Task 2: Comment-preserving config file patch (config package)

**Files:**
- Create: `config/update.go`
- Test: `config/update_test.go`

- [ ] **Step 1: Write the failing test**

Create `config/update_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"

	"github.com/geofffranks/rookies-bot/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("UpdateBotConfigFile", func() {
	var path string

	BeforeEach(func() {
		dir, err := os.MkdirTemp("", "rookies-bot-update-test")
		Expect(err).NotTo(HaveOccurred())
		path = filepath.Join(dir, "config.yml")
		err = os.WriteFile(path, []byte(`# rookies-bot config
discord_token: super-secret-token
season: Fall
championship_id: "9485"
discord_role_name: GT4 Rookie
briefing_folder_id: old-briefing
tracker_folder_id: old-tracker
`), 0600)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(filepath.Dir(path))
	})

	It("updates the requested keys", func() {
		err := config.UpdateBotConfigFile(path, map[string]string{
			"season":             "2026 Summer",
			"championship_id":    "24877",
			"discord_role_name":  "GT4 Rookies Summer",
			"briefing_folder_id": "new-briefing",
			"tracker_folder_id":  "new-tracker",
		})
		Expect(err).NotTo(HaveOccurred())

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var parsed map[string]string
		Expect(yaml.Unmarshal(data, &parsed)).To(Succeed())

		Expect(parsed["season"]).To(Equal("2026 Summer"))
		Expect(parsed["championship_id"]).To(Equal("24877"))
		Expect(parsed["discord_role_name"]).To(Equal("GT4 Rookies Summer"))
		Expect(parsed["briefing_folder_id"]).To(Equal("new-briefing"))
		Expect(parsed["tracker_folder_id"]).To(Equal("new-tracker"))
	})

	It("preserves comments and untouched keys (including secrets)", func() {
		err := config.UpdateBotConfigFile(path, map[string]string{"season": "2026 Summer"})
		Expect(err).NotTo(HaveOccurred())

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("# rookies-bot config"))
		Expect(string(data)).To(ContainSubstring("discord_token: super-secret-token"))
	})

	It("appends a key that is not already present", func() {
		err := config.UpdateBotConfigFile(path, map[string]string{"new_key": "new-value"})
		Expect(err).NotTo(HaveOccurred())

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		var parsed map[string]string
		Expect(yaml.Unmarshal(data, &parsed)).To(Succeed())
		Expect(parsed["new_key"]).To(Equal("new-value"))
	})

	It("returns an error when the file does not exist", func() {
		err := config.UpdateBotConfigFile("/no/such/file.yml", map[string]string{"season": "x"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed reading"))
	})

	It("returns an error when the file is not a YAML mapping", func() {
		Expect(os.WriteFile(path, []byte("- just\n- a\n- list\n"), 0600)).To(Succeed())
		err := config.UpdateBotConfigFile(path, map[string]string{"season": "x"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not a YAML mapping"))
	})
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `rtk proxy go test ./config/`
Expected: FAIL — `undefined: config.UpdateBotConfigFile`.

- [ ] **Step 3: Write the implementation**

Create `config/update.go`:

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// UpdateBotConfigFile rewrites the bot config file at path, setting each
// key/value pair in updates. It round-trips through a yaml.Node so existing
// comments, formatting, and untouched keys (including secrets) are preserved.
// Missing keys are appended to the top-level mapping.
func UpdateBotConfigFile(path string, updates map[string]string) error {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied CLI config path
	if err != nil {
		return fmt.Errorf("failed reading %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("failed parsing %s: %w", path, err)
	}

	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config file %s is not a YAML mapping", path)
	}
	mapping := root.Content[0]

	for key, value := range updates {
		setMappingValue(mapping, key, value)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("failed serializing updated config %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		return fmt.Errorf("failed writing %s: %w", path, err)
	}
	return nil
}

// setMappingValue sets the scalar value for key in a YAML mapping node,
// appending the key/value pair if key is not already present. Mapping node
// Content is a flat slice of [key0, val0, key1, val1, ...].
func setMappingValue(mapping *yaml.Node, key, value string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			valNode := mapping.Content[i+1]
			valNode.Kind = yaml.ScalarNode
			valNode.Tag = "!!str"
			valNode.Style = 0
			valNode.Value = value
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `rtk proxy go test ./config/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add config/update.go config/update_test.go
git commit -m "feat(config): add comment-preserving UpdateBotConfigFile"
```

---

## Task 3: SimGrid list/detail endpoints + GetNextRound refactor (simgrid package)

**Files:**
- Modify: `simgrid/client.go`
- Test: `simgrid/client_test.go`

- [ ] **Step 1: Write the failing test**

Append these specs inside the top-level `Describe("SimGridClient", ...)` block in `simgrid/client_test.go` (after the existing `GetNextRound` block, before `BuildDriverLookup`):

```go
	Describe("ListUpcomingChampionships", func() {
		It("returns the parsed list of id/name items", func() {
			mux.HandleFunc("/championships", func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Query().Get("status")).To(Equal("upcoming"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"id":24877,"name":"GT4 Rookies - Summer"},{"id":24879,"name":"Multiclass Open - Summer"}]`))
			})

			items, err := client.ListUpcomingChampionships()
			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(2))
			Expect(items[0].ID).To(Equal(24877))
			Expect(items[0].Name).To(Equal("GT4 Rookies - Summer"))
		})

		It("returns an error on HTTP failure", func() {
			mux.HandleFunc("/championships", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", http.StatusInternalServerError)
			})
			_, err := client.ListUpcomingChampionships()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetChampionship", func() {
		It("returns the parsed detail including host and start date", func() {
			mux.HandleFunc("/championships/24877", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":24877,"name":"GT4 Rookies - Summer","host_name":"TRACKILICIOUS","start_date":"2026-06-08T23:45:00.000Z","races":[{"track":{"name":"Misano"}}]}`))
			})

			champ, err := client.GetChampionship("24877")
			Expect(err).NotTo(HaveOccurred())
			Expect(champ.ID).To(Equal(24877))
			Expect(champ.Name).To(Equal("GT4 Rookies - Summer"))
			Expect(champ.HostName).To(Equal("TRACKILICIOUS"))
			Expect(champ.Races[0].Track.Name).To(Equal("Misano"))
		})
	})

	Describe("Championship.StartYear", func() {
		It("parses the RFC3339 start date into a year", func() {
			champ := simgrid.Championship{StartDate: "2026-06-08T23:45:00.000Z"}
			year, err := champ.StartYear()
			Expect(err).NotTo(HaveOccurred())
			Expect(year).To(Equal(2026))
		})

		It("returns an error for an unparseable start date", func() {
			champ := simgrid.Championship{StartDate: "not-a-date"}
			_, err := champ.StartYear()
			Expect(err).To(HaveOccurred())
		})
	})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `rtk proxy go test ./simgrid/`
Expected: FAIL — `client.ListUpcomingChampionships undefined`, `champ.ID undefined`, `champ.StartYear undefined`.

- [ ] **Step 3: Write the implementation**

In `simgrid/client.go`, add the `time` and `strconv` imports to the import block:

```go
import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/models"
)
```

Extend the `Championship` struct and add `ChampionshipListItem` (replace the existing `Championship` type definition):

```go
type Championship struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	HostName  string `json:"host_name"`
	StartDate string `json:"start_date"`
	Races     []Race `json:"races"`
}

// StartYear parses the championship's RFC3339 start_date and returns its year.
func (c *Championship) StartYear() (int, error) {
	t, err := time.Parse(time.RFC3339, c.StartDate)
	if err != nil {
		return 0, fmt.Errorf("could not parse championship start date %q: %w", c.StartDate, err)
	}
	return t.Year(), nil
}

// ChampionshipListItem is a single entry in the championships index response,
// which carries only id and name.
type ChampionshipListItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
```

Add the two new fetch methods (place them next to `GetNextRound`):

```go
// ListUpcomingChampionships returns all upcoming multi-race championships
// (id + name only).
func (sgc *SimGridClient) ListUpcomingChampionships() ([]ChampionshipListItem, error) {
	resp, err := sgc.makeRequest("GET", "/championships?status=upcoming&races_count=full_championships")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []ChampionshipListItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetChampionship returns the full detail for a single championship.
func (sgc *SimGridClient) GetChampionship(id string) (*Championship, error) {
	resp, err := sgc.makeRequest("GET", fmt.Sprintf("/championships/%s", id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var champ Championship
	if err := json.Unmarshal(data, &champ); err != nil {
		return nil, err
	}
	return &champ, nil
}
```

Refactor `GetNextRound` to reuse `GetChampionship` (replace the existing method body):

```go
func (sgc *SimGridClient) GetNextRound(id string, prev config.Round) (*config.Round, error) {
	championship, err := sgc.GetChampionship(id)
	if err != nil {
		return nil, err
	}

	nextRoundNum := prev.Number + 1
	nextTrack := ""
	if len(championship.Races) >= nextRoundNum {
		nextTrack = championship.Races[nextRoundNum-1].Track.Name
	}

	return &config.Round{Number: nextRoundNum, Track: nextTrack}, nil
}
```

Note: `strconv` is imported now because Task 4 uses it; if `go vet` flags it as unused after this task only, leave it — Task 4 follows immediately. (If implementing this task in isolation and the build complains about an unused import, add Task 4's `FindSeasonChampionship` in the same change.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `rtk proxy go test ./simgrid/`
Expected: PASS (existing `GetNextRound` specs still pass via the refactor).

If the build fails with `"strconv" imported and not used`, proceed directly to Task 4 (which uses it) and run the tests together.

- [ ] **Step 5: Commit**

```bash
git add simgrid/client.go simgrid/client_test.go
git commit -m "feat(simgrid): add list/detail championship endpoints; reuse in GetNextRound"
```

---

## Task 4: SimGrid FindSeasonChampionship (simgrid package)

**Files:**
- Modify: `simgrid/client.go`
- Test: `simgrid/client_test.go`

- [ ] **Step 1: Write the failing test**

Append this spec inside the top-level `Describe("SimGridClient", ...)` block in `simgrid/client_test.go` (after the `GetChampionship` block from Task 3). It registers a list handler and detail handlers for several candidates:

```go
	Describe("FindSeasonChampionship", func() {
		BeforeEach(func() {
			mux.HandleFunc("/championships", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[
					{"id":1,"name":"TRACKILICIOUS - GT3 - Summer Sprint Series"},
					{"id":2,"name":"GT4 Rookies - Summer"},
					{"id":3,"name":"GT4 Rookies - Winter"},
					{"id":4,"name":"Some Other Org Rookies - Summer"}
				]`))
			})
			mux.HandleFunc("/championships/2", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"id":2,"name":"GT4 Rookies - Summer","host_name":"TRACKILICIOUS","start_date":"2026-06-08T00:00:00.000Z","races":[{"track":{"name":"Misano"}}]}`))
			})
			mux.HandleFunc("/championships/3", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"id":3,"name":"GT4 Rookies - Winter","host_name":"TRACKILICIOUS","start_date":"2026-12-01T00:00:00.000Z","races":[{"track":{"name":"Bathurst"}}]}`))
			})
			mux.HandleFunc("/championships/4", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"id":4,"name":"Some Other Org Rookies - Summer","host_name":"SOMEONE ELSE","start_date":"2026-06-08T00:00:00.000Z","races":[{"track":{"name":"Spa"}}]}`))
			})
		})

		It("returns the single trackilicious Rookies championship matching the term", func() {
			champ, err := client.FindSeasonChampionship("TRACKILICIOUS", "Summer")
			Expect(err).NotTo(HaveOccurred())
			Expect(champ.ID).To(Equal(2))
			Expect(champ.Name).To(Equal("GT4 Rookies - Summer"))
			Expect(champ.Races[0].Track.Name).To(Equal("Misano"))
		})

		It("ignores non-Rookies events and other hosts (host match is case-insensitive)", func() {
			champ, err := client.FindSeasonChampionship("trackilicious", "Winter")
			Expect(err).NotTo(HaveOccurred())
			Expect(champ.ID).To(Equal(3))
		})

		It("returns an error when no championship matches the term", func() {
			_, err := client.FindSeasonChampionship("TRACKILICIOUS", "Spring")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no upcoming"))
			Expect(err.Error()).To(ContainSubstring("Spring"))
		})
	})

	Describe("FindSeasonChampionship with multiple matches", func() {
		It("returns an error listing the ambiguous matches", func() {
			mux.HandleFunc("/championships", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`[{"id":10,"name":"GT4 Rookies - Summer A"},{"id":11,"name":"GT4 Rookies - Summer B"}]`))
			})
			mux.HandleFunc("/championships/10", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"id":10,"name":"GT4 Rookies - Summer A","host_name":"TRACKILICIOUS","start_date":"2026-06-08T00:00:00.000Z","races":[]}`))
			})
			mux.HandleFunc("/championships/11", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"id":11,"name":"GT4 Rookies - Summer B","host_name":"TRACKILICIOUS","start_date":"2026-06-08T00:00:00.000Z","races":[]}`))
			})

			_, err := client.FindSeasonChampionship("TRACKILICIOUS", "Summer")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("multiple"))
		})
	})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `rtk proxy go test ./simgrid/`
Expected: FAIL — `client.FindSeasonChampionship undefined`.

- [ ] **Step 3: Write the implementation**

Add to `simgrid/client.go` (after `GetChampionship`):

```go
// FindSeasonChampionship finds the single upcoming championship hosted by host
// whose name contains both "rookies" and the given season term. Candidates are
// pre-filtered by name from the cheap index call, then confirmed against the
// detail endpoint's host_name. Returns an error when zero or more than one
// championship matches.
func (sgc *SimGridClient) FindSeasonChampionship(host, term string) (*Championship, error) {
	items, err := sgc.ListUpcomingChampionships()
	if err != nil {
		return nil, fmt.Errorf("failed listing upcoming championships: %w", err)
	}

	lowerTerm := strings.ToLower(term)
	var matches []*Championship
	for _, item := range items {
		lowerName := strings.ToLower(item.Name)
		if !strings.Contains(lowerName, "rookies") || !strings.Contains(lowerName, lowerTerm) {
			continue
		}
		champ, err := sgc.GetChampionship(strconv.Itoa(item.ID))
		if err != nil {
			return nil, fmt.Errorf("failed fetching championship %d: %w", item.ID, err)
		}
		if strings.EqualFold(champ.HostName, host) {
			matches = append(matches, champ)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no upcoming %s Rookies championship matching season %q found", host, term)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, fmt.Sprintf("%q (#%d)", m.Name, m.ID))
		}
		return nil, fmt.Errorf("multiple upcoming %s Rookies championships match season %q: %s", host, term, strings.Join(names, ", "))
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `rtk proxy go test ./simgrid/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add simgrid/client.go simgrid/client_test.go
git commit -m "feat(simgrid): add FindSeasonChampionship by host and season term"
```

---

## Task 5: Drive servicer folder methods + fake regen (gcloud package)

**Files:**
- Modify: `gcloud/interfaces.go`
- Modify: `gcloud/gcloud.go`
- Regenerate: `gcloud/fakes/drive_servicer.go`

This task adds the interface methods, the real (untested, thin SDK wrapper) adapters, and regenerates the counterfeiter fake so later tasks can stub them. No new unit test here — the fake-backed test lives in Task 6.

- [ ] **Step 1: Extend the DriveServicer interface**

In `gcloud/interfaces.go`, replace the `DriveServicer` interface with:

```go
//counterfeiter:generate . DriveServicer
type DriveServicer interface {
	CopyFile(ctx context.Context, templateID, folderID, title string) (*drive.File, error)
	GetFile(ctx context.Context, id string) (*drive.File, error)
	FindFolder(ctx context.Context, parentID, name string) (*drive.File, error)
	CreateFolder(ctx context.Context, parentID, name string) (*drive.File, error)
}
```

- [ ] **Step 2: Implement the real adapters**

In `gcloud/gcloud.go`, add to the `realDriveService` adapter block (after the existing `CopyFile` method). Also ensure `strings` is imported (it already is) and add `fmt` (already imported):

```go
func (r *realDriveService) GetFile(ctx context.Context, id string) (*drive.File, error) {
	return r.svc.Files.Get(id).Fields("id, name, parents").Context(ctx).Do()
}

func (r *realDriveService) FindFolder(ctx context.Context, parentID, name string) (*drive.File, error) {
	escaped := strings.ReplaceAll(name, "'", `\'`)
	q := fmt.Sprintf("'%s' in parents and name = '%s' and mimeType = 'application/vnd.google-apps.folder' and trashed = false", parentID, escaped)
	list, err := r.svc.Files.List().Q(q).Fields("files(id, name)").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if len(list.Files) == 0 {
		return nil, nil
	}
	return list.Files[0], nil
}

func (r *realDriveService) CreateFolder(ctx context.Context, parentID, name string) (*drive.File, error) {
	folder := &drive.File{
		Name:     name,
		Parents:  []string{parentID},
		MimeType: "application/vnd.google-apps.folder",
	}
	return r.svc.Files.Create(folder).Fields("id").Context(ctx).Do()
}
```

- [ ] **Step 3: Verify the package compiles**

Run: `rtk proxy go build ./gcloud/`
Expected: success (the existing fake does not yet implement the new methods, but the fake is only referenced from test files, so a non-test build passes).

- [ ] **Step 4: Regenerate the DriveServicer fake**

Run (explicit `-o` per repo convention):

```bash
go run github.com/maxbrunsfeld/counterfeiter/v6 -o gcloud/fakes/drive_servicer.go github.com/geofffranks/rookies-bot/gcloud DriveServicer
```

Expected: `Wrote `gcloud/fakes/drive_servicer.go``. The regenerated file now has `FakeDriveServicer.GetFile`, `.FindFolder`, `.CreateFolder` plus their `*Returns`/`*ArgsForCall`/`*CallCount` helpers.

- [ ] **Step 5: Verify the whole module still builds and tests pass**

Run: `rtk proxy go build ./...`
Expected: success.
Run: `rtk proxy go test ./gcloud/`
Expected: PASS (existing gcloud specs unaffected).

- [ ] **Step 6: Commit**

```bash
git add gcloud/interfaces.go gcloud/gcloud.go gcloud/fakes/drive_servicer.go
git commit -m "feat(gcloud): add Drive folder get/find/create methods and regen fake"
```

---

## Task 6: Client.EnsureSeasonFolder (gcloud package)

**Files:**
- Modify: `gcloud/gcloud.go`
- Test: `gcloud/gcloud_test.go`

- [ ] **Step 1: Write the failing test**

Append a new top-level `Describe` to `gcloud/gcloud_test.go`:

```go
var _ = Describe("EnsureSeasonFolder", func() {
	var (
		fakeDrive *fakes.FakeDriveServicer
		client    *gcloud.Client
	)

	BeforeEach(func() {
		fakeDrive = new(fakes.FakeDriveServicer)
		client = &gcloud.Client{Drive: fakeDrive}
		// current folder lives under parent "parent-1"
		fakeDrive.GetFileReturns(&drive.File{Id: "current-folder", Parents: []string{"parent-1"}}, nil)
	})

	It("returns the existing folder id when one already exists (idempotent)", func() {
		fakeDrive.FindFolderReturns(&drive.File{Id: "existing-season"}, nil)

		id, err := client.EnsureSeasonFolder(context.Background(), "current-folder", "2026 Summer")
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal("existing-season"))
		Expect(fakeDrive.CreateFolderCallCount()).To(Equal(0))

		_, parentID, name := fakeDrive.FindFolderArgsForCall(0)
		Expect(parentID).To(Equal("parent-1"))
		Expect(name).To(Equal("2026 Summer"))
	})

	It("creates a new folder under the current folder's parent when none exists", func() {
		fakeDrive.FindFolderReturns(nil, nil)
		fakeDrive.CreateFolderReturns(&drive.File{Id: "new-season"}, nil)

		id, err := client.EnsureSeasonFolder(context.Background(), "current-folder", "2026 Summer")
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal("new-season"))

		Expect(fakeDrive.CreateFolderCallCount()).To(Equal(1))
		_, parentID, name := fakeDrive.CreateFolderArgsForCall(0)
		Expect(parentID).To(Equal("parent-1"))
		Expect(name).To(Equal("2026 Summer"))
	})

	It("returns an error when the current folder lookup fails", func() {
		fakeDrive.GetFileReturns(nil, errors.New("drive down"))
		_, err := client.EnsureSeasonFolder(context.Background(), "current-folder", "2026 Summer")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("drive down"))
	})

	It("returns an error when the current folder has no parent", func() {
		fakeDrive.GetFileReturns(&drive.File{Id: "current-folder", Parents: nil}, nil)
		_, err := client.EnsureSeasonFolder(context.Background(), "current-folder", "2026 Summer")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no parent"))
	})

	It("returns an error when folder creation fails", func() {
		fakeDrive.FindFolderReturns(nil, nil)
		fakeDrive.CreateFolderReturns(nil, errors.New("create failed"))
		_, err := client.EnsureSeasonFolder(context.Background(), "current-folder", "2026 Summer")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})
```

Add `"context"` to the test file's import block if not already present (it is not in the current `gcloud_test.go`; add it):

```go
import (
	"context"
	"errors"
	"fmt"
	"strings"
	...
)
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `rtk proxy go test ./gcloud/`
Expected: FAIL — `client.EnsureSeasonFolder undefined`.

- [ ] **Step 3: Write the implementation**

Add to `gcloud/gcloud.go` (in the `--- Methods ---` section, near `GeneratePenaltyTracker`):

```go
// EnsureSeasonFolder finds or creates a folder named seasonName as a sibling of
// currentFolderID (i.e. under the same parent). It returns the id of the
// existing folder if one already exists, making re-runs idempotent.
func (c *Client) EnsureSeasonFolder(ctx context.Context, currentFolderID, seasonName string) (string, error) {
	current, err := c.Drive.GetFile(ctx, currentFolderID)
	if err != nil {
		return "", fmt.Errorf("failed looking up current folder %s: %w", currentFolderID, err)
	}
	if len(current.Parents) == 0 {
		return "", fmt.Errorf("current folder %s has no parent folder to create %q alongside", currentFolderID, seasonName)
	}
	parentID := current.Parents[0]

	existing, err := c.Drive.FindFolder(ctx, parentID, seasonName)
	if err != nil {
		return "", fmt.Errorf("failed searching for existing %q folder: %w", seasonName, err)
	}
	if existing != nil {
		return existing.Id, nil
	}

	created, err := c.Drive.CreateFolder(ctx, parentID, seasonName)
	if err != nil {
		return "", fmt.Errorf("failed creating %q folder: %w", seasonName, err)
	}
	return created.Id, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `rtk proxy go test ./gcloud/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gcloud/gcloud.go gcloud/gcloud_test.go
git commit -m "feat(gcloud): add EnsureSeasonFolder find-or-create helper"
```

---

## Task 7: Thread the config path into DiscordClient (discord + runner + main)

**Files:**
- Modify: `discord/discord.go`
- Modify: `runner.go`
- Test: `main_test.go`

This task changes `NewDiscordClient`'s signature to accept the config-file path and adds the `configPath` + `mu` fields, then updates the runner factory and its test. No behavior change yet; it keeps the build green.

- [ ] **Step 1: Update the failing test**

In `main_test.go`, update the `newDiscordClient` factory field in the `BeforeEach` of `Describe("Runner.before", ...)` to the new three-argument signature:

```go
			newDiscordClient: func(_ *config.Config, _ *gcloud.Client, _ string) (discord.BotDiscordClient, error) {
				return fakeDC, nil
			},
```

And update the standalone reassignment in the "failed to connect to discord" spec:

```go
	It("returns an error wrapping 'failed to connect to discord' when discord creation fails", func() {
		r.newDiscordClient = func(_ *config.Config, _ *gcloud.Client, _ string) (discord.BotDiscordClient, error) {
			return nil, errors.New("bad token")
		}
		err := r.before(cCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to connect to discord"))
	})
```

- [ ] **Step 2: Run the test to verify it fails (compile error)**

Run: `rtk proxy go test ./...`
Expected: FAIL to compile — the `Runner.newDiscordClient` field type still has the two-arg signature, and `discord.NewDiscordClient` takes two args.

- [ ] **Step 3: Update DiscordClient**

In `discord/discord.go`:

Add `"sync"` and `"strconv"` to the import block (strconv is used in Task 9/10; add now to avoid a second edit):

```go
import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
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
```

Add `configPath` and `mu` to the `DiscordClient` struct:

```go
type DiscordClient struct {
	botClient     bot.Client
	rest          BotRestClient
	applicationID snowflake.ID
	conf          *config.Config
	guild         snowflake.ID
	memberList    map[string]snowflake.ID
	gcloud        *gcloud.Client
	configPath    string
	mu            sync.Mutex
}
```

Update `NewDiscordClient` to accept and store the path:

```go
func NewDiscordClient(conf *config.Config, gc *gcloud.Client, configPath string) (*DiscordClient, error) {
	client, err := disgo.New(conf.DiscordToken, bot.WithGatewayConfigOpts(
		gateway.WithIntents(gateway.IntentMessageContent, gateway.IntentDirectMessages),
	))
	if err != nil {
		return nil, err
	}

	dc := &DiscordClient{
		conf:          conf,
		botClient:     client,
		rest:          client.Rest(),
		applicationID: client.ApplicationID(),
		gcloud:        gc,
		configPath:    configPath,
	}

	client.AddEventListeners(bot.NewListenerFunc(dc.onMessageCreate))
	return dc, nil
}
```

(`sync.Mutex` is a non-comparable field, so `NewTestDiscordClient`'s struct literal must not copy a `DiscordClient` by value — it already returns a pointer, so this is fine. Leave `NewTestDiscordClient` as-is; internal tests set `configPath` directly.)

- [ ] **Step 4: Update the runner**

In `runner.go`, change the `newDiscordClient` field type and the factory in `newRunner`, and pass the path in `before`:

```go
type Runner struct {
	conf             *config.Config
	dc               discord.BotDiscordClient
	loadConfig       func(string, string) (*config.Config, error)
	newGCloudClient  func(context.Context) (*gcloud.Client, error)
	newDiscordClient func(*config.Config, *gcloud.Client, string) (discord.BotDiscordClient, error)
	stopChan         chan os.Signal
}

func newRunner() *Runner {
	return &Runner{
		loadConfig:      config.Load,
		newGCloudClient: gcloud.NewClient,
		newDiscordClient: func(conf *config.Config, gc *gcloud.Client, configPath string) (discord.BotDiscordClient, error) {
			return discord.NewDiscordClient(conf, gc, configPath)
		},
	}
}
```

In `Runner.before`, pass the config path from the CLI flag:

```go
	r.dc, err = r.newDiscordClient(r.conf, gc, cCtx.String("config"))
	if err != nil {
		return fmt.Errorf("failed to connect to discord: %s", err)
	}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `rtk proxy go test ./...`
Expected: PASS across all packages.

- [ ] **Step 6: Commit**

```bash
git add discord/discord.go runner.go main_test.go
git commit -m "refactor: thread config path into DiscordClient for live reconfiguration"
```

---

## Task 8: Add the two commands to !help (discord package)

**Files:**
- Modify: `discord/discord.go`
- Test: `discord/discord_internal_test.go`

- [ ] **Step 1: Write the failing test**

In `discord/discord_internal_test.go`, add two specs to the existing `Describe("helpMessage", ...)` block:

```go
	It("lists the !new-season command", func() {
		Expect(helpMessage()).To(ContainSubstring("!new-season"))
	})

	It("lists the !new-season-apply command", func() {
		Expect(helpMessage()).To(ContainSubstring("!new-season-apply"))
	})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `rtk proxy go test ./discord/`
Expected: FAIL — help message does not contain the new commands.

- [ ] **Step 3: Update helpMessage**

In `discord/discord.go`, replace the `helpMessage` function body's return with the two new entries appended:

```go
func helpMessage() string {
	return "**Rookies Bot — Commands**\n\n" +
		"`!help`\n" +
		"  Show this message.\n\n" +
		"`!announce-penalties`\n" +
		"  Attach a round penalty YAML; posts the formatted penalty breakdown (quali bans / pit starts, R1 & R2).\n\n" +
		"`!race-setup`\n" +
		"  Attach a round penalty YAML; generates the next round config and race-day setup.\n\n" +
		"`!new-season`\n" +
		"  Preview the next-season reconfiguration (championship, schedule, config values). Makes no changes.\n\n" +
		"`!new-season-apply`\n" +
		"  Apply the next-season reconfiguration: create Drive folders, update the bot config live, and post the round-0 config.\n"
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `rtk proxy go test ./discord/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add discord/discord.go discord/discord_internal_test.go
git commit -m "feat(discord): document !new-season commands in !help"
```

---

## Task 9: !new-season preview (discord package)

**Files:**
- Modify: `discord/discord.go`
- Test: `discord/discord_internal_test.go`

This task adds the read-only preview: the shared derive logic (`runNewSeason` with `apply=false`), the preview message, the dispatch case, and the `newSeason` event handler. The apply branch is stubbed to return an error until Task 10 so the function compiles and the preview path is fully testable.

- [ ] **Step 1: Write the failing test**

In `discord/discord_internal_test.go`, add a new top-level `Describe`. It builds a SimGrid httptest server that serves the upcoming list and the detail for the matched championship. The client's `conf.Season` is `"Fall"`, so the rotated next term is `"Winter"`:

```go
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
		Expect(msg).To(ContainSubstring("Bathurst"))      // round 1 track / schedule
		Expect(msg).To(ContainSubstring("2026 Winter"))   // season string
		Expect(msg).To(ContainSubstring("GT4 Rookies Winter")) // role name
		Expect(msg).To(ContainSubstring("!new-season-apply"))  // how to commit
	})

	It("makes no changes in preview mode (config file path never touched)", func() {
		// configPath is empty; if preview tried to write config it would error.
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `rtk proxy go test ./discord/`
Expected: FAIL — `client.runNewSeason undefined`.

- [ ] **Step 3: Write the implementation**

In `discord/discord.go`, add the dispatch cases to `onMessageCreate`'s switch:

```go
	switch event.Message.Content {
	case "!help":
		sendBotResponse(event, helpMessage(), "")
	case "!announce-penalties":
		d.announcePenalties(event)
	case "!race-setup":
		d.raceSetup(event)
	case "!new-season":
		d.newSeason(event, false)
	case "!new-season-apply":
		d.newSeason(event, true)
	}
```

Add the handler, the orchestrator, and the preview builder (place near `raceSetup`). The apply branch is a placeholder error for now — Task 10 fills it in:

```go
func (d *DiscordClient) newSeason(event *events.MessageCreate, apply bool) {
	var msg, attachment string
	defer func() { sendBotResponse(event, msg, attachment) }()

	sgClient := simgrid.NewClient(d.conf.SimGridApiToken)
	var err error
	msg, attachment, err = d.runNewSeason(apply, sgClient)
	if err != nil {
		msg = err.Error()
	}
}

// runNewSeason derives the next season (read-only) and, when apply is true,
// commits the change: creates Drive folders, rewrites the config file, updates
// the live in-memory config, and writes a round-0 config. It returns the
// message to post and an optional attachment file path.
func (d *DiscordClient) runNewSeason(apply bool, sgClient *simgrid.SimGridClient) (string, string, error) {
	currentTerm, err := config.ParseSeasonTerm(d.conf.Season)
	if err != nil {
		return "", "", fmt.Errorf("could not determine current season: %w", err)
	}
	nextTerm, err := config.NextTerm(currentTerm)
	if err != nil {
		return "", "", err
	}

	champ, err := sgClient.FindSeasonChampionship("TRACKILICIOUS", nextTerm)
	if err != nil {
		return "", "", err
	}
	if len(champ.Races) == 0 {
		return "", "", fmt.Errorf("championship %q (#%d) has no races scheduled yet", champ.Name, champ.ID)
	}

	year, err := champ.StartYear()
	if err != nil {
		return "", "", err
	}
	season := fmt.Sprintf("%d %s", year, nextTerm)
	role, err := config.RoleNameForTerm(nextTerm)
	if err != nil {
		return "", "", err
	}
	round1 := champ.Races[0].Track.Name
	champID := strconv.Itoa(champ.ID)

	if !apply {
		return buildNewSeasonPreview(champ, season, role, round1), "", nil
	}

	return "", "", fmt.Errorf("applying a new season is not implemented yet")
}

// buildNewSeasonPreview renders the read-only proposal. It deliberately omits
// secrets and shows only the season-level values that will change.
func buildNewSeasonPreview(champ *simgrid.Championship, season, role, round1 string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🗓 **New Season Preview**\n\n")
	fmt.Fprintf(&b, "Championship: %s (#%d) — host %s\n", champ.Name, champ.ID, champ.HostName)
	fmt.Fprintf(&b, "Schedule:\n")
	for i, race := range champ.Races {
		fmt.Fprintf(&b, "  R%d %s\n", i+1, race.Track.Name)
	}
	fmt.Fprintf(&b, "\nWill set:\n")
	fmt.Fprintf(&b, "  season             %s\n", season)
	fmt.Fprintf(&b, "  championship_id    %d\n", champ.ID)
	fmt.Fprintf(&b, "  discord_role_name  %s\n", role)
	fmt.Fprintf(&b, "  briefing_folder    %q (created/reused under the current briefing folder's parent)\n", season)
	fmt.Fprintf(&b, "  tracker_folder     %q (created/reused under the current tracker folder's parent)\n", season)
	fmt.Fprintf(&b, "Round-0 announces: Round 1 — %s\n", round1)
	fmt.Fprintf(&b, "\n▶ Run `!new-season-apply` to commit these changes.")
	return b.String()
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `rtk proxy go test ./discord/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add discord/discord.go discord/discord_internal_test.go
git commit -m "feat(discord): add !new-season preview command"
```

---

## Task 10: !new-season-apply commit + round-0 config (discord package)

**Files:**
- Modify: `discord/discord.go`
- Test: `discord/discord_internal_test.go`

- [ ] **Step 1: Write the failing test**

In `discord/discord_internal_test.go`, add a new top-level `Describe`. It wires a fake Drive servicer for folder creation, a SimGrid server for the lookup, and a temp config file the command rewrites:

```go
var _ = Describe("runNewSeason apply", func() {
	var (
		client     *DiscordClient
		stub       *stubRest
		sgServer   *httptest.Server
		sgClient   *simgrid.SimGridClient
		fakeDrive  *fakes.FakeDriveServicer
		gcClient   *gcloud.Client
		configPath string
	)

	BeforeEach(func() {
		stub = &stubRest{}
		fakeDrive = &fakes.FakeDriveServicer{}
		gcClient = &gcloud.Client{Drive: fakeDrive}

		// folders resolve to a parent then create new season folders
		fakeDrive.GetFileReturns(&drive.File{Id: "current", Parents: []string{"parent-1"}}, nil)
		fakeDrive.FindFolderReturns(nil, nil)
		fakeDrive.CreateFolderReturnsOnCall(0, &drive.File{Id: "briefing-new"}, nil)
		fakeDrive.CreateFolderReturnsOnCall(1, &drive.File{Id: "tracker-new"}, nil)

		dir, err := os.MkdirTemp("", "rookies-bot-newseason-test")
		Expect(err).NotTo(HaveOccurred())
		configPath = dir + "/config.yml"
		Expect(os.WriteFile(configPath, []byte(`season: Fall
championship_id: "9485"
discord_role_name: GT4 Rookie
briefing_folder_id: briefing-current
tracker_folder_id: tracker-current
discord_token: keep-me-secret
`), 0600)).To(Succeed())

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
		os.RemoveAll(filepathDir(configPath))
		// clean up the round-0 file written to CWD
		_, _ = os.Stat("2026-winter-round-0.yml")
		_ = os.Remove("2026-winter-round-0.yml")
	})

	It("creates folders, rewrites config, updates live config, and attaches round-0", func() {
		msg, attachment, err := client.runNewSeason(true, sgClient)
		Expect(err).NotTo(HaveOccurred())

		// folders created
		Expect(fakeDrive.CreateFolderCallCount()).To(Equal(2))

		// live in-memory config updated (no restart)
		Expect(client.conf.Season).To(Equal("2026 Winter"))
		Expect(client.conf.ChampionshipId).To(Equal("555"))
		Expect(client.conf.DiscordRoleName).To(Equal("GT4 Rookies Winter"))
		Expect(client.conf.BriefingFolderID).To(Equal("briefing-new"))
		Expect(client.conf.TrackerFolderID).To(Equal("tracker-new"))

		// config file rewritten, secret preserved
		data, err := os.ReadFile(configPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("2026 Winter"))
		Expect(string(data)).To(ContainSubstring("GT4 Rookies Winter"))
		Expect(string(data)).To(ContainSubstring("keep-me-secret"))

		// round-0 attachment written with next round = 1 at Bathurst
		Expect(attachment).To(Equal("2026-winter-round-0.yml"))
		rcData, err := os.ReadFile(attachment)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(rcData)).To(ContainSubstring("number: 1"))
		Expect(string(rcData)).To(ContainSubstring("Bathurst"))

		// message confirms and explains next step
		Expect(msg).To(ContainSubstring("2026 Winter"))
		Expect(msg).To(ContainSubstring("!race-setup"))
	})

	It("returns an error when folder creation fails (no config written)", func() {
		fakeDrive.CreateFolderReturnsOnCall(0, nil, errorMsgErr("drive create failed"))
		_, _, err := client.runNewSeason(true, sgClient)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("briefing folder"))

		// config file unchanged
		data, _ := os.ReadFile(configPath)
		Expect(string(data)).To(ContainSubstring("season: Fall"))
	})
})

// filepathDir returns the directory of a path without importing path/filepath
// at call sites that already have it; defined here for test readability.
func filepathDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "."
	}
	return p[:i]
}

// errorMsgErr is a tiny error helper for apply tests.
func errorMsgErr(msg string) error { return fmt.Errorf("%s", msg) }
```

Note: the internal test file already imports `os`, `strings`, `fmt`, `net/http`, `net/http/httptest`, the `fakes` package, `gcloud`, `simgrid`, `config`, and `drive`. No new imports are required.

- [ ] **Step 2: Run the test to verify it fails**

Run: `rtk proxy go test ./discord/`
Expected: FAIL — `runNewSeason` apply branch returns the "not implemented yet" error, so the happy-path assertions fail.

- [ ] **Step 3: Write the implementation**

In `discord/discord.go`, replace the placeholder apply branch at the end of `runNewSeason` (the `return "", "", fmt.Errorf("applying a new season is not implemented yet")` line) with the real apply logic:

```go
	// apply: create the season's Drive folders (idempotent find-or-create)
	ctx := context.Background()
	briefingID, err := d.gcloud.EnsureSeasonFolder(ctx, d.conf.BriefingFolderID, season)
	if err != nil {
		return "", "", fmt.Errorf("failed setting up briefing folder: %w", err)
	}
	trackerID, err := d.gcloud.EnsureSeasonFolder(ctx, d.conf.TrackerFolderID, season)
	if err != nil {
		return "", "", fmt.Errorf("failed setting up tracker folder: %w", err)
	}

	// persist the five season-level values, preserving comments and secrets
	updates := map[string]string{
		"season":             season,
		"championship_id":    champID,
		"discord_role_name":  role,
		"briefing_folder_id": briefingID,
		"tracker_folder_id":  trackerID,
	}
	if err := config.UpdateBotConfigFile(d.configPath, updates); err != nil {
		return "", "", fmt.Errorf("failed updating config file: %w", err)
	}

	// update the live in-memory config so the change takes effect without a restart
	d.mu.Lock()
	d.conf.Season = season
	d.conf.ChampionshipId = champID
	d.conf.DiscordRoleName = role
	d.conf.BriefingFolderID = briefingID
	d.conf.TrackerFolderID = trackerID
	d.mu.Unlock()

	attachment, err := writeRoundZeroConfig(season, round1)
	if err != nil {
		return "", "", fmt.Errorf("failed generating round-0 config: %w", err)
	}

	return buildNewSeasonApplied(champ, season, role, round1, briefingID, trackerID), attachment, nil
```

Add the round-0 writer and the applied-message builder (near `buildNewSeasonPreview`):

```go
// writeRoundZeroConfig writes a round-0 config (next round = round 1 at the
// season opener, no penalties, no previous round) to the working directory and
// returns the file name.
func writeRoundZeroConfig(season, round1Track string) (string, error) {
	rc := &config.RoundConfig{
		NextRound: config.Round{Number: 1, Track: round1Track},
	}
	data, err := yaml.Marshal(rc)
	if err != nil {
		return "", err
	}
	slug := strings.ToLower(strings.ReplaceAll(season, " ", "-"))
	fileName := fmt.Sprintf("%s-round-0.yml", slug)
	if err := os.WriteFile(fileName, data, 0600); err != nil {
		return "", err
	}
	return fileName, nil
}

// buildNewSeasonApplied renders the confirmation posted after a successful
// apply. It omits secrets and explains the next step.
func buildNewSeasonApplied(champ *simgrid.Championship, season, role, round1, briefingID, trackerID string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✅ **New Season Applied: %s**\n\n", season)
	fmt.Fprintf(&b, "Championship: %s (#%d)\n", champ.Name, champ.ID)
	fmt.Fprintf(&b, "Updated config:\n")
	fmt.Fprintf(&b, "  season             %s\n", season)
	fmt.Fprintf(&b, "  championship_id    %d\n", champ.ID)
	fmt.Fprintf(&b, "  discord_role_name  %s\n", role)
	fmt.Fprintf(&b, "  briefing_folder_id %s\n", briefingID)
	fmt.Fprintf(&b, "  tracker_folder_id  %s\n", trackerID)
	fmt.Fprintf(&b, "\nThe bot is now using the new season — no restart needed.\n")
	fmt.Fprintf(&b, "Attached round-0 config announces Round 1 — %s. Run `!race-setup` with it to announce week 1.", round1)
	return b.String()
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `rtk proxy go test ./discord/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add discord/discord.go discord/discord_internal_test.go
git commit -m "feat(discord): implement !new-season-apply with live reconfigure and round-0"
```

---

## Task 11: Full verification, lint, and deploy reminder

**Files:** none (verification only)

- [ ] **Step 1: Run the full lint + test suite the way CI does**

Run: `rtk proxy go build ./...`
Expected: success.

Run: `make test`
Expected: lint (`go fmt`, `go vet`, `staticcheck`, `gosec`) clean and all Ginkgo specs pass with `--fail-on-pending --fail-on-empty --require-suite`.

If `gosec` flags the new `os.ReadFile`/`os.WriteFile` in `config/update.go`, confirm the `// #nosec G304` annotation on the `ReadFile` line is present and add one to the `WriteFile` line only if gosec flags it (path is operator-supplied, not external input).

- [ ] **Step 2: Manual smoke check of the command wiring (optional but recommended)**

Confirm by reading [discord/discord.go](../../../discord/discord.go) that:
- `onMessageCreate` routes `!new-season` → `newSeason(event, false)` and `!new-season-apply` → `newSeason(event, true)`.
- Both commands are still gated behind `isAllowedUser` (the guard at the top of `onMessageCreate` covers all cases).
- `helpMessage()` lists both commands.

- [ ] **Step 3: Final commit if any lint fixups were needed**

```bash
git add -A
git commit -m "chore: lint fixups for new-season command"
```

(Skip if there were no changes.)

- [ ] **Step 4: Remind the user to deploy**

After merge, the bot must be redeployed to pick up the new binary: remind the user to run `make deploy` (builds + pushes the image and restarts the container on the production host). Note: once deployed, the `!new-season-apply` command itself no longer requires a restart — that is the whole point — but shipping this new code does.

---

## Self-Review

**Spec coverage:**
- §simgrid (list/detail/find) → Tasks 3, 4. ✅
- §season rotation + role map → Task 1. ✅
- §config persistence (yaml.Node) → Task 2. ✅
- §gcloud folders (get-parent/find/create + EnsureSeasonFolder + fake regen) → Tasks 5, 6. ✅
- §discord (dispatch, help, preview/apply, configPath wiring, live update under mutex, round-0) → Tasks 7, 8, 9, 10. ✅
- §side-effects (season → "<year> <term>", filename normalization) → Task 10 (`writeRoundZeroConfig` slug) ✅; existing `writeNextRoundConfig` already lowercases — its space handling is unchanged and out of scope for this feature.
- Deviation from the spec's illustrative preview: the preview shows the **folder name** that will be created/reused rather than resolving and printing the parent folder id, keeping preview a pure SimGrid (zero-Drive, zero-side-effect) operation. Intentional; noted in Task 9.

**Placeholder scan:** No TBD/TODO/"handle errors appropriately" — every code and test step contains complete content. The Task 9 apply branch is an explicit, intentional placeholder error that Task 10 replaces (called out in both tasks). ✅

**Type consistency:**
- `FindSeasonChampionship(host, term string) (*simgrid.Championship, error)` — defined Task 4, called Task 9/10. ✅
- `Championship` fields `ID int`, `Name`, `HostName`, `StartDate string`, `Races []Race`; `StartYear() (int, error)` — defined Task 3, used Tasks 4/9/10. ✅
- `EnsureSeasonFolder(ctx, currentFolderID, seasonName string) (string, error)` — defined Task 6, called Task 10. ✅
- `UpdateBotConfigFile(path string, updates map[string]string) error` — defined Task 2, called Task 10. ✅
- `ParseSeasonTerm`/`NextTerm`/`RoleNameForTerm` — defined Task 1, used Task 9. ✅
- `NewDiscordClient(conf, gc, configPath)` — changed Task 7; runner + main_test updated in the same task. ✅
- Drive fake helpers `GetFileReturns`/`FindFolderReturns`/`CreateFolderReturns(OnCall)` — produced by the Task 5 regen, used in Tasks 6/10. ✅

No truth-table block needed: there are no condition-form (`not X` vs enumerated) choices in this plan; the season cycle is an ordered-list lookup, not a guard condition.
