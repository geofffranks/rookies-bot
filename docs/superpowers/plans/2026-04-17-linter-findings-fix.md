# Linter Findings Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve the 17 findings (12 `staticcheck` + 5 `gosec v2`) surfaced after the linters were added to `make lint`, so `make lint` exits clean.

**Architecture:** Fixes split into two buckets:
1. **Mechanical rewrites** — `ST1005` (lowercase error strings, strip trailing punctuation) and `S1005` (remove blank-ident). Existing `Ginkgo` tests assert on the error strings via `ContainSubstring`, so test assertions must be updated in lockstep with the production code.
2. **Security-judgment annotations** — `gosec` findings that are either (a) real fixes (G306 perms, G104 error propagation) or (b) justified suppressions (G304 user-supplied config paths, G107 Discord CDN URL). The **LM Studio MCP** (server name `lm-studio`, backed by qwen 3.5) is used to draft `#nosec` justification comments — a short text-transform task that fits its sweet spot.

**Tech Stack:** Go 1.25, Ginkgo v2, staticcheck (via `go tool`), gosec v2 (via `go tool`), LM Studio MCP (`lm-studio` server, qwen 3.5).

**Pre-flight:** Verify baseline.

```bash
go tool staticcheck ./... 2>&1 | grep -c ST1005   # expect 11
go tool staticcheck ./... 2>&1 | grep -c S1005    # expect 1
go tool gosec ./... 2>&1 | grep -c "^\["          # expect 5
```

---

### Task 1: ST1005 — `config/config.go` error strings

**Files:**
- Modify: `config/config.go:89`, `config/config.go:97`, `config/config.go:102`
- Modify (test lockstep): `config/config_test.go:49`, `config/config_test.go:123`, `config/config_test.go:129`, `config/config_test.go:137`

- [ ] **Step 1: Update production error strings**

Change `config/config.go:89`:
```go
return nil, fmt.Errorf("failed parsing YAML data: %s. Use something like https://yaml-online-parser.appspot.com to find the syntax error and try again", err)
```
(lowercased `F`, stripped trailing period — the sentence before the URL already has a period, satisfies ST1005)

Change `config/config.go:97`:
```go
return fmt.Errorf("failed reading %s: %s", file, err)
```

Change `config/config.go:102`:
```go
return fmt.Errorf("failed parsing %s: %s", file, err)
```

- [ ] **Step 2: Update test assertions**

`config/config_test.go:49`: `ContainSubstring("Failed parsing YAML data")` → `ContainSubstring("failed parsing YAML data")`
`config/config_test.go:123`: `ContainSubstring("Failed reading")` → `ContainSubstring("failed reading")`
`config/config_test.go:129`: `ContainSubstring("Failed reading")` → `ContainSubstring("failed reading")`
`config/config_test.go:137`: `ContainSubstring("Failed parsing")` → `ContainSubstring("failed parsing")`

- [ ] **Step 3: Verify**

```bash
go tool staticcheck ./config/...
go run github.com/onsi/ginkgo/v2/ginkgo ./config/
```
Expected: staticcheck clean on `config/`; all config specs PASS.

- [ ] **Step 4: Commit**

```bash
git add config/config.go config/config_test.go
git commit -m "lint: lowercase config error strings (ST1005)"
```

---

### Task 2: ST1005 — `discord/discord.go` error strings

**Files:**
- Modify: `discord/discord.go:176`, `:233`, `:238`, `:243`, `:248`, `:253`
- Modify (test lockstep): `discord/discord_internal_test.go:390`, `:397`, `:417`, `:426`, `:435`

- [ ] **Step 1: Update production error strings**

`discord/discord.go:176` — lowercase + strip trailing period:
```go
return nil, fmt.Errorf("could not find driver %d in registered SimGrid drivers. Please double check the car number and try again. Drivers may have changed their number, or withdrawn since the last race", carNumber)
```
(starts lowercase already; trailing period removed)

`discord/discord.go:233`:
```go
return "", "", fmt.Errorf("failed building driver list: %w", err)
```

`discord/discord.go:238`:
```go
return "", "", fmt.Errorf("failed generating penalty summary: %w", err)
```

`discord/discord.go:243`:
```go
return "", "", fmt.Errorf("failed to generate penalty message: %w", err)
```

`discord/discord.go:248`:
```go
return "", "", fmt.Errorf("failed to send penalty announcement: %w", err)
```

`discord/discord.go:253`:
```go
return "", "", fmt.Errorf("failed to pin penalty announcement: %w", err)
```

- [ ] **Step 2: Update test assertions**

`discord/discord_internal_test.go:390`: `"Failed building driver list"` → `"failed building driver list"`
`discord/discord_internal_test.go:397`: `"Failed generating penalty summary"` → `"failed generating penalty summary"`
`discord/discord_internal_test.go:417`: `"Failed to generate penalty message"` → `"failed to generate penalty message"`
`discord/discord_internal_test.go:426`: `"Failed to send penalty announcement"` → `"failed to send penalty announcement"`
`discord/discord_internal_test.go:435`: `"Failed to pin penalty announcement"` → `"failed to pin penalty announcement"`

