# End-to-End Testing Framework

**Status:** Proposal
**Audience:** Admiral CLI maintainers
**Depends on:** [`server-admin-subcommands.md`](server-admin-subcommands.md)

## Motivation

Unit tests prove individual functions behave as intended. They do not
catch the failure modes we actually ship: a command that parses flags
correctly but wires them to the wrong API field, a response formatter
that looks right in a mocked fixture but crashes on the server's
real pagination shape, a `--force` flag that was renamed on the server
but not the CLI.

The admiral-cli surface is now large enough — runners, environments,
deployments, sources, credentials, apps, infra, modules, state — that
"exercise the real thing against a real server" has shifted from a
nice-to-have to the only credible regression net. Manual testing is
already the de facto coverage mechanism. This proposal codifies that
work: each manual flow is captured as a script, run in CI, and assumed
green unless a deliberate change invalidates the expected output.

## Scope

- **In scope.** Running the compiled `admiral` binary against a real
  admiral-server stack booted from docker compose, seeded with a
  synthetic user + known PAT, exercising the full CLI surface command
  by command and asserting on stdout / stderr / exit code.
- **Out of scope.** UI tests (Playwright or similar will be a
  separate suite against the same stack). Agent tests (likewise).
  Load / performance testing. Chaos testing. Multi-tenant isolation
  testing. Upgrade/migration compatibility testing.

## Goals

1. **Coverage grows incrementally.** Manual testing sessions become
   scripts. No big-bang suite to build before any value ships. The first
   script exercising the runner lifecycle is the first unit of value.
2. **Deterministic.** Same inputs, same outputs, every run. Volatile
   values (IDs, timestamps, tokens) are either pinned or matched with
   regex, never compared as literal strings.
3. **Isolated between scripts.** Each script starts from a known state:
   schema migrated, user + PAT seeded, domain tables empty. One
   script's failure cannot contaminate the next.
4. **Fast enough for inner loop.** Full suite runs in under a few
   minutes locally. Individual scripts run in seconds after stack
   boot. If this slips, the suite will rot.
5. **Zero coupling to server internals.** The harness talks to the
   server through the CLI and through `admiral-server` subcommands
   only. No direct database access, no importing server Go packages,
   no scraping logs.

## Non-goals

- Replacing unit tests. Unit tests still catch fast, localized bugs;
  E2E catches integration bugs. Keep both.
- Testing every edge case of every flag. E2E exercises representative
  paths; unit tests own exhaustive permutations.
- Running against a deployed / hosted admiral. This suite is for
  throwaway ephemeral stacks only. If we ever want smoke tests against
  staging, that is a separate suite with a narrower assertion surface.
- Being portable to non-Docker environments. If you don't have docker
  compose, you can't run E2E. Acceptable.

## Dependencies

This suite cannot run until admiral-server grows the subcommands
described in [`server-admin-subcommands.md`](server-admin-subcommands.md):

- `admiral-server user create --email <reserved-domain>`
- `admiral-server pat create --user <email> --token <literal>`
- `admiral-server migrate up` and `migrate down --all`

Until those land, the scaffold in this repo stays build-tagged off by
default and serves as a shape-of-things preview. Local iteration on the
harness itself is possible against a hand-set-up stack by stubbing
those calls.

## Architecture

```
                  ┌──────────────────────────────────────┐
                  │          TestMain (once)             │
                  │  - docker compose up --wait          │
                  │  - admiral-server migrate up         │
                  │  - admiral-server user/pat create    │
                  │  - build admiral CLI → bin/          │
                  └──────────────┬───────────────────────┘
                                 │
                  ┌──────────────▼───────────────────────┐
                  │  testscript.Run(params)              │
                  │  for each .txtar in testdata/script: │
                  │    Setup(env):                       │
                  │      - migrate down+up (reset)       │
                  │      - user/pat create (re-seed)     │
                  │      - env.ADMIRAL_TOKEN=<known>     │
                  │      - env.PATH prepended with bin/  │
                  │    run script top-to-bottom          │
                  └──────────────┬───────────────────────┘
                                 │
                  ┌──────────────▼───────────────────────┐
                  │          TestMain (teardown)         │
                  │  - docker compose down -v            │
                  └──────────────────────────────────────┘
```

