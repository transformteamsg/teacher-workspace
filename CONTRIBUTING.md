# Contributing to Teacher Workspace

Teacher Workspace is a unified platform that consolidates teacher-facing applications into day-to-day workflows. The repository is a monorepo: a Go backend under `server/` and a React frontend under `apps/host/`.

## Development Setup

### Prerequisites

- **Go** >= 1.26.5
- **Node.js** >= 24
- **pnpm** >= 11

Recommended install on macOS:

```bash
brew install mise pnpm
mise use --global go@1.26.5 node@24
```

On Linux or Windows, install [mise](https://mise.jdx.dev/) and [pnpm](https://pnpm.io/installation) via your package manager, then run the same `mise use` command.

### First-time setup

From the repo root:

```bash
cp .env.example .env
pnpm install
make install-tools
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

Open <http://localhost:3000>. The Go server is the entry point: in development it proxies every request to the Rsbuild dev server, so hot reload still works.

To check a production build, where the server serves `apps/host/dist` instead of proxying:

```bash
pnpm build
TW_ENV=production go run ./server/cmd/tw
```

### Common commands

Server:

```bash
go test -race ./...                    # all tests, as CI runs them
go test ./server/internal/config       # single package
go test -run TestName ./server/...     # single test
make fmt                               # golangci-lint fmt, rewrites files in place
make lint                              # golangci-lint run, reports without fixing
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
            └── hooks/         Custom React hooks
```

A few conventions worth knowing:

- **`server/internal/` vs `server/pkg/`**: code imported only by this binary lives in `internal/`. Code that could be reused (or extracted later as a standalone library) goes in `pkg/`.
- **`components/` vs `containers/`**: containers are route-level (mounted by routes in `App.tsx`); components are route-agnostic. `components/ui/` is shadcn-generated, so regenerate rather than hand-edit.
- **`helpers/`**: pure functions only (no React imports, no side effects). If it touches state or hooks, it belongs in `hooks/` or a component.

## Branching & Workflow

Work happens on short-lived feature branches off `main`. Open a pull request back to `main` when ready.

### Branch naming

Use the format `<type>/<short-description>`, where `<type>` is one of the [Commit Conventions](#commit-conventions) types: `feat`, `fix`, `docs`, `refactor` (non-behavioral changes), `test`, `chore` (tooling, config, dependencies).

Examples: `feat/session-middleware`, `fix/server-startup-race`, `docs/contributing-guide`.

## Code Style

Don't use em-dashes (`—`) in code, comments, or documentation. Use colons, parentheses, or separate sentences instead.

### Go

Use **keyed struct literals** (field names) even when every field is set, including table-driven test cases. Positional literals break silently on field add/reorder.

- Yes: `User{Name: "a", Age: 1}`
- No: `User{"a", 1}`

Formatting: `make fmt`. Linting and static analysis: `make lint`.

#### Doc comments

- **Type comments**: start with the type name. Describe its role and non-obvious semantics (zero-value, invariants, ownership, mutability, concurrency). Include only what callers can't infer from the fields.
- **Method comments**: start with the method name. Describe from the caller's view (what, not how). Document non-obvious semantics (nil handling, mutation, errors, ordering, concurrency, zero-value).
- **Field comments**: describe the field, not surrounding workflow. Start with `contains` (slices/maps), `reports whether` (bools), `is`, or `holds`. If the field name implies the noun, focus on the qualifier.
- **Code comments**: explain _why_, not _what_. Comment only non-obvious logic, invariants, or edge cases, and tie them to stable intent so they don't rot.

### TypeScript / JavaScript

Formatting: `pnpm format` (oxfmt). Linting: `pnpm lint` (oxlint).

A [lefthook](https://lefthook.dev/) pre-commit hook (installed by `pnpm install`) runs both on staged files, auto-fixing and re-staging them.

## Test Conventions

### Go

Tests aim for clear, actionable failure messages and isolation from implementation details.

#### Structure

- One parent test per function or method under test, named `Test<Func>` (top-level) or `Test<Type>_<Method>` (methods). All cases live as `t.Run` subtests inside.
- Small pure helpers can have standalone tests without subtests.
- Related subtests can be grouped under an intermediate `t.Run` (for example `t.Run("rejects invalid input", ...)`); table-driven cases inside use only the distinguishing trait.

#### Naming

- Subtest names start with the outcome, optionally followed by the scenario: `"returns error on timeout"`, `"rejects invalid key"`.
- Table-driven cases inside a grouping `t.Run` use just the distinguishing trait: `"missing XXX"`, `"wrong status code"`.

#### Isolation

- Don't construct setup state or read assertion values via other methods on the unit under test. A bug in the helper would propagate as a misleading failure in an unrelated test.
- Set and inspect unexported fields directly (tests live in the same package and can touch them).

#### Setup

- Prefer `t.Setenv`, `t.TempDir`, and `t.Chdir` to manual save/restore. They restore the previous state automatically, including on failure.
- Register `t.Cleanup` immediately after the state is mutated, not further down the test body.

#### Assertions

Use `want/got` style. `want` goes on the left of the comparison: `!=` for equality, `==` for change-from-baseline.

The default failure message format is `want: <expected>; got: <actual>`. The trace line anchors the subject, so omit it. Prefix a subject only for convention (`err`, `ok`) or one-sided relational descriptors (`containing X`).

Bind sides individually in the `if` initialiser. Bare locals ("named") stay as-is; field access, calls, lookups, literals, and expressions ("fresh") get bound to `want`/`got`:

- both named: `if a != b { ... }`
- mixed: `if got := <fresh>; named != got { ... }` (use `want :=` if the fresh side is the expected)
- both fresh: `if want, got := X, Y; want != got { ... }`
- unary, named: `if err != nil { ... }`
- unary, fresh: `if got := <fresh>; <condition> { ... }`

Outer setup variables use semantic names (`snap`, `user`) so `want`/`got` stay free.

For multi-value returns (`value, ok := m[k]`), capture both and split each failure mode into its own `if`. Use `t.Fatal` on the `ok` check when a follow-up value comparison depends on presence.

##### Failure message templates

- Equality: `t.Errorf("want: %q; got: %q", want, got)`
- Property: `t.Errorf("want: non-empty; got: %q", got)` (descriptors: `nil`, `non-nil`, `empty`, `non-empty`)
- Boolean: `t.Error("want: true; got: false")`
- Panic: `t.Fatal("want: panic; got: nil")` / `t.Errorf("want: no panic; got: %v", r)`
- Error (nil): `t.Fatalf("want err: nil; got: %v", err)`
- Error (sentinel): `t.Errorf("want err: %v; got: %v", ErrInvalidInput, err)`
- Map presence: `t.Fatal("want ok: true; got: false")` / `t.Error("want ok: false; got: true")`
- Containment (generic): `t.Errorf("want err: containing %q; got: %q", "timeout", err)`
- Containment (constant): `t.Errorf("want c: in AlphabetBase58; got: %q", c)`
- Change from baseline: `if want, got := "id-1", sess.id; want == got { t.Errorf("want: != %q; got: %q", want, got) }`

### TypeScript / JavaScript

No conventions documented yet.

## Commit Conventions

- **Single summary line by default.** Details belong in the PR description. Add a body only when the reason isn't recoverable from the diff: why a version is pinned, why a workaround exists, why the obvious approach didn't work.
- **Conventional commit format:** `<type>(<scope>): <message>` or `<type>: <message>`. Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`. When used, scope identifies the file or package being changed.
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
