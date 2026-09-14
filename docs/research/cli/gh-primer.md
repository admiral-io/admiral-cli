# gh (GitHub CLI) + Primer CLI design guidelines — research report

Source of truth: `gh version 2.97.0 (2026-07-31)` run locally (authenticated, live calls against
`cli/cli`, `rust-lang/rust`, `microsoft/vscode`), the `cli/cli` source on `trunk`
(`internal/ghcmd/cmd.go`, `pkg/cmdutil/errors.go`, `internal/tableprinter`, `pkg/cmd/run/shared`,
`pkg/iostreams/color.go`, `docs/command-development.md`, `docs/command-line-syntax.md`), and the
Primer CLI guidelines. Note: `https://primer.style/cli/` now 308-redirects to the archived
`github.com/primer/cli`; the live content is in `primer/design` at `content/native/cli/{foundations,
components,getting-started}.mdx` (also vendored into `cli/cli/docs/primer/`). Quotes below are from
those files.

All output snippets below are real, trimmed. TTY output was captured with `GH_FORCE_TTY=<cols>`
(+ `NO_COLOR=1` where escape codes would obscure the text); piped output with `| sed 's/\t/^I/g'`
so tabs are visible.

---

## 1. Grammar

**noun → verb, two levels, flags after.** Primer's table (foundations/language):

| gh | `<command>` | `<subcommand>` | [value] | [flags] | [value] |
|---|---|---|---|---|---|
| gh | issue | view | 234 | --web | - |
| gh | pr | create | - | --title | "Title" |
| gh | repo | fork | cli/cli | --clone | false |
| gh | pr | status | - | - | - |
| gh | issue | list | - | --state | closed |

- "**Command:** The object you want to interact with. **Subcommand:** The action you want to take
  on that object."
- Nouns are singular (`pr`, `issue`, `repo`, `run`, `workflow`, `secret`, `variable`, `cache`).
  Aliases exist for verbs, declared in help: `gh pr ls` (for list), `gh pr new` (for create);
  and for whole commands (`gh co` = `pr checkout`; `gh agent-tasks`, `gh agent`, `gh agents` all
  alias `agent-task`).
- Depth is strictly `gh <noun> <verb>`; there is no third level of nouns. Sub-resources are reached
  via flags on the parent verb: `gh run view --job 456789`, `gh run view 123 --log`, `gh pr checks`.
- Identity is positional and polymorphic: `gh pr view [<number> | <url> | <branch>]`,
  `gh run view [<run-id>]`, `gh workflow run [<workflow-id> | <workflow-name>]`,
  `gh repo view [<repository>]`. Omitting it means "the one implied by context" (current branch's
  PR, current repo) or triggers an interactive picker (`gh run view` with no id lists runs to pick).
- Primer Do/Don't: "Use a flag for modifiers of actions" / "Avoid making modifiers their own
  commands"; "Use understood shorthands to save characters to type" (`pr`, `repo`); "Avoid language
  that can be interpreted in multiple ways ('open in browser' or 'open a pull request')".
- Usage-line syntax (`docs/command-line-syntax.md`, mirrored in Primer components/syntax):
  literal text plain; `<placeholder>` in angle brackets, dash-case multiword (`<issue-number>`);
  `[optional]`; `{a | b}` required mutually exclusive; `[<number> | <url>]` optional exclusive;
  `<pr-number>...` repeatable.

## 2. Hierarchy / scoping

The only parent scope in gh is the repository, and it is resolved in this order:
1. `-R, --repo [HOST/]OWNER/REPO` flag (an *inherited* flag shown under `INHERITED FLAGS` on every
   repo-scoped command; `gh workflow --help` shows it once on the parent).
