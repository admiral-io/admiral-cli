# PaaS / app-deployment CLIs: heroku, vercel, fly, railway, render, doctl, netlify

Sources: `heroku` 11.10.0 and `vercel` 59.1.4 run locally (help + one unauthenticated-looking
`vercel ls` that turned out to hit the API because the machine is logged in — output kept, no
further calls made); flyctl/railway/render/heroku behaviour verified against their GitHub
source (superfly/flyctl, railwayapp/cli, render-oss/cli, heroku/cli) and official docs; the
Heroku CLI style guide and Jeff Dickey's "12 Factor CLI Apps" (fetched via archive.org).

---

## 1. Heroku (`heroku`, oclif)

### Grammar
- **`topic:command`**, colon-separated, no spaces: `heroku releases:rollback v41 -a shop`,
  `heroku apps:favorites:add`. Multi-word segments are kebab-case (`pg:credentials:repair-default`).
- Topics are **plural nouns**, commands are **verbs**. Style guide rule: *"Never create `*:list`
  commands — the root command of a topic lists those nouns"*: `heroku apps`, `heroku releases`,
  `heroku config`, `heroku ps` all list. Detail is `topic:info` (`apps:info`, `releases:info`,
  `addons:info`, `pipelines:info`).
- Top-level shortcuts exist for the most common ones (`heroku logs`, `heroku ps`, `heroku run`,
  `heroku open` = `apps:open`, `heroku rollback` = hidden alias of `releases:rollback`).
- Aliases are declared explicitly and shown in help (`ALIASES  $ heroku config:remove`,
  `$ heroku dyno:scale`).
- 395 commands total; root help splits **TOPICS** (nouns) from **COMMANDS** (things runnable at
  root) with lowercase, period-less one-liners.

### Hierarchy / scoping
- The app is a **flag, never a positional**: `-a, --app=<value>  (required) [env: HEROKU_APP] app
  to run command against`. Second-level scope (dyno, process type, release) is positional or a
  narrower flag (`ps:restart [DYNO]`, `-p, --process-type`, `-d, --dyno-name`).
- Default resolution order (from `@heroku-cli/command` `flags/app.ts`): `--app` > `HEROKU_APP`
  env > git remotes. `-r, --remote=<value>  git remote of app to use` picks among remotes
  (`heroku.remote` git config sets the default remote). Exactly one heroku remote → used silently;
  more than one and the flag is required → hard error:
  ```
  Multiple apps in git remotes
    Usage: --remote staging
       or: --app shop-staging
    Your local git repository has more than 1 app referenced in git remotes.
    ...
    Heroku remotes in repo:
    shop-prod (heroku)
    shop-staging (staging)
  ```
- Environments/stages are **pipelines**: apps are coupled to a pipeline with a stage
  (`pipelines:add`, `pipelines:update --stage`), and `pipelines:promote -a shop-staging
  [--to shop-prod,shop-eu]` moves the *release* downstream. `apps:info` shows
  `Pipeline: shop - staging`. `pipelines:diff` compares upstream to downstream.
- Missing scope: `heroku logs` with no app → `›   Error: The following error occurred:
  ›     Missing required flag app  ›   See more help with --help`, exit **2**.

### Input
- Style guide: *"Prefer flags to args ... 1 type of argument is fine, 2 types are very suspect,
  and 3 are never good."* Multiple args of the **same** type are fine (`config:set A=1 B=2`,
  `config:unset A B`, `ps:scale web=3:Standard-2X worker+1`).
- Key=value pairs as positionals for config vars; **clear** is a separate command (`config:unset`),
  not `--set KEY=`.
- `--` passes remaining args through (`heroku run -s standard-2x -- myscript.sh -a arg1`).
- Global `--prompt  interactively prompt for command arguments and flags` (opt-in prompting).
- Destructive: `apps:destroy` uses **type-the-name**: `-c, --confirm=<value>` skips it. Prompt text:
  ```
   ▸    WARNING: This will delete ⬢ shop including all add-ons.
  To proceed, type shop or re-run this command with --confirm shop:
  ```
  Mismatch → `Confirmation foo did not match shop. Aborted.` `--confirm` must equal the app name
  (not a bare `--yes`), so a scripted destroy still names its target.
- Style guide: *"If prompting is required to complete a command ... the user will not be able to
  script the command. Ensure that args or flags can always be provided to bypass the prompt."*

### Output (human)
- **Action lines** (`ux.action.start/stop`) go to **stderr** with a spinner on TTY and a static
  line when piped: `Setting config vars and restarting example... done, v10`,
  `Rolling back ⬢ shop to v41... done, v43`, `Destroying ⬢ shop (including all add-ons)... done`.
- Tables: **no borders** (`cli.table()`), header row in Title Case, columns padded, grep-friendly.
  `heroku regions` example from the style guide:
  ```
  ID         Location                 Runtime
  ─────────  ───────────────────────  ──────────────
  eu         Europe                   Common Runtime
  ```
- `heroku releases` is a **headerless** table (column keys `v`, description, user, created_at)
  under a styled header `=== ⬢ shop Releases - Current: v43`; row: `v43  Rollback to v41
  jeff@heroku.com  2015/11/17 17:37:41 (~ 1h ago)`. Version coloured by status (red failed,
  yellow pending, grey inactive); the release-phase status word is appended to the description
  (`release command failed`) and the description is truncated with `…` **only when stdout is a
  TTY** (`process.stdout.isTTY ? truncate : s`). Default `-n` is 15, newest first.
