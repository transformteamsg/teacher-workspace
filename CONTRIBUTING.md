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

#### Signing in with `private_key_jwt`

```bash
mkdir -p .certs
openssl req -x509 -newkey rsa:2048 -sha256 -nodes -days 365 \
  -subj "/CN=teacher-workspace" \
  -keyout .certs/client.key -out .certs/client.crt
```

Set the server's client credentials in `.env`:

```bash
TW_EDUPASS_CLIENT_AUTH_METHOD=private_key_jwt
TW_EDUPASS_CLIENT_PRIVATE_KEY_FILE=.certs/client.key
TW_EDUPASS_CLIENT_CERTIFICATE_FILE=.certs/client.crt
```

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

By default the server keeps sessions in memory, so they are lost on restart and are not shared between processes. To run against a shared store instead, start the local Valkey and point the server at it:

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

See [Go Test Conventions](docs/go-test-conventions.md).

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
