# Published CLI design guidelines — what they say, where they agree, what Admiral should adopt

Scope: the widely-cited *guidelines* (not specific tools). Each section quotes the source
verbatim where the rule is crisp. Part B is a topic matrix across sources; Part C is the
opinionated rule set for Admiral. Sources that could not be fetched directly are noted.

Sources fetched (2026-09-13):

| # | Source | URL | Notes |
|---|---|---|---|
| 1 | Command Line Interface Guidelines (clig.dev) | https://clig.dev/ | Full text |
| 2 | 12 Factor CLI Apps, Jeff Dickey (Heroku/oclif) | https://medium.com/@jdxcode/12-factor-cli-apps-dd3c227a0e46 | Medium 403; read via mirrors github.com/flameddd/blog and panlw.github.io |
| 3 | Heroku CLI Style Guide | https://devcenter.heroku.com/articles/cli-style-guide | Full text |
| 4 | GNU Coding Standards §4.4 Errors, §4.8 CLI, --version, --help, Option Table | https://www.gnu.org/prep/standards/html_node/Command_002dLine-Interfaces.html | Full text |
| 4b | POSIX.1-2017 XBD §12 Utility Conventions | https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap12.html | Full text |
| 5 | NO_COLOR; CLICOLOR/CLICOLOR_FORCE | https://no-color.org/ ; https://bixense.com/clicolors/ | Full text |
| 6 | Azure CLI command guidelines; Microsoft command-line syntax key; gcloud command conventions + wide flags | github.com/Azure/azure-cli/…/command_guidelines.md ; learn.microsoft.com/…/command-line-syntax-key ; docs.cloud.google.com/sdk/gcloud/reference/topic/command-conventions | Full text |
| 7 | Charm (Lip Gloss / Bubble Tea READMEs) | github.com/charmbracelet/{lipgloss,bubbletea} | READMEs; blog index only |
| 8 | cobra v1.9.1 (user guide + `command.go` source in local module cache); kong README; urfave/cli v3 docs | github.com/spf13/cobra | Source-verified |
| 9 | XDG Base Directory spec | https://specifications.freedesktop.org/basedir/latest/ | Full text |
| 10 | sysexits(3) FreeBSD; bash manual "Exit Status"; Python argparse; gh exit codes | man.freebsd.org ; gnu.org/software/bash ; docs.python.org ; cli.github.com | bash page fetched from memory (gnu.org rate-limited) |
| 11 | JSON Lines; jc + "Bringing the Unix Philosophy to the 21st Century"; gh `--json` docs + maintainer rationale (cli/cli#385) | jsonlines.org ; kellyjonbrazil.github.io/jc ; cli.github.com/manual/gh_help_formatting | Full text; gh issue read via `gh api` |

---

## Part A — Source summaries

### A1. Command Line Interface Guidelines (clig.dev)

The canonical modern reference. Organised as Philosophy + Guidelines. Key rules, quoted:

**Philosophy**
- "if a command is going to be used primarily by humans, it should be designed for humans first."
- "Where possible, a CLI should follow patterns that already exist."
- "Software should *be* robust: unexpected input should be handled gracefully, operations should be idempotent where possible."
- "Abandon a standard when it is demonstrably harmful to productivity or user satisfaction." (Raskin, quoted)

**The Basics**
- "Use a command-line argument parsing library where you can."
- "Return zero exit code on success, non-zero on failure. Exit codes are how scripts determine whether a program succeeded or failed." (no finer-grained scheme is prescribed)
- "Send output to `stdout`. The primary output for your command should go to `stdout`."
- "Send messaging to `stderr`. Log messages, errors, and so on should all be sent to `stderr`."

**Help**
- "Display help when passed `-h` or `--help` flags. This also applies to subcommands which might have their own help text."
- "When `myapp` or `myapp subcommand` requires arguments to function, and is run with no arguments, display concise help text." Concise help = "A description of what your program does. One or two example invocations. Descriptions of flags, unless there are lots of them."
- "Provide a support path for feedback and issues. A website or GitHub link in the top-level help text is common."
- "Lead with examples." — "Users tend to use examples over other forms of documentation, so show them first in the help page."
- "Display the most common flags and commands at the start of the help text."
- "Use formatting in your help text. Bold headings make it much easier to scan."
- "If the user did something wrong and you can guess what they meant, suggest it."
- "If your command is expecting to have something piped to it and `stdin` is an interactive terminal, display help immediately and quit."

**Output**
- "Human-readable output is paramount. Humans come first, machines second."
- "The most simple and straightforward heuristic for whether a particular output stream is being read by a human is *whether or not it's a TTY*."
- "Have machine-readable output where it does not impact usability." / "A user should be able to pipe output to `grep` and it should do what they expect."
- "If human-readable output breaks machine-readable output, use `--plain` to display output in plain, tabular text format for integration with tools like `grep` or `awk`."
- "Display output as formatted JSON if `--json` is passed. JSON allows for more structure than plain text."
- "Display output on success, but keep it brief. It's rare that printing nothing at all is the best default behavior."
- "Provide a `-q` option to suppress all non-essential output."
- "If you change state, tell the user."
- "Suggest commands the user should run. When several commands form a workflow, suggesting to the user commands they can run next helps them learn."
- "Use color with intention… Don't overuse it—if everything is a different color, then the color means nothing."
- "Disable color if: `stdout` or `stderr` is not an interactive terminal (a TTY). The `NO_COLOR` environment variable is set and it is not empty. The `TERM` environment variable has the value `dumb`. The user passes the option `--no-color`."
- "If `stdout` is not an interactive terminal, don't display any animations. This will stop progress bars turning into Christmas trees in CI log output."
- "Don't treat `stderr` like a log file, at least not by default. Don't print log level labels (`ERR`, `WARN`, etc.) or extraneous contextual information."
- "Use a pager (e.g. `less`) if you are outputting a lot of text." / "A good sensible set of options to use for `less` is `less -FIRX`."

**Errors**
- "Catch errors and rewrite them for humans." Example: "Can't write to file.txt. You might need to make it writable by running `chmod +w file.txt`."
- "Signal-to-noise ratio is crucial." / "If your program produces multiple errors of the same type, consider grouping them under a single explanatory header instead of printing many similar-looking lines."
- "Put the most important information at the end of the output. The eye will be drawn to red text, so use it intentionally and sparingly."
- "If there is an unexpected or unexplainable error, provide debug and traceback information, and instructions on how to submit a bug."
- "Make it effortless to submit bug reports."

**Arguments and flags**
- "Prefer flags to args. It's a bit more typing, but it makes it much clearer what is going on."
- "Have full-length versions of all flags." / "Only use one-letter flags for commonly used flags, particularly at the top-level when using subcommands."
- "Multiple arguments are fine for simple actions against multiple files." / "If you've got two or more arguments for different things, you're probably doing something wrong." (exception: `cp <source> <destination>`)
- Standard flag table: `-a/--all`, `-d/--debug`, `-f/--force`, `--json`, `-h/--help` ("This should only mean help"), `-n/--dry-run`, `--no-input`, `-o/--output` ("Output file"), `-p/--port`, `-q/--quiet`, `-u/--user`, `--version`, `-v` ("This can often mean either verbose or version").
- "Make the default the right thing for most users."
- "Prompt for user input. If a user doesn't pass an argument or flag, prompt for it." / "Never *require* a prompt. Always provide a way of passing input with flags or arguments. If `stdin` is not an interactive terminal, skip prompting and just require those flags/args."
- "Confirm before doing anything dangerous." Three tiers — Mild ("deleting a file. You might want to prompt for confirmation, you might not"), Moderate ("a remote change like deleting a resource of some kind"), Severe ("Deleting something complex, like an entire remote application or server. You don't just want to prompt for confirmation here—you want to make it hard to confirm by accident"). "A common convention is to prompt for the user to type `y` or `yes` if running interactively, or requiring them to pass `-f` or `--force` otherwise."
- "If input or output is a file, support `-` to read from `stdin` or write to `stdout`."
- "If a flag can accept an optional value, allow a special word like 'none'. Don't just use a blank value."
- "If possible, make arguments, flags and subcommands order-independent."
- "Do not read secrets directly from flags… Consider accepting sensitive data only via files, e.g. with a `--password-file` flag, or via `stdin`."

**Interactivity**
- "Only use prompts or interactive elements if `stdin` is an interactive terminal (a TTY)."
- "If `--no-input` is passed, don't prompt or do anything interactive. … If the command requires input, fail and tell the user how to pass the information as a flag."
- "If you're prompting for a password, don't print it as the user types."
- "Let the user escape. Make it clear how to get out. (Don't do what vim does.) If your program hangs on network I/O etc, always make Ctrl-C still work."

**Subcommands**
- "Be consistent across subcommands. Use the same flag names for the same things, have similar output formatting, etc."
- "Use consistent names for multiple levels of subcommand." / "Either `noun verb` or `verb noun` ordering works, but `noun verb` seems to be more common."
- "Don't have ambiguous or similarly-named commands." (e.g. `update` vs `upgrade`)
- "Don't have a catch-all subcommand." / "Don't allow arbitrary abbreviations of subcommands." / "There's nothing wrong with aliases… but they should be explicit and remain stable."

**Robustness**
- "Make things time out. Allow network timeouts to be configured, and have a reasonable default so it doesn't hang forever."
- "Responsive is more important than fast. Print something to the user in <100ms."
- "Show progress if something takes a long time."
- "Make it recoverable. … you should be able to hit `<up>` and `<enter>` and it should pick up from where it left off."
- "Make it crash-only."
- "Validate user input."

**Future-proofing**
- "Keep changes additive where you can."
- "Warn before you make a non-additive change. … If possible, you should detect when they've changed their usage and not show the warning any more."
- "Changing output for humans is usually OK. … Encourage your users to use `--plain` or `--json` in scripts to keep output stable."
- "Don't create a 'time bomb.'"

**Signals**
- "If a user hits Ctrl-C (the INT signal), exit as soon as possible. Say something immediately, before you start clean-up. Add a timeout to any clean-up code so it can't hang forever."
- "If a user hits Ctrl-C during clean-up operations that might take a long time, skip them. Tell the user what will happen when they hit Ctrl-C again, in case it is a destructive action."
- "Your program should expect to be started in a situation where clean-up has not been run."

**Configuration**
- Precedence, highest first: "Flags … The running shell's environment variables … Project-level configuration (e.g. `.env`) … User-level configuration … System wide configuration".
- "Follow the XDG-spec."
- "If you automatically modify configuration that is not your program's, ask the user for consent."

**Environment variables**
- "Environment variables are for behavior that *varies with the context* in which a command is run."
- "For maximum portability, environment variable names must only contain uppercase letters, numbers, and underscores (and mustn't start with a number)."
- "Aim for single-line environment variable values."
- "Avoid commandeering widely used names."
- "Check general-purpose environment variables for configuration values when possible: `NO_COLOR`… `DEBUG`… `EDITOR`… `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY` and `NO_PROXY`… `SHELL`… `TERM`… `TMPDIR`… `HOME`… `PAGER`… `LINES` and `COLUMNS`".
- "Do not read secrets from environment variables. … Secrets should only be accepted via credential files, pipes, `AF_UNIX` sockets, secret management services, or another IPC mechanism." (Note: this is the one clig.dev rule that most real CLIs — gh, kubectl, az, aws — ignore; `GH_TOKEN`-style vars are ubiquitous in CI.)

**Naming / Distribution / Analytics**
- "Use only lowercase letters, and dashes if you really need to." / "Keep it short." / "Make it easy to type."
- "If possible, distribute as a single binary." / "Make it easy to uninstall."
- "Do not phone home usage or crash data without consent."

### A2. 12 Factor CLI Apps (Jeff Dickey, Heroku/oclif)

The 12 factors (via mirrors; quotes are the author's):

1. **Great help is essential** — help via `mycli`, `mycli --help`, `mycli help`, `mycli -h`; reserve `-h,--help` for help only; "provide examples of common usage"; ship shell completion.
2. **Prefer flags to args** — "Flags require a bit more typing, but make the CLI much clearer." "A good rule of thumb is 1 type of argument is fine, 2 types are very suspect, and 3 are never good." Support `--` to stop parsing.
3. **What version am I on?** — `mycli version`, `mycli --version`, `mycli -V`; include diagnostics; "Send version string as User-Agent header for server-side debugging."
4. **Mind the streams** — "stdout is for output, stderr is for messaging." Errors/warnings go to stderr so they survive `> file`.
5. **Handle things going wrong** — an error should contain: "error code, error title, error description, how to fix, URL for more information"; unexpected errors get a traceback; `DEBUG` env var enables verbose logs; error logs should have timestamps and no ANSI codes.
6. **Be fancy!** — colors/dimming, spinners/progress bars; "If the user's stdout isn't connected to a tty… then don't display colors on stdout"; respect `TERM=dumb`, `NO_COLOR`, `--no-color`, plus an app-specific `MYAPP_NOCOLOR=1`.
7. **Prompt if you can** — "if stdin is a tty then prompt rather than forcing the user"; "Never require a prompt though. The user needs to be able to automate your CLI"; confirm destructive actions.
8. **Use tables** — one entry per row; "Never output table borders. It's noisy and a huge pain for parsing"; support `--columns`, `--no-truncate`, `--no-headers`, `--filter`, `--sort`; offer CSV and JSON.
9. **Be speedy** — "<100ms: very fast; 100–500ms: aim here; 500ms–2s: usable; 2s+: users will avoid"; "Even just a spinner will give the impression the CLI is much faster."
10. **Encourage contributions** — open source, README, contribution guide, plugin system.
11. **Be clear about subcommands** — single- vs multi-command CLIs; with no args, list subcommands; Heroku uses `topic:command` ("Colons are preferable to help delineate the command").
12. **Follow XDG-spec** — `~/.config/myapp` config, `~/.local/share/myapp` data, `~/.cache/myapp` cache (`~/Library/Caches/myapp` on macOS, `%LOCALAPPDATA%\myapp` on Windows); "respect environment variables like `XDG_CONFIG_HOME`".

### A3. Heroku CLI Style Guide

- Mission: "The Heroku CLI is for humans before machines."
- Naming: `heroku topic:command`; topics are plural nouns, commands are verbs; "Root topic command should list nouns, never create `*:list` commands" (`heroku config`, not `heroku config:list`); kebab-case only when unavoidable.
- Descriptions: "Fit on 80-character screens", "Begin with lowercase character", "No ending period"; flag descriptions likewise lowercase, concise, no period.
- Flags vs args: flags "allow any order", "enable better autocomplete", "Required flags improve error messages". Bad: `heroku fork destapp -a sourceapp`; good: `heroku fork --from sourceapp --to destapp`. Args OK when "only 1 argument OR arguments are obvious and in clear order".
- Prompting: "Always allow bypassing prompts with args/flags for scripting."
- Two command kinds: *output* commands (`this.log()` → stdout) and *action* commands (`cli.action()` → spinner on stderr): `Enabling maintenance mode for ⬢ myapp... done`.
- Streams: "Stdout: all output. Stderr: warnings, errors, out-of-band information (like `cli.action()`)."
- Color: "Suggested colors: magenta, cyan, blue, green, gray"; "Yellow/red reserved for errors and warnings"; "Disable with: `--no-color`, `COLOR=false`, or non-tty output"; "Avoid too many contrasting colors competing for attention".
- Tables: header row + rows, no borders, grep-parseable (`heroku regions | grep "Common Runtime"`).
- JSON: "Provide `--json` flag when data is too large for tables"; example `heroku releases --json | jq …` — output is a bare array.
- Backward compatibility: "Don't modify existing output after general availability (breaks scripts). Additional information is OK."

### A4. GNU Coding Standards + POSIX Utility Syntax Guidelines

**GNU §4.8 Command-Line Interfaces**
- "All programs should support two standard options: `--version` and `--help`."
- Define long options "equivalent to the single-letter Unix-style options"; long names should be "consistent from program to program" (e.g. verbose is spelled exactly `--verbose`); consult the Option Table.
- Input files as ordinary arguments; output "specified using options, preferably `-o` or `--output`".
- GNU getopt "will normally permit options anywhere among the arguments unless the special argument `--` is used" (a GNU extension over POSIX).

**GNU `--version`**: "print information about its name, version, origin and legal status, all on standard output, and then exit successfully." "The first line is meant to be easy for a program to parse; the version number proper starts after the last space." Format `GNU hello 2.3` / `emacsserver (GNU Emacs) 19.30`. "The program's name should be a constant string; don't compute it from `argv[0]`."

**GNU `--help`**: "output brief documentation for how to invoke the program, on standard output, then exit successfully. Other options and arguments should be ignored once this is seen." "Near the end of the `--help` option's output, please place lines giving the email address for bug reports, the package's home page."

**GNU §4.4 Formatting Error Messages** (the de-facto Unix grammar)
- Non-interactive programs: `program: message` or `program:sourcefile:lineno: message`.
- "The string *message* should not begin with a capital letter when it follows a program name and/or file name, because that isn't the beginning of a sentence. … Also, it should not end with a period."
- "Error messages from interactive programs, and other messages such as usage messages, should start with a capital letter. But they should not end with a period."
- "In an interactive program (one that is reading commands from a terminal), it is better not to include the program name in an error message."

**GNU Option Table** (common long names): `--help`, `--version`, `--verbose`, `--quiet`/`--silent` ("Every program accepting `--quiet` should accept `--silent` as a synonym"), `--force` (`-f` in cp/ln/mv/rm), `--interactive` (`-i` in cp/ln/mv/rm), `--recursive`, `--output`, `--dry-run` (`-n` in make), `--all`.

**POSIX XBD §12.2 Utility Syntax Guidelines** (14 rules; the important ones)
- G1/G2: names 2–9 chars, lowercase letters and digits.
- G3: "Each option name should be a single alphanumeric character"; `-W` reserved for vendor options.
- G5: grouping of short flags behind one `-`.
- G7: "Option-arguments should not be optional."
- G8: multiple option-arguments in one arg separated by commas or blanks.
- G9: "All options should precede operands on the command line." (GNU relaxes this.)
- G10: "The first `--` argument that is not an option-argument should be accepted as a delimiter indicating the end of options."
- G11: "The order of different options relative to one another should not matter… If an option that has option-arguments is repeated, the option and option-argument combinations should be interpreted in the order specified."
- G13: `-` operand means stdin/stdout.
- Notation: `[ ]` optional, `|` mutually exclusive, `...` one-or-more; "The arguments following the last options and option-arguments are named 'operands'."

### A5. NO_COLOR and CLICOLOR / CLICOLOR_FORCE

**no-color.org**: "Command-line software which adds ANSI color to its output by default should check for a `NO_COLOR` environment variable that, when present and not an empty string (regardless of its value), prevents the addition of ANSI color." Precedence: "User-level configuration files and per-instance command-line arguments should override the `NO_COLOR` environment variable." Scope: "This standard only signals the user's intention regarding adding ANSI color to text output" (bold/underline are not covered).

**bixense.com/clicolors**: three rules — `NO_COLOR` set → "Don't output ANSI color escape codes"; `CLICOLOR_FORCE` set (and `NO_COLOR` unset) → "ANSI colors should be enabled no matter what"; `CLICOLOR` set (others unset) → "ANSI colors are supported and should be used when the program is writing to a terminal". Reference logic:

```python
if os.environ.get('NO_COLOR'):        return False
elif os.environ.get('CLICOLOR_FORCE'): return True
elif os.environ.get('CLICOLOR'):       return sys.stdout.isatty()
```
(gh's doc adds the common refinement: `CLICOLOR=0` disables.)

### A6. Azure CLI guidelines, Microsoft syntax key, gcloud conventions

**Azure CLI `command_guidelines.md`**
- "Commands must follow a '[noun] [noun] [verb]' pattern"; "Multi-word subgroups should be hyphenated"; "avoid command subgroups that have no commands"; "If a command subgroup would only have a single command, move it into the parent command group".
- Standard verbs: `create` ("should be idempotent"), `update` ("selectively update properties… preserve existing values"), `set` ("replace all properties"), `show`, `list`, `delete` ("return nothing on success"), `wait` ("polls a GET endpoint until a condition is reached"). "don't use `get` or `new`" (confusable with standard types).
- Args: `-n/--name`, `-g/--resource-group`; "DO NOT put units in argument names. ALWAYS put the expected units in the help text" (`--duration`, not `--duration-in-minutes`); "Arguments that end in `-id` should be GUIDs".
- Output: "Commands must return an object, dictionary or `None`"; "Commands must support all output types (JSON, TSV, table)"; "Command output must go to `stdout`, everything else to `stderr`"; "Log to `logger.error()` or `logger.warning()`; do not use the `print()` function".
- Exit codes: `show` on a 404 "always returns exit code 3".
- Child collections: "expose a group of subcommands" with `add`/`remove`; a "JSON blob for these arguments" is "UNACCEPTABLE".

**Microsoft command-line syntax key**: literal text = type as shown; `<Text inside angle brackets>` = placeholder; `[…]` = optional; `{…}` = "Set of required items. You must choose one"; `|` = mutually exclusive; `…` = repeatable. gh's `docs/command-line-syntax.md` is the same key and adds "Use dash-case" for placeholder names: `<issue-number>`, `[<number> | <url>]`, `{view | create}`, `<pr-number>...`.

**gcloud command conventions** (`gcloud topic command-conventions`)
- Tree of groups; "Each command group typically contains a set of CRUD commands (`create`, `describe`, `list`, `update`, `delete`)". Form: `gcloud GROUP GROUP … COMMAND POSITIONAL … FLAG …`.
- "Flag names are lower case with a `--` prefix. Multi-word flags use `-` (dash)"; "All Boolean flags have a `--no-` prefix variant"; `--flag=value` or `--flag value`; "If a flag is repeated on the command line, then only the rightmost occurrence takes effect, no diagnostic is emitted."
- "The standard output is for explicit information requested by the command." / "The standard error is reserved for diagnostics."
- "Exit status `0` indicates success… Any other exit status indicates an error."
- Wide flags: `--quiet, -q` = "Disable all interactive prompts… If input is required, defaults will be used, or an error will be raised."; `--verbosity=debug|info|warning|error|critical|none` (default `warning`); `--format`; `--flags-file=YAML_FILE`; `--project`; `--user-output-enabled` / `--no-user-output-enabled`; `--log-http` "Log all HTTP server requests and responses to stderr."

### A7. Charm (Lip Gloss, Bubble Tea)

- Lip Gloss: "it can automatically downsample colors to the best available profile, stripping colors (and ANSI) entirely when output is not a TTY." Recommended: use its `Print/Fprint/Sprint` writer functions so degradation follows the destination.
- Bubble Tea: Elm-architecture TUI; "well-suited for simple and complex terminal applications, either inline, full-window, or a mix". Ctrl-C returns `tea.Quit`. Because "your TUI is busy occupying" stdout, debugging must go to a log file (`tea.LogToFile`). Implication: a TUI owns the terminal; it is *not* a substitute for plain, pipeable command output — keep TUIs (`watch`-style dashboards) behind an explicit command or TTY check and never for the default `get`/`list` path.

### A8. cobra (verified against `cobra@v1.9.1/command.go`), kong, urfave/cli

**cobra `Use`** — doc comment: "Use is the one-line usage message. Recommended syntax is as follows: `[ ]` identifies an optional argument. Arguments that are not enclosed in brackets are required. `...` indicates that you can specify multiple values for the previous argument. `|` indicates mutually exclusive information. `{ }` delimits a set of mutually exclusive arguments when one of the arguments is required." Example: `add [-F file | -D dir]... [-f format] profile`.
- `Short` = "the short description shown in the 'help' output"; `Long` = "the long message shown in the 'help <this-command>' output"; `Example` = "examples of how to use the command"; `Aliases`; `SuggestFor` ("similar to aliases but only suggests"); `GroupID` ("group id under which this subcommand is grouped in the 'help' output of its parent"); `Deprecated` ("should print this string when used"); `Hidden`; `Version` (adds `--version` and `-v` shorthand "if the command does not define one" — conflicts with `-v` verbose; Admiral already uses `-v` for verbose, so set `Version` only after defining `-v`).
- `SilenceErrors` = "quiet errors down stream"; `SilenceUsage` = "silence usage when an error occurs". Without them cobra prints `Error: <err>` to stderr and then the full usage block on *every* error, including runtime ones — the well-known anti-pattern. With both true (Admiral does this in `cmd/root.go:96-97`) the app must print the error itself.
- Unknown command path prints: `Error: unknown command "x" for "root"` + optional `Did you mean this?\n\tserver` + `Run 'root --help' for usage.` (Levenshtein distance ≤ 2; `DisableSuggestions`, `SuggestionsMinimumDistance`).
- Args validators: `NoArgs`, `ArbitraryArgs`, `ExactArgs(n)`, `MinimumNArgs`, `MaximumNArgs`, `RangeArgs`, `OnlyValidArgs`, `MatchAll`.
- Flag groups: `MarkFlagRequired`, `MarkFlagsRequiredTogether`, `MarkFlagsMutuallyExclusive`, `MarkFlagsOneRequired`.
- Default usage template sections, in order: `Usage:` / `Aliases:` / `Examples:` / `Available Commands:` (or per-group `{{.Title}}` + `Additional Commands:`) / `Flags:` / `Global Flags:` / `Additional help topics:` / `Use "<path> [command] --help" for more information about a command.` Note the template puts Examples *above* the command list — matches clig.dev's "lead with examples".
- Completion: `ValidArgsFunction`, `RegisterFlagCompletionFunc`, `ShellCompDirective*`; `SetHelpCommandGroupID`, `SetCompletionCommandGroupID` to file the built-ins under a group.

**kong** (contrast): declarative struct tags — `cmd:""`, `arg:""`, `help:""`, `env:"X,Y"` ("resolved in the declared order. The first value found is used"), `negatable:""` (adds `--no-flag`), `enum:"a,b"` ("An enum field must be required or have a valid default"), `xor:`/`and:` groups, `group:"X"` for help grouping, `hidden:""`, `aliases:"x,y"`, `${var}` interpolation into help text. Takeaway: env-var binding and negatable booleans are first-class in kong; cobra needs a viper/hand-rolled layer for env and has no `--no-` generation.

**urfave/cli v3** (contrast): `Sources: cli.EnvVars("APP_LANG")`, `cli.Files(...)`, `altsrc` YAML/TOML chains — "default values are set in the same order as they are defined in the `Sources` param"; command-line always wins. `cli.Exit(msg, code)` implements `ExitCoder` so an action chooses the exit code; `Category` for help grouping; help template headings are UPPERCASE (`NAME:`, `USAGE:`, `COMMANDS:`, `GLOBAL OPTIONS:`); `OnUsageError` hook.

### A9. XDG Base Directory Specification

- `$XDG_CONFIG_HOME` — "user-specific configuration files"; default `$HOME/.config`.
- `$XDG_DATA_HOME` — "user-specific data files"; default `$HOME/.local/share`.
- `$XDG_STATE_HOME` — "user-specific state files"; default `$HOME/.local/state`; for "state data that should persist between (application) restarts" e.g. "actions history (logs, history, recently used files, …)" and "current state of the application that can be reused on a restart".
- `$XDG_CACHE_HOME` — "user-specific non-essential data files"; default `$HOME/.cache`.
- `$XDG_RUNTIME_DIR` — "non-essential runtime files and other file objects (such as sockets, named pipes, ...)".
- `$XDG_CONFIG_DIRS` default `/etc/xdg`; `$XDG_DATA_DIRS` default `/usr/local/share/:/usr/share/`.
- "All paths set in these environment variables must be absolute. If an implementation encounters a relative path in any of these variables it should consider the path invalid and ignore it."
- Mapping for a CLI: config → CONFIG_HOME; credentials/sessions → CONFIG_HOME (gh) or DATA_HOME; last-used context, history → STATE_HOME; discovery/completion caches → CACHE_HOME.

### A10. Exit codes — sysexits(3), bash, argparse, gh, Azure

**sysexits.h (BSD, 1980s)**: 0 `EX_OK`; 64 `EX_USAGE` "The command was used incorrectly, e.g., with the wrong number of arguments, a bad flag, a bad syntax in a parameter"; 65 `EX_DATAERR`; 66 `EX_NOINPUT`; 67 `EX_NOUSER`; 68 `EX_NOHOST`; 69 `EX_UNAVAILABLE` "A service is unavailable"; 70 `EX_SOFTWARE` "An internal software error"; 71 `EX_OSERR`; 72 `EX_OSFILE`; 73 `EX_CANTCREAT`; 74 `EX_IOERR`; 75 `EX_TEMPFAIL` "Temporary failure, indicating something that is not really an error"; 76 `EX_PROTOCOL`; 77 `EX_NOPERM`; 78 `EX_CONFIG`. "Error numbers begin at `EX__BASE` [64] to reduce the possibility of clashing with other exit statuses that random programs may already return." In practice sysexits is used by mail tools and a few Go CLIs; most modern CLIs do not.

**bash manual, "Exit Status"** (from the GNU manual; gnu.org rate-limited the fetch, quoted from the standard text): exit statuses are 0–255; "The exit status of a command that terminated because it received a fatal signal N is 128+N" (so Ctrl-C = 130); 127 = command not found; 126 = found but not executable; "All builtins return an exit status of 2 to indicate incorrect usage, generally invalid options or missing arguments." This is why **2 = usage error** is the de-facto convention.

**Python argparse**: `parse_args()` on invalid input "will print a message to `sys.stderr` and exit with a status code of 2."

**GNU grep/diff/cmp**: 0 = match/same, 1 = no match/differ, 2 = trouble — another reason to keep 1 and 2 distinct.

**gh**: 0 success; 1 "fails for any reason"; 2 "running but gets cancelled"; 4 "requires authentication". **Azure CLI**: `show` 404 → 3. **cobra**: no exit-code machinery; `main` must map errors to codes.

Consensus: `0` success, `1` generic failure, `2` usage/argument error. Anything finer is app-specific and must be documented (gh documents its four; az documents 3).

### A11. Machine-readable output — JSON Lines, jc, gh `--json`

**jsonlines.org**: "Each Line is a Valid JSON Value"; UTF-8, no BOM; "Line Terminator is `'\n'`" (`\r\n` tolerated); "Including a line terminator after the last JSON value in a file is strongly recommended but not required"; extension `.jsonl`; MIME `application/jsonl` "not yet standardized"; "works well with unix-style text processing tools and shell pipelines. It's a great format for log files" and "for passing messages between cooperating processes."

**jc / Kelly Brazil**: "linux and all of its supporting GNU and non-GNU utilities should offer JSON output options." Notes `systemctl` and `iproute2` use `-j`. jc's streaming parsers (`ls-s`, `ping-s`) "output JSON Lines… Process data line-by-line as received… Significantly reduce memory requirements… Enable real-time processing in long-lived pipelines." Also cites FreeBSD `libxo` (`--libxo json`) as the in-tree precedent.

**gh `--json`** (`gh help formatting`): `--json` "requires a comma separated list of fields to fetch"; run with `--json` and no value to list available fields; `--jq` "a string argument in jq query syntax" (embedded gojq, no install); `--template` Go templates with helpers `color`, `autocolor`, `join`, `pluck`, `truncate`, `tablerow`, `tablerender`, `timeago`, `timefmt`, `hyperlink`. List commands emit a **bare array**; view commands a **bare object**. mislav's rationale (cli/cli#385, 2021-02-24): "Anything that we publish as output from any `gh` command automatically becomes the API of GitHub CLI. The bigger any API is, the harder it is to maintain. We couldn't just publish JSON output from every command because, that early on, we couldn't also guarantee that we wouldn't have to change the structure of that raw data." And on non-TTY: "when you pipe that command to a file or a script, we switch to a machine-parseable mode: no color, tab delimiters, and no truncation."

---

## Part B — Topic matrix: what the guidelines say, consensus vs conflict

| # | Topic | clig.dev | 12-Factor / Heroku | GNU / POSIX | Azure / gcloud | cobra & friends | Consensus / conflict |
|---|---|---|---|---|---|---|---|
| 1 | Grammar | `noun verb` "seems to be more common"; no catch-all; no abbreviations; explicit stable aliases | Heroku `topic:command`, topics plural nouns, verbs after; root topic lists (no `:list`) | POSIX: names 2–9 lowercase chars; `--` ends options | Azure "[noun] [noun] [verb]", verbs `create/update/set/show/list/delete/wait`, never `get`/`new`; gcloud GROUP…COMMAND with `create/describe/list/update/delete` | cobra Use-line notation `[ ]`, `...`, `\|`, `{ }` | **Consensus**: noun-first, verb last, fixed CRUD verb set. **Conflict**: `show` (az) vs `describe` (gcloud/kubectl) vs `view` (gh) vs `get` (kubectl) for the read verb; singular (`az webapp`) vs plural (`heroku apps`, `gcloud instances`) nouns. |
| 2 | Hierarchy / scoping | (none specific) | Heroku `-a/--app` flag for parent scope | — | az `-g` resource-group flag with configurable default; gcloud `--project/--zone` flags with `gcloud config set` defaults | — | **Consensus**: parent scope is a *flag* with a *configured default*, never a second positional. |
| 3 | Input | Prefer flags; ≤1 positional type; `-` for stdin; `none` sentinel for "clear"; never secrets in flags; `--no-input` | "1 type of argument is fine, 2 suspect, 3 never good"; `--from/--to`; always bypassable prompts | POSIX G7 option-args not optional; G8 comma lists; G11 repeated options in order | az: units in help not names; `-id` = GUID; sub-collections via `add/remove` subcommands, never JSON blobs; gcloud `--no-foo` for every bool, rightmost repeat wins | kong `negatable`, `enum`; cobra flag groups | **Consensus**: flags over positionals, one positional (the name), `--no-` booleans, secrets only via file/stdin/prompt. **Conflict**: repeated flag semantics — POSIX "interpret in order", gcloud "rightmost wins silently". |
| 4 | Human output | Brief success output; tell user about state changes; suggest next command; pager for long output; no log-level labels on stderr | Heroku: no table borders; header row; `Doing X... done` action lines on stderr; 12F: `--columns/--no-truncate/--no-headers/--sort` | GNU errors: `prog: message`, lowercase, no period | gcloud: stdout for requested info only; az: `delete` returns nothing | — | **Consensus**: borderless tables, header row, spinners/progress on stderr. **Conflict**: az says `delete` prints nothing; clig says "rare that printing nothing at all is the best default". |
| 5 | Machine output | `--json`, `--plain`; "Encourage `--plain` or `--json` in scripts to keep output stable" | Heroku `--json` bare array; 12F: CSV + JSON | — | az `-o json\|jsonc\|tsv\|table\|yaml\|none` + `--query` (JMESPath); gcloud `--format=json\|yaml\|csv\|value(...)` + `--filter` + `--flatten` | — ; gh: `--json FIELDS` + `--jq` + `--template`, bare array for lists | **Consensus**: JSON must exist, be stable, be a *curated* contract (gh), and lists are bare arrays. **Conflict**: `--json` boolean (clig/Heroku) vs `-o json` enum (az/gcloud/kubectl) vs `--json fields` (gh). |
| 6 | TTY awareness | Disable color when non-TTY, `NO_COLOR`, `TERM=dumb`, `--no-color`; no animations when non-TTY; prompts only if stdin TTY | Same + app-specific `MYAPP_NOCOLOR`; Heroku `COLOR=false` | — | gcloud `--quiet` disables prompts | Lip Gloss strips ANSI on non-TTY; gh: `GH_FORCE_TTY`, `CLICOLOR_FORCE` | **Consensus**: isatty check per stream; NO_COLOR honoured; `--color` flag beats env (no-color.org). **Gap**: only bixense/gh define a *force-on* (`CLICOLOR_FORCE`). |
| 7 | Streaming / long-running | Print something <100ms; progress bars; timeouts configurable; Ctrl-C exits fast; crash-only; `<up><enter>` resumes | Spinners; OS notifications for very long ops | — | az `wait` verb + `--no-wait`; gcloud `--async` | jsonlines: one JSON value per line for streams | **Consensus**: async by flag (`--no-wait`/`--async`), a `wait` verb, JSON Lines for streamed structured output. |
| 8 | Status / health | "Make it easy to see the current state of the system" | — | — | az `wait --custom "…"` polls GET | — | Thin; guidelines defer to tools (see r-kubectl/r-cloud reports). |
| 9 | Errors & exit codes | 0/non-zero; rewrite errors for humans + next step; most important info last; unexpected errors → debug + bug URL | 12F: code/title/description/fix/URL; `DEBUG` env | GNU: `prog: message`, lowercase, no period; bash: 2 = usage, 128+N signal; sysexits 64–78 | gcloud: 0 or non-zero; az: 404 → 3 | cobra: `Error: …` + `Run '<path> --help' for usage.`; `Did you mean this?`; argparse 2 | **Consensus**: 0 ok / 1 fail / 2 usage; lowercase, no period, "Error:" prefix optional; include a "try…" hint. **Conflict**: sysexits (64+) vs small integers — modern CLIs (gh/az/kubectl) use small integers. |
| 10 | Pagination & filtering | — | 12F `--filter`, `--sort` | — | az `--query`; gcloud `--filter`, `--sort-by`, `--limit`, `--page-size` | — | Only cloud CLIs specify; `--limit`/`--page-size`/`--filter`/`--sort-by` are the names. |
| 11 | Help text | `-h/--help` everywhere; no-args → concise help; examples first; common flags first; bold headings; support link | Heroku: descriptions lowercase, no period, ≤80 cols | GNU `--help` to stdout, exit 0, bug address at end | az `short-summary`/`long-summary`/examples | cobra: `Short` in list, `Long` in detail, `Example` block, `GroupID` groups, `Use` notation; urfave UPPERCASE headings | **Consensus**: help → stdout, exit 0; Use-line notation `[ ]`/`{ }`/`\|`/`...`; examples block. **Conflict**: Short description case — Heroku lowercase/no period vs cobra ecosystem (kubectl/gh/docker) Capitalised, no period. |
| 12 | Config & auth | flag > env > project file > user file > system; XDG; env vars uppercase `[A-Z0-9_]`; **no secrets in env** | XDG dirs incl. macOS/Windows; `MYAPP_` prefix | — | gcloud `--configuration`, `gcloud config set`; az `az config` | kong `env:` tag; urfave `Sources` ordering; viper same | **Consensus**: flag > env > config > default; XDG_CONFIG_HOME. **Conflict**: clig "no secrets in env" vs universal practice (`GH_TOKEN`, `AZURE_*`, `KUBECONFIG`). |
| 13 | Notable / mistakes | Anti-patterns named: catch-all subcommand, abbreviations, `update`+`upgrade`, borders, log labels on stderr, Christmas-tree CI logs, phoning home | Heroku `topic:command` colon syntax is widely regarded as an oddity nobody else adopted | POSIX G9 (options before operands) is honoured by almost nobody | az "JSON blob arg = UNACCEPTABLE"; gcloud silent rightmost-wins | cobra default: prints usage on every runtime error (fix with `SilenceUsage`) | — |

---

## Part C — Lessons for Admiral (opinionated rules, with sources)

Numbers reference the sources in the table at the top. Where Admiral already does the thing, the rule says so; the point is to write it down so it stays true.

**Exit codes**
1. **`0` success, `1` any runtime failure, `2` usage error** (bad flag, missing/extra positional, unknown command, invalid `-o` value). Do not use sysexits (64+). Sources: bash manual "builtins return 2 to indicate incorrect usage", argparse, gh, clig.dev ("non-zero on failure"). Document any additional code in `admiral help exit-codes` the way gh does; recommended extras: `3` = not found (az precedent) and `4` = authentication required (gh precedent), because scripts commonly branch on exactly those two.
2. **Ctrl-C exits with 130** (128+SIGINT) after printing one line to stderr, and a cancelled `run apply --wait`/`logs -f` must not exit 0. Sources: bash 128+N; clig.dev "exit as soon as possible… Say something immediately, before you start clean-up". Implementation: `signal.NotifyContext` at root, `ctx` threaded into every RPC (already fixed for dial per TASK.md), and `main` maps `context.Canceled` → 130.

**Streams**
3. **stdout = the answer; stderr = everything else** (progress, spinners, "Created app shop", NEXT PAGE TOKEN, warnings, deprecation notices, errors). Sources: clig.dev, 12-Factor "stdout is for output, stderr is for messaging", gcloud "standard error is reserved for diagnostics", az "Command output must go to stdout, everything else to stderr". Admiral's existing choice to print the next-page token on stderr is exactly right; the "No <resource> found." message to stderr matches gcloud's `Listed 0 items.`
4. **Never print log-level labels or timestamps on stderr by default.** Source: clig.dev "Don't treat stderr like a log file… Don't print log level labels (`ERR`, `WARN`, etc.)". The slog default handler leak noted in TASK.md (`2026/09/13 19:39:21 WARN …`) is the violation; keep `output.NewLogHandler` as the only handler and reserve timestamps for `--verbose`.

**TTY detection and color**
5. **Decide "human mode" per stream with isatty, once, at startup**: color and spinners keyed on *stderr* being a TTY for messaging and *stdout* being a TTY for tables; prompts keyed on *stdin* being a TTY. Sources: clig.dev "whether or not it's a TTY", 12-Factor factor 6/7, gh (mislav: "no color, tab delimiters, and no truncation" when piped).
6. **Color precedence, exactly**: `--color=never|always|auto` (default `auto`) > `NO_COLOR` (non-empty → off) > `CLICOLOR_FORCE` (non-empty → on) > `TERM=dumb` (off) > isatty. Also honour `ADMIRAL_NO_COLOR` only if you want parity with 12-Factor's `MYAPP_NOCOLOR`; not required. Sources: no-color.org (flag beats env), bixense clicolors, clig.dev list. Use yellow/red only for warnings/errors (Heroku).
7. **When stdout is not a TTY: no ANSI, no truncation, tabs as column separators, no spinner frames.** Sources: clig.dev "progress bars turning into Christmas trees in CI log output", gh piped mode, 12-Factor `--no-truncate`. Keep the header row (Heroku/12-Factor) unless `--no-headers` is passed.

**Prompts and destructive operations**
8. **Prompt only when stdin is a TTY; never *require* a prompt.** If a needed value is missing and stdin is not a TTY, fail with exit 2 and name the flag that supplies it. Sources: clig.dev "Never require a prompt", 12-Factor factor 7, Heroku "Always allow bypassing prompts".
9. **Two different escape hatches, do not merge them**: `--force`/`-f` skips the *confirmation* on destructive verbs (Admiral's current convention; clig.dev "requiring them to pass `-f` or `--force`"), and a global `--no-input` (alias `--non-interactive`; gcloud calls it `--quiet/-q`, gh uses `GH_PROMPT_DISABLED`) disables *all* prompting including login and value prompts. Under `--no-input` a destructive command without `--force` must **fail**, not proceed (gcloud: "If input is required, defaults will be used, or an error will be raised"). Do not overload `-y/--yes`; pick `--force` and keep it.
10. **Tier the confirmations** per clig.dev: moderate (delete env, cancel run) → `y/N`; severe (delete app, discard changeset with pending changes) → type-the-name, which Admiral already does for `app delete`. `--force` bypasses both tiers.

**Machine-readable output**
11. **Keep `-o table|wide|json|yaml`; do not add a bare `--json` boolean.** Admiral's data has enough shapes (list vs single vs run-with-revisions) that the az/gcloud/kubectl enum is the better fit; the clig.dev/Heroku `--json` is for single-purpose tools. Optionally accept `--json` as a hidden alias for `-o json` so muscle memory from gh does not error.
12. **JSON shape: a bare array for lists, a bare object for single resources — never a `{items: [...]}` wrapper, and never the `NEXT PAGE TOKEN` inside stdout JSON.** Sources: Heroku `heroku releases --json | jq '.[]'`, gh (bare array), az `list` "returns array of resources". Pagination metadata stays on stderr in human mode; in JSON mode expose it via a flag (`--all` to auto-paginate, which gcloud/az/gh do) rather than by wrapping.
13. **The JSON is a contract, so curate it.** Emit the SDK/API object verbatim *only* if the server team treats field removal as a breaking change; otherwise emit a CLI-owned shape and version it. Source: mislav, cli/cli#385 — "Anything that we publish as output… automatically becomes the API of GitHub CLI"; clig.dev "Encourage your users to use `--plain` or `--json` in scripts to keep output stable"; Heroku "Don't modify existing output after general availability… Additional information is OK." Practical rule: additive-only changes to JSON keys; renames require a deprecation cycle.
14. **Add `--jq EXPR` (embedded gojq) before adding `--template`, and add `-o name` (just names, one per line) for shell loops.** Sources: gh `--jq` ("The jq utility doesn't need separate installation"), az `--query`, gcloud `--format=value(name)`. `-o name` is the cheapest `--plain` (clig.dev) Admiral can offer.
15. **Streaming = JSON Lines.** `logs -f`, `run apply --wait`, `events --watch` with `-o json` emit one JSON object per line, `\n`-terminated, flushed per event, never a pretty-printed array. Sources: jsonlines.org ("great format for log files… passing messages between cooperating processes"), jc streaming parsers. Non-streaming `-o json` stays pretty-printed when stdout is a TTY, compact when piped (gh does this for `--jq`).

**Configuration and environment**
16. **Precedence, exactly: flag > `ADMIRAL_*` env > config file (current context) > built-in default.** No project-level `.env` layer unless a real need appears (clig.dev warns "Don't use `.env` as a substitute for a proper configuration file"). Sources: clig.dev order, urfave/kong `Sources`/`env:` ordering, viper.
17. **Env var naming: `ADMIRAL_` + the flag name uppercased with `-`→`_`** (`--server` ↔ `ADMIRAL_SERVER`, `--timeout` ↔ `ADMIRAL_TIMEOUT`, `--config-dir` ↔ `ADMIRAL_CONFIG_DIR`, `--no-input` ↔ `ADMIRAL_NO_INPUT`), uppercase `[A-Z0-9_]` only, single-line values. Sources: clig.dev naming rule, 12-Factor `MYAPP_` prefix, gh `GH_*` (mirrors flags: `GH_HOST`, `GH_REPO`, `GH_CONFIG_DIR`, `GH_DEBUG`, `GH_PAGER`). List every one in `admiral help environment` (gh precedent). Also read the general-purpose ones clig.dev names: `NO_COLOR`, `PAGER`, `EDITOR`, `BROWSER`, `HTTP(S)_PROXY`/`NO_PROXY`, `TERM`.
18. **`ADMIRAL_API_KEY` (or `ADMIRAL_TOKEN`) in env is acceptable for CI, but never as a flag.** clig.dev says no secrets in env at all; every cloud CLI ignores that for CI ergonomics, and so should Admiral — but the flag form (`--token X`) leaks into `ps` and history (clig.dev), so keep `auth login --with-token` reading stdin as the only non-env path. Document the env var as the CI path explicitly.
19. **File locations follow XDG**: config + credentials in `$XDG_CONFIG_HOME/admiral` (`~/.config/admiral`), overridable by `--config-dir`/`ADMIRAL_CONFIG_DIR` (gh: `GH_CONFIG_DIR`); caches (completion, discovery) in `$XDG_CACHE_HOME/admiral`; any "last run id / recently used" state in `$XDG_STATE_HOME/admiral`. Ignore relative XDG values (spec). Sources: XDG spec, 12-Factor factor 12, clig.dev "Follow the XDG-spec".

**Help text**
20. **Use-line grammar = cobra's documented notation, kubectl/gh dialect**: `admiral env get <name> --app <app> [flags]`; placeholders in `<dash-case>`; `[ ]` optional; `{a | b}` required choice; `...` repeatable. Sources: cobra `Use` doc comment, Microsoft syntax key, gh `command-line-syntax.md`. Never put a second positional in a Use line (clig.dev, 12-Factor "2 types are very suspect").
21. **`Short`: imperative verb phrase, Capitalised, no trailing period, ≤ 60 chars** ("List environments for an application"). Flag descriptions: lowercase-first sentence fragment, no period ("output format: table, json, yaml, wide"), and units in the description, not the flag name (az). Sources: cobra ecosystem convention (kubectl, gh, docker, helm all do this), Heroku for the flag-description style, az for units. This is a deliberate choice against Heroku's lowercase command descriptions because cobra's `Available Commands:` block reads as a list of titles.
22. **Every leaf command has an `Example:` block with 2–4 real invocations, shown before flags; root help groups commands with `GroupID`** (e.g. "Core resources", "Runs & changes", "Auth & config"), and files `help`/`completion` under a group via `SetHelpCommandGroupID`/`SetCompletionCommandGroupID`. Sources: clig.dev "Lead with examples", "Display the most common flags and commands at the start", cobra `Example`/`AddGroup`. Put the docs URL and the issue-tracker URL in root `Long` (GNU `--help` "Report bugs to", clig.dev "support path").
23. **`admiral <group>` with no verb prints that group's help and exits 0; a leaf command missing required args prints a one-line error + `Run 'admiral env get --help' for usage.` and exits 2.** Do not dump the full usage block on runtime errors — Admiral already sets `SilenceUsage`/`SilenceErrors`; keep it. Sources: clig.dev concise help rule, cobra default hint text, 12-Factor factor 11.

**Error messages**
24. **Grammar: `admiral: <lowercase message, no trailing period>`, one line, followed on the next line by an optional hint starting with `hint:` or `try:` (e.g. `try: admiral auth login`)**; put the actionable line last because "the eye will be drawn to red text" and "most important information at the end" (clig.dev). Sources: GNU §4.4 (`program: message`, lowercase, no period), clig.dev rewrite-for-humans example, 12-Factor (code/title/description/fix/URL — Admiral needs only message + fix; add a URL for auth/config errors). For unknown commands keep cobra's `Did you mean this?` (clig.dev "suggest it"). For gRPC `NotFound` say what was looked up and where: `environment "prod" not found in application "shop"`; for ambiguous name matches list the candidates and exit 1 (az/kubectl precedent), never pick one.
25. **`--verbose/-v` adds request/response and timing to stderr; `ADMIRAL_DEBUG=1` (or `--debug`) adds stack traces plus a bug-report URL on unexpected errors.** Sources: clig.dev "provide debug and traceback information, and instructions on how to submit a bug", 12-Factor `DEBUG`, gh `GH_DEBUG=api`, gcloud `--log-http`. Keep `-v` meaning verbose (cobra will otherwise steal `-v` for `--version` when `Version` is set — define the verbose flag first, as Admiral does).

**Long-running operations**
26. **Default synchronous with progress on stderr, opt-out with `--no-wait`/`--async`, and a `wait` verb for later.** Sources: az `wait` + `--no-wait`, gcloud `--async`, clig.dev "Show progress if something takes a long time", "Print something to the user in <100ms". While waiting, a second Ctrl-C must be explained the first time (`^C received; press again to abandon the wait — the run keeps going on the server`), because clig.dev requires "Tell the user what will happen when they hit Ctrl-C again, in case it is a destructive action." Cancelling the *wait* is not cancelling the *run*; `run cancel` is the only path that changes server state.
27. **Network timeouts are configurable and finite** (`--timeout`/`ADMIRAL_TIMEOUT`, already present) and streaming commands exempt the deadline from the stream itself but keep it for connect. Source: clig.dev "Make things time out… have a reasonable default so it doesn't hang forever"; the `auth login` browser-wait deadline fix in TASK.md follows the same rule.

**Grammar hygiene (already right, keep it written down)**
28. **Fixed verb set `list/get/create/update/delete` + domain verbs; never `show`/`describe`/`view`/`new` as synonyms; no abbreviations; aliases explicit and permanent.** Sources: clig.dev "Don't have ambiguous or similarly-named commands", "Don't allow arbitrary abbreviations", az "don't use `get` or `new`" (Admiral uses `get`, the kubectl dialect — fine, but then never introduce `show`). Boolean flags get a `--no-` form only where a config default can turn them on (gcloud), otherwise a plain flag; repeated flags append for list-typed flags (POSIX G11) and are an error otherwise (do not adopt gcloud's silent rightmost-wins).