- `heroku ps` is **grouped, not tabular** (dashboard style):
  ```
  === run: one-off dyno
  run.1: up for 5m: bash
  === web (Standard-1X): bundle exec thin start -p $PORT (2)
  web.1: up 2015/11/17 17:37:41 (~ 1h ago)
  web.2: crashed 2015/11/17 17:37:41 (~ 1h ago)
  ```
  `up` green, other states yellow; timestamp dim. `--json` gives the raw dyno array; hidden
  `-x/--extended` switches to a wide table (ID, Process, State, Region, ..., Release, Command).
- `apps:info` is a **styled object** (key: value, keys Title Case, sorted, nested objects
  indented) under `=== ⬢ shop`; `Web URL` coloured as a link. `-s, --shell` prints
  `key=value` lines for `eval`.
- Timestamps: `time.ago()` → `2015/11/17 17:37:41 (~ 1h ago)` for < 25h, bare timestamp after.
- Symbols: `⬢` before every app name (`color.app`), `▸` prefix on warnings/errors, `===` header
  prefix. Colours by *semantic role* (`color.app` magenta, `color.configVar`, `color.user`,
  `color.pipeline`, `color.code`, `color.success/failure/warning`) — the style guide says
  *"Avoid yellow/red except for errors and warnings"* and *"Don't overuse contrasting colors."*
- Stdout vs stderr: *"Stdout: all output and data. Stderr: warnings, errors, and out-of-band
  information (like cli.action())."* — every advisory (`Rollback affects code and config vars;
  it doesn't add or remove addons.`, `To undo, run: heroku rollback v42`) is `ux.warn` → stderr.

### Output (machine)
- `--json` (sometimes `-j`) on list/info commands; **bare array** for lists (`releases --json`
  → `[{description, user:{email,id}, version, status, ...}]`), full API object. Pipelines info
  wraps: `{pipeline, apps}`. `-s, --shell` for config/info.
- `heroku commands --json`, `--columns id,summary`, `--sort`, `--no-truncate`, `--tree` (oclif
  table conventions).

### TTY awareness
- Colour off when not a TTY, with `--no-color`, `COLOR=false`, `HEROKU_COLOR=0`; logs additionally
  `HEROKU_LOGS_COLOR=0`; `--force-colors` on `logs` to keep colour through a pipe.
- Spinner → static "..." line when not TTY; truncation disabled when piped (see releases).

### Streaming & long-running
- `heroku logs -a app [-t|--tail] [-n|--num 50] [-s|--source app|heroku] [-p|--process-type web]
  [-d|--dyno-name web.1]`. Log line format (from platform): `2015-11-17T17:37:41.123456+00:00
  app[web.1]: message`, coloured by source/dyno.
- `releases:output [RELEASE]` streams the release-phase log; `releases:retry`.
- `ps:wait -w 10 -t web` polls until all dynos run the latest release.
- `run -x, --exit-code  passthrough the exit code of the remote command` (off by default!).
- `pipelines:promote` prints `Fetching apps from shop...`, `Waiting for promotion to complete...`,
  then `Promotion successful` + a styled object of targets.
- `apps:open [PATH]`, `pipelines:open`, `addons:open`, `addons:docs` — one `open` per topic.

### Errors & exit codes
- Format: ` ›   Error: <message>` on stderr, red `Error:` label, `›` gutter. Unknown command →
  ` ›   Error: Run heroku help for a list of available commands.` exit **127** (no did-you-mean
  in this version). Missing required flag exit **2**. Generic failure exit 1.
- Not-found messages name the object: `No eligible release found for ⬢ shop to roll back to.`

### Pagination & filtering
- `-n, --num` on `logs`/`releases`; Range headers under the hood; `apps -A/--all`, `-t team`,
  `-s space`, `apps:errors --hours 24 --dyno --router`.

### Help text style
- Sections `USAGE / ARGUMENTS / FLAGS / GLOBAL FLAGS / DESCRIPTION / ALIASES / EXAMPLES /
  TOPICS / COMMANDS`. Usage line shows required flags inline: `$ heroku logs -a <value>
  [--prompt] [-d <value>] ...`. Descriptions lowercase, no period, fit 80 cols. Flags show
  `(required)` and `[env: HEROKU_APP]`, `[default: 10]`. Examples are `$ cmd` followed by
  literal expected output.

### Config & auth
- `heroku login` (browser) or `HEROKU_API_KEY`; `~/.netrc` for tokens; `HEROKU_APP`,
  `HEROKU_ORGANIZATION`; multiple accounts via `accounts:set`.

### Notable / mistakes
- Steal: semantic colour roles; `⬢ app` glyph; "action... done, v43" lines; type-the-name
  confirm with `--confirm NAME`; `-s/--shell` output; grep-safe borderless tables; `run
  --exit-code`; `--remote` mapping to git remotes.
- Widely disliked: colon grammar is hostile to shell completion of nested nouns and reads
  oddly next to git-style tools; `--num` vs `-n` semantics differ per command; `-j` vs
  `--json` inconsistency; `run` not passing exit codes by default; `apps:destroy` hides the
  positional (`static args = {app: hidden}`) so `heroku apps:destroy shop` works but help
  doesn't say so.

---

## 2. Vercel (`vercel` / `vc`)

### Grammar
- **Space-separated verb-first at root** (`vercel deploy`, `vercel ls`, `vercel logs`,
  `vercel inspect`, `vercel rollback`, `vercel promote`, `vercel redeploy`, `vercel rm`), with
  **noun topics for secondary resources** (`vercel env add|ls|rm|pull|update|run`,
  `vercel project add|ls|inspect|rm|rename|update`, `vercel domains`, `vercel alias`,
  `vercel target`, `vercel rolling-release start|approve|abort|complete|fetch`).
