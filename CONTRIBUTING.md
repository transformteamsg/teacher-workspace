# Contributing to Teacher Workspace

Teacher Workspace is a unified platform that consolidates teacher-facing applications into day-to-day workflows. The repository is a monorepo: a Go backend under `server/` and a React frontend under `apps/host/`.

## Development Setup

### Prerequisites

- **[mise](https://mise.jdx.dev/installing-mise.html)** 2026.3.5 or newer, which installs and pins the tools below
  - **Go** 1.27.1
  - **Node.js** 24.19.0
  - **pnpm** 11.22.0
  - **golangci-lint** 2.14.0
- **Docker**, for the local Valkey and for the session store tests

`mise.lock` pins each tool and records how it was verified, so a tampered download is caught before it is installed. Only mise [2026.3.5](https://github.com/jdx/mise/releases/tag/v2026.3.5) and newer writes and checks that record; older versions install the tools without it. `mise.toml` sets `min_version` so that nobody misses the check.

Note that the server depends on [valkey-glide](https://github.com/valkey-io/valkey-glide), which is cgo-based and ships prebuilt native libraries for Linux and macOS only. Builds require `CGO_ENABLED=1` (the default), and Windows is not supported.

Install mise on macOS:

```bash
brew install mise
```

[Activate mise](https://mise.jdx.dev/getting-started.html#activate-mise) for your shell, then add `~/.local/share/mise/shims` to `PATH` for your editor and git hooks.

Some tools, such as golangci-lint, are downloaded from their GitHub releases (the `github:` entries in `mise.toml`), and GitHub rate-limits unauthenticated downloads, failing them with 403 or 429 errors. Authenticate before running `mise install` to avoid this: run `gh auth login`, or set `MISE_GITHUB_TOKEN` to a [GitHub token](https://mise.jdx.dev/dev-tools/github-tokens.html).

### First-time setup

From the repo root:

```bash
mise trust
mise install
cp .env.example .env
pnpm install
```

Edit `.env` to set the `TW_*` variables for your environment.

### Running locally

Run both processes from the repo root, in separate terminals:

```bash
# Terminal 1: host dev server on http://127.0.0.1:3001
pnpm dev
```

```bash
# Terminal 2: Go server on http://localhost:3000
go run ./server/cmd/tw
```

By default the server keeps sessions in memory, so they are lost on restart and are not shared between processes. That store holds at most 50,000 sessions and 64 MiB: past either limit it drops expired sessions first, then the least recently used. Both limits are configurable, see `.env.example`.

To run against a shared store instead, start the local Valkey and point the server at it:

```bash
docker compose up -d
TW_SESSION_STORE_PROVIDER=valkey TW_SESSION_VALKEY_URL=valkey://default:secret@127.0.0.1:6379 \
  go run ./server/cmd/tw
```

The server refuses to start if the store provider is `valkey` and Valkey cannot be reached: it never falls back to the in-memory store, since a silent downgrade would lose sessions in a horizontally scaled deployment.

To inspect sessions while developing, use the `valkey-cli` that ships in the Valkey container:

```bash
# List session keys
docker compose exec -e VALKEYCLI_AUTH=secret valkey valkey-cli --scan --pattern 'session:*'
# Open a prompt
docker compose exec -e VALKEYCLI_AUTH=secret valkey valkey-cli
```

At the prompt, `GET session:<id>` prints a session as JSON, `TTL session:<id>` shows the seconds until it expires, and `MONITOR` streams every command the server receives.

Open <http://localhost:3000>. The Go server is the entry point: in development it fetches the page from the Rsbuild dev server to embed the remote configuration, and proxies every other request, so hot reload still works.

To check a production build, where the server serves `apps/host/dist` instead of proxying:

```bash
pnpm build
TW_ENV=production go run ./server/cmd/tw
```

In production the server parses `apps/host/dist/index.html` once at startup and refuses to start when it is missing, so rebuild and restart together.

### Running against a local remote

Posts and Groups are served by the `pg` remote, and Student Insights by `si`. Neither is compiled into the host: the server reads their manifest URLs at startup and embeds them in the page. Unset, a remote is not registered and its route renders the unavailable fallback.

Point the host at a remote running locally, where the Parents Gateway dev server defaults to port 3004:

```bash
TW_REMOTE_POSTS_MANIFEST_URL=http://127.0.0.1:3004/mf-manifest.json go run ./server/cmd/tw
```

Set both variables to run two remotes at once:

```bash
TW_REMOTE_POSTS_MANIFEST_URL=http://127.0.0.1:3004/mf-manifest.json \
  TW_REMOTE_STUDENT_INSIGHTS_MANIFEST_URL=http://127.0.0.1:3005/mf-manifest.json \
  go run ./server/cmd/tw
```

The host and the remote share a single React instance, so both must be development builds or both production builds. A production host pointed at a remote's dev server fails to render it and shows the route's fallback.

### Common commands

Server:

```bash
go test -race ./...                    # all tests, as CI runs them
go test ./server/internal/config       # single package
go test -run TestName ./server/...     # single test
mise run fmt                           # rewrites files in place
mise run lint                          # reports without fixing
go build -o build/tw ./server/cmd/tw   # production binary
```

Web:

```bash
pnpm format                            # oxfmt, rewrites files in place
pnpm lint                              # oxlint, reports without fixing
pnpm build                             # production bundle
```

## Project Structure

A Go module at the root, plus a pnpm workspace covering `apps/*`.

```
.
├── server/                    Go backend
│   ├── cmd/                   Main entry points (one directory per binary)
│   ├── internal/              Private packages, scoped to this module
│   └── pkg/                   Exported, reusable packages
└── apps/
    └── host/                  React frontend (Module Federation host shell)
        └── src/
            ├── components/    Reusable UI primitives
            ├── containers/    Route-level components
            ├── helpers/       Pure utility functions
            ├── hooks/         Custom React hooks
            └── stores/        App-wide state
```

A few conventions worth knowing:

- **`server/internal/` vs `server/pkg/`**: code imported only by this binary lives in `internal/`. Code that could be reused (or extracted later as a standalone library) goes in `pkg/`.
- **`components/` vs `containers/`**: containers are route-level (mounted by routes in `App.tsx`); components are route-agnostic. `components/ui/` is shadcn-generated, so regenerate rather than hand-edit.
- **`helpers/`**: pure functions only (no React imports, no side effects). If it touches state or hooks, it belongs in `hooks/` or a component.

## Branching & Workflow

Work happens on short-lived feature branches off `main`. Open a pull request back to `main` when ready.

### Branch naming

Use the format `<type>/<short-description>`, where `<type>` is one of the [Commit Conventions](#commit-conventions) types: `feat`, `fix`, `docs`, `refactor` (non-behavioral changes), `test`, `chore` (tooling, config, dependencies), `release` (a version bump, branched as `release/vX.Y.Z`).

Examples: `feat/session-middleware`, `fix/server-startup-race`, `docs/contributing-guide`.

## Code Style

Don't use em-dashes (`—`) in code, comments, or documentation. Use colons, parentheses, or separate sentences instead.

### Go

Use **keyed struct literals** (field names) even when every field is set, including table-driven test cases. Positional literals break silently on field add/reorder.

- Yes: `User{Name: "a", Age: 1}`
- No: `User{"a", 1}`

Formatting: `mise run fmt`. Linting and static analysis: `mise run lint`.

#### Doc comments

Type, function, and method comments start with the name they document and describe the contract from the caller's view, not the implementation. Leave out what callers can't observe (internal steps, unexported fields and helpers, libraries used), but keep any constraint on what callers pass in or get back, even one a dependency imposes.

- **Type comments**: describe the role and non-obvious semantics (zero value, invariants, ownership, mutability, concurrency). Don't restate the declaration (field names and types, method signatures). For an interface, say what every implementation must guarantee (once here, not per method, when all methods share it); how one meets it belongs on that type.
  - Yes: `// Store holds sessions in memory. It is safe for concurrent use.`
  - No: `// Store holds sessions in a map guarded by a sync.RWMutex and evicts expired entries on read.`
  - Yes (interface): `// Store persists sessions by ID. If the context is already done when a method is called, the method returns an error wrapping the context's error.`
  - No (interface): the same context sentence repeated in every method's comment.
- **Function and method comments**: document non-obvious semantics (nil handling, mutation, errors, ordering, concurrency, zero value). Name sentinel errors as doc links (`[ErrNotFound]`), and say "wrapping" when callers must match with `errors.Is`.
  - Yes: `// Set stores val under key. val must be encodable as JSON.`
  - No: `// Set stores val under key, lazily initialising the underlying map on first use.`
- **Field comments**: start with the field name, then `contains` (slices and maps), `reports whether` (bools; for a bool option, what turning it on does), `is`, or `holds`. Describe the field, not the workflow around it. When the name already gives the noun, describe only the qualifier.
  - Yes: `// AuthenticatedTTL is how long an authenticated session is retained in the store before it expires.`
  - No: `// AuthenticatedTTL is read by the middleware and passed to Commit after login.`
- **Code comments**: explain _why_, not _what_, and only for non-obvious logic, invariants, or edge cases. Tie them to lasting intent, not to a change, so they don't rot.
  - Yes: `// A sub-second TTL truncates to Max-Age=0, which net/http omits rather than expires, shipping a cookie that outlives its store entry.`
  - No: `// Reject a TTL under one second.`

### TypeScript / JavaScript

Formatting: `pnpm format` (oxfmt). Linting: `pnpm lint` (oxlint).

A [lefthook](https://lefthook.dev/) pre-commit hook (installed by `pnpm install`) runs both on staged files, auto-fixing and re-staging them.

### Markdown

Don't manually wrap lines. Write each paragraph or list item on a single line and let the editor soft-wrap it.

## Test Conventions

### Go

Tests aim for clear, actionable failure messages and isolation from implementation details.

#### Structure

- One parent test per function or method under test, named `Test<Func>` or `Test<Type>_<Method>`, with every case as a `t.Run` subtest. Small pure helpers can have standalone tests without subtests.
- One behaviour, one subtest. A behaviour is one rule of the unit's contract applied to one scenario (a starting state plus an action), with every outcome the rule promises for it.
  - Keep a rule's outcomes in one subtest. If any of them breaks, the rule is broken, so splitting them only repeats the setup.
  - Give each independent rule its own subtest, even when they share a scenario. Each can break without the others, so a separate subtest keeps its name short and makes a failure point to the rule that broke.
- One code path, one subtest. If two subtests send the unit down the same path and differ only in why a dependency failed, keep one: the dependency's own tests cover its failure modes.
- Use table-driven tests only when cases differ purely in their inputs. A branch in setup or assertions means separate subtests directly under the parent test.
- Order subtests by the unit's lifecycle (for example, load before save), not by outcome.

#### Naming

- Name a subtest for its behaviour: the rule it checks and the scenario, in a short sentence. Don't list the rule's outcomes: the assertions check them, and their failure messages name what broke. The subtest asserts nothing outside that rule, apart from guards that make the assertions safe or meaningful (a nil check before use, a check that the scenario actually happened).
- Use one verb that covers every outcome the subtest asserts: `"replaces the handler's response with a 500 when the session cannot be saved"` covers the status, headers, and body. If no verb covers them all, the subtest checks more than one rule, so split it.
  - Avoid a verb another subtest contradicts: `"passes the handler's response through"`, when the middleware also rewrites `Cache-Control`.
- Use the contract's words from its godoc ("first write", "cannot be saved"), and describe the scenario at the unit's level, not a dependency's internals.
- Name a table-driven subtest for what every case checks, and each case for what differs, so the two read as one sentence. Don't repeat words such as "when" in both.
  - `"sets the session cookie's Secure attribute"` / `"to true when the Secure option is set"`
- Word paired subtests (two sides of one rule) the same way, so their names differ only where the scenarios do.
  - `"saves changes made before the handler's first write"` / `"does not save changes made after the handler's first write"`

#### Isolation

Test a unit the way its callers use it. Such tests survive refactoring and fail only when a caller would notice.

- A unit is a function, method, or type with a contract stated in its doc comment, exported or not. A function split out only to keep another one readable, with no callers or contract of its own, is an implementation detail of that function: test it through that function, so inlining or renaming it doesn't break tests.
- Set up and check state through APIs, the unit's own included, even when one type provides both sides: store with `Set` and read back with `Get`, not through the internal map. A bug in `Get` then also fails `Set`'s tests, but `Get`'s own tests fail alongside and point to it.
- Reach into unexported fields only when no API can set up the scenario or show the outcome, such as seeding a corrupted entry that `Set` would never write, or checking that an expired entry was freed. Don't export a field or add a method just so a test can reach it.
- When other programs read the data a unit writes, check the data where they read it. If other services read a cache's keys straight from Valkey, check the key in Valkey after `Set`, not only through `Get`: if `Set` and `Get` both used the wrong key, `Get` would still find the value.

#### Setup

- Prefer `t.Setenv`, `t.TempDir`, and `t.Chdir` to manual save/restore. They restore the previous state automatically, including on failure.
- Register `t.Cleanup` immediately after the state is mutated, not further down the test body.
- A failing setup call is not an assertion: `t.Fatalf("os.WriteFile: %v", err)`, naming the operation, never a `want`/`got` message. Use `t.Fatal`, not `t.Error`: the case can't run.
- Arrange the starting state so a broken unit can't pass by accident: seed any state the test claims is kept, so an overwrite or delete shows, and use values that differ from the defaults (a handler status of 201, not the recorder's default 200).

#### Assertions

- When a call returns something, assert on what it returns.
- When a call changes something, assert on its direct effects: the response, what a handler receives, what a store holds, what a later call returns.
- Don't assert on the calls the unit makes to its dependencies, except in two cases:
  - The call is the behaviour (`"sends one email"`): count it.
  - The effect only shows once the dependency acts on it, which the dependency's own tests cover: check what was passed instead, such as the TTL given to a cache, not whether the entry expires after a fake clock moves past it.
- Stop the subtest with a `t.Fatal` guard when the checks after it would panic or prove nothing:
  - Before using a value: `if user == nil { t.Fatal("want user: non-nil; got: nil") }`
  - Before indexing: `if len(items) == 0 { t.Fatal("want items: non-empty; got: empty") }`. Use an exact count (`len(items) != 1`) only in the subtest whose behaviour it is, so only that subtest fails when the count changes.
  - Before comparing a looked-up value (`value, ok := m[k]`): check `ok` in its own `if`, then compare the value. In a handler or callback, skipping this lets a nil dereference panic and stop the whole parent test.
  - Before checking the outcome, confirm the scenario happened, for example that a rotated token changed: `if oldToken == newToken { t.Fatalf("want token: != %q; got: %q", oldToken, newToken) }`
- When you add or change an assertion, temporarily break the code it targets and confirm it fails.
- Use `want`/`got` style, with `want` on the left (`!=` for equality, `==` for change from baseline). Failure messages read `want <subject>: <expected>; got: <actual>`, so a failure is clear without opening the test. The subject is the checked expression as written (`user.IsActive()`, `cookie.MaxAge`), or a short name for it when the expression is mostly plumbing (`Set-Cookie` for `rec.Result().Header.Values("Set-Cookie")`). Errors use `err` as their subject, and lookups name what was looked up before `ok` (`cookies["theme"] ok`).
- Call the unit under test in its own statement and assign what it returns to variables named for what they hold (`chained := Chain(h)`), so the action stands apart from the assertions that check it. Don't call it inside an assertion's `if` initialiser: `if got := Chain(h); h != got` hides the action inside the check.
- Bind a value to `want` or `got` in the `if` initialiser when the failure message prints it, so the message shows exactly what the condition tested. A plain variable ("named") needs no binding and prints as it is; a field, lookup, literal, expression, or call other than to the unit under test ("fresh") gets bound. Give setup variables semantic names so `want` and `got` stay free.
  - both named: `if a != b { ... }`
  - one fresh: `if got := <fresh>; a != got { ... }` (bind `want :=` instead when the fresh side is the expected value)
  - both fresh: `if want, got := X, Y; want != got { ... }`
  - one side, named: `if err != nil { ... }`
  - one side, fresh, printed: `if got := <fresh>; len(got) != 0 { ... }`
  - one side, fresh, not printed: `if len(items) == 0 { ... }`, `if !user.IsActive() { ... }`

##### Failure message templates

- Equality: `t.Errorf("want status: %d; got: %d", want, got)`
- Change from baseline (fails on `want == got`): `t.Errorf("want token: != %q; got: %q", want, got)`
- Property: `t.Errorf("want Set-Cookie: empty; got: %q", got)` (descriptors: `nil`, `non-nil`, `empty`, `non-empty`)
- Nil: `t.Error("want result: nil; got: non-nil")` / `t.Fatal("want user: non-nil; got: nil")`. A nil check reports a descriptor on both sides, not the value.
- Boolean: `t.Error("want user.IsActive(): true; got: false")`
- Error (nil): `t.Fatalf("want err: nil; got: %v", err)`
- Error (sentinel): `t.Errorf("want err: %v; got: %v", ErrInvalidInput, err)`
- Presence: `t.Fatal("want cookies[\"theme\"] ok: true; got: false")`
- Containment (generic): `t.Errorf("want err: containing %q; got: %q", "timeout", err)`
- Containment (constant): `t.Errorf("want c: in AlphabetBase58; got: %q", c)`
- Panic: `t.Fatal("want MustParse(\"\"): panic; got: nil")` / `t.Errorf("want MustParse(\"x\"): no panic; got: %v", r)`

#### Logs

- Treat logging as its own rule: check log output only in a subtest named for it (`"logs an error when the session cannot be saved"`), or one whose name promises no effects at all (`"does nothing when …"`).
- Capture logs with a `slog.JSONHandler` on a `bytes.Buffer`, attached the way the code reads its logger (for example `middleware.WithLogger`). Check that only the expected records were written, each with its level, message, and attributes (for an error log, an `err` containing the cause).
- Attach `slog.New(slog.DiscardHandler)` in every subtest of a unit that can log but doesn't check log output, even when the scenario logs nothing, so a stray log can't clutter the output or panic on a nil logger. A unit with no logger to reach, such as a context accessor, needs none.

#### Helpers

- Keep the scenario in the subtest body: the setup that varies, the action, and the assertions that state the behaviour. Extract only mechanics that are identical everywhere, carry no subtest-specific meaning, and are named for what they check or build, so the call can be trusted without opening them. Wait until enough subtests repeat it to show what's truly shared.
- Name a test double for what it does, followed by what it replaces, not for its kind: `failingWriter`, `fixedClock`, `recordingHandler`. A package's one general-purpose, configurable stand-in for an interface can take a generic name instead (`fakeStore`).
- Put helpers, including test doubles and their types, at the bottom of the test file that uses them, so readers meet the tests first. When several test files in the package need one, move it to its own test file (`fakestore_test.go`).

### TypeScript / JavaScript

No conventions documented yet.

## Commit Conventions

- **Single summary line by default.** Details belong in the PR description. Add a body only when the reason isn't recoverable from the diff: why a version is pinned, why a workaround exists, why the obvious approach didn't work.
- **Conventional commit format:** `<type>(<scope>): <message>` or `<type>: <message>`. Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `release`. When used, scope identifies the file or package being changed. `release` takes no scope and its message is the version alone: `release: vX.Y.Z`, which the release workflow matches exactly.
- **Backtick file and variable names**, including in the scope.
- **Be specific but high-level.** Name what changed, not vague descriptions, and not individual functions.
- **Make logical, incremental commits.** Each commit should represent a coherent change.

Examples:

```
feat(`random`): add base58/base62 alphanumeric generator package
docs(`CLAUDE.md`): refine assertion conventions
test(`random`): conform to assertion conventions
```

## PR Process

Open pull requests against `main`. Fill every section of [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md):

- **Summary**: what problem the PR solves and why.
- **Changes**: concrete list of what was changed.
- **Test Plan**: how the change was verified (commands run, scenarios exercised, automated tests added). Delete this section for docs-only or non-code PRs.

### PR title

Squash-merge is enforced: the PR title becomes the commit message in `main`, so it must follow the [Commit Conventions](#commit-conventions). GitHub appends the PR number automatically:

```
feat(`auth`): add session middleware        # PR title
feat(`auth`): add session middleware (#42)  # lands in main
```
