# Dead Code Removal and Coverage Gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove three dead code items and close seven remaining coverage gaps across the `discord`, `gcloud`, `simgrid`, and `models` packages.

**Architecture:** Each task is surgical — edit only the file(s) named. No new interfaces, no new packages, no refactoring beyond what is specified. All tests use Ginkgo v2 + Gomega exclusively.

**Tech Stack:** Go, Ginkgo v2, Gomega, `net/http/httptest`, counterfeiter fakes (`gcloud/fakes`, `discord/fakes`).

---

## File Structure

Files modified by this plan:

| File | Change |
|------|--------|
| `gcloud/gcloud.go` | Fix `< 0` → `== 0` on `penaltyStartIndex` guard (line 110) |
| `gcloud/gcloud_test.go` | Update bug-documentation test to expect error after fix |
| `models/models.go` | Delete `UniqueDriverNumbers()` method |
| `models/models_test.go` | Delete `Describe("UniqueDriverNumbers()")` block |
| `discord/discord.go` | Delete `NewTestDiscordClient` function (lines 691–700) |
| `discord/export_test.go` | Create (new) — re-export `NewTestDiscordClient` for test use |
| `discord/discord_test.go` | Add one `It` inside `Describe("BuildBriefingMessage")` |
| `simgrid/client_test.go` | Add two `It` blocks: `GetNextRound` unmarshal error, `BuildDriverLookup` entrylist 4xx |
| `discord/discord_internal_test.go` | Add `Describe("writeNextRoundConfig")` with one `It` |

---

## Task 1: Fix `penaltyStartIndex` bug in `gcloud/gcloud.go`

**Files:**
- Modify: `gcloud/gcloud.go:110`
- Modify: `gcloud/gcloud_test.go` (update bug-doc test)

### Context

`penaltyStartIndex` is declared as `var penaltyStartIndex int64` (zero value 0). The loop only assigns positive values (`elem.StartIndex`). The guard `if penaltyStartIndex < 0` can never be true — when no Stream heading is found, `penaltyStartIndex` stays 0 and the guard is silently bypassed. The correct check is `== 0`.

A test at the bottom of `gcloud_test.go` documents the current buggy behavior with `Expect(err).NotTo(HaveOccurred())`. After fixing the bug, this test must be updated to assert the correct behavior.

- [ ] **Step 1: Write the failing test**

The existing bug-documentation test near the end of `gcloud_test.go` is:

```go
// BUG DOCUMENTATION: penaltyStartIndex is int64, starts at 0, and the guard
// is `if penaltyStartIndex < 0`. Since 0 is never < 0, a doc with no Stream
// heading silently uses index 0 instead of returning an error. This test
// documents the current (buggy) behavior so any future fix is caught.
It("silently uses index 0 when doc has no Stream heading (documents bug)", func() {
    noStreamDoc := &docs.Document{
        Body: &docs.Body{
            Content: []*docs.StructuralElement{
                {
                    Paragraph: &docs.Paragraph{
                        Elements: []*docs.ParagraphElement{
                            {TextRun: &docs.TextRun{Content: "Some other heading\n"}},
                        },
                        ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_3"},
                    },
                    StartIndex: 1,
                    EndIndex:   20,
                },
            },
        },
    }
    fakeDocsService.GetDocumentReturns(noStreamDoc, nil)
    fakeDocsService.BatchUpdateDocumentReturns(&docs.BatchUpdateDocumentResponse{}, nil)

    // BUG: should return error "no Stream heading found", but currently succeeds
    _, err := client.GenerateBriefing(conf, &models.Penalties{})
    Expect(err).NotTo(HaveOccurred()) // documents current buggy behavior
})
```

Replace this entire `It(...)` block with:

```go
It("returns an error when doc has no Stream heading", func() {
    noStreamDoc := &docs.Document{
        Body: &docs.Body{
            Content: []*docs.StructuralElement{
                {
                    Paragraph: &docs.Paragraph{
                        Elements: []*docs.ParagraphElement{
                            {TextRun: &docs.TextRun{Content: "Some other heading\n"}},
                        },
                        ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_3"},
                    },
                    StartIndex: 1,
                    EndIndex:   20,
                },
            },
        },
    }
    fakeDocsService.GetDocumentReturns(noStreamDoc, nil)

    _, err := client.GenerateBriefing(conf, &models.Penalties{})
    Expect(err).To(HaveOccurred())
})
```