- Aliases shown in the root list: `ls | list`, `rm | remove`, `i | install`, `rr | rolling-release`,
  `cron | crons`. `vercel` with no command **is** `deploy`.
- Root help groups **Basic** vs **Advanced** commands, each line `name  [arg]  description`.

### Hierarchy / scoping
- Scope = team (`-S, --scope`), project (linked), target/environment (`production`, `preview`,
  `development`, or custom via `vercel target`), deployment (URL or `dpl_xxx` ID).
- `vercel link [-p project] [--team slug] [-y]` writes `.vercel/project.json` (`orgId`,
  `projectId`) in the cwd; **every later command defaults to the linked project**. Flags override
  per-call: `--project <NAME_OR_ID>  Project name or ID (defaults to the linked project)`,
  `--cwd <DIR>`, `-A, --local-config vercel.json`, `-Q, --global-config`.
- Not linked + interactive → link wizard; not linked + `--non-interactive` → error. `--non-
  interactive  Run without interactive prompts (default when agent detected)` — the CLI sniffs
  AI agents/CI and disables prompts automatically.
- Deployment is addressed by **URL or ID** everywhere (`inspect`, `logs`, `rollback`, `promote`,
  `redeploy`, `rm`), and can be **piped in on stdin**: `echo my-deployment.vercel.app | vercel
  inspect`. Environment is a filter flag: `--environment production|preview`, `--prod` shorthand.

### Input
- Repeated flags for maps: `-e KEY=v -e KEY2=v`, `-b` build env, `-m KEY=v` metadata (also used
  as a **filter** on `ls -m key=value`). Comma lists for enums: `--status BUILDING,ERROR`,
  `env add API_URL production,preview,development`.
- Secrets: `vercel env add NAME [env]` reads the **value from stdin or a prompt**
  (`cat ~/.npmrc | vercel env add NPM_RC preview`, `vercel env add API_URL production < url.txt`),
  `--value` only "for non-interactive use", `--sensitive/--no-sensitive`, `--force` to overwrite.
- `-y, --yes  Use default options to skip all prompts` on deploy/link/rollback/promote/ls;
  `-f, --force  Force a new deployment even if nothing has changed` means *re-run*, not
  *skip confirmation* — Vercel separates the two.
- `--dry` + `--json` for plan-style preview: `vercel deploy --dry --json` lists every file.

### Output (human)
- Every command prints a banner first: `Vercel CLI 59.1.4` (stderr), then progress lines
  prefixed `> ` with elapsed time in brackets: `Fetching deployments in admiral-io`,
  `> Deployments under admiral-io [131ms]`. Deploy prints
  `🔍  Inspect: https://vercel.com/team/proj/dpl_xxx [2s]` then
  `✅  Preview: https://proj-abc.vercel.app [45s]` (or `Production:`), and **only the URL goes to
  stdout** so `vercel > deployment-url.txt` works (documented example).
- `vercel ls` real output (columns are Title Case, padded, no borders, status has a coloured dot):
  ```
    Age     Project                   Deployment                                          Status      Environment     Duration     Username
    83d     admiral-io/admiral-io     https://admiral-l6n1lysb7-admiral-io.vercel.app     ● Ready     Production      2m           mberwanger
  ```
  Age is compact (`83d`, `2h`), newest first, default `--limit 20`.
- `vercel inspect <url>` is a **describe** view: sections `General` (id, name, target, status,
  url, created, aliases), `Builds`, `Outputs` as key: value blocks; `--logs` swaps in the build
  log; `--wait --timeout 90s` blocks until the deployment leaves BUILDING.
- Log sources marked with glyphs: `λ = serverless, ε = edge/middleware, ◇ = static/external`.
- `vercel logs` non-follow shows one request line per entry and `-x, --expand  Show full log
  message below each request line (default when output is not a TTY)`.

### Output (machine)
- `--json` / `-F, --format json` on `deploy`, `ls`, `inspect`, `env ls`, `metrics`, `logs`
  (JSON **Lines** for logs: `vercel logs --status-code 500 --json | jq '.message'`).
- `vercel api <endpoint>` escape hatch for raw REST.

### TTY awareness
- `--no-color`; `--non-interactive` auto-on for agents/CI; `--expand` default flips on non-TTY;
  telemetry notice on stderr.

### Streaming & long-running
- `logs [url|id] -f/--follow` (prefers active production deployment, falls back to latest
  READY), `-n/--limit 100`, `--since 1h|ISO`, `--until`, `--level error`, `--source`,
  `--status-code 4xx`, `--request-id`, `-q/--query 'status:500 error'`, `-b/--branch`.
- `deploy --no-wait`, `deploy -l/--logs` (print build logs inline), `inspect --wait --timeout`,
  `rollback/promote --timeout 3m` + `rollback status [project]` / `promote status` sub-commands
  to poll a pending operation. `redeploy` rebuilds a previous deployment; `bisect` binary-searches
  deployments for a regression (unique).
- `vercel open` opens the linked project in the dashboard; `--guidance` prints suggested next
  commands after a deploy.

### Status / health / metrics
- `vercel metrics <metric-id> --since 1h --granularity 5m --group-by route -f "environment eq
  'production'" --json`, plus `metrics schema` to discover metric names. `alerts`, `httpstat`.

### Errors & exit codes
- `Error: <message>` on stderr, exit 1. Unknown command is treated as a **path** to deploy
  (`vercel fooz` → `Error: Could not find “~/.../fooz”`) — a consequence of the implicit default
  command; widely considered a footgun.

