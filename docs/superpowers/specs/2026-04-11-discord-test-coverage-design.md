# Design: Discord Test Coverage Expansion

**Date:** 2026-04-11  
**Status:** Approved

## Goal

Increase unit test coverage of `discord`, `gcloud`, `simgrid`, and `runner` packages by:
1. Refactoring event handler business logic into testable inner methods
2. Adding tests for `getRoundConfig` using concrete struct construction
3. Adding tests for all remaining untested/partially-tested paths

Excluded: `time.LoadLocation` error path (untriggerable in practice), `NewDiscordClient`/`OpenGateway`/`Close` (require real Discord token), `sendBotResponse`/`onMessageCreate` (pure wiring).

---

## Section 1 — Refactor: extract business logic from event handlers

### Production code changes

`announcePenalties` and `raceSetup` each become two layers:

**Inner methods (new, testable):**

```go
func (d *DiscordClient) runAnnouncePenalties(roundConfig *config.RoundConfig, sgClient *simgrid.SimGridClient) (msg string, attachment string, err error)
func (d *DiscordClient) runRaceSetup(roundConfig *config.RoundConfig, sgClient *simgrid.SimGridClient) (msg string, attachment string, err error)
```

All business logic moves into these methods. They return `(msg, attachment, error)` — no event interaction.

**Outer methods (existing, keep thin):**

```go
func (d *DiscordClient) announcePenalties(event *events.MessageCreate) {
    var msg, attachment string
    defer func() { sendBotResponse(event, msg, attachment) }()
    roundConfig, err := getRoundConfig(event)
    if err != nil { msg = ...; return }
    sgClient := simgrid.NewClient(d.conf.SimGridApiToken)
    msg, attachment, err = d.runAnnouncePenalties(roundConfig, sgClient)
    if err != nil { msg = err.Error() }
}
```

Same pattern for `raceSetup`. Outer methods stay untested (no business logic).

**Why pass `sgClient` as parameter:** `simgrid.NewClient` is called inside both handlers using `d.conf.SimGridApiToken`. Passing it as a parameter lets tests inject an httptest-backed client without a real token.

### Tests for `runAnnouncePenalties` (in `discord_test.go`)

Uses fake rest client for Discord, httptest server for simgrid.

| Case | Expected |
|------|----------|
| simgrid `BuildDriverLookup` fails | returns error containing "Failed building driver list" |
| `buildPenaltyList` fails (unknown car) | returns error containing "Failed generating penalty summary" |
| `BuildPenaltyMessage` fails (GetMembers error) | returns error containing "Failed to generate penalty message" |
| `SendMessage` fails | returns error containing "Failed to send penalty announcement" |
| `Repin` fails | returns error containing "Failed to pin penalty announcement" |
| Happy path | returns msg containing previous round name, empty attachment |

### Tests for `runRaceSetup` (in `discord_test.go`)

| Case | Expected |
|------|----------|
| simgrid `BuildDriverLookup` fails | returns error |
| `GenerateBriefing` fails | returns error containing "failed to generate briefing doc" |
| `generateNextRoundConfig` fails (Track != "") | returns error containing "failed to generate config for next round" |
| `BuildBriefingMessage` fails | returns error containing "failed to generate briefingmessage" |
| `SendMessage` fails | returns error containing "failed to send briefing announcement" |
| `Repin` fails | returns error containing "failed to pin briefing announcement" |
| `CreateBriefingEvent` fails | returns error containing "failed to create briefing event" |
| Happy path, `NextRound.Track == ""` | no file written, msg contains round name |
| Happy path, `NextRound.Track != ""` | file written, attachment path non-empty, msg contains penalty tracker link |

File writing uses `t.TempDir()` via `GinkgoT()` — or tests `os.Remove` the file in `AfterEach`.

---

## Section 2 — `getRoundConfig` tests (approach C)

`events.MessageCreate` is a concrete struct; `Message.Attachments` is `[]discord.Attachment`. Build directly in tests. Spin up httptest server for download step.

