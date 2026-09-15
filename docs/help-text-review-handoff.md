# Handoff: help-text review of the held-back commands

Written 2026-09-15 at the end of the v0.1.0 release session. Pick this up in a
fresh session on branch `martin/next`.

## State of the world

- **master** = v0.1.0 (released: GitHub Releases, ghcr, Homebrew tap, Scoop)
  plus PR #15 (CI housekeeping) and #16 (badge). Surface: `app env auth config
  whoami version completion`.
- **`martin/next`** = master + one commit (`wip: agent, catalog, changeset,
  credential, run, source, state`). Holds the seven held-back command
  packages, their two e2e files, `COMMAND_TREE.md`, `TASK.md`, and `docs/`.
  Rebased onto master on 2026-09-15; build/vet/test green.
- The seven held-back packages are **byte-identical** to what was reviewed
  on 2026-09-14 — nothing in them has been touched since the review. The
  landed packages (app, auth, config, root, whoami, version) already had
  their review applied; use them as the reference for the patterns below.
- Root help on next uses two groups (`Resources:` / `Other:`) via
  `addGroup` in `cmd/root.go`. On master it is flat. Keep it that way.

## How to see what the user sees

```sh
go build -o /tmp/admiral . && BIN=/tmp/admiral ./scripts/help-dump.sh > /tmp/help.txt
```

Discovers subcommands through cobra's `__complete`, so `Keys:`-style lines
in `Long` text can't confuse it. Diff two dumps to review a change. 109
screens on next.

Style rules: `docs/cli-style-guide.md` §8 (help text) and §2.4 (no proto
enum names in human output). Tests that assert on help strings:
`cmd/error_test.go`, `cmd/app/app_test.go`, `cmd/auth/auth_test.go`.

## Patterns already established on the landed nouns — apply the same

- **Name-or-ID** is stated once in the noun's parent `Long` ("Commands accept
  an X name or ID as the positional argument."), never per leaf. Keep the
  "by ID" example.
- **Every example line has a `# comment`** above it; most common first.
- `get` Long: *"Print the one-line summary of an X. Use 'describe' for the
  full view and '-o json' for the raw record."* — wrapped at 80.
- `update` Long: *"Update an X's name, description, or labels. Only the
  fields you pass are changed."*; `--description` help is "new description".
- `describe` boilerplate is unchanged everywhere: *"describe is a human view.
  Use 'X get -o json' for the raw record."* Change it nowhere or everywhere.
- `describe` commands tolerate section failures (style guide §2.5 rule 10):
  `d.Unavailable(name, cmderr.Format(err))`, and `d.Hint(cmderr.ScopeHint)`
  when any section was `PermissionDenied`. Done for `app` and `env`;
  **not yet** for `agent` (5 reads), `run` (2), `changeset` (1). Text
  review first; this is a follow-on code change.
- Defaults are never restated in a flag description — cobra appends
  `(default …)` itself.
- Long text wraps at ~80 columns.

## Findings to apply, per noun

### Cross-cutting (all seven)

1. **Proto enum names in prose** → ordinary words matching the subcommand
   names. Today: "a TERRAFORM agent", "Create a HELM catalog item", "Create a
   BASIC_AUTH credential", "an OPEN change set", "marked DISCARDED",
   "CREATE / UPDATE / DESTROY / ORPHAN entries", "The credential's TYPE is
   immutable", "GIT, HELM, OCI, or HTTP source", and in one flag list
   `--passphrase-stdin (ssh-key)` next to `--password-stdin (BASIC_AUTH)`.
2. **Placeholders**: `<changeset-id>` (apply/describe/plan) vs `<id>`
   (copy/diff/discard/get) → `<changeset-id>`. `<KEY> <VALUE>` → `<key>
   <value>`. `<token>` and `<source>` → `<name>` (every other verb uses it).
3. **Example IDs**: one realistic display-ID form per resource everywhere
   (`cs-3k7m9p2q4rvw`, `run-vnx81rv3pp4c`); no `cs-1`, `cs-...`, `<uuid>`.
4. **Enum casing in examples** must match the flag help: `--status OPEN` →
   `open`; `--type NUMBER` → `number`.
5. "Updateable" (catalog, source) → "Updatable". "env's" → "environment's"
   in prose.
6. **Duplicate subcommand lists** in `catalog create`, `credential create`,
   `source create` `Long` — drop the hand-written list; move the descriptive
   phrase into each child's `Short` (e.g. *Create a catalog item from a
   Terraform root module*, *Create a credential from an SSH private key*,
   *Create a source for a Git repository*).
7. **Single unwrapped 100–170-char `Long` lines** in: changeset
   copy/create/discard, every `entry`/`var` leaf, run, source, catalog,
   source test, the three `credential create` types. Wrap at 80.

### Voice — internal notes leaking into user help