2. `GH_REPO` env var ("specify the GitHub repository in the `[HOST/]OWNER/REPO` format for commands
   that otherwise operate on a local repository").
3. Git remote inference from cwd (`gh repo set-default` picks among multiple remotes).
4. `GH_HOST` for the host when it can't be inferred.

When scope cannot be resolved the error is blunt and exit 1:
```
$ cd /tmp && gh pr list
failed to run git: fatal: not a git repository (or any of the parent directories): .git
```
Compound scope is one string with slashes, not two flags (`cli/cli`, `github.example.com/org/repo`).
`gh api` reuses the same resolution via `{owner}`/`{repo}`/`{branch}` placeholders in the endpoint.
Per-resource scope for secrets uses level flags: `gh secret set NAME` (repo default) `-e <env>`,
`-o <org>`, `-u` (user).

## 3. Input

- Positional for identity, flags for everything else. Flags always have a long form; short forms
  are single letters and consistent across commands: `-L/--limit`, `-s/--state|--status`,
  `-w/--web`, `-q/--jq`, `-t/--template`, `-R/--repo`, `-l/--label`, `-a/--assignee`,
  `-b/--body`, `-F/--body-file`, `-t/--title`, `-d/--draft`.
- Flag *type names* in help are semantic, not Go types: `--jq expression`, `--json fields`,
  `--repo [HOST/]OWNER/REPO`, `--commit SHA`, `--created date`, `--event event`,
  `--assignee login`, `--base branch`, `--body-file file`, `--field key=value`.
- Repeated flags for lists (`--label bug --label "priority 1"`); `strings` type also accepts comma
  lists (`-e cli/cli -e cli/go-gh` documented as "Comma separated list of repos").
- File / stdin: `-F, --body-file file  Read body text from file (use "-" to read from standard input)`.
  `gh api -F key=@path` / `@-` for stdin; `--input -` for whole body from stdin;
  `gh workflow run --json` = "Read workflow inputs as JSON via STDIN".
- key=value: `gh api -f key=value` (raw string) vs `-F key=value` (typed: `true`/`false`/`null`/ints
  converted; `key[]=v1 key[]=v2` arrays; `key[sub]=v` nesting).
- Secrets: `gh secret set NAME -b value` OR "reads from standard input if not specified";
  `--env-file` dotenv bulk load.
- Set/clear a field: `gh pr edit` uses paired verbs, not empty strings:
  `--add-label`/`--remove-label`, `--add-assignee`/`--remove-assignee`,
  `--add-reviewer`/`--remove-reviewer`, `--add-project`/`--remove-project`,
  `--milestone name` / `--remove-milestone` (boolean). Internally `cmdutil.NilStringFlag` /
  `NilBoolFlag` "distinguish omitted values from explicit empty/false values".
- Interactive prompts: Primer — "Use prompts for entering information. Use a prompt when user
  intent is unclear. Make sure to provide flags for all prompts." Yes/No prompt default "is in
  caps" (`[Y/n]`). Prompts are suppressed when stdin/stdout are not a TTY, when
  `GH_PROMPT_DISABLED` is set, or `gh config set prompt disabled`. Every prompt has a flag path and a
  non-interactive guard message:
  ```
  $ GH_PROMPT_DISABLED=1 gh pr create --dry-run
  must provide `--title` and `--body` (or `--fill` or `fill-first` or `--fillverbose`) when not running interactively
  $ GH_PROMPT_DISABLED=1 gh repo delete someone/nonexistent-xyz
  --yes required when not running interactively
  ```
- Confirmation: `--yes` (`gh repo delete --yes  Confirm deletion without prompting`;
  `gh release delete -y, --yes  Skip the confirmation prompt`). `--confirm` was deprecated in
  favour of `--yes`. `--force` is reserved for a *different* semantic (`gh run cancel --force
  Force cancel a workflow run`, i.e. server-side force). Repo delete uses type-the-name:
  `Type cli/cli to confirm deletion:`; and `--yes` is deliberately *ignored* when no repo argument
  is given ("Warning: `--yes` is ignored since no repository was specified").
- `--dry-run` on `pr create` ("Print details instead of creating the PR").
- `--fill` family: derive inputs from context (git commits) instead of prompting.
- `@me` sentinel for "current user" in `--assignee @me`, `--author @me`.

## 4. Output (human)

**Lists (TTY):** header line for context on stdout, blank line, table with UPPERCASE column names,
underlined+dim headers, relative times, truncation to terminal width.
```
$ gh pr list -R cli/cli --limit 3

Showing 3 of 55 open pull requests in cli/cli

ID      TITLE                                   BRANCH                              CREATED AT
#14373  docs: Add step to install working v...  acoulton:patch-1                    about 6 days ago
#14355  Remove obsolete Git test seams          williammartin-clean-git-test-seams  about 9 days ago
```
(colors: `#14373` green = open, `#14355` gray = draft, branch cyan, time gray 242.)

```
$ gh run list -R cli/cli --limit 2
STATUS  TITLE                                 WORKFLOW                          BRANCH  EVENT     ID           ELAPSED  AGE
✓       Triage Scheduled Tasks                Triage Scheduled Tasks            trunk   schedule  34796468421  13s      about 5 minutes ago
✓       Dependabot PR Triage (skills-driven)  Dependabot PR Triag...            trunk   schedule  34795322291  4m50s    about 26 minutes ago
```
(`run list` has no "Showing" header; TITLE and BRANCH bold, ID cyan, AGE muted.)

Column conventions: `ID`, `TITLE`, `STATUS` (as a symbol column), `STATE` (word), `ELAPSED`
(`4m50s`, `13s`, `5854h23m39s` — Go `Duration.String()`), `AGE` / `CREATED AT`
(`about 6 days ago`, `about 26 minutes ago` — `text.RelativeTimeAgo`; abbreviated form `2m`, `3h`,
`5d`, then `Jan _2, 2006` exists as `FuzzyAgoAbbr`). Symbol column is 1 char wide.

**Status symbols** (`pkg/cmd/run/shared.Symbol`, `iostreams.ColorScheme`):
| state | symbol | color |
|---|---|---|
| completed+success | `✓` | green |
| completed+skipped/neutral | `-` | muted gray |
| completed+anything else (failure, cancelled, timed_out…) | `X` (ASCII capital X, not ✗) | red |
| not completed (queued/in_progress/waiting) | `*` | yellow |
| warning | `!` | yellow |
Primer's documented set: `✓ Success`, `- Neutral`, `✗ Failure`, `+ Changes requested`, `! Alert`.
gh ships `X` rather than `✗` for font-support reasons. "Only use color/iconography to enhance
meaning, not to communicate meaning" — hence the piped form writes the word.

**Detail view (`run view`, TTY and piped are identical):**
```
$ gh run view -R cli/cli 34735195871

X trunk Bump Go · 34735195871
Triggered via schedule about 22 hours ago

JOBS
X bump-go in 1m3s (ID 103665259647)
  ✓ Set up job
  ✓ Checkout repository
  X Bump Go version
  - Post Set up Go
  ✓ Complete job

ANNOTATIONS
X Process completed with exit code 1.
bump-go: .github#362


To see what failed, try: gh run view 34735195871 --log-failed
View this run on GitHub: https://github.com/cli/cli/actions/runs/34735195871
```
Pattern: status-symbol headline (`<sym> <branch> <title> · <id>`), one-line provenance, UPPERCASE
section headers (`JOBS`, `ANNOTATIONS`), 2-space indented children, then a **"next step" hint** and
the URL last. For an in-progress run the footer changes to
`For more information about a job, try: gh run view --job=<job-id>`. Primer components/detail:
"Single item views show more detail than list views. The body of the item is rendered indented.
The item's URL is shown at the bottom."

**`pr view` (TTY):** title + `owner/repo#N`, one-line state sentence
(`Open • acoulton wants to merge 1 commit into trunk from patch-1 • about 6 days ago`),
`+17 -0 • ✓ Checks passing`, `Reviewers:`/`Labels:` lines, then the markdown body rendered
(glamour) and indented. **Piped** it becomes `key:<TAB>value` lines, `--`, raw body:
```
title:	docs: Add step to install working version on Ubuntu Pro
state:	OPEN
author:	acoulton (Andrew Coulton)
labels:	external, ready-for-review
number:	14373
url:	https://github.com/cli/cli/pull/14373
--
### Description
...
```

**`pr checks`:** summary line + counts, then table with symbol column:
```
All checks were successful
0 cancelled, 0 failing, 2 successful, 5 skipped, and 0 pending checks

   NAME                                             DESCRIPTION  ELAPSED  URL
✓  PR Triaging/check-requirements / check-requi...               8s       https://github.com/cli/cli/actions/runs/341...
-  PR Triaging/check-requirements / close-unmet...                        https://github.com/cli/cli/actions/runs/341...
```

**Empty state:** TTY → message on **stderr**, exit **0**; piped → nothing on stdout, exit 0;
`--json` → `[]`.
```
$ gh pr list -R cli/cli --label nope
no pull requests match your search in cli/cli        # stderr, exit 0
$ gh run list -R x/y --status queued
no runs found
```
Implemented as `cmdutil.NoResultsError` — `ghcmd` prints it to stderr only `if IsStdoutTTY()` and
returns `exitOK` ("no results is not a command failure").

**Mutation feedback** goes to **stderr** with an icon; the *data* (URL) goes to stdout:
`gh pr create` → `fmt.Fprintln(opts.IO.Out, pr.URL)`; `gh pr close` →
`✓ Closed pull request cli/cli#123 (title)` on ErrOut (note: check icon colored **red** for
close/delete — `cs.SuccessIconWithColor(cs.Red)`; Primer: "Use checks for success of closing or
deleting / Don't use alerts when closing or deleting"). Warnings: `! Skipped deleting the remote
branch...`, `Warning: 1 uncommitted change`. Browser: `Opening github.com/cli/cli/pull/1 in your
browser.` on stderr.

**stdout vs stderr rule** (command-development.md): "Keep data on the command's established stdout
path and diagnostics on its stderr path; do not merge streams or leak interactive decoration into
pipes." The `Showing 3 of 55 ...` list header is stdout in TTY, omitted when piped.

**Color palette:** Primer — "Terminals reliably recognize the 8 basic ANSI colors… bright versions
less reliably… Some terminals do not reliably support 256-color." gh uses green/red/yellow/cyan/
bold + one 256-color gray (`38;5;242`) for muted, and offers `GH_ACCESSIBLE_COLORS` to fall back
to 4-bit. Semantic use: cyan = branch/ID, bold = repo/title, gray = muted/labels/time, green/red/
yellow = state.

## 5. Output (machine)

There is **no `-o json`**. The design is `--json <fields>` (required field list) + optional
`--jq <expr>` or `--template <go-template>`:

- `--json` with no argument or an unknown field prints the field list (exit 1):
  ```
  $ gh run list --json
  Specify one or more comma-separated fields for `--json`:
    attempt
    conclusion
    createdAt
    ...
  $ gh pr list --json bogus
  Unknown JSON field: "bogus"
  Available fields:
    additions
    ...
  ```
  The same list is printed in every `--help` under a `JSON FIELDS` section.
- Shape is **curated**, camelCase, per-command (`pr list` and `pr view` share one field set; `run
  list` has 16 fields, `run view` adds `jobs`). Lists are a **bare array**; single objects are a
  bare object. Only requested fields are emitted, sorted alphabetically:
  ```
  $ gh run list -R cli/cli --limit 2 --json status,conclusion,name,databaseId
  [{"conclusion":"success","databaseId":34796468421,"name":"Triage Scheduled Tasks","status":"completed"}, ...]
  ```
  Compact when piped; docs say pretty-printed on a terminal. Empty list → `[]`.
- `--jq` is built in (gojq): "The jq utility does not need to be installed". `--jq` without
  `--json` → ``cannot use `--jq` without specifying `--json` `` (exit 1).
- `--template` gets extra funcs: `tablerow`, `tablerender`, `timeago`, `timefmt`, `truncate`,
  `color`, `autocolor` (color only on TTY), `hyperlink`, `join`, `pluck`, plus Sprig
  `contains/hasPrefix/hasSuffix/regexMatch`. Users can rebuild the human table themselves:
  ```
  --template '{{range .}}{{tablerow (printf "#%v" .number | autocolor "green") .title (timeago .updatedAt)}}{{end}}'
  ```
- `--json` forces exit 0 on `auth status` even with auth problems ("always exit with zero
  regardless of any authentication issues, unless there is a fatal error").
- `gh api` is the escape hatch for the full API object, with `--jq`, `--template`, `--paginate`,
  `--slurp`, `-i` (include headers), `--silent`, `--cache 1h`, `--verbose`.
- Every help page carries the same footer: `For more information about output formatting flags,
  see gh help formatting`.

## 6. TTY awareness

Primer scriptability, "Differences to note in machine output":
> No color or styling · State is explicitly written, not implied from color · Tabs between columns
> instead of table layout, since `cut` uses tabs as a delimiter · No truncation · Exact date
> format · No header

Observed:
```
$ gh pr list -R cli/cli --limit 2 | cat
14373^Idocs: Add step to install working version on Ubuntu Pro^Iacoulton:patch-1^IOPEN^I2026-09-07T10:43:26Z
14355^IRemove obsolete Git test seams^Iwilliammartin-clean-git-test-seams^IDRAFT^I2026-09-04T15:50:28Z

$ gh run list -R cli/cli --limit 1 | cat
completed^Isuccess^ITriage Scheduled Tasks^ITriage Scheduled Tasks^Itrunk^Ischedule^I34796468421^I13s^I2026-09-14T01:36:27Z
```
Concrete deltas: `#14373` → `14373`; a **STATE column appears** (`OPEN`/`DRAFT`) that in TTY was
carried by color; the single symbol column becomes **two columns** `status` + `conclusion`
(`completed`, `success`); `about 6 days ago` → RFC3339; no header row; no "Showing N of M"; no
truncation; no color. Source: `tableprinter.AddTimeField` — "In TTY mode displays the fuzzy time
difference… In non-TTY mode it just displays t with the time.RFC3339 format", and in
`run/list.go`: `if tp.IsTTY() { tp.AddField(symbol, WithColor(...)) } else { tp.AddField(status);
tp.AddField(conclusion) }`.

Controls: `NO_COLOR` (any value), `CLICOLOR=0`, `CLICOLOR_FORCE=1` (color even when piped),
`GH_FORCE_TTY=<cols|percent>` (terminal layout when redirected, with a width), `GH_PAGER`/`PAGER`
(pager used for long output like `pr view`, `--json` output not paged), `GH_PROMPT_DISABLED`,
`GH_SPINNER_DISABLED` ("replace the spinner animation with a textual progress indicator"),
`GH_MDWIDTH`. No `--color` flag at all; no `--no-color` flag; env only.

**AI-agent detection (new, worth noting):** `internal/agents/detect.go` recognises Claude Code,
Codex, Copilot CLI, Gemini CLI, Cursor, Amp, opencode, goose, kiro… via env vars (`AI_AGENT`,
`CLAUDECODE`, `CODEX_SANDBOX`, `COPILOT_CLI`, …). When an agent is detected gh (a) disables the
spinner and (b) on a flag/usage error prints the **full help** (examples + JSON FIELDS) to stderr
instead of the terse usage: "giving AI agents the examples, JSON fields and environment variables
they need to correct themselves without a second round trip."

## 7. Streaming & long-running

- `gh run watch <run-id> [--interval 3] [--exit-status] [--compact]`: TTY uses the alternate screen
  buffer (`ESC[?1049h`, `ESC[0;0H`, `ESC[J`) and repaints the whole `run view` every N seconds:
  ```
  Refreshing run status every 5 seconds. Press Ctrl+C to quit.

  * rollup-EpKtQu5 CI rust-lang/rust#162742 · 34794670924
  Triggered via pull_request about 40 minutes ago

  JOBS
  ✓ Calculate job matrix in 34s (ID 103825353833)
    ✓ Set up job
    - Test citool
  * PR - test-aarch64-gnu-llvm-21-1 (ID 103825455061)
    ✓ Set up job
    - Install cargo in AWS CodeBuild
  ```
  Piped: same text, no alt-screen, simply appended each tick. `--compact` = "Show only relevant/
  failed steps". Exit: 0 when done unless `--exit-status` ("Exit with non-zero status if run fails")
  → then the *watched thing's* failure becomes exit 1. Example in help:
  `$ gh run watch && notify-send 'run is done!'`.
- `gh pr checks --watch [--interval 10] [--fail-fast]`: same pattern; `--fail-fast` = "Exit watch
  mode on first check failure".
- `gh run view --exit-status`: "Exit with non-zero status if run failed"; example
  `gh run view 0451 --exit-status && echo "run pending or passed"`. Exit code **8** (`exitPending`,
  `cmdutil.PendingError`) is reserved for "Outcome pending, not a failure" (used by `pr checks`).
- Logs: `gh run view <id> --log` (whole run) / `--log --job <job-id>` / `--log-failed` ("View the
  log for any failed steps"). Log lines are tab-separated `job<TAB>step<TAB>timestamped line`:
  ```
  bump-go	UNKNOWN STEP	2026-09-13T03:19:30.6037158Z ##[group]Runner Image Provisioner
  ```
  No `--follow`, no `--since`, no `--tail` (logs are fetched post-hoc as a zip; streaming is not
  supported). `gh run view` without `--log` always ends with the hint
  `To see what failed, try: gh run view <id> --log-failed`.
- `-w, --web` on nearly every noun: "Open … in the browser" (`pr view -w`, `pr list -w`, `run view
  -w`, `pr checks -w`, `gh browse [<number>|<path>|<commit-sha>]`). Primer principle: "Bias towards
  terminal, but make it easy to get to the browser… Many commands output the relevant URL at the
  end."
- Spinners (Primer components/progress): "For processes that might take a while, include a
  progress indicator with context on what's happening." gh: `gh config set spinner disabled`,
  `GH_SPINNER_DISABLED`, and auto-disabled for AI agents.
- Cancellation: Ctrl-C → exit **2**, gh prints `\n` to stderr "to ensure the next shell prompt
  will start on its own line".

## 8. Status / health / metrics

- `gh status`: a cross-repo dashboard, two-column box layout (`│` separators) with sections
  `Assigned Issues | Assigned Pull Requests`, `Review Requests | Mentions`, `Repository Activity`;
  empty cells say `Nothing here ^_^`. Flags `-o org`, `-e repo` (exclude, repeatable).
- `gh pr status`: sectioned, indented, sentence-style empties:
  ```
  Relevant pull requests in admiral-io/admiral-cli

  Current branch
    There is no pull request associated with [martin/pats]

  Created by you
    You have no open pull requests
  ```
- `gh run list -s/--status` enumerates every state in the help:
  `{queued|completed|in_progress|requested|waiting|pending|action_required|cancelled|failure|neutral|skipped|stale|startup_failure|success|timed_out}`
  (mixes *status* and *conclusion* into one filter for convenience; JSON keeps them separate as
  `status` + `conclusion`).
- No metrics/top commands. Durations `1m3s`, `4m50s`; timestamps relative in TTY, RFC3339 piped.
- Refresh intervals are seconds, flag `-i, --interval int` (default 3 for `run watch`, 10 for
  `pr checks --watch`).

## 9. Errors & exit codes

`gh help exit-codes`: 0 success · 1 any failure · 2 cancelled · 4 requires authentication;
source adds **8 = pending** (`exitPending`). "It is possible that a particular command may have
more exit codes". Also: `NoResultsError` → 0; extension exit codes pass through.

Error message style — **lowercase, no `error:` prefix, on stderr**, often followed by a hint line:
```
unknown flag: --bogus                          # + usage block (FlagError)
unknown command "prr" for "gh"

Did you mean this?
	org
	pr
invalid argument "bogus" for "-s, --state" flag: valid values are {open|closed|merged|all}
cannot use `--jq` without specifying `--json`
--yes required when not running interactively
HTTP 401: Bad credentials (https://api.github.com/graphql)
Try authenticating with:  gh auth login -h github.com
GraphQL: Could not resolve to a PullRequest with the number of 999999. (repository.pullRequest)
GraphQL: Could not resolve to a Repository with the name 'cli/doesnotexist-xyz'. (repository)
error connecting to api.github.com
check your internet connection or https://githubstatus.com
```
Rules from `printError`: print `err`; if it's a `FlagError` or "unknown command", append the usage
string (full help if an AI agent is driving); 401 → append `Try authenticating with:  <cmd>`;
SAML → `Authorize in your browser:  <url>`; missing scopes → suggestion. Not-found is surfaced
as the raw API error (a widely-noted wart — see §13). Domain-level failures use the icon prefix on
stderr: `X Pull request cli/cli#14373 is not mergeable: the base branch policy prohibits the
merge.` followed by a remedy sentence.

Error helper taxonomy (`pkg/cmdutil/errors.go`): `FlagErrorf` (invalid flags → shows usage),
`MutuallyExclusive`, `SilentError` (exit 1, no message), `CancelError`, `PendingError`,
`NoResultsError`.

Verbose: `GH_DEBUG=1` (verbose to stderr) / `GH_DEBUG=api` (adds HTTP traces with token redacted:
`> Authorization: token ████████`). There is no global `--verbose`; `gh api --verbose` and
`gh run view -v/--verbose` ("Show job steps") are local.

## 10. Pagination & filtering

- `-L, --limit int  Maximum number of items to fetch (default 30)` (20 for `run list`). gh pages
  internally up to the limit; **there is no page-token flag** on list commands. Full pagination
  only via `gh api --paginate [--slurp]`.
- Filters are named flags with enum defaults: `-s, --state {open|closed|merged|all} (default
  "open")`, `-a/--assignee`, `-A/--author`, `-l/--label` (repeatable, AND), `-B/--base`,
  `-H/--head`, `-d/--draft`, `--app`; `run list`: `-b/--branch`, `-c/--commit SHA`,
  `-e/--event`, `-u/--user`, `-w/--workflow`, `--created date`, `-a/--all` (include disabled).
- Free-form query escape hatch: `-S, --search query` using GitHub search syntax
  (`--search "status:success review:required"`).
- Client-side shaping is left to `--jq`.
- Sorting: none exposed except through `--search "sort:updated-desc"`.

## 11. Help text style

Structure (cobra template customised in `pkg/cmd/root/help.go`):
```
<Long description paragraphs, sentence case, backticked flags, may end with
"For more information about output formatting flags, see `gh help formatting`.">

USAGE
  gh pr view [<number> | <url> | <branch>] [flags]

ALIASES
  gh pr ls

FLAGS
  -c, --comments          View pull request comments
  -q, --jq expression     Filter JSON output using a jq expression
      --json fields       Output JSON with the specified fields
  -w, --web               Open a pull request in the browser

INHERITED FLAGS
      --help                     Show help for command
  -R, --repo [HOST/]OWNER/REPO   Select another repository using the [HOST/]OWNER/REPO format

JSON FIELDS
  additions, assignees, author, ...

EXAMPLES
  # View a specific run
  $ gh run view 12345

  # Exit non-zero if a run failed
  $ gh run view 0451 --exit-status && echo "run pending or passed"

LEARN MORE
  Use `gh <command> <subcommand> --help` for more information about a command.
  Read the manual at https://cli.github.com/manual
  Learn about exit codes using `gh help exit-codes`
```
- Section headers UPPERCASE. Root help groups commands: `CORE COMMANDS`, `GITHUB ACTIONS COMMANDS`,
  `ALIAS COMMANDS`, `ADDITIONAL COMMANDS`, `HELP TOPICS`; each entry `name:  Short` (colon after
  the name, aligned).
- Short descriptions: imperative, sentence case, **no trailing period** (`List pull requests`,
  `Manage gists`, `View details about workflow runs`, `Make an authenticated GitHub API request`).
  Exceptions with periods exist (`Work with GitHub Projects.`) and read as mistakes. Flag
  descriptions likewise: capitalised, no period, enum choices inline as `{a|b|c}`, default shown
  by pflag `(default "open")`. Preview features tagged `(preview)`.
- Examples: `# comment` line then `$ gh ...` (heredoc in source, per command-development.md: "Use
  `heredoc.Doc` for command examples, with `#` explanatory comment lines and `$ ` command
  prefixes"). Trailing shell idioms are shown (`&& echo`, `#=>`).
- Help topics are first-class commands: `gh help formatting|exit-codes|environment|reference|
  accessibility|actions|mintty|telemetry`; `gh help reference` dumps the whole tree as markdown.
- Primer components/help: required sections "Usage, Core commands, Flags, Learn more, Inherited
  flags"; optional "Additional commands, Examples, Arguments, Feedback". Language: "Use sentence
  case / Don't use title case"; "Use language accurate to GitHub.com".

## 12. Config & auth

- Config dir `GH_CONFIG_DIR` → `$XDG_CONFIG_HOME/gh` → `~/.config/gh`; `config.yml` (settings) +
  `hosts.yml` (per-host accounts). `gh config set <key> <value> [--host h]`, `get`, `list`,
  `clear-cache`. Keys are documented in `gh config --help` with type and default:
  `git_protocol {https|ssh}`, `editor`, `prompt {enabled|disabled}`, `pager`, `browser`,
  `spinner`, `accessible_colors`, `telemetry {enabled|disabled|log}`.
- Precedence: env var > flag? No — **flag > env > config > inferred**, e.g. `-R` > `GH_REPO` >
  git remote; `GH_PAGER` > config `pager` > `PAGER`; `GH_TOKEN` > `GITHUB_TOKEN` > stored
  credentials ("takes precedence over previously stored credentials").
- Multi-account per host, `gh auth switch`; `gh auth status` prints per host with `✓`/`X` and
  exits 1 if any account is broken (unless `--json`):
  ```
  github.com
    ✓ Logged in to github.com account mberwanger (GITHUB_TOKEN)
    - Active account: true
    - Git operations protocol: https
    - Token: ghp_************************************
    - Token scopes: 'repo', 'workflow', ...
  ```
  Token source is named in parentheses (`(GITHUB_TOKEN)` vs `(keyring)`), token masked unless
  `-t/--show-token`.
- `gh auth login --with-token < token.txt` for non-interactive; `gh auth token` prints the token.
- `gh alias set <name> '<expansion>'` with `$1` params and `--shell`; user-level customisation is
  a documented design lever (Primer "Customizability").

## 13. Notable / unique; widely-regarded mistakes

Worth stealing:
- **`--json` field-list discovery** (empty `--json` prints the fields; `JSON FIELDS` in help; unknown
  field lists all valid ones). Curated camelCase shape, bare array.
- **`--exit-status`** to make "the status of the thing I looked at" the process exit code, opt-in,
  and a separate **exit 8 = pending**.
- **Two columns when piped where TTY had one symbol** (`status`, `conclusion`), and the STATE word
  appearing only when piped.
- **"To see what failed, try: …"** next-step hints and URL at the bottom of every detail view.
- **Empty list → stderr message, exit 0, `[]` in JSON.**
- `--yes` ignored when the destructive target was not given explicitly.
- Semantic flag type names in help (`fields`, `expression`, `login`, `[HOST/]OWNER/REPO`).
- AI-agent detection → full help on usage errors, spinner off.
- `GH_FORCE_TTY=120` for testing TTY rendering in CI.
- `hyperlink`/`autocolor` template funcs; `timeago`.

Widely-regarded mistakes / gaps (from issues + observed):
- Not-found errors leak raw GraphQL text (`GraphQL: Could not resolve to a PullRequest with the
  number of 999999. (repository.pullRequest)`) instead of `pull request #999999 not found in
  cli/cli`.
- No `-o json` / no way to get the whole object without enumerating fields; `--json` list is per
  command and sometimes incomplete, pushing users to `gh api`.
- `run list --status` conflates status and conclusion in one enum.
- `X` (letter) for failure looks like a column value in tab output and is inconsistent with
  Primer's `✗`.
- No page tokens on list commands: `--limit 5000` just fetches everything.
- No log streaming (`--follow`) for runs; `run watch` repaints the full view (heavy on long jobs;
  `--compact` was added later to mitigate).
- Inconsistent trailing periods in a few Shorts (`Work with GitHub Projects.`) and flag descs
  (`Set the new title.`).
- `--web` is `-w` but `--workflow` is also `-w` on `run list` — short-letter collisions across
  siblings.

---

## Lessons for Admiral

1. **Keep noun→verb, two levels, parent as flag** (gh: `gh run view 123 --job 456`, `-R`). Never
   add a third noun level; reach sub-resources with flags on the verb (`admiral run logs <id>
   --phase apply`, `admiral run get <id> --revisions`). Inherit `--app/--env` on the parent command
   so they show under `INHERITED FLAGS` once.
2. **Scope precedence = flag > env > config > inferred.** Add `ADMIRAL_APP` / `ADMIRAL_ENV` env
   vars (gh: `GH_REPO`, `GH_HOST`) so CI scripts don't repeat `--app shop --env prod`; error
   plainly when scope can't be resolved (`no application specified; pass --app or set ADMIRAL_APP`).
3. **Piped tables: no header, tab-separated, no truncation, no color, RFC3339 times, state as a
   word.** Copy gh's `tableprinter` contract exactly (`AddTimeField`: fuzzy in TTY, RFC3339 piped).
   Where TTY shows a status *symbol*, piped output should show `status<TAB>conclusion` words.
4. **Status symbols:** `✓` green success, `X`/`✗` red failure, `*` yellow in-progress, `-` gray
   skipped/neutral, `!` yellow warning; one-character `STATUS` column first. Color enhances, never
   carries, meaning (Primer).
5. **Detail views end with a hint and a URL** (gh `run view`): `To see what failed, try: admiral
   run logs <id> --failed` and `View this run in Admiral: https://…`. Use UPPERCASE section headers
   (`JOBS`, `ANNOTATIONS` → `REVISIONS`, `PHASES`), 2-space indent for children.
6. **Empty lists: message to stderr, exit 0; JSON emits `[]`** (matches existing Admiral memory
   "No <resource> found." — keep it; gh proves the exit-0 convention).
7. **Add `--exit-status` to `run get`/`run watch`** (gh `run view/watch --exit-status`) rather than
   making a failed run fail the CLI by default; reserve a distinct exit code (gh: 8) for "still
   pending" so `--wait --timeout` scripts can branch.
8. **Exit codes: 0 ok, 1 error, 2 cancelled (Ctrl-C; print `\n` to stderr first), 4 auth required**,
   plus 8 pending. Document them in `admiral help exit-codes` and link from every help footer.
9. **Error text: lowercase, no `error:` prefix, one line, then an indented remedy line** — gh's
   `Try authenticating with:  admiral auth login`, `Did you mean this?`, `check your internet
   connection or https://status…`. Do better than gh on not-found: `run "abc123" not found in
   shop/prod`.
10. **Keep `-o json|yaml` but add `--jq` (embedded gojq) and `--template`** with gh's helper set
    (`tablerow`, `timeago`, `truncate`, `autocolor`, `hyperlink`). gh's `--json fields` discovery
    is worth copying as `-o json --fields a,b` or at minimum a `JSON FIELDS` section in help.
    Refuse `--jq` without a JSON output mode with gh's exact wording.
11. **Every prompt has a flag; every destructive verb has `--force`/`-f` (Admiral) ≡ gh `--yes`,
    and a non-interactive guard**: `--force required when not running interactively`. Support
    `ADMIRAL_PROMPT_DISABLED` (gh: `GH_PROMPT_DISABLED`) and `admiral config set prompt disabled`.
    Copy gh's safety rule: ignore `--force` when the target name was not given explicitly.
12. **Set vs clear:** prefer paired flags (`--add-label/--remove-label`, `--remove-milestone`) over
    `--label ""`; track omitted-vs-empty with nil-able flags (gh `NilStringFlag`).
13. **`-w/--web` on every `get`/`list`/`run` verb**, printing `Opening <url> in your browser.` to
    stderr; print the resource URL as the last line of create/apply output on **stdout**
    (`gh pr create` prints only the URL to stdout).
14. **Watching:** `admiral run watch <id> [-i 3] [--compact] [--exit-status]` repainting the `run
    get` view; `--compact` = only active/failed steps. Piped mode: append snapshots, no alt-screen.
    Unlike gh, do offer `run logs -f/--follow` (gh's lack of streaming is its biggest Actions gap).
15. **Help text:** UPPERCASE sections (`USAGE`, `ALIASES`, `FLAGS`, `INHERITED FLAGS`, `JSON
    FIELDS`, `EXAMPLES`, `LEARN MORE`); Shorts imperative, sentence case, no trailing period;
    flag descriptions capitalised, no period, enums inline `{plan|apply|all}`; semantic value
    names (`--app name`, `--jq expression`, `--fields fields`); examples as `# comment` +
    `$ admiral …`. Group root help (`CORE COMMANDS` / `RUNNER COMMANDS` / `ADDITIONAL COMMANDS` /
    `HELP TOPICS`).
16. **Env surface named after the tool**: `ADMIRAL_TOKEN`, `ADMIRAL_HOST`, `ADMIRAL_APP`,
    `ADMIRAL_ENV`, `ADMIRAL_DEBUG[=api]`, `ADMIRAL_PAGER`, `ADMIRAL_CONFIG_DIR`,
    `ADMIRAL_FORCE_TTY`, `ADMIRAL_SPINNER_DISABLED`; honour `NO_COLOR`, `CLICOLOR_FORCE`.
    Document all in `admiral help environment`.
17. **Detect AI agents** (gh `internal/agents/detect.go`: `CLAUDECODE`, `CODEX_*`, `COPILOT_CLI`,
    `AI_AGENT`…): disable spinners and print full help (examples + JSON fields) on usage errors.
    Admiral explicitly targets agent surfaces; this is cheap and high-leverage.
18. **`admiral status`** as a cross-app dashboard (gh `status` / `pr status`): sectioned, sentence-
    style empties (`You have no runs in progress`), scoped by `--app`/`--org`.
19. **Pagination:** keep `--page-size/--page-token`, but add gh's `-L/--limit` as the primary
    human-facing knob with a small default (20–30) and let the CLI page internally; print the
    NEXT PAGE TOKEN hint to stderr only in TTY mode (gh pattern for out-of-band context).
20. **Auth status output** (gh): per-host block, `✓ Logged in to … as … (ADMIRAL_TOKEN)` naming the
    credential source, token masked unless `--show-token`, exit 1 if broken unless `-o json`.
