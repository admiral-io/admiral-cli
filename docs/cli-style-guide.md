# Admiral CLI Style Guide

The `admiral` CLI is the first-party client for the Admiral platform. It has two
audiences that must both be served by the same commands: **people at a terminal**
and **scripts and AI agents** driving it non-interactively. Every rule below is
written so that the human path and the machine path never fight each other.

The rules are distilled from a survey of kubectl, argocd, gh, gcloud, aws, az,
heroku, fly, vercel, railway, render, doctl, docker, terraform, helm and stripe,
plus the published guidelines (clig.dev, 12-Factor CLI, Heroku style guide, GNU,
POSIX, NO_COLOR, XDG, cobra). The per-tool research and verbatim output captures
live in [`docs/research/cli/`](research/cli/); rules cite the tool they come from.

Where this guide changes something the CLI does today, the change is listed in
[§14 Changes from current behaviour](#14-changes-from-current-behaviour).

---

## 0. Principles

1. **Humans first, machines equal.** Default output is for a person reading a
   terminal. `-o json` is a contract for everyone else and must be as complete
   and stable as the human output is readable. (clig.dev, gh)
2. **The same thing looks the same everywhere.** A run row, an age, a status
   word, an error line, a confirmation prompt: one shape each, reused by every
   command. Consistency beats local cleverness. (clig.dev "Be consistent across
   subcommands")
3. **stdout is the answer, stderr is everything else.** Progress, prompts,
   warnings, hints, "no results", next-page tokens: stderr. (gcloud, Heroku,
   12-Factor)
4. **Never require a prompt.** Every prompt has a flag that answers it; when
   stdin is not a terminal, prompts become errors that name the flag. (clig.dev,
   gh, fly)
5. **Make state changes visible and undoable.** Say what changed, print the
   next command to run, and make destructive actions hard to do by accident.
   (clig.dev, Heroku rollback UX)
6. **Follow the platform's vocabulary.** App, environment, change set, run,
   revision, component, module, source, credential, agent. The CLI never
   invents synonyms for server concepts.
7. **No command is exempt.** `config list`, `auth status`, `whoami`, `agent
   status` and every other utility command use the same tables, describe
   layout, `-o` formats, stderr rules and error shape as the resource
   commands. The single exception is `version`, whose first line stays a
   bare `admiral <semver>` so `admiral version | cut -d' ' -f2` keeps
   working. (GNU `--version`)

---

## 1. Command grammar

### 1.1 Shape

```
admiral <noun> <verb> [<name>] [flags]
admiral <noun> <verb> <type> <name> [flags]      # typed create: source create git, credential create ssh-key
admiral <noun> <sub-noun> <verb> [<name>] [flags] # one level of child nouns: agent token create
admiral <verb> [flags]                            # daily-loop verbs: status, logs, open, whoami, version
```

- **Noun first, verb last.** Nouns are singular (`app`, `env`, `run`,
  `changeset`, `module`, `source`, `credential`, `agent`). Plural and short forms
  are declared as `Aliases` and appear in help; they are never inferred.
  (gh, az; clig.dev "no arbitrary abbreviations")
- **Depth is at most three tokens before the name.** Sub-nouns exist only when
  the child is a real resource with its own lifecycle (`agent token`). Anything
  else reaches the child through a flag on the parent's verb
  (`run logs <id> --component api`, not `run component logs`). (gh)
- **A command that takes a positional never has subcommands**, and a noun with
  subcommands never takes a positional. `admiral env` prints help; it does not
  list. This is the ambiguity Heroku's colon grammar was invented to avoid;
  space-separated nouns keep it only by honouring this rule. (Heroku style
  guide, 12-Factor #11)
- **Verb set is fixed**: `list`, `get`, `describe`, `create`, `update`,
  `delete`, plus domain verbs that name an operation the server exposes
  (`plan`, `apply`, `cancel`, `rollback`, `discard`, `copy`, `test`,
  `versions`, `status`, `jobs`, `logs`, `wait`, `diff`, `top`). Never introduce
  `show`, `view`, `info`, `new`, `rm`, `ls` as canonical names; `ls`/`rm` may be
  aliases. (clig.dev "no similarly-named commands", az "never `get` or `new`"
  — Admiral picked `get`, the kubectl dialect, so `show` is out.)
- **Daily-loop verbs at the root** are allowed for the four things people type
  fifty times a day, exactly as fly and railway do alongside their full noun
  trees: `admiral status`, `admiral logs`, `admiral open`, `admiral whoami`.
  They are thin aliases over the scoped noun verbs (`env describe`, `run logs`)
  and must accept the same `--app/--env` flags. Nothing else goes at the root.

### 1.2 Identity and scoping

- **The child's name is positional; the parents are flags, or the path.**
  `admiral env get prod --app shop` and `admiral env get shop/prod` are the
  same command. A positional (or `--env`) that contains `/` is a path, most
  general segment first: `app/env`, later `app/env/component`. Each segment
  is a name or ID; names cannot contain `/`, so the split is unambiguous.
  `admiral run get run-vnx81r` needs no parent because run IDs are globally
  unique. (gcloud fully-qualified names, gh `-R owner/repo`)
- **Path or flag, never both.** `env get shop/prod --app shop` is a usage
  error even though the values agree — `--app cannot be combined with a
  path` — because a rule with no exception is the one people remember. A
  second bare positional beside a path inherits its parent: `env diff
  shop/prod staging`. Lists keep the parent as a flag (`env list --app
  shop`) because a list names no child.
- **A positional accepts a name or an ID.** Server IDs are unambiguous (UUIDs,
  `run-…`, `cs-…` display IDs), so the CLI resolves whichever it is given. The
  same rule applies to parent flags: `--app` takes a name or ID. Existing
  `--id`/`--app-id`/`--env-id` flags become hidden aliases and are removed in a
  later release. (az `--ids`, gcloud fully-qualified positionals)
- **Ambiguous names are an error, never a guess.** List the candidates with
  their IDs and exit 1. (kubectl, az)
- **Scope has two sources, and nothing implicit.** The path on the line,
  else the `--app`/`--env` flag, else a usage error that names both:
  `no application specified; pass --app or give the environment as
  app/env`. There is no environment-variable or config default. `ADMIRAL_APP`
  and `ADMIRAL_ENV` shipped in v0.1.0 and were removed (§14 #25): a scope
  inherited from the shell is invisible on the line that deletes `prod`, and
  the path form makes being explicit cost one word. (`FLY_APP` and `GH_REPO`
  exist because those tools lack a path form on most verbs; `gh -R
  owner/repo` is the form people actually use)
- **Lists that span a parent gain a leading scope column.** `run list --app
  shop` (all envs) shows `ENV` first, the way kubectl adds `NAMESPACE` under
  `-A`. A list always requires enough scope to be finite: `run list` requires
  `--app`; `env list` requires `--app`; top-level resources need nothing.

### 1.3 Flags

- Every flag has a long form. Short forms are single letters reserved for the
  common global set and never reused with a different meaning anywhere:

  | Short | Long | Meaning |
  |---|---|---|
  | `-o` | `--output` | output format |
  | `-f` | `--force` | skip confirmation |
  | `-v` | `--verbose` | debug output to stderr |
  | `-h` | `--help` | help |
  | `-w` | `--watch` | keep refreshing (list/get/status) |
  | `-l` | `--selector` | label selector on list |
  | `-m` | `--message` | free-text message on a mutation |
  | `-q` | `--quiet` | names/IDs only (list), suppress non-essential output |

  `-f` never means `--file`; file inputs are `--file` / `--values`. `-i` is
  not used (the `--insecure` short form is dropped). (clig.dev flag table,
  gh short-flag hygiene)
- **Flag names are kebab-case, lowercase, no units in the name**
  (`--timeout 5m`, not `--timeout-seconds`). Values are Go durations (`30s`,
  `5m`, `2h`), RFC3339 timestamps, or the enum listed in the help. (az,
  kubectl)
- **Booleans are plain switches.** No `--no-x` twin unless a config default can
  turn `x` on. (gcloud)
- **List-typed flags repeat**: `--label team=a --label tier=1`. Comma splitting
  inside one value is also accepted for enums (`--status Failed,Canceled`) but
  never for `key=value` flags, whose values may contain commas. (kubectl, gh)
- **Enum flags list their values in the help text**: `--phase string   phase to
  fetch: plan, apply (default: most recent)`. Invalid values fail with `invalid
  value "x" for --phase: must be one of plan, apply` and exit 2. (docker
  compose, gh)
- **Secrets never travel in flags.** Accept `--password-stdin`, `--token-stdin`,
  `--private-key-file`, or a TTY prompt with echo off. The bare `--password`
  form is deleted. (clig.dev, docker `--password-stdin`, vercel `env add`)
- **Positionals beyond the name are limited to homogeneous lists**
  (`changeset remove-var KEY [KEY...]`). Two positionals of different kinds are
  forbidden except `set-var KEY VALUE`, which reads as one unit. (12-Factor
  "2 types are very suspect")

### 1.4 Setting, merging and clearing fields

One convention for every `update`:

| Field kind | Set / merge | Remove one | Clear all |
|---|---|---|---|
| scalar string (`--description`, `--name`) | `--description "text"` | — | `--description ""` |
| reference (`--credential`, `--runner`) | `--credential ci-deploy` | — | `--clear-credential` |
| map (`--label`, `--var`) | `--label k=v` (merges) | `--remove-label k` | `--clear-labels` |
| list (`--depends-on`) | `--depends-on x` (appends) | `--remove-depends-on x` | `--clear-depends-on` |

`--label` on `update` **merges**; to replace, pass `--clear-labels --label
k=v` in one call. Flag help states which it is. Empty-string clearing is
implemented with nil-able flags so "omitted" and "set to empty" are
distinguishable. (gcloud set/update/remove/clear quartet, gh `NilStringFlag`,
stripe `--field=""`)

---

### 1.5 Shell completion of server names

Every argument that names a server resource completes from the server, the
way `kubectl get deploy bil<TAB>` fills in `billing-api`. This is part of a
command's definition, not a later polish pass: a positional or flag that
goes through `resolve.X` without a `complete.X` is incomplete.

- **What completes.** Every name-or-ID positional (`app get <name>`,
  `env describe <name>`, `source delete <name>`, …) and every parent flag
  (`--app`, `--env`, `--agent`, `--source`, `--credential`, …). Enum flags
  complete from their fixed list (`--scope`, `--phase`, `--status`). Static
  choices such as `source create <type>` complete from the subcommand list,
  which cobra does on its own. (kubectl, gh `-R`, gcloud)
- **Names only, never IDs.** Nobody tab-completes a UUID; `-o name` and
  `list` exist for that. A candidate carries its description after a tab
  (`billing-api\tBilling and invoicing`) so zsh and fish can show it.
- **Scope comes from the line, as for the command.** A bare environment
  name (positional or `--env`) completes inside the `--app` already on the
  line. A path completes segment by segment: `env describe sh<TAB>` offers
  `shop/` with no trailing space, `env describe shop/pr<TAB>` offers `prod`.
  With neither `--app` nor a slash there is nothing to offer except
  application prefixes, and an empty result is silent, not an error.
- **Never prompt, never block, never write to stdout.** The shell owns the
  terminal during completion; the shell scripts discard stderr, and anything
  on stdout is parsed as a candidate. The round trip is bounded (2s), runs
  under `ADMIRAL_NO_INPUT`, and every failure — not logged in, server down,
  secret store locked — yields no candidates with
  `ShellCompDirectiveError`. `ShellCompDirectiveNoFileComp` is set on every
  success so an empty list never falls back to filenames. (kubectl)
- **Bounded fetch.** Page at the server's maximum and stop at 500 names;
  filter by prefix client-side. No caching until a resource proves too slow
  to list on every Tab. (kubectl caches discovery, not names)
- **How.** One function per resource in `internal/complete`
  (`complete.Apps(opts)`, `complete.Envs(opts)`, …). Positionals use
  `ValidArgsFunction: complete.First(complete.Apps(opts))`; the parent flag
  helpers in `internal/flags` (`flags.App`, `flags.Env`) register the flag
  completion themselves, so a command that registers its scope flags the
  standard way gets completion for free. `complete.Envs` handles both the
  bare and the path form.
- **Verify with** `admiral __complete app get bil` — it prints the
  candidates and the directive without a shell in the loop.

## 2. Output: human mode

"Human mode" is decided per stream by `isatty`: colour and truncation follow
stdout, spinners and prompts follow stderr/stdin. When stdout is a pipe the
table layout is unchanged but colour and truncation are off (see §4).

### 2.1 Tables (`list`, and `get` in table mode)

`get` prints the **same one-row table** as `list`. There is no special
single-object layout; that is what `describe` is for. (kubectl)

Rules, all from kubectl's SIG-CLI conventions unless noted:

- Headers are **UPPERCASE with hyphens between words**: `CHANGE-SET`,
  `LAST-RUN`, `CREATED-BY`. Never spaces inside a header.
- **`NAME` (or `ID`) is the first column and `AGE` is the last.** A scope column
  (`ENV`, `APP`) goes before `NAME` when the list spans parents.
- Columns are separated by three spaces (`tabwriter` padding 3), no borders,
  no colour on the header, no row separators. (Heroku "never output table
  borders")
- **Absent values print `<none>`; not-yet-known values print `<unknown>`.**
  Never a blank cell, never `-`. Column output must survive `awk '{print $3}'`.
- **Free-text columns are truncated** to 40 characters with a single `…` only
  when stdout is a TTY. IDs and names are never truncated.
- **Every status resource carries a `STATUS` column, and where the server has
  a health concept a `HEALTH` column immediately after it.** Both are single
  CamelCase tokens (§2.4). (argocd `STATUS HEALTH`)
- `-o wide` appends, in order, the columns that are useful but noisy: `ID`,
  `CREATED-BY`, and any full-width fields the narrow row truncated.
- The **narrow row is shared** between `list`, `get`, and the echo after
  `create`/`update` for a given resource; `printXRow` in `cmd/<pkg>/output.go`
  is the single source of truth.

Default columns:

| Resource | Narrow columns | `-o wide` adds |
|---|---|---|
| app | `NAME DESCRIPTION LABELS AGE` | `ID CREATED-BY` |
| env | `NAME APP HEALTH DESCRIPTION LABELS AGE` | `ID RUNNER LAST-RUN CREATED-BY` |
| run | `ID STATUS CHANGE-SET TITLE AGE` (+ leading `ENV` when unscoped) | `DURATION TRIGGERED-BY STARTED` |
| changeset | `ID TITLE ENV STATUS ENTRIES AGE` | `CREATED-BY UPDATED` |
| module / source / credential | `NAME <anchor> DESCRIPTION LABELS AGE` (anchor = `TYPE`, `URL`, `TYPE`) | `ID CREATED-BY` |
| agent | `NAME STATUS LAST-SEEN DESCRIPTION LABELS AGE` | `ID VERSION ENVS` |
| agent token | `NAME AGENT EXPIRES AGE` | `ID CREATED-BY` |

Example:

```
$ admiral run list --env shop/prod
ID            STATUS     CHANGE-SET   TITLE                        AGE
run-vnx81r    Succeeded  cs-7f2a1     Bump api to 1.4.0            3h
run-8k2m1q    Failed     cs-6e019     Rotate DB credentials        2d
run-zz09aa    Canceled   <none>       <none>                       41d
```

### 2.2 Empty results

Print `No <plural> found[ in <scope>].` to **stderr**, nothing to stdout, exit
0. `-o json` prints `[]`, `-o name` prints nothing. Never print a lonely header
row. (kubectl `No resources found in X namespace.`, gh, gcloud `Listed 0
items.`; argocd's bare header is the anti-pattern)

```
$ admiral run list --env shop/staging
No runs found in shop/staging.
```

### 2.3 Age and time

- **`AGE` uses kubectl's `HumanDuration` verbatim**: `13s`, `5m12s`, `2h`,
  `5d3h`, `41d`, `2y`. Never "ago", never more than two units, never past two
  units after 8 days. Replace `output.formatDuration` with this table.
- **Absolute timestamps** appear only in `describe`, `-o wide`, and logs. In
  `describe` the format is kubectl's local RFC1123 (`Wed, 01 Jul 2026 18:13:49
  -0400`); in tables and log lines it is RFC3339 UTC (`2026-07-01T22:13:49Z`).
- **Durations of runs** use Go's `Duration.String()` truncated to seconds:
  `2m13s`, `47s`, `1h4m2s`. (gh `ELAPSED`)

### 2.4 Status and health vocabulary

- One **CamelCase token** per value, no spaces, no underscores:
  `Pending`, `Queued`, `Planning`, `Planned`, `Applying`, `Succeeded`,
  `PartiallyFailed`, `Failed`, `Canceled`, `Superseded`, `Blocked`,
  `Deferred`. (kubectl, argocd; replaces today's `PARTIALLY_FAILED`)
- Agent presence is `Online`, `Stale`, `Offline` (thresholds in §2.5).
- Health is `Healthy`, `Progressing`, `Degraded`, `Suspended`, `Missing`,
  `Unknown`. An environment's health is the **worst of its components**, in
  that priority order (most healthy first). The rule is documented in `env
  describe --help`. (argocd)
- The proto enum name (`RUN_STATUS_SUCCEEDED`) never appears in human output.
  It does appear in `-o json`, because JSON is the API object (§3.1).
- On a TTY the status word is coloured (green terminal states, yellow
  in-progress, red failed, dim canceled/superseded). Colour never carries
  meaning alone: the word is always printed. (Primer "colour enhances, never
  communicates")
- There is **no glyph column in tables**. Glyphs (`✓ ✗ … !`) appear only in
  progress lines on stderr and in `describe`/`status` headlines. (kubectl
  purity; keeps tables greppable)

### 2.5 `describe`

`describe` is the human deep-dive: everything `get` shows plus the children and
recent history, in kubectl's key/value layout. It exists on every resource that
has children or history (`app`, `env`, `changeset`, `run`, `agent`) and has no
`-o` flag; the machine path is `get -o json` plus the child `list` verbs.
(kubectl)

Layout rules, taken from `kubectl describe`:

1. **Top block**: `Key:` in Title Case, colon attached, values aligned in one
   column for the block (tabwriter, padding 2). Keys are ordered identity →
   description → relationships → state → timing → URL.
2. **Multi-value fields continue on indented lines** under the same key
   (`Labels:` one `k=v` per line, sorted).
3. **Absent is `<none>`**, never blank, never omitted.
4. **Sections** are a Title Case header followed by a colon on its own line;
   content is indented two spaces. Nested key/value blocks nest another two.
5. **Embedded tables** use Title Case headers with a dashes underline of the
   same width, two-space indent, no truncation.
6. **Timestamps are absolute local** (`Wed, 01 Jul 2026 18:13:49 -0400`);
   embedded tables that list history use an `Age` column.
7. **`Events:` (or the resource's history table) is always the last section**,
   newest last.
8. **The final line is a next-step hint** when one exists, e.g.
   `To see the transcript, run: admiral run logs run-vnx81r`. (gh `run view`)
9. **`URL:` links to the web UI** and is rendered as an OSC-8 hyperlink on a
   TTY. (argocd, gh)
10. **A section whose read fails renders as `<unavailable: reason>`** under
    its header, and the command still exits 0. The primary record is the
    view; the sections are assembled from further reads, and a missing scope
    or an endpoint the server does not serve costs that section, not the
    whole describe. The reason is `cmderr.Format(err)`; when any section was
    `PermissionDenied`, the closing hint is `cmderr.ScopeHint`. Lookup of the
    primary record itself still fails hard. (`output.Describe.Unavailable`;
    kubectl describe prints what it can when events are forbidden)

#### `admiral app describe shop`

```
Name:         shop
ID:           2f1c9a7e-3b1d-4c6e-9d5a-1f0e8c7b6a54
Description:  Storefront and checkout services
Labels:       team=commerce
              tier=1
Created:      Wed, 01 Jul 2026 18:13:49 -0400
Created By:   martin@admiral.io
URL:          https://app.admiral.io/apps/shop

Environments:
  Name     Runner     Health    Last Run              Age
  ----     ------     ------    --------              ---
  prod     gke-prod   Healthy   run-vnx81r Succeeded  41d
  staging  gke-stage  Degraded  run-8k2m1q Failed     41d

Recent Runs:
  ID          Env      Status     Change Set  Title                  Age
  --          ---      ------     ----------  -----                  ---
  run-vnx81r  prod     Succeeded  cs-7f2a1    Bump api to 1.4.0      3h
  run-8k2m1q  staging  Failed     cs-6e019    Rotate DB credentials  2d
```

#### `admiral env describe shop/prod` — the operator view

`env describe` is the command an operator reaches for to answer "what is
configured here, and is it healthy?" without opening the UI. It is
deliberately the longest view in the CLI. It renders, in order: identity, the
agent and whether it is still reporting, the health verdict and *why*, each
component in full (module, source, resolved ref, version, last run, health,
message, metrics), variables, the last run, and events.

```
Name:          prod
Application:   shop
ID:            7c2d1e90-4f8a-4b3c-8e2d-0a9b8c7d6e5f
Description:   Production, us-east-1
Labels:        tier=1
Agent:         gke-prod (Online, last seen 4s ago, v1.12.0)
Health:        Degraded (api)
Last Run:      run-vnx81r Succeeded 3h ago
URL:           https://app.admiral.io/apps/shop/envs/prod

Conditions:
  Type            Status  Since  Reason           Message
  ----            ------  -----  ------           -------
  AgentReporting  True    41d    Heartbeat        last heartbeat 4s ago
  Reconciled      True    3h     RunSucceeded     run-vnx81r applied cs-7f2a1
  Healthy         False   2d     ComponentFailed  api: 1/3 replicas available
  Drifted         False   3h     NoDrift          last drift check 10m ago

Components:
  network:
    Kind:        Infrastructure
    Module:      terraform/vpc 1.4.0  (source git/platform-modules @ v1.4.0 = 9f2c1e7)
    Depends On:  <none>
    Status:      Succeeded (run-vnx81r, 3h ago)
    Health:      Healthy
    Message:     <none>
    Outputs:     vpc_id = vpc-0a1b2c3d
                 subnet_ids = [subnet-1, subnet-2, subnet-3]
    Resources:   12 managed, 0 drifted (checked 10m ago)

  api:
    Kind:        Workload
    Module:      helm/api 2.1.3  (source helm/charts-prod @ 2.1.3)
    Depends On:  network
    Status:      Succeeded (run-vnx81r, 3h ago)
    Health:      Degraded
    Message:     Deployment api: 1/3 replicas available; pod api-7d9f-x2k1 CrashLoopBackOff
    Values:      replicas = 3
                 image.tag = 1.4.0
    Metrics:     cpu 1.20 cores   mem 512MiB / 1GiB (50.0%)   restarts 7   reported 12s ago

  worker:
    Kind:        Workload
    Module:      helm/worker 2.1.3  (source helm/charts-prod @ 2.1.3)
    Depends On:  network, api
    Status:      Succeeded (run-vnx81r, 3h ago)
    Health:      Healthy
    Message:     <none>
    Values:      replicas = 2
    Metrics:     cpu 0.35 cores   mem 180MiB / 512MiB (35.2%)   restarts 0   reported 12s ago

Variables:
  Key       Type    Sensitive  Value
  ---       ----    ---------  -----
  DB_URL    string  true       <redacted>
  REPLICAS  int     false      3
  REGION    string  false      us-east-1

Last Run:
  ID:          run-vnx81r
  Status:      Succeeded
  Change Set:  cs-7f2a1 (Bump api to 1.4.0)
  Finished:    Wed, 01 Jul 2026 18:16:02 -0400
  Duration:    2m13s

Events:
  Age  Type     Reason           Message
  ---  ----     ------           -------
  4s   Normal   Heartbeat        agent gke-prod reported 3 components
  2d   Warning  ComponentFailed  api: pod api-7d9f-x2k1 CrashLoopBackOff
  3h   Normal   RunSucceeded     run-vnx81r applied 2 components
  41d  Normal   Created          environment created by martin@admiral.io

To see why api is degraded, run: admiral run logs run-vnx81r --component api
```

Rules specific to this view:

- **`Health:` names the culprit.** `Degraded (api)`, `Progressing (api, worker)`,
  `Unknown (agent stale)`. The word alone is never enough. (argocd's worst-of
  rule plus gh's "say what failed")
- **`Conditions:` is the kubectl conditions table** (`Type Status Since Reason
  Message`), fixed set: `AgentReporting`, `Reconciled`, `Healthy`, `Drifted`.
  A `False` row is the reason the env is not green; an operator reads the
  table top to bottom and stops at the first `False`.
- **Each component is a nested key/value block**, not a table row, because the
  operator needs the module, its source, the *resolved* ref (`@ v1.4.0 =
  9f2c1e7`), the run that produced the current state, the health message from
  the agent, and outputs or values. `Module:` is `<type>/<name> <version>`
  followed by the source and resolved revision in parentheses. `Status:` is the
  run status plus which run and when. `Message:` is the agent's verbatim health
  detail or `<none>`.
- **`Metrics:` is one line per component**, present only when the agent has
  reported them, ending in `reported <age> ago` so staleness is visible. Units
  follow §2.12. When the agent is stale the line reads
  `Metrics:     <stale> (last reported 12m ago)`.
- Sections that are empty are still printed with `<none>` so the eye learns
  where to look. `--show-values` / `--show-outputs` expand truncated maps;
  `--component api` restricts the view to one component with full detail.

#### When the agent stops reporting

Agent presence is a first-class status with thresholds the help text states:

| Agent status | Meaning |
|---|---|
| `Online` | heartbeat within the last 60s |
| `Stale` | no heartbeat for 60s–10m; last-known component state is kept but flagged |
| `Offline` | no heartbeat for 10m+, or the agent deregistered |

Effects, everywhere the agent appears:

```
Agent:         gke-prod (Stale, last seen 4m12s ago, v1.12.0)
Health:        Unknown (agent stale)

Conditions:
  Type            Status  Since   Reason         Message
  ----            ------  -----   ------         -------
  AgentReporting  False   4m      HeartbeatLost  last heartbeat 4m12s ago; state below is from then
  Reconciled      True    3h      RunSucceeded   run-vnx81r applied cs-7f2a1
  Healthy         Unknown 4m      AgentStale     component health not refreshed since 4m12s ago
```

- Component `Health:` becomes `Unknown (agent stale)`; the last-known value is
  shown after it in parentheses: `Unknown (agent stale; was Healthy)`.
- `env list` and `status` show `HEALTH Unknown` and, on a TTY, dim the row.
- `agent list` shows `STATUS Stale` with `LAST-SEEN 4m12s`; `agent describe`
  adds a `Last Heartbeat:` line and an `Events:` entry `HeartbeatLost`.
- Runs targeting a stale or offline agent are accepted by the server and sit in
  `Queued`; `run apply` prints `» Waiting for agent gke-prod (Stale, last seen
  4m12s ago)...` every 10s, and `--wait-timeout` applies.
- Metrics lines are marked `<stale>` rather than dropped, so an operator can
  still see the last numbers.

#### `admiral env diff shop/prod staging` — comparing environments

Change sets move between environments with `changeset copy <id> --env
staging` (already exists). To *see* how two environments differ before doing
that, `env diff` prints a side-by-side of components and variables, in the
same `~/+/-` grammar as a plan, read as "what would have to change to make
the right side match the left":

```
$ admiral env diff shop/prod staging
Components (prod -> staging):
  ~ api        version  2.1.3 -> 2.1.2
  ~ api        values.replicas  3 -> 1
  - cache      helm/redis 7.2.0   (only in prod)
    network    terraform/vpc 1.4.0   (same)
    worker     helm/worker 2.1.3     (same)

Variables (prod -> staging):
  ~ REPLICAS   3 -> 1
  ~ REGION     us-east-1 -> us-east-2
  + DEBUG      <none> -> true       (only in staging)
    DB_URL     <redacted>            (same)

2 components differ, 1 only in prod; 3 variables differ.
```

Unchanged rows are printed unprefixed and dimmed so the shape of the whole
environment stays visible; `--changed-only` hides them. Exit code follows
`--detailed-exitcode` (0 same, 2 differences). Two positionals are allowed
here for the same reason `cp src dst` is: the pair is one unit and the order
carries meaning. (terraform plan grammar, argocd `app diff`, clig.dev's `cp`
exception)

#### `admiral changeset describe cs-7f2a1`

```
ID:           cs-7f2a1
Title:        Bump api to 1.4.0
Description:  Picks up the retry fix from #412
Application:  shop
Environment:  prod
Status:       Open
Created:      Wed, 01 Jul 2026 18:02:11 -0400
Created By:   martin@admiral.io
URL:          https://app.admiral.io/apps/shop/envs/prod/changesets/cs-7f2a1

Entries:
  Slug     Action   Module      Version          Depends On
  ----     ------   ------      -------          ----------
  api      Update   helm/api    2.1.2 -> 2.1.3   network
  cache    Add      helm/redis  7.2.0            <none>
  legacy   Destroy  helm/legacy 0.9.1            <none>

Variables:
  Key       Action  Type    Sensitive  Value
  ---       ------  ----    ---------  -----
  REPLICAS  Set     int     false      2 -> 3
  OLD_FLAG  Remove  string  false      <none>

Runs:
  ID          Status     Age
  --          ------     ---
  run-vnx81r  Succeeded  3h

To plan this change set, run: admiral changeset plan cs-7f2a1
```

#### `admiral run describe run-vnx81r`

```
ID:            run-vnx81rv3pp4c
Application:   shop
Environment:   prod
Status:        Succeeded
Change Set:    cs-7f2a1 (Bump api to 1.4.0)
Message:       promote 1.4.0 after canary
Triggered By:  martin@admiral.io
Runner:        gke-prod
Started:       Wed, 01 Jul 2026 18:13:49 -0400
Finished:      Wed, 01 Jul 2026 18:16:02 -0400
Duration:      2m13s
URL:           https://app.admiral.io/apps/shop/envs/prod/runs/run-vnx81rv3pp4c

Revisions:
  Component  Kind            Phase  Status     Changes      Error
  ---------  ----            -----  ------     -------      -----
  network    Infrastructure  apply  Succeeded  ~1           <none>
  api        Workload        apply  Succeeded  +1 ~2        <none>
  worker     Workload        plan   Deferred   <none>       <none>

Events:
  Age  Type    Reason          Message
  ---  ----    ------          -------
  3h   Normal  Queued          assigned to gke-prod
  3h   Normal  PlanSucceeded   network: 1 to change; api: 1 to add, 2 to change
  3h   Normal  ApplySucceeded  2 components applied in 2m13s

To see the transcript, run: admiral run logs run-vnx81r
```

The `Changes` column uses terraform's symbols and omits zeros: `+1 ~2 -0`
becomes `+1 ~2`; no changes is `<none>`.

#### `admiral agent describe gke-prod`

```
Name:         gke-prod
ID:           ...
Description:  GKE production cluster, us-east-1
Labels:       cluster=gke-prod
Status:       Online
Version:      1.12.0
Last Seen:    Wed, 01 Jul 2026 21:44:10 -0400
Created:      ...
URL:          ...

Tokens:
  Name      Expires                          Age
  ----      -------                          ---
  ci        Thu, 01 Jan 2027 00:00:00 -0500  41d
  martin    <none>                           40d

Recent Jobs:
  ID       Type   Run         Status     Age
  --       ----   ---         ------     ---
  job-1a2  apply  run-vnx81r  Succeeded  3h
```

### 2.6 `status` (dashboard)

`admiral status [app/env] [-w] [--interval 5s]` is the fly-style
dashboard for one environment: a `Key = value` block, then the components
table, then in-flight runs. It is `env describe` minus the history, refreshed
in place with `-w`. Piped or in `-o json` it prints one snapshot and exits.
(fly `status --watch`, gh `run watch`)

```
$ admiral status shop/prod
App          shop
Environment  prod
Health       Degraded
Runner       gke-prod (Online)
Last run     run-vnx81r Succeeded 3h ago
URL          https://app.admiral.io/apps/shop/envs/prod

COMPONENT   KIND            VERSION   STATUS     HEALTH       UPDATED
network     Infrastructure  1.4.0     Succeeded  Healthy      3h
api         Workload        2.1.3     Failed     Degraded     2d
worker      Workload        2.1.3     Succeeded  Healthy      3h

In progress: none
```

### 2.7 Mutations

- A mutation that **returns the resource** (`create`, `update`, `rollback`)
  echoes its narrow row on stdout (the same `printXRow`), or the full object
  under `-o json|yaml`, or the bare name under `-o name`.
- A mutation that **returns nothing** (`delete`, `cancel`, `discard`, `token
  revoke`) prints one confirmation line to **stderr** in the kubectl shape
  `<type> "<name>" <verbed>`: `environment "staging" deleted`, `run run-8k2m1q
  canceled`. stdout stays empty so `$(…)` and pipelines stay clean.
  (kubectl, gcloud SilentCommand)
- **Suggest the next step** after a mutation that starts a workflow, on stderr:

  ```
  $ admiral changeset plan cs-7f2a1
  Planned cs-7f2a1 as run-p9x2ka.
  To apply exactly this plan, run: admiral run apply run-p9x2ka
  ```
  (terraform `-out` hand-off, clig.dev "suggest commands the user should run")

### 2.8 Plan output

`changeset plan`, `changeset diff` and `run plan` render the terraform diff
grammar, on stdout:

```
Admiral will perform the following actions in shop/prod:

  # api will be updated in-place
  ~ component "api" {
      ~ version = "2.1.2" -> "2.1.3"
      ~ values.replicas = 2 -> 3
    }

  # cache will be created
  + component "cache" {
      + module  = "helm/redis"
      + version = "7.2.0"
    }

  # legacy will be destroyed
  - component "legacy" {
      - module  = "helm/legacy"
      - version = "0.9.1"
    }

Plan: 1 to add, 1 to change, 1 to destroy.
```

- Legend symbols: `+` add, `~` update in place, `-` destroy, `-/+` replace,
  `<=` import. Print the legend only when more than one symbol is used.
- Unknown values print `(known after apply)`; sensitive values `(sensitive
  value)`.
- The summary line is fixed-shape and greppable: `Plan: N to add, N to change,
  N to destroy.` No changes prints `No changes. shop/prod matches cs-7f2a1.`
- `--detailed-exitcode` makes the exit status **0 = no changes, 1 = error,
  2 = changes present** for CI gates. (terraform)

### 2.9 Long-running operations

`run apply`, `changeset apply`, `run rollback`, `run cancel --wait` and
`env delete` are **synchronous by default** and print per-component progress
to **stderr** in terraform's address-prefixed form, with a heartbeat every 10s:

```
$ admiral run apply run-p9x2ka
Applying run-p9x2ka to shop/prod (3 components)
Watch this run at https://app.admiral.io/apps/shop/envs/prod/runs/run-p9x2ka

network: Applying...
network: Apply complete after 12s
api: Applying...
api: Still applying... [10s elapsed]
api: Still applying... [20s elapsed]
api: Apply complete after 24s
cache: Applying...
cache: Apply complete after 8s

Run run-p9x2ka succeeded in 44s: 1 added, 1 changed, 1 destroyed.
```

- The final line is fixed-shape: `Run <id> <status> in <duration>: N added, N
  changed, N destroyed.` and goes to **stdout**, followed by the run's narrow
  row if `-o` is table, or the object under `-o json`.
- **The outcome of the watched run is the exit code**: a run that ends
  `Failed`, `PartiallyFailed` or `Canceled` exits 1 with `Error: run run-p9x2ka
  failed: api: helm upgrade timed out` on stderr. (kubectl `rollout status`,
  render, railway; never heroku's opt-in `-x`)
- `--no-wait` returns immediately after the server accepts the run and prints
  only the run ID on stdout (`vercel deploy` prints only the URL). Pair it with
  `admiral run wait <id>` later.
- **`admiral run wait <id> [--for succeeded|finished] [--wait-timeout 30m]
  [--interval 5s]`** polls and exits 0 when the condition is met, 1 when the
  run fails, and **124** on timeout. The flag help states the contract verbatim
  ("polls every 5s; exits 1 if the run fails, 124 if --wait-timeout elapses").
  (aws `wait`, az `wait`, kubectl `wait --for`)
- Long-running commands are exempt from the RPC deadline (`--timeout`) for the
  stream itself; the deadline still applies to each unary call.
  `--wait-timeout` is the only cap on a wait.
- **Ctrl-C cancels the wait, not the run.** The first ^C prints `Interrupted;
  run run-p9x2ka continues on the server. Cancel it with: admiral run cancel
  run-p9x2ka` to stderr and exits 130. (clig.dev signals; §7)
- Spinners appear only when stderr is a TTY. Piped, the same lines print
  without animation. (clig.dev "Christmas trees in CI")

### 2.10 Logs

`admiral run logs <id>` (and the root `admiral logs` alias, which resolves the
latest run in the scoped env) streams engine transcripts. Flags are docker's,
verbatim:

| Flag | Meaning |
|---|---|
| `-f, --follow` | keep streaming while the run is in progress. **Default on while the run is not finished, off once it is**; help says so. (fly streams by default; railway/heroku do not — Admiral keys it on run state) |
| `-n, --tail N` | last N lines per component (default all; 50 when following more than one component) |
| `--since 10m \| RFC3339` | relative Go duration or absolute |
| `--until` | same forms; either `--since` or `--until` implies `--follow=false` (railway) |
| `-t, --timestamps` | prefix each line with RFC3339 UTC |
| `--component NAME` | one component; default all, alphabetical |
| `--phase plan\|apply` | default: latest phase per component |
| `--prefix` / `--no-prefix` | `component/phase │ ` prefix; on by default when more than one component is rendered (kubectl `--prefix`, compose `--no-log-prefix`) |
| `--failed` | only components whose phase failed (gh `--log-failed`) |

Line shape with prefix: `api/apply   │ Waiting for rollout to finish: 1 of 3
updated replicas are available`. Prefix colour is per component on a TTY.
Components with no transcript are reported on stderr as `skipped worker: no
apply transcript (status: Deferred)` — never as an error unless nothing at
all rendered.

### 2.11 History and rollback

- `admiral run list` **is** the history; there is no separate `history` verb.
  Newest first, `--limit 20` by default.
- **Rollback is always a new forward run.** Admiral never rewinds state; `run
  rollback <run-id>` creates a new run whose desired configuration (component
  versions and variables) is the one that `<run-id>` resolved to, and applies
  it forward. The output makes that explicit — the old run is the *source*,
  the new run is the *result*:

  ```
  $ admiral run rollback run-p9x2ka --env shop/prod
  » Re-apply the configuration from run-p9x2ka (Succeeded, 3h ago) to shop/prod as a new run? [y/N] y
  » Rolling shop/prod forward to the configuration of run-p9x2ka... done, run-r8t1zz
  » Rollback re-applies component versions and variables; it does not restore data or state files.
  » To reverse this, run: admiral run rollback run-q3v7mm
  ID           STATUS      CHANGE-SET   TITLE                                 AGE
  run-r8t1zz   Succeeded   <none>       Re-apply configuration of run-p9x2ka  12s
  ```

  Omitting the target means "the last run that succeeded before the current
  one". The stderr caveat line states what rollback does *not* touch, and the
  reversal hint names the run to go back to. (helm `rollback` creates a new
  revision; heroku `releases:rollback` creates v43 "Rollback to v41" and warns
  what it does not affect)
- **Open question (product):** whether rollback should exist as a verb at all,
  or whether the same outcome should be expressed as `changeset copy --from-run
  run-p9x2ka` followed by the normal plan/apply review. The guide keeps the
  verb because every deployment CLI surveyed has one and operators reach for it
  under pressure, but its title, caveat line and confirmation must make the
  "new forward run" semantics unmistakable. If the platform cannot guarantee a
  clean re-apply of an older module version (e.g. destructive migrations), the
  verb should refuse with `Error: run-p9x2ka cannot be re-applied: module
  helm/api 2.1.2 is marked non-reversible` rather than proceed.

### 2.12 Metrics

When metrics land, `admiral env top app/env` (and `run top`) follows
`docker stats`: one row per component, units inside the values, live redraw
only on a TTY, one snapshot when piped or with `--no-stream`.

```
COMPONENT   CPU      MEM              MEM %   REPLICAS   RESTARTS
api         1.20     512MiB / 1GiB    50.0%   3/3        0
worker      0.35     180MiB / 512MiB  35.2%   2/2        1
```

CPU is cores (`1.20`), memory uses binary units (`512MiB`), percentages have
one decimal. The help text says it is a snapshot, not a monitoring system.
(kubectl `top`, docker `stats`)

---

## 3. Output: machine mode

### 3.1 `-o json` and `-o yaml`

- `-o json|yaml` emits the **full API object** exactly as protojson renders it
  (camelCase keys, enum names as strings, RFC3339 timestamps, unpopulated
  fields present). The CLI never inserts display strings, relative ages or
  truncated text into JSON. (kubectl; docker's `RunningFor: "5 days ago"` is
  the anti-pattern)
- **A list is a bare JSON array; a single resource is a bare object.** No
  `{"applications": [...]}` wrapper, no CLI-invented envelope. (gh, gcloud, az,
  heroku, fly)
- **Pagination is transparent.** `list` follows page tokens automatically up
  to `--limit N` (default 100; `--limit 0` means all). `--page-size` and
  `--page-token` remain for callers that want manual paging; when they are
  used and more pages exist, `next page token: <t>` goes to **stderr** in every
  output mode. JSON output is therefore always a plain array. (kubectl
  `--chunk-size`, gh `--limit`)
- Pretty-printed with two-space indent when stdout is a TTY, compact
  single-line when piped. (gh)
- **JSON is a contract.** Fields are only ever added; a rename or removal
  requires a release note and a deprecation cycle. This applies to the
  server's protos too, so the SDK team owns it jointly. (gh maintainers,
  Heroku "don't modify existing output after GA")

### 3.2 `-o name`

Prints one bare identifier per line (name for CRUD resources, ID for runs,
change sets and tokens), nothing else. This is the xargs contract:

```
admiral env list --app shop -o name | xargs -I{} admiral env delete shop/{} -f
```

`-q/--quiet` on `list` is an alias for `-o name`. (kubectl `-o name`, docker
`-q`, fly `-q`)

### 3.3 `--jq`

Every command that supports `-o json` also accepts `--jq EXPR`, evaluated with
an embedded gojq so nothing needs installing. `--jq` without `-o json` sets it
implicitly. (gh, az `--query`) `--template` is not offered; `-o json | …` covers
it.

### 3.4 Streaming: JSON Lines

Anything that streams (`run logs -f`, `run apply`, `run wait`, `status -w`,
`env events -w`) under `-o json` emits **one JSON object per line**, flushed per
event, never a pretty-printed array. The envelope is stable:

```json
{"time":"2026-07-01T22:14:01Z","type":"revision.status","level":"info","message":"api: Apply complete after 24s","run":"run-p9x2ka","component":"api","phase":"apply","data":{"status":"REVISION_STATUS_SUCCEEDED","elapsedSeconds":24}}
```

- `time`, `type`, `level`, `message` are always present; `message` is the exact
  line a human would have seen. Consumers that meet an unknown `type` should
  render `message`.
- The first event of a stream is `{"type":"stream.start","version":"1"}`; the
  last is `stream.end` carrying the final status and the process exit code.
- Log lines are `{"type":"log","component":"api","phase":"apply","line":"…"}`.
(terraform `-json` UI, jsonlines.org, railway `up --json`)

### 3.5 `--dry-run`

Every mutating command accepts `--dry-run`, which validates locally, prints the
request it would send (method, resource, body with secrets masked) to stdout as
JSON, and exits 0 without calling the server. (stripe, kubectl
`--dry-run=client`) `run apply --dry-run` is spelled `changeset plan`.

---

## 4. TTY, colour and non-interactive mode

| Signal | Effect |
|---|---|
| stdout not a TTY | no ANSI on stdout, no truncation, no hyperlinks, JSON compact, live views print one snapshot |
| stderr not a TTY | no ANSI on stderr, no spinner (progress lines still print), no `next page token` hint suppression |
| stdin not a TTY | prompts are errors (§5) |
| `NO_COLOR` non-empty | no ANSI anywhere |
| `CLICOLOR_FORCE` non-empty | ANSI even when piped |
| `TERM=dumb` | no ANSI |
| `--color never\|always\|auto` | overrides all of the above (default `auto`) |
| `CI`, `ADMIRAL_NO_INPUT`, or an AI-agent marker (`CLAUDECODE`, `CODEX_SANDBOX`, `COPILOT_CLI`, `AI_AGENT`, …) | non-interactive mode: no prompts, no spinners, full help on usage errors (gh `internal/agents`, vercel) |
| `ADMIRAL_FORCE_TTY=<cols>` | render as a terminal of that width when piped, for tests (gh) |

- **Colour is by semantic role, not mood**: app names one colour, env another,
  IDs dim, status words green/yellow/red/dim only. Yellow and red are reserved
  for warnings, errors and failed states. (Heroku, Primer)
- **Colour is encouraged where it aids scanning** — status words, the
  culprit in `Health: Degraded (api)`, section headers in `describe`, the
  `Error:` label, per-component log prefixes — and implemented through one
  small style table in `internal/output` backed by a library (lipgloss or
  equivalent) that degrades to plain text when `iostreams.ColorEnabled()` is
  false. Commands never emit escape codes directly. Bold and dim are colour
  for this purpose and follow the same switch.
- Colour is applied to **both** stdout and stderr by the same decision;
  terraform's coloured stderr under `NO_COLOR` is the cautionary tale.
- Never page output. `ADMIRAL_PAGER`/`PAGER` is honoured only by `describe` and
  `logs` when stdout is a TTY and the output exceeds the screen. (aws v2's
  default pager is the most complained-about change in its history)
- Table width: fit the terminal; assume 80 columns when stdout is not a TTY and
  no `COLUMNS` is set. (gcloud)

---

## 5. Prompts and confirmation

- **Prompt only when stdin and stderr are TTYs.** Prompts are written to
  stderr. (gcloud)
- **Every prompt has a flag that answers it**, and the non-interactive error
  names that flag:

  ```
  Error: --force required when not running interactively
  ```
  (gh, fly)
- **Two escape hatches, not one.** `--force/-f` skips the *confirmation* on a
  destructive verb. `--no-input` (env `ADMIRAL_NO_INPUT=1`) disables *all*
  prompting, including value prompts and login; under `--no-input` a
  destructive command without `--force` **fails**, it does not proceed.
  `-y/--yes` is not used. (gcloud `--quiet` semantics, clig.dev `--no-input`)
- **Tiered confirmation** (clig.dev):

  | Tier | Verbs | Prompt |
  |---|---|---|
  | moderate | `env delete`, `run cancel`, `changeset discard`, `credential delete`, `token revoke`, `run rollback` | `Delete environment shop/staging? [y/N]` |
  | severe | `app delete` (cascades), `changeset discard` with pending entries, `env delete` with live components | red stderr line `Deleting shop is not reversible and removes 2 environments, 5 components and 41 runs.` then `Type shop to confirm:` |

  `--force` bypasses both tiers, but is **ignored with a warning when the target
  was not named explicitly** (`--force` on a resolved-from-context target still
  prompts). (gh `repo delete`)
- Declining prints `canceled` to stderr and exits 1. (argocd, gcloud `Aborted
  by user.`)
- **Apply approval** for `run apply` on an env labelled production uses
  terraform's wording: `Only 'yes' will be accepted to approve.` With a
  reviewed plan (`run apply <run-id>` from `changeset plan`) no prompt is
  shown, because the plan was the review. (terraform saved-plan rule)

---

## 6. Errors and exit codes

### 6.1 Message shape

```
Error: environment "prod" not found in application "shop"
Run 'admiral env list --app shop' to see environments.
```

- Line 1: `Error: ` prefix, then a **lowercase, one-line message with no
  trailing period**, on stderr. (cobra/helm/fly/terraform use `Error:`; GNU
  supplies the lowercase/no-period rule)
- Line 2 (optional, strongly encouraged): a remedy that names the exact command
  or flag, placed last because the eye lands there. Patterns:
  `Run 'admiral auth login' to sign in.` / `Pass --app or give the
  environment as app/env.` /
  `See 'admiral run logs --help' for usage.` (clig.dev, railway, stripe)
- **Not-found says what was looked up and where.** Never `not found` alone
  (helm's `Error: release: not found` is the anti-pattern).
- **Ambiguity lists the candidates** with IDs and asks for `--id`.
- gRPC framing (`rpc error: code = … desc =`) is stripped; the server's
  description is shown verbatim; `Unavailable` becomes `could not reach the
  Admiral API at <host>: <reason>`; `DeadlineExceeded` names `--timeout`.
- Unknown commands and flags keep cobra's `Did you mean this?` and append
  `Run 'admiral <path> --help' for usage.`; usage errors never dump the full
  usage block unless an AI agent is detected (§4). Go internals
  (`strconv.Atoi`, stack traces) never reach the user without `--verbose`.
- Domain failures inside a wait use the same shape: `Error: run run-p9x2ka
  failed: api: helm upgrade timed out` then `Run 'admiral run logs run-p9x2ka
  --failed' to see the transcript.`

### 6.2 Exit codes

| Code | Meaning | Source |
|---|---|---|
| 0 | success (including "no results" and "no changes") | all |
| 1 | runtime failure: server error, not found, failed run under `--wait`, declined prompt | all |
| 2 | usage error: unknown command/flag, missing positional, invalid enum value, `--jq` without JSON | bash, argparse, gcloud, az, heroku |
| 2 | `plan --detailed-exitcode`: changes present | terraform |
| 4 | authentication required or expired | gh |
| 124 | `--wait-timeout` elapsed | GNU `timeout` |
| 130 | interrupted by Ctrl-C | bash 128+SIGINT |

Documented in `admiral help exit-codes`. No other codes without adding them to
that page. (gh `help exit-codes`)

---

## 7. Signals and timeouts

- `SIGINT`/`SIGTERM` cancel the root context; in-flight RPCs are cancelled
  server-side; the process prints `Interrupted.` (or the run-continues message
  from §2.9) and exits 130. A second signal kills immediately.
- Every unary RPC carries the `--timeout` / `ADMIRAL_TIMEOUT` deadline (default
  30s). Streams are exempt from it but honour `--wait-timeout`.
- The browser login flow times out after 5 minutes with an error that suggests
  `auth login --with-token`.
- Print *something* within 100ms: a spinner on a TTY, or the first progress
  line. (clig.dev)

---

## 8. Help text

Structure (cobra, customised): `Usage`, `Aliases`, `Examples`, `Available
Commands` grouped, `Flags`, `Global Flags`, footer.

- **`Use` line grammar**: `env get <name> [flags]`, placeholders in
  `<dash-case>`, `[optional]`, `{a | b}` required choice, `...` repeatable.
  Never two positionals of different kinds. (cobra, gh syntax key)
- **`Short`**: imperative verb phrase, Capitalised, no trailing period, ≤ 60
  characters: `List environments for an application`. Group nouns read `Manage
  environments`. (kubectl, gh, docker; deliberate choice against Heroku's
  lowercase)
- **Flag descriptions**: lowercase sentence fragment, no period, enum values
  and defaults inline: `--app string   application name or ID`.
  Units live in the description, never the name. (Heroku, az)
- **Every leaf command has an `Example` block** of 2–4 real invocations, each
  preceded by a `# comment`, no `$` prefix, most common first. (clig.dev "lead
  with examples", kubectl)
- **Long-running verbs carry an "Automation notes:" paragraph** in `Long`
  describing the poll loop, JSON events and exit codes, for scripts and
  agents. (railway, stripe `[Agent guidance]`)
- **Root help groups commands** with `GroupID`:

  ```
  Resources:    app env changeset run
  Catalog:      module source credential catalog
  Agents:       agent
  Daily loop:   status logs open
  Account:      auth whoami config
  Other:        completion version help
  ```
- `admiral <noun>` with no verb prints that noun's help and exits 0. A leaf
  command missing its positional prints the one-line error plus `Run 'admiral
  env get --help' for usage.` and exits 2.
- Help topics are first-class: `admiral help environment` (every `ADMIRAL_*`
  and general-purpose env var), `admiral help exit-codes`, `admiral help
  output` (formats, `--jq`, JSON Lines), `admiral help reference` (whole tree
  as markdown, for agents). (gh, stripe `--map`)
- Root `Long` ends with the docs URL and the issue tracker. (GNU, clig.dev)

---

## 9. Configuration and environment

- **Precedence, exactly**: flag > `ADMIRAL_*` env > config file (current
  context) > built-in default. No project-level `.env` layer. (clig.dev,
  gcloud, gh)
- **Env var names are mechanical**: `ADMIRAL_` + flag name uppercased with
  `-`→`_`: `ADMIRAL_SERVER`, `ADMIRAL_TIMEOUT`, `ADMIRAL_CONFIG_DIR`,
  `ADMIRAL_OUTPUT`, `ADMIRAL_NO_INPUT`, `ADMIRAL_API_KEY`, `ADMIRAL_DEBUG`.
  Uppercase `[A-Z0-9_]` only, single-line values. Scope (`--app`, `--env`)
  is the one flag family with no env or config layer; see §1.2. Also honoured: `NO_COLOR`, `CLICOLOR_FORCE`, `TERM`, `PAGER`,
  `BROWSER` (a command, not a path), `HTTP(S)_PROXY`/`NO_PROXY`, `COLUMNS`.
- `ADMIRAL_API_KEY` in the environment is the documented CI path; a `--token`
  flag never exists because it leaks into `ps` and shell history. `auth login
  --with-token` reads stdin. (clig.dev on secrets, gh `GH_TOKEN`)
- **Files follow XDG**: config and credentials in `$XDG_CONFIG_HOME/admiral`
  (`~/.config/admiral`), overridable by `--config-dir`/`ADMIRAL_CONFIG_DIR`;
  caches in `$XDG_CACHE_HOME/admiral`; relative XDG values are ignored.
  Preferences and credentials never share a file. (XDG spec, kubectl kuberc)
- `admiral config list` shows **each value with its source** (`flag`, `env
  ADMIRAL_SERVER`, `config`, `default`). (az `config get`, aws `configure list`)
- `admiral auth status` uses the §2.5 describe layout (`Authenticated:`,
  `Method:`, `Stored In:`, `Server:`, then for sessions `Account:`,
  `Auth Server:`, `Expires:`, `Scopes:`), never prints a token, and exits 4
  when broken unless `-o json`. `whoami -o json` has a documented stable
  schema. (gh, stripe)

---

## 10. Filtering, sorting, pagination

Uniform on every `list`:

| Flag | Meaning |
|---|---|
| `-l, --selector k=v,k!=v,k in (a,b),!k` | label selector, server-side (kubectl grammar) |
| `--status Failed,Canceled` | typed enum filter, values listed in help (compose `--status`) |
| `--limit N` | stop after N items across pages (default 100; 0 = all) |
| `--sort-by COLUMN[,~COLUMN]` | column name, `~` for descending; never a JSONPath (gcloud) |
| `--page-size N`, `--page-token T` | manual paging; token echoed on stderr |
| `--all` | lift a default filter (e.g. include superseded runs) |

The help text states the application order: `--selector and --status are
applied by the server; --sort-by and --limit by the CLI, in that order.`

---

## 11. Verbs that open the browser

`-w/--web` on every `get`, `describe`, `list` and `status`, and a root
`admiral open [app/env]`, print `Opening <url> in your browser.` to stderr
and open it via `BROWSER` or the OS default. Every `describe` and every
`create` prints the resource `URL:`. (gh, fly, heroku `open`)

---

## 12. Testing the contract

- Golden tests render every `printXRow`, every `describe`, and every error
  through `ADMIRAL_FORCE_TTY=100` and through a pipe, and compare both.
- Every `-o json` list output is asserted to be a bare array; every single
  `get` a bare object; every stream a sequence of newline-terminated objects
  each carrying `time`, `type`, `message`.
- Every prompt has a test that runs it with a closed stdin and asserts the
  `--force required when not running interactively` error and exit 2.
- Every list has an empty-result test asserting empty stdout, the stderr
  message, and exit 0.
- Every `complete.X` has a test that it filters by prefix, follows pages, and
  returns `ShellCompDirectiveError` with no candidates when the client
  cannot be built or the list call fails.

---

## 13. Checklist for a new command

- [ ] Noun singular, verb from the fixed set, aliases declared, ≤ 3 tokens
      before the name.
- [ ] Positional is the child name-or-ID, or its path (`app/env`); parents
      are `--app`/`--env`; path or flag, never both; no env or config
      fallback.
- [ ] Every name-or-ID positional has `ValidArgsFunction: complete.First(…)`;
      every parent flag and enum flag has a flag completion (§1.5).
- [ ] `Short` capitalised imperative without period; `Example` block with
      `# comment` lines; enum values and defaults in flag help.
- [ ] Table: UPPER-HYPHEN headers, NAME first, AGE last, `<none>` placeholders,
      shared `printXRow`; `-o wide` columns defined; `-o json` full object;
      `-o name` bare identifier.
- [ ] Empty list → stderr message, exit 0, `[]` in JSON.
- [ ] Status words CamelCase; ages via `HumanDuration`.
- [ ] Mutations: row echo or stderr confirmation; next-step hint; `--dry-run`.
- [ ] Destructive: `util.Confirm`/`util.ConfirmName`, `--force`, non-TTY error.
- [ ] Long-running: sync by default, stderr progress, `--no-wait`,
      `--wait-timeout`, outcome = exit code, JSON Lines under `-o json`,
      "Automation notes" in help.
- [ ] Errors: `Error: lowercase message` + remedy line; not-found names the
      scope; exit code from §6.2.
- [ ] Colour by role only; nothing animated when not a TTY.

---

## 14. Changes from current behaviour

These are the places where the guide and the code disagree today. Each is a
deliberate decision, with the alternative it rejects.

| # | Today | Guide | Why |
|---|---|---|---|
| 1 | `-` for empty cells (`output.EmptyMarker`) | `<none>` / `<unknown>` | a description can legitimately be `-`; kubectl's brackets are unambiguous and still awk-safe |
| 2 | `UPPER_SNAKE` status words (`PARTIALLY_FAILED`) | CamelCase (`PartiallyFailed`) | UPPER values under UPPER headers are shouty; kubectl/argocd/gh all use CamelCase words |
| 3 | `AGE` is single-unit (`3h`) | kubectl `HumanDuration` (`5m12s`, `5d3h`) | more precision where it matters (<10m, <8d), same shape people already read |
| 4 | `-o json` list = full `ListXResponse` (`{"applications":[…],"nextPageToken":…}`) | bare array; token on stderr; `--limit` auto-pages | resource-specific wrapper keys are aws's anti-pattern; 5 of the surveyed CLIs use bare arrays; agents get all pages via `--limit` |
| 5 | `--id`, `--app-id`, `--env-id` flags | positional/`--app` accept name or ID; `*-id` flags hidden then removed | halves the flag surface; IDs are syntactically distinct from names |
| 6 | header `CHANGE SET` | `CHANGE-SET` | no spaces in headers so `awk` column counting works |
| 7 | delete/cancel print nothing or a stdout line | one stderr line `<type> "<name>" <verbed>` | stdout stays empty for `$(…)`; matches kubectl wording, gcloud stream rule |
| 8 | `--force` is the only prompt control | add `--no-input`/`ADMIRAL_NO_INPUT` and agent/CI detection | scripts need "never prompt" separately from "yes, delete" |
| 9 | `run logs` has no `--follow`, `--tail`, `--since`, `--timestamps` | docker's flag set; follow defaults on for in-progress runs | streaming is the single biggest gap vs. fly/railway/vercel |
| 10 | `run apply` returns when accepted | synchronous with progress; `--no-wait`; `run wait` | every deployment CLI surveyed blocks by default and makes the run's outcome the exit code |
| 11 | `--insecure` has `-i` | no short form | `-i` is conventionally interactive; keep the short set tiny |
| 12 | `--password`, `--token` flags on credential create | stdin/file/prompt only | clig.dev secrets rule; process lists leak flags |
| 13 | no `--jq`, no `-o name` | both | cheapest possible "plain" output; gojq is 1 dependency |
| 14 | errors are `Error: <msg>` only | + remedy line | every good error in the survey names the fixing command |
| 15 | exit codes 0/1/130 | + 2 usage, 4 auth, 124 timeout, `--detailed-exitcode` | scripts branch on usage vs runtime vs auth; document in `help exit-codes` |
| 16 | label semantics differ per resource (COMMAND_TREE #5) | `--label` merges everywhere; `--remove-label`; `--clear-labels` | one table for set/remove/clear |
| 17 | `--clear-credential` is unique (COMMAND_TREE #14) | `--clear-<ref>` for references, `""` for scalars, `--clear-<map>s` for maps | the quartet is the convention, not the exception |
| 18 | `source create terraform --tf-*` vs `helm --chart-name` (COMMAND_TREE #11) | drop type prefixes: `--namespace`, `--module-name`, `--system`, `--chart` | the type is already in the command path |
| 19 | `--message/-m` only on `run plan`/`rollback` (COMMAND_TREE #12) | `-m` on every verb that creates a run: `plan`, `apply`, `rollback`, `cancel` | a run's message is its audit line; helm's `--description` on every mutation |
| 20 | `credential update` carries every type's flags (COMMAND_TREE #13) | `credential update <type> <name>` mirroring `create` | flags that only apply to one type are a smell |
| 21 | `env describe` prints a components table | full per-component blocks, `Conditions:`, agent presence, metrics (§2.5) | the operator question is "what is configured and why isn't it green"; a table row cannot answer it |
| 22 | agent status is a bare word | `Online`/`Stale`/`Offline` with thresholds; env health becomes `Unknown (agent stale)` | a stale agent must never look like a healthy environment |
| 23 | no way to compare environments | `env diff <a> <b>` in plan grammar | `changeset copy` moves changes between envs; operators need to see the gap first |
| 24 | only `auth login --scope` completes; names are typed in full | every name positional and parent flag completes from the server (§1.5); done for `app` and `--app`, remaining resources tracked in COMMAND_TREE.md | the operator's mental model is kubectl's: type a prefix, Tab, move on |
| 25 | `--app`/`--env` default from `ADMIRAL_APP`/`ADMIRAL_ENV` (shipped in v0.1.0) | an environment is addressed as `app/env`; path or flag, never both; the env vars are removed (§1.2) | a scope inherited from the shell is invisible on the line that deletes prod; with the path form, explicit costs one word |

Open questions the guide does not settle:

- Whether `admiral link` (a `.admiral/` file in the repo, railway/vercel style)
  should join path > flag as a third scope source. Recommended: not until
  there is a repo-to-app mapping the server knows about, and never as an
  environment variable (§14 #25).
- Whether `describe -o json` should exist as a composite document. The guide
  says no (kubectl); if agents need it, add child `list` verbs
  (`run revisions`, `env components`, `env vars`) rather than a bespoke shape.
- Whether `agent` should be renamed `runner` to match the server's
  vocabulary; the guide follows whatever the server calls it.
- Whether `run rollback` stays a verb or becomes `changeset copy --from-run`
  (§2.11). Either way it is a new forward run, never a rewind.
- The agent staleness thresholds (60s / 10m) are placeholders until the server
  defines heartbeat intervals; the CLI displays whatever the server reports and
  never computes presence itself.

---

## 15. Worked examples

One session, end to end, showing the rules above in use. Lines marked `»`
are stderr; everything else is stdout. Exit codes are shown where they matter.

### Create

```
$ admiral app create shop --description "Storefront and checkout" --label team=commerce
NAME   DESCRIPTION               LABELS          AGE
shop   Storefront and checkout   team=commerce   0s
» URL: https://app.admiral.io/apps/shop

$ admiral env create shop/prod --runner gke-prod --label tier=1
NAME   APP    HEALTH    DESCRIPTION   LABELS   AGE
prod   shop   Unknown   <none>        tier=1   0s

$ admiral env create prod --app shop -o name
prod
```

The echoed row is the same `printEnvRow` that `list` and `get` use.

### List, get, empty result

```
$ admiral env list --app shop
NAME      APP    HEALTH     DESCRIPTION            LABELS   AGE
prod      shop   Healthy    <none>                 tier=1   41d
staging   shop   Degraded   Pre-prod, us-east-1    tier=2   41d

$ admiral env get prod --app shop -o wide
NAME   APP    HEALTH    DESCRIPTION   LABELS   AGE   ID                                     RUNNER     LAST-RUN     CREATED-BY
prod   shop   Healthy   <none>        tier=1   41d   7c2d1e90-4f8a-4b3c-8e2d-0a9b8c7d6e5f   gke-prod   run-vnx81r   martin@admiral.io

$ admiral run list --app shop --env staging --status Failed
» No runs found in shop/staging.
$ echo $?
0
```

### Machine output

```
$ admiral env list --app shop -o json | jq -r '.[].name'
prod
staging

$ admiral env list --app shop -o name
prod
staging

$ admiral run list --app shop --env prod --limit 1 --jq '.[0].status'
"RUN_STATUS_SUCCEEDED"

$ admiral env list --app shop -o json
[{"id":"7c2d1e90-…","name":"prod","applicationId":"2f1c9a7e-…","labels":{"tier":"1"},"createdAt":"2026-05-22T14:02:11Z",…},{…}]
```

Bare array, protojson field names, enum names untouched. Compact because
stdout is a pipe; on a TTY the same output is indented.

### Propose a change and plan it

```
$ admiral changeset create --app shop --env prod --title "Bump api to 1.4.0"
ID         TITLE               ENV    STATUS   ENTRIES   AGE
cs-7f2a1   Bump api to 1.4.0   prod   Open     0         0s

$ admiral changeset update api --changeset cs-7f2a1 --version 2.1.3
$ admiral changeset add cache --changeset cs-7f2a1 --module helm/redis --version 7.2.0
$ admiral changeset set-var REPLICAS 3 --changeset cs-7f2a1 --type int

$ admiral changeset plan cs-7f2a1 -m "promote 1.4.0 after canary"
Admiral will perform the following actions in shop/prod:

  # api will be updated in-place
  ~ component "api" {
      ~ version         = "2.1.2" -> "2.1.3"
      ~ values.replicas = 2 -> 3
    }

  # cache will be created
  + component "cache" {
      + module  = "helm/redis"
      + version = "7.2.0"
    }

Plan: 1 to add, 1 to change, 0 to destroy.

» Planned cs-7f2a1 as run-p9x2ka.
» To apply exactly this plan, run: admiral run apply run-p9x2ka
```

### Apply

Interactive — progress on stderr, result on stdout:

```
$ admiral run apply run-p9x2ka
» Applying run-p9x2ka to shop/prod (2 components)
» Watch this run at https://app.admiral.io/apps/shop/envs/prod/runs/run-p9x2ka
»
» api: Applying...
» api: Still applying... [10s elapsed]
» api: Still applying... [20s elapsed]
» api: Apply complete after 24s
» cache: Applying...
» cache: Apply complete after 8s
»
Run run-p9x2ka succeeded in 33s: 1 added, 1 changed, 0 destroyed.
ID           STATUS      CHANGE-SET   TITLE               AGE
run-p9x2ka   Succeeded   cs-7f2a1     Bump api to 1.4.0   33s
```

In CI — detach, then wait:

```
$ RUN=$(admiral run apply run-p9x2ka --no-wait)
$ admiral run wait "$RUN" --for succeeded --wait-timeout 30m
» api: Apply complete after 24s
» cache: Apply complete after 8s
Run run-p9x2ka succeeded in 33s: 1 added, 1 changed, 0 destroyed.
$ echo $?
0
```

For an agent — JSON Lines:

```
$ admiral run apply run-p9x2ka -o json
{"time":"2026-07-01T22:13:49Z","type":"stream.start","version":"1","run":"run-p9x2ka"}
{"time":"2026-07-01T22:13:50Z","type":"revision.status","level":"info","message":"api: Applying...","run":"run-p9x2ka","component":"api","phase":"apply","data":{"status":"REVISION_STATUS_APPLYING"}}
{"time":"2026-07-01T22:14:14Z","type":"revision.status","level":"info","message":"api: Apply complete after 24s","run":"run-p9x2ka","component":"api","phase":"apply","data":{"status":"REVISION_STATUS_SUCCEEDED","elapsedSeconds":24}}
{"time":"2026-07-01T22:14:22Z","type":"run.status","level":"info","message":"Run run-p9x2ka succeeded in 33s: 1 added, 1 changed, 0 destroyed.","run":"run-p9x2ka","data":{"status":"RUN_STATUS_SUCCEEDED","added":1,"changed":1,"destroyed":0}}
{"time":"2026-07-01T22:14:22Z","type":"stream.end","run":"run-p9x2ka","exitCode":0}
```

### A failed run

```
$ admiral run apply run-q3v7mm
» Applying run-q3v7mm to shop/prod (1 component)
» Watch this run at https://app.admiral.io/apps/shop/envs/prod/runs/run-q3v7mm
»
» api: Applying...
» api: Still applying... [10s elapsed]
» api: Apply failed after 5m0s
»
» Error: run run-q3v7mm failed: api: helm upgrade timed out waiting for rollout
» Run 'admiral run logs run-q3v7mm --failed' to see the transcript.
$ echo $?
1

$ admiral run logs run-q3v7mm --failed --tail 3
api/apply   │ Waiting for rollout to finish: 1 of 3 updated replicas are available...
api/apply   │ Error: UPGRADE FAILED: timed out waiting for the condition
api/apply   │ Error: exit status 1
```

### Recover

```
$ admiral run rollback run-p9x2ka --app shop --env prod
» Re-apply the configuration from run-p9x2ka (Succeeded, 6m ago) to shop/prod as a new run? [y/N] y
» Rolling shop/prod forward to the configuration of run-p9x2ka... done, run-r8t1zz
» Rollback re-applies component versions and variables; it does not restore data or state files.
» To reverse this, run: admiral run rollback run-q3v7mm
ID           STATUS      CHANGE-SET   TITLE                                 AGE
run-r8t1zz   Succeeded   <none>       Re-apply configuration of run-p9x2ka  12s
```

### Status and the operator view

```
$ admiral status shop/prod
App          shop
Environment  prod
Health       Healthy
Agent        gke-prod (Online, last seen 3s ago)
Last run     run-r8t1zz Succeeded 12s ago
URL          https://app.admiral.io/apps/shop/envs/prod

COMPONENT   KIND            VERSION   STATUS      HEALTH    UPDATED
api         Workload        2.1.2     Succeeded   Healthy   12s
cache       Workload        7.2.0     Succeeded   Healthy   6m

In progress: none
```

`admiral env describe prod --app shop` (§2.5) is the full version with
conditions, per-component modules, sources, resolved refs, messages, metrics,
variables and events. `admiral env diff prod staging --app shop` (§2.5)
shows the gap between two environments before `changeset copy`.

### Errors

```
$ admiral env get prd --app shop
» Error: environment "prd" not found in application "shop"
» Run 'admiral env list --app shop' to see environments.
$ echo $?
1

$ admiral env get prod
» Error: no application specified
» Pass --app or give the environment as app/env.
$ echo $?
2

$ admiral run logs run-p9x2ka --phase destroy
» Error: invalid value "destroy" for --phase: must be one of plan, apply
$ echo $?
2

$ admiral app list
» Error: not signed in
» Run 'admiral auth login' to sign in, or set ADMIRAL_API_KEY.
$ echo $?
4

$ admiral evn list
» Error: unknown command "evn" for "admiral"
»
» Did you mean this?
»         env
»
» Run 'admiral --help' for usage.
$ echo $?
2
```

### Destructive

```
$ admiral env delete staging --app shop
» Delete environment shop/staging? [y/N] n
» canceled
$ echo $?
1

$ admiral app delete shop
» Deleting shop is not reversible and removes 2 environments, 3 components and 41 runs.
» Type shop to confirm: shop
» application "shop" deleted

$ admiral env delete staging --app shop < /dev/null
» Error: --force required when not running interactively
$ echo $?
2

$ admiral env delete staging --app shop --force
» environment "staging" deleted
```

### Ctrl-C mid-apply

```
$ admiral run apply run-p9x2ka
» api: Applying...
^C
» Interrupted; run run-p9x2ka continues on the server.
» Cancel it with: admiral run cancel run-p9x2ka
$ echo $?
130
```

Two properties hold across every example: stdout only ever carries a table
row, a plan, a final result line, or JSON — so `$(…)` and `| jq` always work
— and the last line of every error is the command that fixes it.
