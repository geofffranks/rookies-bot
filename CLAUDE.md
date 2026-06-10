# rookies-bot — Claude Code notes

## Testing (Ginkgo)

**Always write a `*_suite_test.go` bootstrap for every test package.**
Without it, Ginkgo specs compile and run zero tests with no error — silent failure.
Minimum bootstrap:

```go
package foo_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "testing"
)

func TestFoo(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Foo Suite")
}
```

**All test files must use Ginkgo — never fall back to `testing.T`.**
Using `t.Run()` subtests when the plan specifies Ginkgo is a spec violation, even if tests pass.
When writing plans, add an explicit note: "All test files MUST use Ginkgo. Do not use `testing.T`."

**TDD order: RED before GREEN, always — especially for bug fixes.**
Write the failing test first, confirm it fails for the right reason, then implement the fix.
Tests written after a fix pass immediately and prove nothing — you never saw them catch the bug.

**Use `rtk proxy go test -v ./pkg/` to see full Ginkgo output.**
Plain `go test -v` is intercepted and summarized by RTK, hiding individual spec names and failure messages.
Use `rtk proxy` any time you need spec-level detail.

## Mocks (counterfeiter)

**Always specify `-o` explicitly in `//counterfeiter:generate` directives.**
Counterfeiter defaults to `<pkgname>fakes/` (e.g., `discordfakes/`), not `fakes/`.
This project uses `fakes/` — omitting `-o` will silently place generated files in the wrong directory.

```go
//counterfeiter:generate -o fakes/fake_bot_rest_client.go . BotRestClient
```

(verify: current `discord/interfaces.go` and `gcloud/interfaces.go` omit `-o` but fakes exist in `discord/fakes/`
and `gcloud/fakes/` — confirm these still generate correctly before adding new directives without `-o`.)

Commit any `vendor/go.mod/go.sum` side-effects from `go generate` in the same commit as the generated fake.

## Architecture gotchas

**`discord/fakes` imports `discord` — do not import `discord/fakes` from internal `package discord` tests.**
This creates a circular import and will not compile.

Rules by test file type:
- `*_internal_test.go` (`package discord`) — only test unexported functions; no fakes imports
- `*_test.go` (`package discord_test`) — use `fakes.Fake*` freely
- If a helper must live in `package discord` but be usable by external tests, put it in `discord/export_test.go`
  (compiled only during `go test`, visible to both internal and external test packages)

(verify: `discord/` layout has `discord_internal_test.go` + `discord_test.go` + `discord_suite_test.go` + `fakes/` —
pattern is already established; maintain it when adding new test files.)

## Workflow

**Run `make deploy` after any code changes to push to production.**
`make all` intentionally excludes deploy to avoid accidental pushes — deploy must be run explicitly.
Reminder: at the end of any session where code changed, run `make deploy`.