**Test location:** `discord_internal_test.go` (package `discord`, accesses unexported function).

| Case | Expected |
|------|----------|
| 0 attachments | error "no race penalty YAML file" |
| 2 attachments | error "too many attachments" |
| 1 attachment, download fails (unreachable URL) | error "unexpected error downloading" |
| 1 attachment, server returns non-YAML | error "unable to parse race penalty YAML file" |
| 1 attachment, valid YAML | returns `*config.RoundConfig` with correct fields |

---

## Section 3 — `generateNextRoundConfig` tests

**Test location:** `discord_internal_test.go`. Takes `sgc *simgrid.SimGridClient` and `gc *gcloud.Client` directly.

| Case | Expected |
|------|----------|
| simgrid returns 4xx | error "failed getting details for next round" |
| simgrid OK, `GeneratePenaltyTracker` fails | error "failed generating penalty tracker" |
| Happy path | returns `*config.RoundConfig` with `PenaltyTrackerLink` set |

Uses httptest server for simgrid, fake Drive for gcloud.

---

## Section 4 — `generateUpdates` uncovered paths

**Test location:** `gcloud_test.go`, via `GenerateBriefing`.

| Case | Expected |
|------|----------|
| Doc body has no H3 "Stream" heading (`penaltyStartIndex` stays 0, condition is `< 0`) | **NOTE: bug exists** — `penaltyStartIndex` starts at 0 (valid index), check is `< 0`. A doc with no Stream heading at index > 0 will silently use index 0. Test documents current behavior. |
| Non-empty `QualiBansR1CarriedOver` | batch update contains driver name with "(carried over)" |
| Non-empty `QualiBansR2CarriedOver` | batch update contains driver name with "(carried over)" |
| Non-empty `PitStartsR1CarriedOver` | batch update contains driver name with "(carried over)" |
| Non-empty `PitStartsR2CarriedOver` | batch update contains driver name with "(carried over)" |
| Non-empty `QualiBansR1` (non-carried-over) | batch update contains driver name without "(carried over)" |
| Non-empty `PitStartsR1` (non-carried-over) | same |
| Non-empty `QualiBansR2` (non-carried-over) | same |
| Non-empty `PitStartsR2` (non-carried-over) | same |

---

## Section 5 — `generatePenaltyMessage` error paths for categories 2-8

Each penalty category has a `return "", err` branch when `getDriverId` returns a non-`DiscordHandleNotFoundError`. Trigger by having `GetMembers` fail. To hit category N specifically, put the driver only in that category.

**Test location:** `discord_test.go`, via `BuildPenaltyMessage`.

Categories to test (GetMembers error → error propagated):
- `PitStartsR1`
- `QualiBansR2`
- `PitStartsR2`
- `PitStartsR1CarriedOver`
- `QualiBansR1CarriedOver` (already partially covered, verify error return path specifically)
- `QualiBansR2CarriedOver`
- `PitStartsR2CarriedOver`

Each test: configure `GetMembers` to return error, put one driver in that category, call `BuildPenaltyMessage`, expect error.

---

## Section 6 — Remaining small gaps

### `simgrid.UsersForChampionship` — JSON unmarshal error

| Case | Expected |
|------|----------|
| Server returns 200 with non-JSON body | error from `json.Unmarshal` |

### `runner.newRunner` smoke test

| Case | Expected |
|------|----------|
| Call `newRunner()` | returned struct has non-nil `loadConfig`, `newGCloudClient`, `newDiscordClient` |

**Test location:** `main_test.go`.

---

## Architecture notes

- No new interfaces needed — `simgrid.SimGridClient` is a concrete struct already; pass as `*simgrid.SimGridClient`.
- `runAnnouncePenalties` and `runRaceSetup` stay as unexported methods — they're implementation detail of the command handlers.
- File I/O in `runRaceSetup` (writing next round config yaml) stays in the inner method. Tests use `AfterEach` to clean up the written file, or pass a predictable filename derived from config fields.
- `generateNextRoundConfig` remains a package-level function (not a method) — no change to its signature.
