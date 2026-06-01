# New-Season Self-Reconfigure — Design

**Date:** 2026-06-01
**Status:** Approved (pending spec review)

## Goal

Replace the manual "edit `config.yml` + restart the bot" chore performed every
new racing season with a live Discord admin command that reconfigures the
running bot. The command looks up / derives everything it can, previews the
changes, and on confirmation rewrites the config, updates the live in-memory
config (no restart), and emits a round-0 config for announcing week 1.

The five season-level config values currently hand-edited each season:

| Key | How it's determined now (manual) | How the command determines it |
|---|---|---|
| `season` | typed, e.g. `Fall` | `"<year> <term>"`, term by rotation, year from the matched championship |
| `championship_id` | looked up on SimGrid by hand | SimGrid search (upcoming + trackilicious + Rookies + term) |
| `discord_role_name` | typed, rotates by season | hardcoded term→name table |
| `briefing_folder_id` | folder created by hand in Drive | find-or-create `<season>` folder under the current briefing folder's parent |
| `tracker_folder_id` | folder created by hand in Drive | find-or-create `<season>` folder under the current tracker folder's parent |

## Locked decisions

1. **Live Discord command** (not CLI). Bot reconfigures itself; no restart.
2. **Two exact-match commands, no arguments:**
   - `!new-season` — preview only (read-only, zero side effects).
   - `!new-season-apply` — re-derive + commit.
3. **Season auto-rotates** from the last season in config; the admin never types a season.
4. **Season cycle:** New Year → Spring → Summer → Fall → Winter → (New Year, year+1). Year increments only on rollover back to New Year.
5. **Year source:** the matched championship's `start_date` year (term is rotated, year is read).
6. **Drive folder parents:** derive from the current briefing/tracker folder's parent; find-or-create the season folder as a sibling (idempotent).
7. **Config persistence:** yaml.v3 Node round-trip — preserve comments, secrets, and untouched keys.
8. **No season override argument** (YAGNI; fix by editing config + re-running if a season is skipped).

## Component design

### simgrid package — championship search

Confirmed against the live API (2026-06-01):

- `GET /championships?status=upcoming&races_count=full_championships` returns a
  **bare JSON array** of `{ "id": int, "name": string }`. (Do **not** pass
  `include_total_count`; that wraps the result in `{data, total_count}`.)
- `GET /championships/{id}` detail includes the fields we need:
  `name`, `host_name` (e.g. `"TRACKILICIOUS"`), `start_date`
  (RFC3339, e.g. `"2026-06-08T23:45:00.000Z"`), and the existing `races[]`
  (each `race.track.name`).

New types:

```go
type ChampionshipListItem struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}
```

Extend the existing `Championship` detail struct with:
`Name string json:"name"`, `HostName string json:"host_name"`,
`StartDate string json:"start_date"` (keep `Races []Race`).

New methods on `SimGridClient`:

- `ListUpcomingChampionships() ([]ChampionshipListItem, error)` — the index call above.
- `GetChampionship(id string) (*Championship, error)` — full detail. Refactor
  `GetNextRound` to call this instead of duplicating the fetch.
- `FindSeasonChampionship(host, term string) (*Championship, error)`:
  1. List upcoming.
  2. Filter `name` contains `"rookies"` AND contains `term` (both case-insensitive).
  3. For each candidate, `GetChampionship`; keep where `host_name` equals `host` (case-insensitive).
  4. Exactly one match → return it. Zero → error
     `no upcoming <host> Rookies championship matching season "<term>" found`.
     More than one → error listing the ambiguous matches (id + name).

Round-1 track for round-0 = `champ.Races[0].Track.Name`.

### season rotation + role map (new helper, in config package)

```go
var seasonCycle = []string{"New Year", "Spring", "Summer", "Fall", "Winter"}

var roleNames = map[string]string{
    "New Year": "GT4 Rookie New",
    "Spring":   "GT4 Rookies Springs",
    "Summer":   "GT4 Rookies Summer",
    "Fall":     "GT4 Rookies Fall",
    "Winter":   "GT4 Rookies Winter",
}
```