Two cadences: **once per suite** (compose up, build CLI, compose down)
and **once per script** (migrate reset, re-seed). The split is the
single most important performance decision — booting the compose stack
per script would blow the time budget; skipping reset between scripts
would introduce nondeterministic contamination.

## Directory layout

```
test/e2e/
├── main_test.go              # TestMain + testscript driver (build-tag e2e)
├── harness.go                # Harness struct: compose, migrate, seed, reset
├── doc.go                    # Package doc, running instructions
└── testdata/
    └── script/
        ├── runner_lifecycle.txtar
        ├── runner_tokens.txtar       (future)
        ├── env_lifecycle.txtar       (future)
        └── ...
```

One `.txtar` per coherent user flow. "Coherent" means a sequence that
makes sense to a human reviewer: "create a runner, issue it a token,
verify it shows up in the list, revoke the token, delete the runner."
Not one script per command — that fragments context and makes the
setup / reset overhead dominate.

Scripts live under `testdata/script/` because Go's `testdata/`
directory is excluded from `go build` by convention, and testscript
expects a subdirectory it can scan.

## Component design

### Compose orchestration

The harness uses `testcontainers-go`'s compose module to bring up
admiral-server's existing `docker-compose.yml` (path configurable via
`ADMIRAL_COMPOSE_PATH`). No test-specific compose file is needed if
the main one works; if it includes components we don't want
(standalone UI, agent), we introduce a `docker-compose.test.yml`
override that disables them. Kept out of this repo — lives next to
the main compose file in admiral-server.

Wait-for-ready is handled via testcontainers' `Wait` options, keyed
on admiral-server's health endpoint. Compose boot is the slowest
phase; budget 20-40 seconds.

### Seed and reset

All state mutation goes through the `admiral-server` binary, never
direct SQL. The harness knows four commands:

- `admiral-server migrate up` — initial schema creation.
- `admiral-server migrate down --all` — drops everything. Called
  between scripts.
- `admiral-server user create --email e2e@admiral.local` — synthetic
  user, reserved-domain email (see server-admin-subcommands doc).
- `admiral-server pat create --user e2e@admiral.local --token $KNOWN`
  — mints the test PAT with a pinned value so `ADMIRAL_TOKEN` is
  stable across resets.

Reset sequence: `migrate down --all && migrate up && user create && pat create`.
Typical time: ~1-2 seconds on Postgres with a small schema. Watch this
number; if it crosses 5 seconds as the schema grows, move to a
template-DB pattern or a dedicated `admiral-server db reset` command.

### testscript driver

[`rogpeppe/go-internal/testscript`](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript)
drives the `.txtar` files. Chosen because it is the de facto Go CLI
E2E tool (used by the Go toolchain itself, Cue, Gopls, and many
others), and because ordered-sequence-of-commands is exactly what
testscript does natively.

Key `testscript.Params` fields:

- `Dir: "testdata/script"` — where scripts live.
- `Setup` — per-script hook; calls the harness reset and injects env.
- `RequireExplicitExec: true` — scripts must use `exec admiral ...`,
  not bare `admiral ...`, for clarity about what's being tested.
- `UpdateScripts: os.Getenv("UPDATE") == "1"` — regenerate golden
  expectations. Set when intentional output changes happen; review
  the diff before committing.

### Env exposed to each script

| Var                 | Value                       | Purpose                                |
|---------------------|-----------------------------|----------------------------------------|
| `ADMIRAL_TOKEN`     | the pinned PAT              | CLI picks it up for auth               |
| `ADMIRAL_ENDPOINT`  | admiral URL from compose    | overrides default `localhost:<port>`   |
| `PATH`              | prepended with build dir    | `exec admiral` finds the freshly built binary |
| `HOME`              | temp dir per script         | isolates `~/.config/admiral/` writes   |

The last one matters: if the CLI writes to `~/.config/admiral/config.json`
during a test (e.g. `admiral config set token`), we don't want that
bleeding into the developer's real config. testscript's per-script
temp dir plus a redirected `HOME` handles it.

## Writing a test script

Scripts are plain-text `.txtar` files. Format:

```
# Comments start with #. Use them liberally to explain intent.

# A command line starts with a verb (exec, cp, mv, env, ...).
exec admiral runner create my-runner --description 'e2e runner'

# After a command, zero or more assertion lines apply to its output.
# `stdout PATTERN` matches a regex against stdout.
# `stderr PATTERN` matches against stderr.
# Prefix with ! to negate ("must NOT match").
stdout 'Created runner'
! stderr .
```

