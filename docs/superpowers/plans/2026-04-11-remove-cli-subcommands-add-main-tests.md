# Remove CLI Subcommands + Add Main Package Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `announce-penalties` and `race-setup` CLI subcommands (now handled by the discord bot itself), then add Ginkgo unit tests for the remaining `bot` and `before` logic by extracting them into a testable `Runner` struct.

**Architecture:** Extract the `Before` and `bot` CLI actions from `main()` closures into methods on an injectable `Runner` struct with factory functions for config, gcloud, and discord. Define `BotDiscordClient` interface in the `discord` package (where `DiscordClient` already implements it) so counterfeiter can generate a fake importable by `package main` test files.

**Tech Stack:** Go, Ginkgo v2, Gomega, counterfeiter v6, urfave/cli v2

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `discord/interfaces.go` | Modify | Add `BotDiscordClient` interface + counterfeiter directive |
| `discord/fakes/fake_bot_discord_client.go` | Generate | Counterfeiter fake for `BotDiscordClient` |
| `runner.go` | Create | `Runner` struct with `newRunner()`, `before()`, `bot()` |
| `main.go` | Modify | Remove announce-penalties/race-setup; use Runner; drop unused helpers |
| `main_suite_test.go` | Create | Ginkgo bootstrap for main package |
| `main_test.go` | Create | Tests for `Runner.before` and `Runner.bot` |

---

### Task 1: Add `BotDiscordClient` to `discord/interfaces.go` and regenerate fakes

**Files:**
- Modify: `discord/interfaces.go`
- Generate: `discord/fakes/fake_bot_discord_client.go`

- [ ] **Step 1: Add the interface and counterfeiter directive**

Edit `discord/interfaces.go` to add the `context` import and the new interface. Final file contents:

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

//counterfeiter:generate . BotDiscordClient
type BotDiscordClient interface {
	OpenGateway(ctx context.Context) error
	Close(ctx context.Context)
}
```

- [ ] **Step 2: Regenerate fakes**

```bash
go generate ./discord/...
```

Expected: `discord/fakes/fake_bot_discord_client.go` is created (alongside the existing `discord/fakes/bot_rest_client.go`).

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

Expected: exits 0, no errors.

- [ ] **Step 4: Commit**

```bash
git add discord/interfaces.go discord/fakes/fake_bot_discord_client.go
git commit -m "feat: add BotDiscordClient interface to discord package"
```

---

### Task 2: Create `runner.go`

**Files:**
- Create: `runner.go`

- [ ] **Step 1: Write the failing test stubs** (they will fail until Task 3)

Skip — tests come in Task 3. Implement the struct first so the compiler accepts the test file.

- [ ] **Step 2: Create `runner.go`**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/discord"
	"github.com/geofffranks/rookies-bot/gcloud"

	"github.com/urfave/cli/v2"
)

// Runner holds injectable factories used by the CLI before/bot actions.
type Runner struct {
	conf             *config.Config
	dc               discord.BotDiscordClient
	loadConfig       func(string, string) (*config.Config, error)
	newGCloudClient  func(context.Context) (*gcloud.Client, error)
	newDiscordClient func(*config.Config, *gcloud.Client) (discord.BotDiscordClient, error)
	// stopChan is used to unblock the bot; nil means use os.Interrupt.
	stopChan chan os.Signal
}

func newRunner() *Runner {
	return &Runner{
		loadConfig:      config.Load,
		newGCloudClient: gcloud.NewClient,
		newDiscordClient: func(conf *config.Config, gc *gcloud.Client) (discord.BotDiscordClient, error) {
			return discord.NewDiscordClient(conf, gc)
		},
	}
}

// before is the urfave/cli Before hook. It loads config and wires up clients.
func (r *Runner) before(cCtx *cli.Context) error {
	var err error
	r.conf, err = r.loadConfig(cCtx.String("config"), "")
	if err != nil {
		return fmt.Errorf("could not load configs: %s", err)
	}

	gc, err := r.newGCloudClient(cCtx.Context)
	if err != nil {
		return fmt.Errorf("failed to connect to Google APIs: %s", err)
	}

	r.dc, err = r.newDiscordClient(r.conf, gc)
	if err != nil {
		return fmt.Errorf("failed to connect to discord: %s", err)
	}
	return nil
}

// bot is the urfave/cli action for the "bot" subcommand.
func (r *Runner) bot(_ *cli.Context) error {
	ctx := context.TODO()
	if err := r.dc.OpenGateway(ctx); err != nil {
		return err
	}

	fmt.Printf("rookies-bot is now running. Press CTRL+C to exit.\n")

	stop := r.stopChan
	if stop == nil {
		stop = make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)
	}
	<-stop

	r.dc.Close(ctx)
	return nil
}
```

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