- `ParseSeasonTerm(season string) (term string, err error)` — strip an optional
  leading 4-digit year and surrounding whitespace; match the remainder
  (case-insensitive) against `seasonCycle` using a longest match so `"New Year"`
  wins over `"New"`. Error if no term matches.
- `NextTerm(term string) (string, error)` — next entry in `seasonCycle`
  (wrap Winter→New Year). The year is **not** computed here (it comes from the
  championship).
- `RoleNameForTerm(term string) (string, error)` — table lookup.
- Season string built by the caller: `fmt.Sprintf("%d %s", year, term)`.

### gcloud package — Drive folders

Extend `DriveServicer`:

```go
GetFile(ctx, id string) (*drive.File, error)            // need Parents → request fields "id,name,parents"
FindFolder(ctx, parentID, name string) (*drive.File, error) // Files.List, q below; nil if not found
CreateFolder(ctx, parentID, name string) (*drive.File, error) // mimeType application/vnd.google-apps.folder
```

`FindFolder` query:
`'<parentID>' in parents and name = '<name>' and mimeType = 'application/vnd.google-apps.folder' and trashed = false`.

New `Client` method:

```go
func (c *Client) EnsureSeasonFolder(ctx, currentFolderID, seasonName string) (newFolderID string, err error)
```

1. `GetFile(currentFolderID)` → take `Parents[0]` as the parent.
2. `FindFolder(parent, seasonName)` → if found, return its ID (idempotent).
3. Else `CreateFolder(parent, seasonName)` → return new ID.

Called twice (briefing, tracker). Regenerate the `DriveServicer` counterfeiter
fake with an explicit `-o gcloud/fakes/drive_servicer.go`.

### config package — comment-preserving update

```go
func UpdateBotConfigFile(path string, updates map[string]string) error
```

- Read file → `yaml.Unmarshal` into a `*yaml.Node`.
- Walk the document's root mapping node; for each key in `updates`, replace the
  value node's `Value` (and force a scalar tag). Append a new key/value pair to
  the mapping if the key is absent.
- `yaml.Marshal` the node back out (preserves head/line comments and other keys)
  and write with `0600`.

Only these keys are touched: `season`, `championship_id`, `discord_role_name`,
`briefing_folder_id`, `tracker_folder_id`. Secrets and comments are left intact.

### discord package — command + orchestration

**Wiring:** thread the `--config` path into the client. Add `configPath string`
to `DiscordClient`; add a param to `NewDiscordClient` and the
`newDiscordClient` factory in `runner.go`; pass `cCtx.String("config")` in
`runner.before`. Update `main_test.go` / runner tests for the new signature.

**Dispatch:** add two cases to the existing `switch event.Message.Content`:

```go
case "!new-season":       d.newSeason(event, false) // preview
case "!new-season-apply": d.newSeason(event, true)  // apply
```

**`!help`:** add entries to `helpMessage()`:

```
`!new-season`
  Preview the next-season reconfiguration (championship, schedule, config values). No changes made.

`!new-season-apply`
  Apply the next-season reconfiguration: create Drive folders, update the bot config live, and post the round-0 config.
```

**`newSeason(event, apply bool)` flow (shared derive, branch on apply):**

Derive (both modes — read-only):
1. `term0 := ParseSeasonTerm(d.conf.Season)`; `term := NextTerm(term0)`.
2. `champ := simgrid.FindSeasonChampionship("TRACKILICIOUS", term)`.
3. `year := parse start_date → .Year()`; `season := "<year> <term>"`.
4. `role := RoleNameForTerm(term)`.
5. `round1 := champ.Races[0].Track.Name`.
6. Resolve folder parents for the preview (the parent names/ids via `GetFile`);
   do **not** create folders in preview.

Preview (`apply == false`): post the preview message (section "Preview message"
below). No writes.

Apply (`apply == true`):
7. `briefingID := gcloud.EnsureSeasonFolder(briefingFolderID, season)`;
   `trackerID := gcloud.EnsureSeasonFolder(trackerFolderID, season)`.