| Command | Today | Proposed |
|---|---|---|
| `run` | "Mutations route through 'admiral changeset plan/apply'; this surface is observation + recovery only." | "Runs are created by 'admiral changeset plan' and 'apply'. Use these commands to inspect, cancel, or roll back runs." |
| `changeset apply` | "Operators interact with the change set; the underlying run id is resolved server-side." | drop |
| `changeset var remove` | "Write a tombstone variable entry." / Short "Stage a variable deletion (tombstone)" | "Stage the removal of a variable. On deploy the key is deleted from the environment." |
| `changeset diff` | "…whose values_template references…" | "…whose values template references…" |
| `changeset entry update` | "Patches the component's HEAD on successful deploy with the non-empty fields below." | "On deploy, the fields you set here replace the component's current values." |
| `credential update` | "the secret material in auth_config" | "the secret (password, token, or private key)" |
| `changeset discard` | "Has no side effects on the environment; the change set record is retained for audit." | "The environment is not changed. The change set is kept for audit." |
| `agent token revoke` | "The agent using it gets 401 on its next request." | "An agent still using it is rejected on its next request." |
| `source test` | "Validate that the attached credential authenticates against the source URL. Persists outcome on the source." | "Check that Admiral can reach the source with its attached credential. The result is recorded and shown by 'source describe'." |
| `changeset entry destroy` | "run the engine's destroy verb (terraform destroy / helm uninstall / kubectl delete)" | keep the parenthetical, drop "engine's destroy verb" |

### Gaps vs style guide §8

- **No `Example` block**: `changeset get`, `changeset discard`, `changeset
  entry destroy/orphan/remove`, `changeset var remove`, `source test`,
  `source versions`, `state pull/push`.
- **`Long` equals `Short`**: `changeset get`, `run get`, `run list`. Write
  one useful sentence or leave `Long` empty.
- **Examples without `# comment`**: most of `changeset` and `run`.
- `--changeset` is a persistent flag on `changeset entry` / `changeset var`,
  so it renders under *Global Flags* on the leaves. Every `entry`/`var` leaf
  needs an example that shows it, since it is required.
- `changeset entry create` documents the name rules (lowercase, digits,
  hyphens, starts with a letter, ≤63) — fine, but nowhere else does; decide
  whether that belongs in a help topic instead.

### Defects

- `agent create --kind` and `changeset var set --type`: default printed
  twice (`(default terraform) (default "terraform")`). Delete the
  hand-written one.
- `state pull` / `state push` `Long`: *"Temporarily disabled in V2. See
  follow-up for component-name-based state access."* — dev-process text.
  **Decision needed from Martin:** hide the `state` command (`Hidden: true`)
  until it works, or reword to *"Not available in this release."* Both
  children also lack examples.

### `Short` rewrites

| Command | Today | Proposed |
|---|---|---|
| `agent token get` | Get agent token details | Get an agent token |
| `changeset apply` | Apply the change set's planned run | Apply a change set's latest plan |
| `changeset plan` | Plan the change set | Plan a change set |
| `changeset discard` | Discard an OPEN change set | Discard an open change set |
| `changeset entry create` | Stage a CREATE entry for a new component | Stage a new component |
| `changeset entry update` | Stage an UPDATE entry for an existing component | Stage changes to a component |
| `changeset entry destroy` | Stage a DESTROY entry for a component | Stage a component for destruction |
| `changeset entry orphan` | Stage an ORPHAN entry for a component | Stage a component to be orphaned |
| `changeset entry remove` | Remove an entry from an OPEN change set | Remove a staged entry |
| `changeset var set` | Stage a variable set | Stage a variable value |
| `run logs` | Print the engine transcript(s) for a run | Print the logs for a run |
| `--expires-in` (agent token create) | expiry duration (e.g. 720h); unset = no expiry | expiry duration (e.g. 720h); omit for no expiry |

## Suggested order

1. Cross-cutting items 1–7 noun by noun (agent → source → credential →
   catalog → run → changeset → state), re-dumping and diffing after each.
2. Voice table, gaps, defects, Shorts.
3. `go build ./... && go vet ./... && go test ./...`; re-dump; skim every
   changed screen once.
4. Then, as a separate commit: `Unavailable` sections on `agent`, `run`,
   `changeset describe`.
5. Commit on `martin/next`. Do not open a PR to master — the held-back
   nouns ship together later.

## Not part of this slice

- Homebrew `brews` → `homebrew_casks`: deliberately deferred (see memory
  note *homebrew-cask-plan*). Sign + notarize macOS builds first.
- `ListEnvironments` / `ListRuns` / `ListChangeSets` are `Unimplemented` on
  api.admiral.io (`SERVER_FOLLOWUPS.md`); `env` verbs can't be exercised
  live until that deploys.