Expected: exits 0.

---

### Task 3: Write the Ginkgo suite bootstrap for the main package

**Files:**
- Create: `main_suite_test.go`

- [ ] **Step 1: Write the suite file**

```go
package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRookiesBot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main Suite")
}
```

Note: `TestMain` is reserved by Go's testing package for special setup; use `TestRookiesBot` instead.

- [ ] **Step 2: Verify the suite compiles**

```bash
go test -list '.*' .
```

Expected: exits 0 (no specs yet, but the suite file compiles).

---

### Task 4: Write failing tests for `Runner.before`

**Files:**
- Create: `main_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"context"
	"errors"
	"flag"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/discord/fakes"
	"github.com/geofffranks/rookies-bot/gcloud"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"
)

func newTestCLIContext() *cli.Context {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Value: "config.yml"},
		},
	}
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	_ = set.String("config", "config.yml", "")
	return cli.NewContext(app, set, nil)
}

var _ = Describe("Runner.before", func() {
	var (
		r      *Runner
		cCtx   *cli.Context
		fakeDC *fakes.FakeBotDiscordClient
		testConf *config.Config
	)

	BeforeEach(func() {
		fakeDC = new(fakes.FakeBotDiscordClient)
		testConf = &config.Config{}
		r = &Runner{
			loadConfig: func(_, _ string) (*config.Config, error) {
				return testConf, nil
			},
			newGCloudClient: func(_ context.Context) (*gcloud.Client, error) {
				return nil, nil
			},
			newDiscordClient: func(_ *config.Config, _ *gcloud.Client) (discord.BotDiscordClient, error) {
				return fakeDC, nil
			},
		}
		cCtx = newTestCLIContext()
	})

	It("returns an error wrapping 'could not load configs' when config loading fails", func() {
		r.loadConfig = func(_, _ string) (*config.Config, error) {
			return nil, errors.New("disk full")
		}
		err := r.before(cCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("could not load configs"))
	})

	It("returns an error wrapping 'failed to connect to Google APIs' when gcloud fails", func() {
		r.newGCloudClient = func(_ context.Context) (*gcloud.Client, error) {
			return nil, errors.New("no credentials")
		}
		err := r.before(cCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to connect to Google APIs"))
	})

	It("returns an error wrapping 'failed to connect to discord' when discord creation fails", func() {
		r.newDiscordClient = func(_ *config.Config, _ *gcloud.Client) (discord.BotDiscordClient, error) {
			return nil, errors.New("bad token")
		}
		err := r.before(cCtx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to connect to discord"))
	})

	It("sets r.conf and r.dc on success", func() {
		err := r.before(cCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(r.conf).To(Equal(testConf))
		Expect(r.dc).To(Equal(fakeDC))
	})
})
```

Note: you'll need `"github.com/geofffranks/rookies-bot/discord"` in the import block for the `discord.BotDiscordClient` type used in the `newDiscordClient` lambda. Add it to the import list.

The full import block for `main_test.go`:

```go
import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/geofffranks/rookies-bot/config"
	"github.com/geofffranks/rookies-bot/discord"
	"github.com/geofffranks/rookies-bot/discord/fakes"
	"github.com/geofffranks/rookies-bot/gcloud"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"
)
```