8. `UpdateBotConfigFile(d.configPath, {season, championship_id, discord_role_name,
   briefing_folder_id, tracker_folder_id})`.
9. Under a mutex, update live `d.conf.BotConfig` fields (Season, ChampionshipId,
   DiscordRoleName, BriefingFolderID, TrackerFolderID).
10. Generate round-0 config (next round = R1 @ round1, empty penalties), write a
    temp file, attach to the response.
11. Post a confirmation message summarizing what changed (no secrets).

**Concurrency:** add a `sync.Mutex` to `DiscordClient` guarding the in-memory
config mutation in step 9. (Existing handlers read `d.conf` without locking;
full read-side locking is out of scope — this is an admin-only, once-per-season
operation.)

### Round-0 config

Build `config.RoundConfig{NextRound: config.Round{Number: 1, Track: round1}}`
(empty penalties, zero previous round — matches the example
`~/Downloads/summer-round-0.yml`). Marshal to YAML, write to a temp file named
`<season-slug>-round-0.yml` (lowercased, spaces → `-`, e.g.
`2026-summer-round-0.yml`), attach to the apply response. Reuse / generalize the
existing `writeNextRoundConfig` helper.

## Preview message (example, no secrets)

```
🗓 New Season Preview

Championship: GT4 Rookies - Summer (#24877)
  host TRACKILICIOUS · starts 2026-06-08 · registrations open
Schedule: R1 Misano · R2 Zandvoort · R3 Watkins Glen · R4 Random Track
  · R5 Bathurst · R6 Valencia · R7 Laguna Seca · R8 Random Track

Will set:
  season             2026 Summer
  championship_id    24877
  discord_role_name  GT4 Rookies Summer
  briefing_folder    "2026 Summer" under <briefing parent folder>
  tracker_folder     "2026 Summer" under <tracker parent folder>
Round-0 announces: Round 1 — Misano

▶ Run !new-season-apply to commit these changes.
```

## Error handling

- Each derive step returns a descriptive error; on failure the handler posts the
  error to Discord (mirroring the existing `sendBotResponse` defer pattern) and
  makes **no** changes.
- Zero or multiple championship matches → explicit error (see simgrid section).
- Drive / config-write failures in apply → post the error; partial state is
  possible (e.g. folders created but config not yet written) — apply is
  idempotent on re-run (find-or-create folders; config patch overwrites).

## Side effects to be aware of

`config.Season` changes shape from `"Fall"` to `"<year> <term>"`
(e.g. `"2026 Summer"`). It is embedded in:
- penalty-tracker doc titles (`gcloud.GeneratePenaltyTracker`),
- the briefing `[SEASON]` replacement token,
- generated round-config filenames (`writeNextRoundConfig`).

These now carry the year. Filenames must normalize the space in the season
(e.g. `2026-summer-round-1-misano.yml`).

## Testing (Ginkgo, in-package where unexported)

- `config`: season term parsing (with/without year, "New Year" longest match,
  invalid), `NextTerm` full cycle incl. wrap, role-name table,
  `UpdateBotConfigFile` preserves comments + other keys and updates the five.
- `simgrid`: `FindSeasonChampionship` against a faked HTTP transport — match,
  zero-match error, multi-match error, host/term/Rookies filtering, round-1
  track extraction. Reuse the existing client test scaffolding.
- `gcloud`: `EnsureSeasonFolder` via the regenerated `DriveServicer` fake —
  find-existing (idempotent) vs create-new, parent derived from current folder.
- `discord`: preview vs apply branching, help text includes both commands,
  dispatch routes the two commands, apply updates live config + attaches
  round-0. Honor the `discord/fakes` circular-import constraint (use
  `export_test.go` helpers for internal tests; counterfeiter fakes with explicit
  `-o` paths).

## Out of scope

- Buttons / interaction components (preview+apply commands instead).
- A season override argument.
- Read-side locking of `d.conf` across all handlers.
- Generating the round-1 briefing / penalty tracker (that remains
  `!race-setup` with the round-0 config attached).