### Pagination & filtering
- `ls --limit 20 (max 100)`, `-N, --next <MS>` cursor (timestamp) printed at the bottom of the
  table as `To display the next page, run vercel ls --next 1584722256178`, `--status`, `-m`,
  `--environment`, `-a/--all` (across projects).

### Help text style
- `▲ vercel logs [url|deploymentId] [options]`, prose description, `Options:` table with short
  and long (`-f,  --follow`), `Global Options:`, then `Examples:` as `- Sentence` + `$ cmd`.
  Descriptions Sentence-case, no period.

### Config & auth
- `~/.vercel` (`-Q`) global auth/config; `.vercel/project.json` per directory; `-t/--token`,
  `VERCEL_TOKEN`; `-S/--scope` team; `vercel switch` to change default team.

### Notable / mistakes
- Steal: `link` file for directory→project scoping; deployment addressable by URL or ID;
  stdin-fed secrets; `--dry --json`; JSON-lines logs; `rollback status`; `--expand` defaulting on
  non-TTY; only the URL on stdout after deploy; `--non-interactive` auto-detected for agents.
- Mistakes: implicit default command eats typos; `-d` means `--debug` globally but `--deployment`
  in `logs`; `-f` means force, `-F` means format.

---

## 3. Fly.io (`fly` / `flyctl`, cobra)

### Grammar
- **Noun topics with verbs beneath** (`fly apps list|create|destroy|open|releases|restart`,
  `fly machine list|status|start|stop|restart|destroy|update`, `fly secrets set|unset|list`,
  `fly volumes ...`) **plus top-level verbs for the daily loop**: `fly launch`, `fly deploy`,
  `fly status`, `fly logs`, `fly releases`, `fly scale`, `fly ssh console`, `fly open`
  (`dashboard`). Both `fly apps` and `fly machine` exist because the daily-loop verbs are the
  80% path.
- Aliases: `fly`/`flyctl`, `machine`/`machines`/`m`, `list`/`ls`, `destroy`/`delete`/`remove`/`rm`
  (all declared in code and printed in help). Root help description strings are Sentence case,
  no period ("View App status", "Manage Machines").

### Hierarchy / scoping
- App via `-a, --app string  Application name`; resolution in `RequireAppName`: `--app` >
  `FLY_APP` env > `app = "..."` in `fly.toml` (`-c, --config` to point at another file).
  Missing → `Error: the config for your app is missing an app name, add an app field to the
  fly.toml file or specify with the -a flag`.
- Environments are **separate apps** (`shop-staging`, `shop-prod`) + separate `fly.toml`s; there
  is no env concept. Machines/regions are the second level (`-m/--machine`, `-r/--region`,
  `-s/--select` interactively).
- Org via `-o/--org`; `fly apps list` shows the `OWNER` column.

### Input
- `-e, --env stringArray  Set of environment variables in the form of NAME=VALUE pairs. Can be
  specified multiple times.`; `--regions strings` comma list.
- `-y, --yes  Accept all confirmations (also --auto-confirm)`; `--now  Deploy now without
  confirmation`; `--detach`.
- `apps destroy <app name(s)>` (ArbitraryArgs) prompts `Destroy app shop?` after a **red stderr
  line** `Destroying an app is not reversible.`; non-interactive without `--yes` →
  `yes flag must be specified when not running interactively`.
- `secrets set KEY=value` / `secrets unset KEY`; `secrets import` from stdin (`cat .env | fly
  secrets import`).

### Output (human)
- Tables via tablewriter with **all borders off, no header line, header auto-upper-cased**
  (`cfg.Header.Formatting.AutoFormat = tw.On`) — i.e. UPPERCASE headers, space-aligned, blank
  line after. `fly apps list`:
  ```
  NAME          OWNER           STATUS          PLATFORM        LATEST DEPLOY
  testrun       personal        deployed        machines        21h17m ago
  my-app        personal        suspended       machines        2023-11-15T23:33:07Z
  ```
  `fly releases`: `VERSION  STATUS  DESCRIPTION  USER  DATE` (+ `DOCKER IMAGE` with `--image`),
  rows `v12  complete  Deploy image  jeff@x.io  2h3m ago`, 25 newest.
- `fly status` is a **dashboard**: a `VerticalTable` (key = value block) titled `App` in bold,
  then a `Machines` table:
  ```
  App
    Name     = testrun
    Owner    = personal
    Hostname = testrun.fly.dev
    Image    = testrun:deployment-01GQ...

  Machines
  PROCESS ID              VERSION REGION  STATE   ROLE  CHECKS          LAST UPDATED
  app     06e82d43ad1587  5       yyz     started       1 total, 1 passing  2023-01-17T21:42:33Z
  ```
  Footnote glyphs: `†` standby machine, `💀` host unreachable, with a `Notes:` legend printed
  only when used. Yellow banner `Updates available: ... Run \`flyctl image update\``.
  `--watch --rate 5` redraws every N seconds; `--all` includes completed instances.
- `fly machine status <id>` = header lines + `Machine` key=value block + `Event Logs` table
  (`STATE EVENT SOURCE TIMESTAMP INFO`), `-d` appends the JSON config.
- `fly machine list` prints a summary + link **before** the table:
  `1 machines have been retrieved from app testrun.` / `View them in the UI here
  (https://fly.io/apps/testrun/machines/)` (OSC-8 hyperlink when supported), table titled with
  the app name, unreachable rows marked `*` with a footnote. `-q/--quiet` prints IDs only.