- [ ] **Step 3: Verify**

```bash
go tool staticcheck ./discord/...
rtk proxy go test -v ./discord/
```
Expected: staticcheck clean on `discord/`; all discord specs PASS.

- [ ] **Step 4: Commit**

```bash
git add discord/discord.go discord/discord_internal_test.go
git commit -m "lint: lowercase discord error strings (ST1005)"
```

---

### Task 3: ST1005 — `simgrid/client.go` error string

**Files:**
- Modify: `simgrid/client.go:157`
- Modify (test lockstep): `simgrid/client_test.go` (line 218 or 227 — grep to confirm)

- [ ] **Step 1: Update production error string**

`simgrid/client.go:157`:
```go
return nil, fmt.Errorf("unknown driver: %#v", driver)
```

- [ ] **Step 2: Update test assertion**

```bash
rtk grep '"Unknown driver"' simgrid/client_test.go
```
Replace each `ContainSubstring("Unknown driver")` → `ContainSubstring("unknown driver")`.

- [ ] **Step 3: Verify**

```bash
go tool staticcheck ./simgrid/...
go run github.com/onsi/ginkgo/v2/ginkgo ./simgrid/
```
Expected: staticcheck clean on `simgrid/`; all simgrid specs PASS.

- [ ] **Step 4: Commit**

```bash
git add simgrid/client.go simgrid/client_test.go
git commit -m "lint: lowercase simgrid error string (ST1005)"
```

---

### Task 4: S1005 — `models/models.go` blank identifier

**Files:**
- Modify: `models/models.go:59`

- [ ] **Step 1: Remove unnecessary blank identifier**

`models/models.go:59` — change:
```go
for num, _ := range l {
```
to:
```go
for num := range l {
```

- [ ] **Step 2: Verify**

```bash
go tool staticcheck ./models/...
go run github.com/onsi/ginkgo/v2/ginkgo ./models/
```
Expected: staticcheck clean on `models/`; all specs PASS.

- [ ] **Step 3: Commit**

```bash
git add models/models.go
git commit -m "lint: drop blank identifier in map range (S1005)"
```

---

### Task 5: G306 — tighten file perms on generated round config

**Files:**
- Modify: `discord/discord.go:99`

**Context:** `writeNextRoundConfig` writes the generated `<season>-round-N-<track>.yml` to CWD, where it is immediately read back and uploaded as a Discord attachment. 0600 is sufficient (only this process reads it).

- [ ] **Step 1: Change permissions**

`discord/discord.go:99`:
```go
err = os.WriteFile(nextConfigFileName, data, 0600)
```

- [ ] **Step 2: Verify**

```bash
go tool gosec ./discord/... 2>&1 | grep G306
```
Expected: no G306 output.

```bash
rtk proxy go test -v ./discord/
```
Expected: all specs PASS.

- [ ] **Step 3: Commit**

```bash
git add discord/discord.go
git commit -m "lint: tighten round config file perms to 0600 (G306)"
```

---

### Task 6: G104 — propagate `os.Setenv` error in `config.Load`

**Files:**
- Modify: `config/config.go:78`

- [ ] **Step 1: Handle the error**

`config/config.go:78` — change:
```go
os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", config.GoogleServiceAccountToken)
return config, nil
```
to:
```go
if err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", config.GoogleServiceAccountToken); err != nil {
    return nil, fmt.Errorf("failed setting GOOGLE_APPLICATION_CREDENTIALS: %w", err)
}
return config, nil
```

- [ ] **Step 2: Verify**

```bash
go tool gosec ./config/... 2>&1 | grep G104
go run github.com/onsi/ginkgo/v2/ginkgo ./config/
```
Expected: no G104 on `config/config.go:78`; all specs PASS.

- [ ] **Step 3: Commit**

```bash
git add config/config.go
git commit -m "lint: propagate os.Setenv error in config.Load (G104)"
```

---

### Task 7: G304 — annotate `config/config.go` file-inclusion suppression

**Files:**
- Modify: `config/config.go:95`

**Context:** `loadFile` reads paths supplied by the operator via CLI args (`bot_config_path`, `round_config_path`). The operator is trusted; there is no external caller. `#nosec G304` with justification is the correct resolution.

- [ ] **Step 1: Draft the justification comment via the `lm-studio` MCP**

Call the LM Studio MCP tool (server name `lm-studio`, backed by qwen 3.5) with this prompt (goal: one-line rationale, ≤100 chars, suitable for a `#nosec` trailing comment):

```
Draft a one-line justification (≤100 chars, no leading "#") for a Go `#nosec G304` suppression.

Context: the function `loadFile(file string, config interface{}) error` reads a YAML
config file whose path comes from a trusted operator via CLI args in a Discord bot. There
is no network-facing caller. Return only the sentence, no quotes, no prose around it.
```

Use the returned text as `<REASON>` in Step 2. If the `lm-studio` MCP is unavailable, fall back to: `operator-supplied CLI path, no external caller`.

- [ ] **Step 2: Apply the `#nosec` annotation**