Note: `BatchUpdateDocumentReturns` is removed — the error is returned before `BatchUpdate` is called.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/gfranks/workspace/rookies-bot
rtk proxy go test ./gcloud/... -v -run "returns an error when doc has no Stream heading"
```

Expected: FAIL — test will catch the existing (buggy) behavior where `err` is nil.

- [ ] **Step 3: Apply the production fix**

In `gcloud/gcloud.go` line 110, change:

```go
if penaltyStartIndex < 0 {
```

to:

```go
if penaltyStartIndex == 0 {
```

- [ ] **Step 4: Run the new test to verify it passes**

```bash
rtk proxy go test ./gcloud/... -v -run "returns an error when doc has no Stream heading"
```

Expected: PASS

- [ ] **Step 5: Run the full gcloud test suite**

```bash
rtk proxy go test ./gcloud/... -v
```

Expected: all tests pass. The previously-passing tests use `makeDoc(5)` or `makeDoc(10)` — both have non-zero `streamIndex`, so `penaltyStartIndex` will be assigned a positive value and the `== 0` guard will not fire.

- [ ] **Step 6: Commit**

```bash
git add gcloud/gcloud.go gcloud/gcloud_test.go
git commit -m "fix: penaltyStartIndex guard — change < 0 to == 0 to catch missing Stream heading"
```

---

## Task 2: Delete `UniqueDriverNumbers()` dead method

**Files:**
- Modify: `models/models.go` (delete method)
- Modify: `models/models_test.go` (delete test block)

### Context

`UniqueDriverNumbers()` is an exported method on `*Penalties` that is never called in production code. It is safe to delete. Its three tests in `models/models_test.go` must also be removed.

- [ ] **Step 1: Delete the method from `models/models.go`**

Find and delete the entire `UniqueDriverNumbers` method:

```go
func (p *Penalties) UniqueDriverNumbers() []int {
	return uniqueDrivers(append(p.QualiBansR1,
		append(p.QualiBansR1CarriedOver,
			append(p.QualiBansR2,
				append(p.QualiBansR2CarriedOver,
					append(p.PitStartsR1,
						append(p.PitStartsR1CarriedOver,
							append(p.PitStartsR2, p.PitStartsR2CarriedOver...)...,
						)...,
					)...,
				)...,
			)...,
		)...,
	))
}
```

- [ ] **Step 2: Delete the test block from `models/models_test.go`**

Find and delete the entire `Describe("UniqueDriverNumbers()")` block:

```go
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
```

- [ ] **Step 3: Run the models test suite**

```bash
rtk proxy go test ./models/... -v
```

Expected: all tests pass, deleted tests are gone.

- [ ] **Step 4: Verify nothing else references `UniqueDriverNumbers`**

```bash
rtk grep "UniqueDriverNumbers" /Users/gfranks/workspace/rookies-bot
```

Expected: no matches.

- [ ] **Step 5: Commit**

```bash
git add models/models.go models/models_test.go
git commit -m "chore: remove unused UniqueDriverNumbers method and its tests"
```

---

## Task 3: Move `NewTestDiscordClient` out of production code

**Files:**
- Modify: `discord/discord.go` (delete function at lines 691–700)
- Create: `discord/export_test.go` (re-export via `package discord` test file)

### Context

`NewTestDiscordClient` is a test helper living in production code. Moving it to `discord/export_test.go` (package `discord`, file suffix `_test.go`) means it is compiled only during tests and is invisible in the production binary. External test files in `package discord_test` can still call `discord.NewTestDiscordClient(...)` because symbols exported from `package discord` test files are accessible to same-package and external test files during `go test`.

- [ ] **Step 1: Create `discord/export_test.go`**

Create a new file `discord/export_test.go` with this exact content:

```go
package discord

import (
	"github.com/disgoorg/snowflake/v2"
	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/gcloud"
)

// NewTestDiscordClient creates a DiscordClient with injected dependencies for testing.
func NewTestDiscordClient(rest BotRestClient, applicationID snowflake.ID, conf *config.Config, gc *gcloud.Client) *DiscordClient {
	return &DiscordClient{
		rest:          rest,
		applicationID: applicationID,
		conf:          conf,
		gcloud:        gc,
	}
}
```

- [ ] **Step 2: Delete `NewTestDiscordClient` from `discord/discord.go`**

Find and delete these lines (the comment + function):

```go
// NewTestDiscordClient creates a DiscordClient with injected dependencies for testing.
func NewTestDiscordClient(rest BotRestClient, applicationID snowflake.ID, conf *config.Config, gc *gcloud.Client) *DiscordClient {
	return &DiscordClient{
		rest:          rest,
		applicationID: applicationID,
		conf:          conf,
		gcloud:        gc,
	}
}
```

- [ ] **Step 3: Build and test**

```bash
rtk proxy go build ./...
```

Expected: builds cleanly — production binary no longer contains `NewTestDiscordClient`.

```bash
rtk proxy go test ./discord/... -v
```

Expected: all tests pass — `discord_test.go` continues to call `discord.NewTestDiscordClient(...)` because `export_test.go` re-exports it during test compilation.

- [ ] **Step 4: Commit**

```bash
git add discord/discord.go discord/export_test.go
git commit -m "refactor: move NewTestDiscordClient out of production code into export_test.go"
```

---

## Task 4: Add `BuildBriefingMessage` GetMembers error propagation test

**Files:**
- Modify: `discord/discord_test.go` (add one `It` inside existing `Describe("BuildBriefingMessage")`)

### Context

`BuildBriefingMessage` calls `generatePenaltyMessage`, which calls `getDriverId`, which calls `GetMembers`. When `GetMembers` returns a non-nil error, `getDriverId` returns it as a non-`DiscordHandleNotFoundError`, and `generatePenaltyMessage` propagates it back to `BuildBriefingMessage`. This path is untested. The `BuildBriefingMessage` `Describe` block already has `fakeRest.GetMembersReturns([]dgo.Member{}, nil)` in its `BeforeEach` — override it inside the new `It`.

- [ ] **Step 1: Write the failing test**

Inside the `Describe("BuildBriefingMessage")` block in `discord_test.go`, after the last existing `It` (the "returns error when GetChannel fails" test at the end), add:

```go
It("returns error when GetMembers fails during penalty message generation", func() {
    fakeRest.GetMembersReturns(nil, &errorMsg{msg: "members unavailable"})
    penalties := &models.Penalties{
        PitStartsR1: []models.Driver{{CarNumber: 1, FirstName: "Test", LastName: "Driver"}},
    }
    _, err := dc.BuildBriefingMessage(penalties, "https://example.com/doc", &conf.RoundConfig)
    Expect(err).To(HaveOccurred())
})
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
rtk proxy go test ./discord/... -v -run "BuildBriefingMessage.*returns error when GetMembers fails"
```

Expected: FAIL — `BuildBriefingMessage` currently returns nil error because `GetMembers` returns an empty list (from `BeforeEach`), and empty penalties produce `"None!"` messages without error.

Wait — this test should PASS immediately once written (since the path already exists). If it passes on first run, skip Step 3 and go directly to the full suite run. The important thing is it must be a real behavior test.

Actually: the BeforeEach sets `fakeRest.GetMembersReturns([]dgo.Member{}, nil)`. The `It` overrides with `fakeRest.GetMembersReturns(nil, &errorMsg{...})`. Since the penalties have a real driver in `PitStartsR1`, `getDriverId` will call `GetMembers`, receive the error, and return it. `generatePenaltyMessage` propagates it, `BuildBriefingMessage` returns it. The test asserts `Expect(err).To(HaveOccurred())` — this should pass.

Run to confirm:

```bash
rtk proxy go test ./discord/... -v -run "BuildBriefingMessage"
```

Expected: all `BuildBriefingMessage` tests pass including the new one.

- [ ] **Step 3: Run the full discord test suite**

```bash
rtk proxy go test ./discord/... -v
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add discord/discord_test.go
git commit -m "test: add BuildBriefingMessage GetMembers error propagation coverage"
```

---

## Task 5: Add `GetNextRound` JSON unmarshal error test

**Files:**
- Modify: `simgrid/client_test.go` (add one `It` inside existing `Describe("GetNextRound")`)

### Context

`GetNextRound` calls `makeRequest`, reads the response body, then calls `json.Unmarshal`. If the server returns HTTP 200 but a non-JSON body, `json.Unmarshal` returns an error which propagates. This path is untested.

The existing `GetNextRound` `Describe` block registers handlers on `mux` inside each `It`. Use the same pattern.

- [ ] **Step 1: Write the failing test**

Inside the `Describe("GetNextRound")` block (after the existing three `It` blocks), add:

```go
It("returns an error when server returns 200 with non-JSON body", func() {
    mux.HandleFunc("/championships/champ1", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(`}{not json`))
    })
    _, err := client.GetNextRound("champ1", config.Round{Number: 1})
    Expect(err).To(HaveOccurred())
})
```

- [ ] **Step 2: Run the test to verify it passes**

```bash
rtk proxy go test ./simgrid/... -v -run "GetNextRound.*non-JSON"
```

Expected: PASS — `json.Unmarshal` fails on `}{not json` and returns an error.

- [ ] **Step 3: Run the full simgrid test suite**

```bash
rtk proxy go test ./simgrid/... -v
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add simgrid/client_test.go
git commit -m "test: add GetNextRound JSON unmarshal error coverage"
```

---

## Task 6: Add `BuildDriverLookup` entrylist 4xx error test

**Files:**
- Modify: `simgrid/client_test.go` (add one `It` inside existing `Describe("BuildDriverLookup")`)

### Context

`BuildDriverLookup` first calls `UsersForChampionship` (fetches `/participating_users`), then calls `GetEntriesForChampionship` (fetches `/entrylist`). The `BeforeEach` for the `BuildDriverLookup` `Describe` already registers a successful `/participating_users` handler. The missing test is when `/entrylist` returns 4xx — `GetEntriesForChampionship` should return an error containing "HTTP request failure".

- [ ] **Step 1: Write the failing test**

Inside the `Describe("BuildDriverLookup")` block (after the existing four `It` blocks), add:

```go
It("returns an error when the entrylist API fails", func() {
    mux.HandleFunc("/championships/champ1/entrylist", func(w http.ResponseWriter, r *http.Request) {
        http.Error(w, "not found", http.StatusNotFound)
    })
    _, err := client.BuildDriverLookup("champ1")
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("HTTP request failure"))
})
```

- [ ] **Step 2: Run the test to verify it passes**

```bash
rtk proxy go test ./simgrid/... -v -run "BuildDriverLookup.*entrylist API fails"
```

Expected: PASS — `GetEntriesForChampionship` calls `makeRequest` which checks the HTTP status code and returns an error wrapping "HTTP request failure" on 4xx.

- [ ] **Step 3: Run the full simgrid test suite**

```bash
rtk proxy go test ./simgrid/... -v
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add simgrid/client_test.go
git commit -m "test: add BuildDriverLookup entrylist API failure coverage"
```

---

## Task 7: Add `writeNextRoundConfig` os.WriteFile error test

**Files:**
- Modify: `discord/discord_internal_test.go` (add new `Describe("writeNextRoundConfig")` block)

### Context

`writeNextRoundConfig` generates a filename by calling `strings.ToLower(fmt.Sprintf("%s-round-%d-%s.yml", season, conf.NextRound.Number, strings.ReplaceAll(conf.NextRound.Track, " ", "-")))` then calls `os.WriteFile`. If the generated path includes a nonexistent directory (e.g., because `Track` contains `/`), `os.WriteFile` returns an error. This error path is not yet tested.

The function is package-private (`writeNextRoundConfig`), so this test goes in `discord_internal_test.go` (package `discord`).

- [ ] **Step 1: Write the failing test**

At the end of `discord/discord_internal_test.go`, before the closing `}` of the outermost suite scope, add a new top-level `Describe` block:

```go
var _ = Describe("writeNextRoundConfig", func() {
    It("returns an error when the target directory does not exist", func() {
        conf := &config.RoundConfig{
            NextRound: config.Round{
                Number: 1,
                Track:  "nonexistent/subdir",
            },
        }
        // Track contains "/" so the generated filename becomes:
        // "2026-round-1-nonexistent/subdir.yml"
        // os.WriteFile will fail because "nonexistent/" directory doesn't exist.
        _, err := writeNextRoundConfig(conf, "2026")
        Expect(err).To(HaveOccurred())
    })
})
```

- [ ] **Step 2: Run the test to verify it passes**

```bash
rtk proxy go test ./discord/... -v -run "writeNextRoundConfig"
```

Expected: PASS — `os.WriteFile` fails because `"2026-round-1-nonexistent/subdir.yml"` references a nonexistent directory.

- [ ] **Step 3: Run the full discord internal test suite**

```bash
rtk proxy go test ./discord/... -v
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add discord/discord_internal_test.go
git commit -m "test: add writeNextRoundConfig os.WriteFile error path coverage"
```

---

## Final verification

After all tasks are complete, run the full test suite:

```bash
rtk proxy go test ./... -v
```

Expected: all tests pass across all packages.