### Patterns and conventions

- **Match on substrings, not entire lines.** Volatile data like IDs
  and timestamps will drift; match the minimum that proves the
  behavior (`stdout 'Created runner'`, not the whole banner line).
- **For structured output, use `-o json` or `-o yaml`.** Table output
  is for humans; JSON/YAML is deterministic and diff-friendly.
  Prefer `admiral runner get foo -o json` and assert on the JSON
  fields you care about.
- **Volatile values get regex.** An ID assertion looks like
  `stdout '"id"\s*:\s*"[a-z0-9-]{36}"'`, not a literal UUID.
- **Pin what you can.** Runner names, token names, environment names
  are caller-supplied; use known values in the script so later
  assertions can reference them as literals.
- **Empty-list output is stderr.** Matches the CLI's kubectl-style
  convention — `No runners found.` goes to stderr with exit 0.
  Assertions: `stderr 'No runners found'`, not `stdout`.

### Handling async state transitions

Some flows involve server-side state changes that don't land
synchronously — "start a runner agent, verify status becomes
`in_progress`." testscript doesn't have a native poll construct, but
the CLI does: `admiral runner get <name> --wait --wait-for=in_progress --timeout=30s`
(or equivalent). Push the waiting into the CLI where possible; it
makes the scripts readable and the logic reusable.

If a command doesn't have a `--wait`, fall back to:

```
exec sh -c 'for i in $(seq 1 30); do admiral runner get r1 -o json | grep in_progress && exit 0; sleep 1; done; exit 1'
```

Ugly, but honest about what's happening. If this pattern appears more
than twice, add a `--wait` flag to the relevant CLI command instead.

## Running

### Local dev loop

```
# One-time setup: build admiral-server binary, point env at compose file
export ADMIRAL_COMPOSE_PATH=/path/to/admiral-server/docker-compose.yml
export ADMIRAL_SERVER_BINARY=/path/to/admiral-server/bin/admiral-server

# Run the whole suite
go test -tags=e2e ./test/e2e/...

# Run one script
go test -tags=e2e ./test/e2e/... -run TestCLI/runner_lifecycle

# Regenerate golden output after an intentional change
UPDATE=1 go test -tags=e2e ./test/e2e/... -run TestCLI/runner_lifecycle
# → review diff, commit

# See what's happening
go test -tags=e2e -v ./test/e2e/...
```

### CI

A GitHub Actions job on PRs to `master`:

1. Build the admiral-server binary (from the admiral-server repo, via
   `gh release download` or a sibling job in a monorepo setup).
2. Build the admiral CLI binary.
3. `go test -tags=e2e ./test/e2e/...`.
4. On failure, upload the compose logs as an artifact. The harness
   dumps `docker compose logs` into the test output on `Close()` if
   any script failed, so failures are debuggable without re-running.

Cache docker layers to keep boot fast. Expect ~3-5 minutes wall-clock
for a modest suite.

## Open questions

1. **Where does the admiral-server binary come from in CI?** If the
   repos are separate, we need a release artifact or a sibling
   checkout. If they're a monorepo at some point, a sibling build
   step. Resolves when the server-side changes land.
2. **Do we run against a pinned server version or HEAD?** Pinned is
   stable (CLI tests don't break because server changed); HEAD catches
   contract drift faster. Lean toward pinned with a separate nightly
   job against HEAD.
3. **Parallelism?** testscript supports parallel scripts via a flag.
   With per-script reset, parallelism needs per-script databases or a
   serialized reset. Start serial; revisit if suite gets slow.
4. **Coverage reporting?** Go's `-cover` works with E2E if we build
   the CLI binary with coverage instrumentation. Nice-to-have; defer.

## Future work

- **Second suite for UI** (Playwright) against the same compose
  stack, run as a separate Go test or a separate CI job.
- **Third suite for agent** once the agent has a stable external
  surface.
- **Contract fixtures** generated from the server's OpenAPI / gRPC
  schema, so the CLI's request shapes can be validated against the
  server's expectations without a full E2E run.
- **Snapshot testing for structured output.** `-o json` responses can
  be captured as golden files and diffed, reducing in-script regex.
- **Chaos / negative-path coverage.** What happens when the server
  returns 5xx mid-stream? When a runner token is revoked while a
  request is in flight? Not urgent; worth listing as a future tier.