- Timestamps: `format.RelativeTime` → `just now`, `12s ago`, `3m4s ago`, `21h17m ago`, else
  `Jan 2 2006 15:04`; machine timestamps are raw RFC3339. Empty list: `No apps found` /
  `No machines are available on this app testrun` (stdout, exit 0).
- Deploy progress on **stderr** via `TextBlock`: `==> Building image` (green), `-->` for done
  lines, `>` detail lines; then
  ```
  Watch your app at https://fly.io/apps/aged-water-8803/monitoring

  Updating existing machines in 'example-app-8803' with canary strategy
    [1/2] Machine 3287457df77785 [app] update finished: success
    [2/2] Machine 91857266c41638 [app] update finished: success
    Finished deploying

  Visit your newly deployed app at https://example-app-8803.fly.dev/
  ```
  (`✔`/`✖` glyphs on TTY). Strategy is a flag: `--strategy canary|rolling|bluegreen|immediate`,
  `--wait-timeout 5m0s`, `--max-concurrent 8`.
- Logs: `2023-03-07T16:18:08Z app[5683606c41098e] lhr [info]Starting init ...` — timestamp,
  source[machine], region, [level], message. Streams **by default**; `-n, --no-tail` fetches the
  buffer and exits. `-j/--json` gives one JSON object per line.

### Output (machine)
- `-j, --json  JSON output` on nearly everything; `render.JSON` = 4-space-indented, **bare
  array** for lists, full API objects. `fly status --json` is a **curated** object (`ID, Name,
  Deployed, Status, Hostname, Version, AppURL, Organization, PlatformVersion, Machines`).
  `fly config show` = JSON of fly.toml.

### TTY awareness
- Colour scheme from `iostreams` (auto-off when not TTY / `NO_COLOR`); prompts detect
  non-interactive and return a typed error telling you which flag to pass.

### Streaming & long-running
- `fly logs` (stream default, `--no-tail`, `-m machine`, `-r region`), `fly deploy` watches
  machines until healthy or `--detach`, `fly status --watch`, `fly machine status`,
  `fly checks list`, `fly ping`. `fly dashboard` / `fly apps open` open the browser.

### Errors & exit codes
- `Error: <msg>` red on stderr, optionally followed by a **description**, a **suggestion**, and
  `View more information at <docURL>` (typed error carries all three); `(Request ID: …)`
  appended. `--debug` prints a stack trace. Exit codes: **0 ok, 1 error, 126 deadline
  exceeded, 127 interrupted (Ctrl-C)**; "unchanged deploy" prints the error but exits 0 so CI
  stays green. Unknown command additionally prints `Run 'fly --help' for usage.`
  GitHub Actions: `FLY_GHA_ERROR_ANNOTATION` emits `::error` annotations.