`config/config.go:95` — change:
```go
data, err := os.ReadFile(file)
```
to:
```go
data, err := os.ReadFile(file) // #nosec G304 -- <REASON>
```

- [ ] **Step 3: Verify**

```bash
go tool gosec ./config/... 2>&1 | grep G304
```
Expected: no G304 output for `config.go:95`.

- [ ] **Step 4: Commit**

```bash
git add config/config.go
git commit -m "lint: suppress G304 on operator-supplied config path"
```

---

### Task 8: G304 — annotate `discord/discord.go` attachment-open suppression

**Files:**
- Modify: `discord/discord.go:213`

**Context:** `sendBotResponse` calls `os.Open(attachment)` where `attachment` is a filename this process itself wrote earlier in the same request via `writeNextRoundConfig`. The name is constructed from config values, not external input.

- [ ] **Step 1: Draft justification via the `lm-studio` MCP**

Call the LM Studio MCP tool (server `lm-studio`, qwen 3.5) with this prompt:
```
Draft a one-line justification (≤100 chars, no leading "#") for a Go `#nosec G304`
suppression.

Context: `os.Open(attachment)` in a Discord reply handler. `attachment` is a filename
that this same process wrote moments earlier via `os.WriteFile` using a name built from
the round config (season, round number, track). No external input reaches the path.
Return only the sentence.
```
Fallback if the `lm-studio` MCP is unavailable: `path written by this process from config, not user input`.

- [ ] **Step 2: Apply the `#nosec` annotation**

`discord/discord.go:213` — change:
```go
file, err := os.Open(attachment)
```
to:
```go
file, err := os.Open(attachment) // #nosec G304 -- <REASON>
```

- [ ] **Step 3: Verify**

```bash
go tool gosec ./discord/... 2>&1 | grep G304
```
Expected: no G304 output for `discord.go:213`.

- [ ] **Step 4: Commit**

```bash
git add discord/discord.go
git commit -m "lint: suppress G304 on self-written attachment path"
```

---

### Task 9: G107 — annotate `discord/discord.go` Discord CDN URL suppression

**Files:**
- Modify: `discord/discord.go:62`

**Context:** `downloadAttachment` calls `http.Get(url)` where `url` is `events.MessageCreate` → `attachments[0].URL` — a Discord CDN URL supplied by Discord itself. Discord is the trust boundary here.

- [ ] **Step 1: Draft justification via the `lm-studio` MCP**

Call the LM Studio MCP tool (server `lm-studio`, qwen 3.5) with this prompt:
```
Draft a one-line justification (≤100 chars, no leading "#") for a Go `#nosec G107`
suppression.

Context: `http.Get(url)` where `url` is a Discord CDN attachment URL from a message
event we already chose to process. Discord is the trust boundary. Return only the
sentence.
```
Fallback if the `lm-studio` MCP is unavailable: `Discord CDN attachment URL from trusted message event`.

- [ ] **Step 2: Apply the `#nosec` annotation**

`discord/discord.go:62` — change:
```go
resp, err := http.Get(url)
```
to:
```go
resp, err := http.Get(url) // #nosec G107 -- <REASON>
```

- [ ] **Step 3: Verify**

```bash
go tool gosec ./discord/... 2>&1 | grep G107
```
Expected: no G107 output.

- [ ] **Step 4: Commit**

```bash
git add discord/discord.go
git commit -m "lint: suppress G107 on Discord CDN attachment URL"
```

---

### Task 10: Final verification + deploy reminder

- [ ] **Step 1: Full lint**

```bash
make lint
```
Expected: exit 0, no findings from `go fmt`, `go vet`, `staticcheck`, or `gosec`.

- [ ] **Step 2: Full test suite**

```bash
make test
```
Expected: all Ginkgo specs PASS.

- [ ] **Step 3: Confirm no staticcheck/gosec noise remains**

```bash
go tool staticcheck ./... ; echo "exit=$?"
go tool gosec ./... 2>&1 | tail -5
```
Expected: staticcheck exit 0; gosec summary `Issues: 0`.

- [ ] **Step 4: Remind operator to deploy**

Output to user: **"All linter findings resolved. Run `make deploy` to push to production."**

---

## Self-Review Summary

- **Spec coverage:** 17 findings → mapped to Tasks 1–9 (ST1005×11 in T1–T3; S1005 in T4; G306 in T5; G104 in T6; G304×2 in T7–T8; G107 in T9). T10 is the gate.
- **Test lockstep:** `ContainSubstring` assertions updated alongside every error-string change (T1–T3).
- **No placeholders:** `<REASON>` is filled in by the `local_llm` step immediately before use; concrete fallback supplied for each.
- **Type consistency:** no new types; signatures unchanged except `config.Load` still returns `(*Config, error)` after T6.
- **LM Studio MCP usage:** drafting three short `#nosec` justifications (T7–T9) via the `lm-studio` server (qwen 3.5) — fits the "text transform from a clear prompt" sweet spot. Mechanical edits stay with direct `Edit` because a precise one-char change is faster and more reliable than a round-trip.