- [ ] **Step 2: Run the tests to confirm they compile and pass**

```bash
go test -v ./...
```

Expected: The four `Runner.before` specs pass; `Runner.bot` specs do not yet exist.

---

### Task 5: Write tests for `Runner.bot` and commit

**Files:**
- Modify: `main_test.go`

- [ ] **Step 1: Append the `Runner.bot` describe block to `main_test.go`**

```go
var _ = Describe("Runner.bot", func() {
	var (
		r      *Runner
		fakeDC *fakes.FakeBotDiscordClient
		stop   chan os.Signal
	)

	BeforeEach(func() {
		fakeDC = new(fakes.FakeBotDiscordClient)
		stop = make(chan os.Signal, 1)
		r = &Runner{
			dc:       fakeDC,
			stopChan: stop,
		}
	})

	It("returns an error when OpenGateway fails", func() {
		fakeDC.OpenGatewayReturns(errors.New("gateway error"))
		err := r.bot(nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("gateway error"))
	})

	It("calls Close and returns nil after receiving the stop signal", func() {
		// Pre-load the stop channel so bot() doesn't block.
		stop <- os.Interrupt

		err := r.bot(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fakeDC.CloseCallCount()).To(Equal(1))
	})
})
```

- [ ] **Step 2: Run all tests**

```bash
go test -v ./...
```

Expected: All 6 specs (4 `before` + 2 `bot`) pass. All previously existing specs in `discord/`, `gcloud/`, `config/`, `models/`, `simgrid/` also pass.

- [ ] **Step 3: Commit tests and runner**

```bash
git add runner.go main_suite_test.go main_test.go
git commit -m "feat: extract Runner for testability; add main package Ginkgo tests"
```

---

### Task 6: Simplify `main.go` — remove announce-penalties/race-setup

**Files:**
- Modify: `main.go`

The current `main.go` has:
- Two closure vars: `announcePenalties`, `raceSetup`
- Four outer vars: `sgClient`, `penalties`, `driverLookup` (plus `conf`, `dc`, `gc` which disappear too)
- Three helper funcs: `buildPenaltyList`, `buildPenalizedDriverList`, `generateNextRoundConfig`
- A `Before` that conditionally sets up SimGrid when the command is not `bot`

After this task, `main.go` is just the wiring:

- [ ] **Step 1: Replace `main.go` entirely with the simplified version**

```go
package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	r := newRunner()

	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:        "bot",
				Usage:       "bot",
				Description: "Starts a long-running discord bot for rookies-bot",
				Action:      r.bot,
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Value: "config.yml"},
		},
		Before: r.before,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
	}
}
```

- [ ] **Step 2: Build to confirm no compilation errors**

```bash
go build ./...
```

Expected: exits 0.

- [ ] **Step 3: Run the full test suite**

```bash
go test -v ./...
```

Expected: All specs pass.

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "refactor: remove announce-penalties and race-setup CLI subcommands"
```

---

## Self-Review Checklist

- [x] `announce-penalties` subcommand removed from `main.go` ✓ (Task 6)
- [x] `race-setup` subcommand removed from `main.go` ✓ (Task 6)
- [x] `buildPenaltyList`, `buildPenalizedDriverList`, `generateNextRoundConfig` removed from main package ✓ (those functions remain in `discord` package where they're used)
- [x] SimGrid initialization logic removed from `Before` ✓ (Task 6)
- [x] `bot` action tested: OpenGateway error + stop signal success ✓ (Task 5)
- [x] `before` action tested: config error, gcloud error, discord error, success ✓ (Task 4)
- [x] counterfeiter used for `BotDiscordClient` fake ✓ (Task 1)
- [x] Ginkgo suite bootstrap present ✓ (Task 3)
- [x] No placeholders — all code blocks are complete
- [x] `BotDiscordClient` placed in `discord` package to avoid circular import with counterfeiter fakes ✓