### Help text style
- cobra, wrapped flag usage, Sentence-case Short with **trailing period** on some ("List
  applications.", "Permanently destroy one or more apps.") and none on others — inconsistent.
  Usage `fly apps destroy <app name(s)> [flags]`.

### Config & auth
- `~/.fly/config.yml`, `FLY_ACCESS_TOKEN` / `-t`, `fly auth login|token`, `fly tokens create`
  scoped deploy tokens, `FLY_APP`, `FLY_REGION`, per-project `fly.toml`.

### Notable / mistakes
- Steal: the `status` dashboard (key=value block + machines table + footnote legend);
  `releases` columns `VERSION STATUS DESCRIPTION USER DATE`; deploy `==>`/`-->` step blocks on
  stderr with `[1/2] Machine … update finished: success`; `Visit your newly deployed app at URL`
  closer; typed errors with suggestion + doc URL; exit 126/127 split; `--no-tail` (streaming
  default for a *logs* verb); `-q` id-only lists; red "not reversible" warning before confirm.
- Mistakes: dual `apps`/`machine` trees with overlapping verbs (`fly apps releases` vs
  `fly releases`); inconsistent trailing periods; timestamp formats differ table to table; two
  spinner/text styles depending on subsystem age.

---

## 4. Railway (`railway`, Rust/clap)

### Grammar
- **Flat verbs for the loop** (`up`, `deploy`, `redeploy`, `down`, `restart`, `logs`, `status`,
  `open`, `run`, `shell`, `ssh`, `link`, `unlink`, `list`, `init`) and **noun topics with verbs**
  for the rest (`variable list|set|delete`, `environment new|delete`, `deployment list`,
  `domain list|status`, `volume list|add|delete`, `service files browse`). Singular nouns.

### Hierarchy / scoping
- Project > environment > service. **`railway link [-p project] [-e env] [-s service]`** stores
  the triple in `~/.railway/config.json` keyed by **absolute directory path**; lookups walk up
  parent directories (`get_closest_linked_project_directory`). `railway service` / `railway
  environment` re-link one level interactively. `railway unlink` drops it.
- Every command accepts the global overrides
  `-s, --service  Target service (name or ID)`, `-e, --environment  Target environment (name or
  ID)`, `-p, --project PROJECT_ID`, and help text says `(defaults to linked service)`.
- Not linked → `No linked project found. Run railway link to connect to a project`; env
  missing → `Environment "dev" not found.\nRun \`railway environment\` to connect to an
  environment.`; auth → `Unauthorized. Please login with \`railway login\``. Every error names
  the command that fixes it.
- `railway status` is a **context** view: `Workspace:`, `Project:` (purple bold), `Project ID:`
  (dim), `Environment:` (blue bold) / `None` (red bold), `Unmerged: 2 changes` (yellow), then a
  `Linked service` card and a divider + per-service resource cards. `--json` dumps the project
  with environment instances. `railway status --project X` **requires** `--environment`.

### Input
- `variable set KEY=value [KEY2=value2]`, `variable delete KEY`. `-y, --yes  Skip confirmation
  prompts` global. `up -m "message"`, `up --detach` (`--no-wait` alias), `up -c/--ci` ("Stream
  build logs only, then exit (equivalent to setting $CI=true)"), `up --json`.
- Time flags: `--since 30s|5m|2h|1d|1w|ISO` (`-S`), `--until` (`-U`); either **disables
  streaming**.

### Output (human)
- `railway up` prints spinner steps (`Indexing`, `Uploading`, `✓ Bundled (12345 bytes)`), then
  `  Build Logs: https://railway.com/project/.../service/...?id=...`, streams build then deploy
  logs, ends with **`Deploy complete`** (green bold) or `Deploy failed` / `Deploy crashed`
  (red bold, exit 1). If stdout is not a TTY and not CI mode it does **not** stream.
- `logs` tokens: `-d/--deployment`, `-b/--build`, `--http`, `--network`, `--dns` (mutually
  exclusive group), `-n/--lines N` (visible alias `--tail`) "disables streaming", `--latest`
  (even if failed), `--filter` query syntax, `--status 500|>=400|500..599`, `--method`,
  `--path`, `--request-id`. Defaults to the most recent **successful** deployment.
- Colours: `owo-colors` — purple project, blue environment, dim IDs, yellow `Warning:` prefix on
  stderr (`Warning: unable to load bucket details: …`).

### Output (machine)
- `--json` everywhere; `up --json` emits JSON lines: `{"deploymentId":..,"logsUrl":..}` on
  detach, then `{"status":"success"}` / `{"status":"failed"}`. `logs --json` = one object per
  line with timestamp/message/attributes. Help text has an **"Automation notes:"** block:
  *"`railway up --detach --json` starts an upload and deployment, but it does not wait ... Poll
  with `railway deployment list --json` and inspect logs with `railway logs --json --lines 100`."*

### TTY / config / auth
- Spinners only on TTY; `CI=true` switches to CI mode; `RAILWAY_TOKEN` (project) vs
  `RAILWAY_API_TOKEN` (account); `railway login --browserless` (device code). `railway
  completion bash|zsh|fish`, `railway docs`, `railway open`, `railway mcp install`,
  `railway skills install` (agent docs).

### Notable / mistakes
- Steal: directory-keyed link with parent-dir walk; `--since/--until` implying non-stream;
  `--latest` to force the newest deployment; "Automation notes" in help; error messages that
  name the fixing command; `Deploy complete` / `Deploy failed` terminal verdict + exit 1;
  `--lines` with `--tail` alias.
- Mistakes: `up` vs `deploy` (deploy = templates) confuses; `-S/-U` short flags are unusual;
  `status` mixes local link state with remote resource state.

---

## 5. Render (`render`, Go/cobra, Bubble Tea TUI)

- Grammar: plural nouns then verbs: `render services [create]`, `render deploys list|create
  [SERVICE_ID]`, `render jobs`, `render workspace set`, `render workspaces`, `render logs`,
  `render psql`, `render ssh`, `render restart`, `render blueprints validate`.
- **Interactive by default**: `-o, --output  Set output format to interactive, json, yaml, or
  text. Auto-switches to text on non-TTY`. Precedence: `--output` flag > `RENDER_OUTPUT` env >
  TTY/CI auto-detection. Global `--confirm  Skip all confirmation prompts`. Hidden
  `--pretty-json`, `--json-record-per-line`.
- Scope: workspace is **sticky state** (`render workspace set`), resource is a positional ID
  (`srv-abc123`, `dpg-...`) or picked from a TUI list when omitted in interactive mode. No
  directory linking.
- Logs: `render logs --resources srv-abc123 --tail`, `-r` comma list "(Required in
  non-interactive mode)", `--start/--end` RFC3339, `--text a,b`, `--level`, `--type`,
  `--instance`, `--host`, `--status-code`, `--method`, `--path`, `--limit`, `--direction
  backward|forward`, `--tail  Stream new logs`. "Unlike in the Render Dashboard, you can view
  logs for multiple resources at once."
- `deploys create --wait` "a failed deploy exits with a non-zero status"; `--commit SHA`,
  `--image URL`. `psql -c "query" -o json`, `-- --csv` pass-through.
- Auth: `render login` (browser, expiring CLI token) or `RENDER_API_KEY` (takes precedence);
  `~/.render/cli.yaml`, `RENDER_CLI_CONFIG_DIR`. Cobra help with `GroupID` command groups and
  `Example:` blocks (`# comment` + command).
- Steal: `-o interactive|json|yaml|text` with env default and TTY fallback; explicit note of
  what is required in non-interactive mode; `--confirm` as the global skip flag.
- Mistake: TUI-first makes plain `render services` unscriptable without `-o`; positional IDs
  only (no names).

---

## 6. DigitalOcean `doctl apps`

- Grammar: `doctl apps list|get|create|update|delete|logs|restart|console|propose`,
  `create-deployment|list-deployments|get-deployment` (**hyphenated compound verbs instead of a
  nested `deployments` noun**), `spec get|validate`. Aliases `apps`/`app`/`a`, `list`/`ls`,
  `get`/`g`, `delete`/`d`/`rm`, `list-deployments`/`lsd`, `create-deployment`/`cd`, `logs`/`l`.
- Scope: `<app id>` positional (UUID; name accepted for logs), component name second positional
  (`doctl apps logs <app> <component> --type build`), `--deployment ID  Defaults to current
  deployment`.
- Machine output: global `-o, --output text|json` (default text), **`--format ID,Spec.Name,
  DefaultIngress,ActiveDeployment.ID,InProgressDeployment.ID,Created,Updated`** column picker,
  `--no-header  Return raw data with no headers`. Deployment columns: `ID, Cause, Progress,
  Phase, Created, Updated`. Docs repeat: "Only basic information is included with the text
  output format. For complete app details ... use the JSON format."
- Logs: `--type build|deploy|run|run_restarted|autoscale_event` (default run), `-f/--follow`,
  `--tail N` (default -1), `--no-prefix  Removes the prefix from logs. Useful for JSON structured
  logs`, `--job-invocation`, `--event-id`.
- `create-deployment --wait --force-rebuild`, `delete -f/--force  Delete the App without a
  confirmation prompt`. Globals: `--context` (auth contexts), `-t/--access-token`,
  `--interactive  Enable interactive behavior. Defaults to true if the terminal supports it`,
  `--http-retry-max 5`, `--trace`, `-v/--verbose`.
- Steal: `--format` + `--no-header` for shell pipelines; `--no-prefix` on logs; `--type` log
  phase selector that maps to build/deploy/run phases; `--context` named auth contexts.
- Mistake: UUID-only addressing; `create-deployment` verb-noun hyphenation breaks the tree.

---

## 7. Netlify (`netlify`/`ntl`, oclif-derived, colon grammar)

- `topic:command` like Heroku: `env:set|get|list|unset|import|clone`, `open:admin|open:site`,
  `status:hooks`, `logs:deploy|logs:function` (newer `netlify logs --source deploy|functions
  --follow --since 10m --until --level --json`), `deploy --prod --alias x --message "…" --json
  --open --timeout`, `link --id|--name|--git-remote-url|--git-remote-name`, `status --json
  --verbose`, `env:set KEY value --context production --secret`, global `--filter <app>` for
  monorepos, `--auth TOKEN`, `--debug`. Link data is stored in `.netlify/state.json`; the CLI
  auto-detects the site from the git remote if unique.
- Steal: `--open` after deploy; `open:admin` vs `open:site` (dashboard vs the app itself);
  `--context production|deploy-preview|branch:foo` as the env selector for variables.

---

## 8. Cross-cutting: Heroku CLI style guide + 12 Factor CLI Apps

Verbatim rules worth keeping on file:
- *"Descriptions ... begin with lowercase character, do not end in a period."* (Heroku)
- *"Prefer flags to args ... 1 type of argument is fine, 2 types are very suspect, and 3 are
  never good."* Variable-length args of one type are fine (`rm f1 f2`).
- *"stdout is for output, stderr is for messaging."* Progress, warnings, action lines → stderr.
- Error anatomy: `Error: <code> - <one-line>` / description / `Fix with: <command>` / URL:
  ```
  Error: EPERM - Invalid permissions on myfile.out
  Cannot write to myfile.out, file does not have write permissions.
  Fix with: chmod +w myfile.out
  https://github.com/jdxcode/myapp
  ```
- Tables: *"Never output table borders"*; one entry per row so `wc -l`/`grep` work; show a few
  columns by default, `--columns` to add, truncate to width unless `--no-truncate`, headers
  shown unless `--no-headers`, `--filter`, `--sort`, csv/json output.
- *"Never require a prompt ... allow them to override prompts always."* Prompt only if stdin is
  a TTY; type-the-name confirm for destructive actions.
- Respect `TERM=dumb`, `NO_COLOR`, `--no-color`, and an app-specific `MYAPP_NOCOLOR`; no
  spinners/ANSI when not a TTY.
- `mycli`, `mycli --help`, `mycli help`, `mycli -h`, `sub --help`, `sub -h` must all show help;
  `-h/--help` reserved. `version`, `--version`, `-V` (and `-v` unless it means verbose).
- Startup budget: *"100ms–500ms: fast enough, aim here."*
- Colon vs space: Heroku argues colons let a topic command take an argument (`heroku domains
  www.x.com` is unambiguous, `git submodule add` is not) — the exact conflict Admiral already
  ran into ("verbs with a positional name must not also have subcommands").
- XDG paths: `~/.config/myapp`, `~/.local/share/myapp`, cache `~/.cache/myapp` (macOS
  `~/Library/Caches/myapp`).

---

## Lessons for Admiral

1. **Keep space-separated `noun verb` and never mix positionals with subcommands on the same
   node** — this is precisely the ambiguity Heroku's colons were invented to dodge (12-factor
   #11). Where Admiral wants a "topic root that lists" (`admiral env` = `admiral env list`),
   make the bare noun print help, not a list, so the noun can never need a positional.
2. **Promote the daily loop to top-level verbs, fly-style**: `admiral status`, `admiral logs`,
   `admiral deploy`/`admiral apply`, `admiral open`, `admiral link`, alongside the full
   `app/env/run` trees. fly and railway both keep both; users type `fly status` 50× for every
   `fly apps list`.
3. **Add `admiral link` writing `.admiral/link.json` (`app`, `env`, optional `runner`) in the
   repo, looked up by walking parent dirs (railway) or via a file in cwd (vercel `.vercel/
   project.json`).** Then `--app/--env` become overrides with help text `(defaults to linked
   app)` and the error when unlinked is `No linked app. Run 'admiral link' or pass --app`.
   Resolution order, copied from fly: flag > `ADMIRAL_APP`/`ADMIRAL_ENV` env > link file.
4. **Deployment/run addressing should accept the run ID *and* a URL / `latest`**: vercel
   accepts URL-or-ID and stdin; railway has `--latest`; heroku `releases:info` defaults to the
   last release. `admiral run get` with no arg → latest run in the linked env.
5. **Error format: `Error: <msg>` red on stderr, then optional description, then a `Run
   'admiral env list --app shop' to …` suggestion, then a docs URL** (fly's typed
   errors, 12-factor error anatomy). Railway shows the payoff: every not-found/unauthenticated
   error names the command that fixes it.
6. **Exit codes: 0 ok, 1 failure, 2 usage/flag error (heroku), 126 timeout (`--wait`
   exceeded), 127 interrupted (fly).** And make `run apply --wait` / `run get --wait` exit
   non-zero when the *run* fails (render `deploys create --wait`, railway `Deploy failed` →
   exit 1) — do not copy heroku `run` needing `-x` to pass through.
7. **`admiral status` = fly's dashboard**: bold section title, `Key = value` block for the
   env (App, Env, Runner, Last run, URL), then a `Components` table (`NAME  REVISION  STATUS
   HEALTH  LAST UPDATED`), footnote legend only when a glyph is used, and `--watch --rate 5`.
   `--json` returns a curated object, not the raw API dump.
8. **Run history table = fly `releases` + heroku `releases`**: `RUN  STATUS  DESCRIPTION  USER
   AGE`, newest first, default 15–25 rows, mark the current one (`Current: v43` in the header,
   heroku), colour the status word not the whole row, append the failure phase to the
   description (`plan failed`), truncate description with `…` only on a TTY.
9. **Age formatting**: table lists use compact relative (`21h17m ago` fly / `83d` vercel);
   describe views print RFC3339 *and* relative (heroku `2015/11/17 17:37:41 (~ 1h ago)`).
   Never mix both styles in one table (fly does; it looks broken).
10. **Logs verbs**: `admiral logs [--run ID|latest] [-f/--follow] [-n/--tail N] [--since 1h
    --until] [--phase plan|apply|destroy] [--component web] [--timestamps] [--no-prefix]
    [--json]` (JSON Lines). Line prefix `2023-03-07T16:18:08Z apply[web] [info] msg` (fly
    format). Streaming should be **opt-in `-f`** for a `logs` noun on a finished run but
    **default on** while the run is in progress (fly streams by default; railway/heroku don't
    — pick the run-state rule and say so in help). `--since/--until` disable follow (railway).
11. **Deploy/apply progress on stderr** with fly's `==> Step` / `-->` done / `[1/3] component
    web: finished: success` lines, and a final **stdout-only** line that is just the run URL
    or ID (vercel `vercel > url.txt`). `--detach` returns immediately and prints the ID;
    `--json` with detach emits `{"run":"...","url":"..."}` then `{"status":"succeeded"}`
    (railway).
12. **Destructive confirmations**: keep type-the-name for `app delete`, but make the bypass
    `--confirm NAME` (heroku) rather than a bare `--force` so scripts still state the target;
    print a red `Deleting an app is not reversible.` on stderr before the prompt (fly). In
    non-TTY with no flag, fail with `--force must be specified when not running interactively`
    (fly wording) — never hang on a prompt.
13. **Machine output**: `-o json|yaml` full API object, bare array for lists (heroku, fly);
    add `--format COL,COL` + `--no-header` (doctl) or `--columns/--no-truncate/--no-headers`
    (12-factor) so shell users skip jq. Consider `-q/--quiet` printing names only (fly `-q`).
14. **Non-interactive detection**: add `--non-interactive` / `ADMIRAL_NON_INTERACTIVE` and
    auto-enable it when `CI` is set or stdin is not a TTY (vercel also sniffs AI agents).
    Document in help which flags become required in that mode (render: "(Required in
    non-interactive mode)").
15. **Help text**: lowercase, no trailing period, ≤ 80 cols (heroku); show `[env: ADMIRAL_APP]`
    and `(required)` inline in flag help (oclif); always an `Examples:` block with a `#
    comment` line above each command (render/cobra). Add an **"Automation notes:"** paragraph
    on `run apply`, `logs`, `run get` explaining the poll loop (railway).
16. **`admiral open`** (dashboard for the linked app/env) plus `--web` on `run get`/`app get`;
    print the URL as an OSC-8 hyperlink on TTY and plain text otherwise (fly `io.CreateLink`).
    Print `Watch this run at <url>` at the start of a long apply (fly).
17. **Rollback UX** (heroku): `Rolling back shop/prod to v41... done, v43`, then two stderr
    warnings — what rollback does *not* touch, and `To undo, run: admiral run rollback v42`.
18. **Colour by semantic role**, not by mood: app names one colour (heroku magenta `⬢ app`),
    env another (railway blue), IDs dim, status words green/yellow/red only. Provide
    `--no-color`, `NO_COLOR`, `ADMIRAL_NO_COLOR`, and drop colour when stdout is not a TTY.
19. **Environment variables as the config layer**: `ADMIRAL_APP`, `ADMIRAL_ENV`,
    `ADMIRAL_OUTPUT` (render `RENDER_OUTPUT` sets the default `-o`), `ADMIRAL_API_KEY`
    taking precedence over the session (render/railway both do this explicitly).
20. **Avoid**: an implicit default command (vercel's typo-becomes-deploy), verbs with hyphenated
    nouns (`create-deployment`), two overlapping trees for the same resource (`fly apps
    releases` vs `fly releases`), TUI-only lists without a text fallback (render), and
    positional UUIDs where names exist (doctl).
