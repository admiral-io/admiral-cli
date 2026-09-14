# CLI conventions research: gcloud, aws, az

Sources: local `--help` output (gcloud 584.0.0, aws-cli 2.13.21, az 2.89.1), the gcloud SDK Python
source under `google-cloud-sdk/lib/`, the az/knack Python source under the Homebrew cellar, and the
official docs (gcloud scripting guide, gcloud formats/filters/projections topics, AWS output /
filter / pagination guides, az output / query / configuration guides). No authenticated calls were
made; every error/exit-code probe below was run locally. Raw help dumps are in `raw/`.

---

## 1. gcloud

### 1.1 Grammar
- Strict `gcloud [alpha|beta] GROUP [SUBGROUP...] COMMAND` — product group, then resource
  (plural noun), then verb: `gcloud run services list`, `gcloud run revisions list`,
  `gcloud compute instances list`. Root help splits **GROUPS** from **COMMANDS**.
- Resource nouns are **plural** (`services`, `revisions`, `instances`, `configurations`).
- Standard verbs: `list`, `describe`, `create`, `update`, `delete`, plus domain verbs at the
  group level when they span resources (`gcloud run deploy` = create-or-update a service).
- The resource name is **positional**, and the doc states the principle explicitly: *"A
  positional argument is used to define an entity on which a command operates while an option
  is required to set a variation in a command's behavior."*
- Release tracks are a path prefix, not a flag: `gcloud beta run services logs tail`. Every
  help page ends with `NOTES: These variants are also available: $ gcloud alpha ..., $ gcloud
  beta ...`. Running a track-only command on GA gives:
  ```
  ERROR: (gcloud.logging) Invalid choice: 'tail'.
  This command is available in one or more alternate release tracks.  Try:
    gcloud alpha logging tail
    gcloud beta logging tail
  ```
- No short aliases for commands; some flags have short forms (`-q`, `-h`, `-v`).

### 1.2 Hierarchy / scoping
- Parent scope is always a **flag** with a **property fallback**: `--project` (core/project),
  `--region` (run/region), `--zone` (compute/zone). Flag help literally says *"Alternatively, set
  the property [run/region]."*
- The resource positional accepts either a short ID or a *fully qualified* resource name; the
  help enumerates the resolution chain, in order:
  ```
  --namespace=NAMESPACE
     To set the namespace attribute:
     ▸ provide the argument SERVICE on the command line with a fully specified name;
     ▸ provide the argument --namespace on the command line;
     ▸ set the property run/namespace;
     ▸ ... provide the argument project on the command line;
     ▸ set the property core/project.
  ```
- Precedence: flag > `CLOUDSDK_<SECTION>_<PROPERTY>` env var > active named configuration
  (`gcloud config set run/region us-central1`, `gcloud config configurations activate prod`).
- Omitted scope: `gcloud run services list` with no `--region` lists **all regions** and adds a
  `REGION` column (source: `surface/run/services/list.py` `_GlobalList`). Omitted project errors.
- `gcloud run deploy` with no name **prompts with a suggested default** (interactive only).

### 1.3 Input
- Flags are `--name=VALUE` style in docs (space works too). Booleans are `--[no-]flag`.
- **The set/update/remove/clear quartet** for every map/list field (from `run deploy --help`):
  ```
  --clear-env-vars | --env-vars-file=FILE_PATH | --set-env-vars=[KEY=VALUE,...]
  --remove-env-vars=[KEY,...]  --update-env-vars=[KEY=VALUE,...]
  --clear-labels | --remove-labels=[KEY,...]  --labels=[KEY=VALUE,...] | --update-labels=[KEY=VALUE,...]
  ```
  `--set-*` replaces the whole map, `--update-*` merges, `--remove-*` deletes keys,
  `--clear-*` empties; `--labels` is an alias for `--update-labels`. Scalars clear with an
  empty string: *"To reset this field to its default, pass an empty string."* (`--args=""`).
- Lists are comma-separated in one flag (`--set-env-vars=A=1,B=2`), with a `--flags-file=YAML`
  escape hatch for values containing commas/quotes (`gcloud topic flags-file`). Custom
  delimiters via `^:^a,b:c` syntax (`gcloud topic escaping`).
- Prompts: `console_io.PromptContinue` writes to **stderr**, `Do you want to continue (Y/n)?`,
  reprompts with `Please enter 'y' or 'n':`. `--quiet`/`-q` (or `CLOUDSDK_CORE_DISABLE_PROMPTS=1`
  or `core/disable_prompts`) means *"defaults will be used, or an error will be raised"*. Delete
  uses `throw_if_unattended=True, cancel_on_no=True`: piping stdin from a non-tty with no `-q`
  errors *"This prompt could not be answered because you are not in an interactive session.
  You can re-run the command with the --quiet flag..."*; answering no prints `Aborted by user.`
- No `--force`/`--yes`; `--quiet` is the single switch for "no prompts".
- `--async` (`--[no-]async`, default no) on long-running commands: *"Return immediately,
  without waiting for the operation in progress to complete."*

### 1.4 Output (human)
- Every command class has a default format (`calliope/base.py`):
  - `ListCommand` → command-specific `table(...)`; if nothing was listed prints
    `Listed 0 items.` to **stderr** (`Epilog`).
  - `DescribeCommand` → `default` = **YAML** of the full resource.
  - `CreateCommand`/`UpdateCommand`/`DeleteCommand` inherit `SilentCommand` → format `none`:
    stdout is empty, a status line goes to stderr via `log.CreatedResource/DeletedResource`:
    `Created service [foo].` / `Deleted service [foo].` / async: `Delete in progress for
    service [foo].` / failure: `Failed to delete service [foo]: <reason>`. Passing `--format=json`
    to a create prints the created resource.
- Table columns are declared as projections; headers are auto-derived in **"ANGRY_SNAKE_CASE"**
  from the key, or set with `:label=`. Real defaults:
  ```
  # compute instances list (command_lib/compute/instances/flags.py)
  table(name, zone.basename(), machineType.machine_type().basename(),
        scheduling.preemptible.yesno(yes=true, no=''),
        networkInterfaces.internal_ip():label=INTERNAL_IP, external_ip():label=EXTERNAL_IP, status)
  # -> NAME  ZONE  MACHINE_TYPE  PREEMPTIBLE  INTERNAL_IP  EXTERNAL_IP  STATUS

  # run services list (surface/run/services/list.py)
  table(ready_symbol.color(red="[xX]",green="[✓✔]",yellow="[-!…]"):label="",
        firstof(id,metadata.name):label=SERVICE, region:label=REGION, domain:label=URL,
        last_modifier:label="LAST DEPLOYED BY", last_transition_time:label="LAST DEPLOYED AT")
  ```
  giving
  ```
     SERVICE  REGION       URL                              LAST DEPLOYED BY   LAST DEPLOYED AT
  ✔  hello    us-central1  https://hello-abc-uc.a.run.app   me@example.com     2024-05-01T10:00:00.000000Z
  ```
  The unlabeled first column is a **status glyph**: `✔` green ready, `X` red failed, `…` yellow
  unknown/in-progress, `!` "serving but not the revision you wanted"; ASCII fallbacks `+ X .`
  when encoding is not UTF-8 (`api_lib/run/k8s_object.py`).
- `describe` for Cloud Run overrides YAML with a custom "pretty" printer whose header is
  `✔ Service hello in region us-central1` followed by `Labeled` sections (`URL:`, `Ingress:`,
  `Traffic:`, then a `Last updated on ... by ...:` revision block). `--format=yaml` gives the raw
  object; `--format=export` strips server-set metadata for re-apply.
- Timestamps in tables are raw RFC3339 unless the projection applies `.date(tz=LOCAL)`; there
  is no relative "AGE" column convention. `duration()` and `size()` transforms exist.
- Table width fits the terminal, or **80 columns if stdout is not a terminal**; `:wrap` columns
  wrap, others are never truncated (projection attribute `width=`/`wrap=`).
- Color only in `color()` transforms and status glyphs, controlled by `core/disable_color` (no
  `NO_COLOR` support — zero hits in `core/console`). Box tables (`--format="[box]"`) fall back to
  ASCII when not UTF-8 or when `accessibility/screen_reader=true` (spinners become the word
  `working`).
- **stdout/stderr rule (official):** *"The output of successful gcloud CLI commands is written to
  stdout. All other types of responses — prompts, warnings, and errors — are written to stderr."*
  Confirmed: `gcloud config list 2>/dev/null` shows only the INI body; `Your active configuration
  is: [default]` lands on stderr. `gcloud config get unset/prop` prints `(unset)` on stderr, exit 0.

### 1.5 Output (machine)
- One flag, `--format=NAME[ATTRIBUTES](PROJECTION)`. Names: `json`, `yaml`, `csv`, `value`,
  `table`, `flattened`/`text`, `list`, `config`, `diff`, `multi`, `object`, `none`, `disable`,
  `get`, `default`(=yaml).
- `json`/`yaml` are the **full API object**; `list` → **bare JSON array**, `describe` → single
  object. Attribute `[no-undefined]` drops nulls.
- `value(a,b)` = tab-separated, no header — the scripting workhorse:
  `gcloud info --format='value(config.account)'`, `--format="value(uri())"`.
- `csv[no-heading,separator=";"](a,b)`; `table[box,title=X](name:sort=1, status:label=STATE)`.
- Projection **transforms** chain on keys: `zone.basename()`, `createTime.date(tz=LOCAL)`,
  `disks[].interface.list()`, `size(units_out=M)`, `yesno()`, `color(red=STOP)`, `trailoff(20)`,
  `firstof(id,name)`, `if()`, `format("{0}:{1}", a, b)`. Nested lists via `--flatten=disks[]`.
- `--uri` on any list prints only resource URIs (gcloud's `-o name` equivalent).
- Property `core/format` sets a global override; `core/default_format` overrides only the yaml
  default (describe) not the command-specific tables.

### 1.6 TTY awareness
- Prompts: require stdin tty unless `--quiet`; `IsInteractive(error=True)` gates progress
  trackers — non-tty gets a non-spinning tracker that prints plain lines to stderr.
- Tables: width 80 when not a terminal; color only if TERM supports it; no pager by default
  (`[pager]` format attribute opts in).
- `core/show_structured_logs=log|terminal|always|never` emits JSON log lines on stderr when
  stderr is a file — useful for CI log collectors.

### 1.7 Streaming & long-running
- Long-running ops show a **multi-stage progress tracker** on stderr with a spinner, then a
  bold summary (from `command_lib/run/stages.py` and `messages_util.py`):
  ```
  Deploying container to Cloud Run service [hello] in project [p] region [us-central1]
  ✓ Deploying new service... Done.
    ✓ Creating Revision...
    ✓ Routing traffic...
    ✓ Setting IAM Policy...
  Done.
  Service [hello] revision [hello-00001-abc] has been deployed and is serving 100 percent of traffic.
  Service URL: https://hello-abc-uc.a.run.app
  ```
- `--async` returns immediately; message becomes `Service [hello] is being deleted.` /
  `Create in progress for ...`. Separate `gcloud <api> operations describe|wait OPERATION`
  commands poll (e.g. `gcloud compute operations wait`).
- Logs: `gcloud logging read [LOG_FILTER] --freshness=1d --order=desc --limit=N` (newest first
  by default; `--freshness` takes gcloud durations `1d`, `3h`); `gcloud beta logging tail` and
  `gcloud beta run services logs tail SERVICE --log-filter=...` for streaming (beta only, no
  `--follow` flag — tail *is* follow). `run services logs read SERVICE --log-filter="severity>=ERROR"`.
- Date inputs (`gcloud topic datetimes`) accept RFC3339, RFC822, partial ISO, and relative ISO
  durations `-P2W`, `-PT4H`; filters accept `createTime>-P2W`.
- No `--web`; no `--timeout` on waits (per-command).

### 1.8 Status / health / metrics
- Status is a glyph column + `status`/`STATUS` string column (`RUNNING`, `TERMINATED`). No
  `top`/`stats`; metrics live in Cloud Monitoring, not gcloud.

### 1.9 Errors & exit codes
- Format: `ERROR: (gcloud.run.services.list) unrecognized arguments: --bogus` — the failing
  command path in parentheses, then the message. Followed by a recovery hint:
  ```
  ERROR: (gcloud.run) Invalid choice: 'servicez'.
  Maybe you meant:
    gcloud run services list
    gcloud run jobs list
    ...
  To search the help text of gcloud commands, run:
    gcloud help -- SEARCH_TERMS
  ```
  ```
  ERROR: (gcloud.run.services.describe) argument (SERVICE : --namespace=NAMESPACE): Must be specified.
  Usage: gcloud run services describe (SERVICE : --namespace=NAMESPACE) [optional flags]
    optional flags may be  --help | --namespace | --region
  For detailed information on this command and its flags, run:
    gcloud run services describe --help
  ```
- Exit codes: **2** for every usage/parse error (bad command, bad flag, missing arg — verified),
  **1** for runtime/API errors (`exceptions.py`: `sys.exit(getattr(exc, 'exit_code', 1))`).
- `--verbosity=debug|info|warning|error|critical|none` (default warning); `--log-http` dumps
  requests/responses to stderr; `--no-user-output-enabled` silences everything.
- Official scripting doc: rely on exit status, `--format`, `--filter`; **do not** parse stderr
  wording or default stdout — both may change.

### 1.10 Pagination & filtering
- Every `ListCommand` gets the same five: `--filter=EXPRESSION`, `--limit=N` (client-side,
  default unlimited), `--page-size=N` (server page; tables are re-headed per page), `--sort-by=
  [FIELD,...]` (`~field` for descending), `--uri`. Help states the order of application:
  *"--flatten, --sort-by, --filter, --limit."* gcloud auto-follows pages; there is no
  page-token flag exposed to users.
- `--filter` language: `key:pattern` (word match / prefix `*`), `key=value`, `key!=value`,
  `<,<=,>,>=`, `key~REGEX`, `key:*` (defined), `-key:*` (undefined), `key=(a,b)` (any of),
  `AND`/`OR`/`NOT` (uppercase), parentheses. `labels.env=test AND labels.version=alpha`,
  `createTime>=2018-01-15`, `zone ~ us AND -machineType:f1-micro`. Filtering may be client- or
  server-side depending on the API — transparent to the user. Discover keys with
  `--format=yaml --limit=1`.
- `compute instances list` has an extra `--zones=a,b` and a deprecated positional `[NAME ...]`
  ("Use --filter=\"name=( 'NAME' ... )\" instead") — they moved *away* from multiple positionals.

### 1.11 Help text style
- man-page sections in fixed order: `NAME` (`gcloud run services list - list available
  services`, lowercase, no period), `SYNOPSIS`, `DESCRIPTION`, `EXAMPLES` ("To list available
  services:" then `$ gcloud run services list`), `POSITIONAL ARGUMENTS`, `FLAGS`, `LIST COMMAND
  FLAGS` (the shared five), `GCLOUD WIDE FLAGS` (one line listing all global flags), `NOTES`.
- Group help short descriptions are imperative, capitalized, with period: `Manage your App
  Engine deployments.`; command NAME lines are lowercase without period.
- Flag help is prose with `; default="warning"` and `VERBOSITY must be one of: ...` appended
  automatically; mutually exclusive groups are rendered `At most one of these can be specified:`.
- `gcloud help -- SEARCH_TERMS` full-text search; `gcloud topic X` for concept docs;
  `gcloud cheat-sheet`.

### 1.12 Config & auth
- Named configurations (profiles): `gcloud config configurations create|activate|list`;
  `gcloud config set SECTION/PROPERTY VALUE`, `core/` optional (`gcloud config set project X`).
  `gcloud config list` prints INI to stdout; `gcloud config get core/project`.
- Env var scheme `CLOUDSDK_<SECTION>_<PROPERTY>`; `--configuration=NAME` per invocation;
  `--account`, `--impersonate-service-account`, `--access-token-file`.
- Precedence: flag > env > active configuration > installation defaults (`--installation`).

### 1.13 Notable / mistakes
- Steal: SilentCommand convention (mutations print nothing to stdout unless `--format`), the
  set/update/remove/clear flag quartet, `Listed 0 items.` on stderr, `(gcloud.cmd.path)` error
  prefix, "Maybe you meant" suggestions, uniform list flags with documented application order,
  status glyph column, `value()` format for scripting, `--flags-file`.
- Mistakes: the `:` operator semantics are changing mid-flight ("The current default is
  deprecated"); `=` means different things per API; `--format` projection syntax is powerful but
  cryptic and hard to type; describe is YAML but `run` overrides it with a custom layout so users
  can't predict; no `NO_COLOR`; boxed tables never used by default so `table` header alignment
  is whitespace-only; startup time is slow (Python).

---

## 2. aws

### 2.1 Grammar
- `aws [options] <service> <operation> [parameters]`. Operations are **verb-noun, kebab-case,
  1:1 with API actions**: `aws ecs list-services`, `describe-services`, `update-service`,
  `delete-service`, `aws logs tail`. The prefix varies by API team, not by semantics:
  `list-*` returns ARNs/IDs only, `describe-*` returns full objects (ECS), but S3 uses
  `list-buckets`, Lambda uses `get-function`, EC2 uses `describe-instances` for everything.
- Resource identity is **always a flag**, never positional: `--cluster MyCluster --services
  my-http-service`. Exception: hand-written "customizations" (`aws logs tail group_name`,
  `aws s3 cp`, `aws ecs deploy`).
- Nesting depth is two (service, operation) except `aws <svc> wait <condition>` and `aws
  configure sso`.

### 2.2 Hierarchy / scoping
- `--region`, `--profile` global flags; env `AWS_REGION`/`AWS_DEFAULT_REGION`, `AWS_PROFILE`;
  `~/.aws/config` `[profile x] region=...`. Parent resources (cluster) are per-operation flags
  with API-side defaults (`If you do not specify a cluster, the default cluster is assumed.`).
- Missing region does not produce a friendly error: `Invalid endpoint: https://ecs..amazonaws.com`.
- Missing credentials: `Unable to locate credentials. You can configure credentials by running
  "aws configure".` exit **253**.

### 2.3 Input
- Flags only, `--flag value` (space, not `=`). Lists are space-separated repeats of a single
  flag: `--services "a" "b"`. Structures use shorthand `Name=availability-zone,Values=us-west-2a`
  or JSON literals; `--cli-input-json file://x.json` / `--cli-input-yaml`, and
  `--generate-cli-skeleton [input|yaml-input|output]` prints an empty request shape to fill in.
- `file://` and `fileb://` prefixes read parameter values from disk.
- Booleans: `--force | --no-force`.
- **No confirmation prompts anywhere** (`delete-service` runs immediately; `--force` there means
  "delete even if not scaled to zero", an API semantic).
- `--cli-auto-prompt` / `aws configure set cli_auto_prompt on-partial`: interactive fuzzy
  completion of commands and parameters with an F5 output preview.

### 2.4 Output (human)
- **There is no human default.** Default is `json` of the raw API response, wrapper key included:
  ```
  $ aws ecs list-services --cluster MyCluster
  {
      "serviceArns": [
          "arn:aws:ecs:us-west-2:123456789012:service/MyCluster/MyService"
      ]
  }
  ```
  `describe-services` returns `{"services": [ {...} ], "failures": []}` with epoch-float
  timestamps (`"createdAt": 1466801808.595`) in ECS, ISO strings in IAM.
- `--output table` draws a boxed ASCII table titled with the operation name; nested structures
  become nested boxes; columns are alphabetical by key:
  ```
  ------------------------------------------------------
  |                   DescribeVolumes                  |
  +------------+----------------+--------------+-------+
  |     AZ     |      ID        | InstanceId   | Size  |
  +------------+----------------+--------------+-------+
  |  us-west-2a|  vol-e11a5288  |  i-a071c394  |  30   |
  ```
- `--output text`: tab-separated rows prefixed with an UPPERCASE record identifier
  (`USERS   arn:...   2014-10-16T16:03:09+00:00   ...`), columns **sorted alphabetically by key**;
  docs say *"We strongly recommend that if you specify text output, you also always use the
  --query option"* because column order changes when keys differ between records. Missing
  values print `None`.
- v2 sends all output through a **pager by default** (`less -FRX` when `LESS` is unset; `more`
  on Windows); disable with `--no-cli-pager`, `AWS_PAGER=""`, or `cli_pager=` in config.
- `--color on|off|auto` (auto = tty). No `NO_COLOR`.
- `--output off`: suppress stdout entirely, keep exit code and stderr.

### 2.5 Output (machine)
- `--output json|yaml|yaml-stream|text|table|off`; `yaml-stream` emits one YAML document per page
  with `IsTruncated`/`Marker` for fast large lists.
- `--query` = **JMESPath**, evaluated client-side after the full response is assembled (for
  json/yaml) or **per page** (for text — a documented footgun). Idioms:
  `'Volumes[*].[VolumeId,Size]'` (multiselect list → ordered columns),
  `'Volumes[].{Id:VolumeId,Size:Size}'` (multiselect hash → named columns),
  `'Volumes[?Size > \`50\`]'`, `'sort_by(Images,&CreationDate)[-1].ImageId'`,
  `'length(Volumes[?Iops > \`1000\`])'`, `'reverse(sort_by(...))[:5]'`. Backticks for literals.
- Server-side filtering is a per-API parameter: `--filters "Name=status,Values=attached"` (EC2),
  `--filter` (SES), `--filter-expression` (DynamoDB).

### 2.6 TTY awareness
- Pager only when stdout is a tty; color `auto` on tty. Nothing else changes — JSON is JSON.

### 2.7 Streaming & long-running
- `aws logs tail GROUP [--follow] [--since 5m|ISO8601] [--format detailed|short|json]
  [--filter-pattern P] [--log-stream-names A B | --log-stream-name-prefix P]`. `--since` takes
  one unit only (`5h30m` not supported); default 10m; detailed format = ms timestamp + tz +
  stream name + message; `short` = short timestamp + message; `json` pretty-prints JSON lines.
  `--follow` polls; Ctrl-C exits.
- `aws <svc> wait <condition>` subcommands are generated from waiter specs and the help states
  the contract verbatim: *"It will poll every 15 seconds until a successful state has been
  reached. This will exit with a return code of 255 after 40 failed checks."* e.g.
  `aws ecs wait services-stable --cluster c --services s`.
- Mutations return the API response immediately; there is no `--wait`; callers chain `wait`.

### 2.8 Status / health / metrics
- None in the CLI proper; `describe-*` JSON carries `status`, `runningCount`, `desiredCount`,
  `deployments[].rolloutState`. Metrics via `aws cloudwatch get-metric-data` (raw).

### 2.9 Errors & exit codes
- Parse errors print the generic usage block first, then `aws: error: ...`:
  ```
  usage: aws [options] <command> <subcommand> [<subcommand> ...] [parameters]
  To see help text, you can run:
    aws help
    aws <command> help
    aws <command> <subcommand> help

  aws: error: the following arguments are required: --services
  ```
  Unknown flag: `Unknown options: --bogus`. Unknown command/enum: `aws: error: argument
  command: Invalid choice, valid choices are:` followed by the **entire** service list in two
  columns (300+ lines).
- Documented exit codes (`aws help return-codes`): **0** ok; **1** s3 transfer failed; **2**
  parse failure (in practice v2 returns 252); **130** SIGINT; **252** syntax/unknown param/bad
  value (verified for all four probes); **253** environment/config invalid (no creds);
  **254** service returned an error; **255** general catch-all.
- Service errors: `An error occurred (ClusterNotFoundException) when calling the
  DescribeServices operation: Cluster not found.`
- `--debug` dumps botocore HTTP traces to stderr.

### 2.10 Pagination & filtering
- Auto-pagination: CLI follows `NextToken` and concatenates pages into one JSON result.
  `--no-paginate` = first page only; `--page-size N` = server page size, output unchanged;
  `--max-items N` = client-side cap, and when truncated adds a **CLI-owned** `"NextToken":
  "eyJNYXJrZXIi..."` (base64 of marker + truncate amount) to the output; `--starting-token T`
  resumes. Docs warn to use the same `--page-size` and `--max-items` to avoid gaps/dupes.
- No `--sort-by`, no `--limit` (use `--max-items` or `--query '[:5]'`).

### 2.11 Help text style
- man-page via groff: `NAME`, `DESCRIPTION`, `SYNOPSIS`, `OPTIONS`, `GLOBAL OPTIONS` (repeated
  in full on every page), `EXAMPLES` (every one prefixed by a quoting-rules NOTE), `OUTPUT`
  (typed schema `serviceArns -> (list)`). `NAME` lines are empty (`list-services -`).
  Types shown as `(string)`, `(list)`, `(boolean)`. `help` is a subcommand (`aws ecs help`), and
  `--help` is not.

### 2.12 Config & auth
- `aws configure` interactive wizard (`AWS Access Key ID [None]:`); `aws configure set|get|list
  |list-profiles|export-credentials|sso`. `~/.aws/credentials` for secrets, `~/.aws/config` for
  everything else; `--profile`, `AWS_PROFILE`. `aws configure list` shows Name/Value/Type/Location
  so users can see **where** each value came from.
- Precedence: flag > env var > profile in config.

### 2.13 Notable / mistakes
- Steal: `wait` subcommands with explicit poll/timeout contract; `--generate-cli-skeleton` +
  `--cli-input-json`; `aws configure list` with a Location column; `--output off`; the
  `return-codes` help topic; `logs tail --since 5m --format short`.
- Widely criticised (and observable above): verb-noun grammar that mirrors API names so
  `list-`/`describe-`/`get-` mean different things per service; raw response shapes with wrapper
  keys and epoch floats; no human-readable default so every use needs `--query`; identity only
  via flags (`--services` is a list even for one); no prompts on destructive ops; v2 pager
  surprise; `text` output column ordering instability; enormous "Invalid choice" dumps; the
  `--filters Name=..,Values=..` shorthand; 252/253/254/255 codes were only documented after v1
  used 255 for everything.

---

## 3. az

### 3.1 Grammar
- `az GROUP [SUBGROUP...] VERB`: `az webapp list`, `az webapp log tail`, `az webapp deployment
  slot swap`, `az container logs`, `az containerapp logs show`. Group help separates
  **Subgroups:** and **Commands:**.
- Nouns are **singular** (`webapp`, `vm`, `group`, `container`). Standard verbs: `list`,
  `show` (not describe/get), `create`, `update`, `delete`, plus `start|stop|restart`, `wait`,
  `browse` (open in browser), and ad-hoc `list-*` commands where a verb didn't fit
  (`list-runtimes`, `list-instances`, `list-publishing-profiles`).
- Names are **flags**, never positional: `--name/-n`, `--resource-group/-g`. Almost every flag
  has a one-letter short form.
- Maturity tags inline in help: `config [Experimental]`, `interactive [Preview]`, `up
  [Deprecated]`, `--keep-dns-registration [Deprecated]` with a WARNING line.

### 3.2 Hierarchy / scoping
- Subscription → resource group → resource, all flags: `--subscription`, `-g`, `-n`, plus
  `--slot/-s` for the sub-resource. Alternative: `--ids ID [ID...]` — pass one or more full ARM
  resource IDs and omit `-g/-n` entirely (help block "Resource Id Arguments"). This lets
  `az vm wait --deleted --ids $(az vm list -g RG --query "[].id" -o tsv)`.
- Defaults: `az config set defaults.group=MyRG defaults.location=westus2 defaults.web=myapp`
  — flag help says so on every parameter: *"You can configure the default group using `az
  configure --defaults group=<name>`."* Once a default exists the argument is no longer
  `[Required]`. `--local` writes defaults to `./.azure/config` for the current directory.
  `az config param-persist on` remembers `-g`, `-n`, `--location` from the last command.
- Missing required: `ERROR: the following arguments are required: --resource-group/-g,
  --name/-n, --plan/-p` followed by `Examples from command's help:`.

### 3.3 Input
- `--flag value`; booleans as `--https-only true|false` (`Allowed values: false, true`) or bare
  switches. Lists are space-separated. Every `update` command carries the **Generic Update
  Arguments**: `--set property.path=value`, `--add property.list key=value`, `--remove
  property.list <index>` / `--remove propertyToRemove`, `--force-string` — a JSON-patch escape
  hatch that works on any field even without a dedicated flag.
- Confirmation: `--yes/-y` (*"Do not prompt for confirmation."*); prompt is `Are you sure you
  want to perform this operation? (y/n):` via `prompt_y_n`; no tty → `WARNING: Unable to prompt
  for confirmation as no tty available. Use --yes.` then `Operation cancelled.` (exit 1).
  Global off-switch `core.disable_confirm_prompt=true` / `AZURE_CORE_DISABLE_CONFIRM_PROMPT`.
- Secrets: `-o none` recommended for commands that echo secrets; `clients.show_secrets_warning`
  scans output and prints `WARNING: [Warning] This output may compromise security by showing
  the following secrets: ...`.
- `--no-wait` on long-running mutations; `az <res> wait --created|--updated|--deleted|--exists|
  --custom "provisioningState!='InProgress'" --interval 30 --timeout 3600`.

### 3.4 Output (human)
- Default is **`json`** (full ARM object, `list` → bare array, `show` → object). Docs: *"Commands
  that could return more than one object return an array, and commands that always return only a
  single object return a dictionary."*
- `-o table` auto-derives columns from top-level scalar keys, **drops `id`, `type`, `etag`**
  (`knack/output.py SKIP_KEYS`), drops nested objects/lists and nulls, capitalises the first
  letter of each camelCase key, and renders with `tabulate(tablefmt="simple")`:
  ```
  Name         ResourceGroup    Location
  -----------  ---------------  ----------
  DemoVM010    DEMORG1          westus
  ```
  Commands may register a hand-written table transformer; otherwise the columns are whatever the
  API returned. To pick columns: `--query "[].{Name:name, State:state}" -o table`.
- `-o jsonc` / `yamlc` = colorised. `-o tsv` = values only, keys sorted alphabetically, no
  guarantee of order — docs say force order with `--query '[].[id, location, name]'`.
- Timestamps are raw ISO from ARM; no age column.
- Log levels prefix stderr messages: `WARNING: ...`, `ERROR: ...` (colored when both stdout and
  stderr are ttys). Chatty by default (`WARNING: You have 2 update(s) available...`); silence
  with `--only-show-errors` / `AZURE_CORE_ONLY_SHOW_ERRORS=1`.

### 3.5 Output (machine)
- `--output/-o json|jsonc|yaml|yamlc|table|tsv|none`, default via `az config set
  core.output=table` / `AZURE_CORE_OUTPUT`.
- `--query` JMESPath on every command (knack core). Same idioms as aws:
  `"[?state=='Running']"`, `"[].{hostName: defaultHostName, state: state}"`,
  `"sort_by([].{Name:name, Size:storageProfile.osDisk.diskSizeGb}, &Size)"`,
  `"[?contains(name,'prod')]"`, `az account show --query id -o tsv` → variable.
- Query runs before the table formatter, so `--query ... -o table` is the standard way to make a
  custom table.

### 3.6 TTY awareness
- Color enabled only when `stdout.isatty() and stderr.isatty()` and not `core.no_color`
  (`knack/cli.py`); honours `KNACK_NO_COLOR` env, not `NO_COLOR`. Prompts require stdin tty
  (`NoTTYException`). Spinner (`humanfriendly`) goes to stderr; a long-running op logs
  `Starting <command>` / progress only with `--verbose`. Survey/upgrade nags are suppressed when
  stderr is not a tty.

### 3.7 Streaming & long-running
- Mutations **block** until the ARM operation reaches `Succeeded`, then print the final resource
  JSON. `--no-wait` returns immediately (prints nothing); pair with `az X wait --created`.
- Logs: `az webapp log tail -n app -g rg [--provider application|http] [--slot]` (stream only,
  no history); `az container logs -n grp -g rg [--container-name c] [--follow]`; `az container
  attach` (stdout/stderr + startup diagnostics); `az containerapp logs show -n app -g rg
  [--follow] [--tail 20 (0-300)] [--format json|text (default json)] [--type console|system]
  [--revision R] [--replica P] [--container C]`. Note `az webapp log download` for history.
- `az webapp browse` opens the app URL in a browser.

### 3.8 Status / health / metrics
- `az webapp show --query state` (`Running`/`Stopped`); `az vm get-instance-view`; `az monitor
  metrics list --resource ID --metric "Percentage CPU" --interval PT1M` for metrics (raw JSON
  time series). No `top`/`stats` verbs.

### 3.9 Errors & exit codes
- Format `ERROR: <message>` then recovery lines:
  ```
  ERROR: 'lisst' is misspelled or not recognized by the system.
  Did you mean 'list' ?

  Examples from command's help:
  az webapp list --resource-group MyResourceGroup
  List all web apps in MyResourceGroup.
  ```
  ```
  ERROR: unrecognized arguments: --bogus

  https://aka.ms/cli_ref
  Read more about the command in reference docs
  ```
  ```
  ERROR: Please run 'az login' to setup account.
  ```
- Exit codes: **2** for parse errors (misspelled command, bad flag, missing required — verified),
  **1** for everything else (`CLIError`, not logged in, cancelled prompt). Errors are classified
  internally (`UserFault`, `ServiceError`, `ClientError`, `ResourceNotFoundError`...) for
  telemetry and `core.error_recommendation`, but the code is still 1.
- `--verbose` = INFO logs; `--debug` = full HTTP request/response dump.

### 3.10 Pagination & filtering
- ARM paging is followed automatically; no page flags at all. Filtering is `--query` only, plus
  scoping flags (`-g`). No `--limit`/`--sort-by`.

### 3.11 Help text style
- Compact, colon-aligned:
  ```
  Command
      az webapp list : List web apps.

  Arguments
      --resource-group -g : Name of resource group. You can configure the default group using `az
                            configure --defaults group=<name>`.
      --show-details      : Include detailed site configuration of listed web apps in output.

  Global Arguments
      --debug ... --help -h ... --only-show-errors ... --output -o ... --query ... --subscription ... --verbose

  Examples
      List all running web apps.
          az webapp list --query "[?state=='Running']"
  ```
  Sections in order: Command/Group, Arguments (with `[Required]` / `[Preview]` / `[Deprecated]`
  markers and `Allowed values:` / `Default:`), `Global Policy Arguments`, `Resource Id Arguments`,
  `Wait Condition Arguments`, `Generic Update Arguments`, `Global Arguments`, `Examples`.
  Short descriptions are imperative, capitalized, period-terminated: `Delete a web app.`,
  `Get the details of a web app.`, `Start live log tracing for a web app.`
- Examples are reused as error recovery text ("Examples from command's help"). Examples marked
  `(autogenerated)` are synthesized.
- `az find "az webapp"` AI-assisted search; `az interactive` REPL; `az --help` prints a telemetry
  notice every time.

### 3.12 Config & auth
- `az login` (browser/device code), `az account set -s SUB`, `az account show`. Config INI at
  `~/.azure/config` (`AZURE_CONFIG_DIR`), env vars `AZURE_<SECTION>_<KEY>` (`AZURE_CORE_OUTPUT`,
  `AZURE_DEFAULTS_GROUP`, `AZURE_CORE_ONLY_SHOW_ERRORS`). Precedence: flag > env > config.
  `az config get` prints every value **with its source** (`"source": "AZURE_CORE_OUTPUT"` or file
  path). `az init` interactive chooser between "interaction" and "automation" presets.

### 3.13 Notable / mistakes
- Steal: `--ids` (accept full identifiers instead of name+parent), `az config get` showing
  source, `az X wait --created/--deleted/--custom`, `--no-wait`, `--only-show-errors`, `-o none`
  for secret-returning commands, `-o tsv` for variable capture, examples-as-error-hints, `Did you
  mean`, `[Required]`/`[Preview]` markers in help, "table drops id/type/etag".
- Mistakes: everything is a flag so common commands are verbose (`az webapp show -n x -g y`);
  JSON default means humans see 200-line blobs; auto-table shows whatever scalar keys the API
  had, so tables differ per resource and hide nested status; `list-*` bolt-on verbs; warning
  spam on every command (update nags, SyntaxWarnings from Python deps); Python startup ~1s;
  `KNACK_NO_COLOR` instead of `NO_COLOR`; exit code 1 for every runtime error class.

---

## Lessons for Admiral

1. **Keep `group resource verb` with the child name positional and parents as flags** (gcloud).
   Avoid az's all-flags grammar (`-n x -g y`) and aws's verb-noun. Keep nouns consistently
   singular or plural — pick one (gcloud: plural; az: singular; Admiral today: singular is fine).
2. **Give every parent-scope flag a config fallback and say so in the flag help**, exactly
   like gcloud/az: `--app APP  Application name. Alternatively set with 'admiral config set app'.`
   Precedence flag > `ADMIRAL_APP` env > config. Print the resolved scope on stderr for mutations.
3. **Mutations are silent on stdout** (gcloud `SilentCommand`): `create/update/delete` print
   `Created environment [prod].` / `Deleted run [r-123].` to **stderr**; the resource body goes to
   stdout only when `-o json|yaml` is given. This keeps `$(admiral env create ...)` clean and
   makes `-o json` the only contract scripts depend on.
4. **`describe` = YAML of the full object by default, `get` = one-row table** (gcloud
   `DescribeCommand` vs list format). Don't invent a third pretty format unless it's a domain
   view (gcloud run's `✔ Service hello in region us-central1` header + labeled sections is the
   model for `run describe`: header line with status glyph, then `URL:`/`Traffic:` sections).
5. **Adopt a status-glyph first column** for list output (gcloud run): unlabeled column with
   `✔` ready (green), `X` failed (red), `…` in progress (yellow), `!` degraded; ASCII fallback
   `+ X . !` when not UTF-8. Cheap, scannable, and survives `-o wide`.
6. **Empty list → `Listed 0 items.` style message on stderr, nothing on stdout** (gcloud). Already
   Admiral's rule; keep `-o json` returning `[]`.
7. **Lists are bare JSON arrays; singletons are objects** (gcloud, az). Never wrap in an
   envelope key the way aws does (`{"services": [...]}`) — it forces `--query` for everything.
   If a page token is needed, keep it on stderr (current behaviour) or add `--all` to auto-follow.
8. **Standardise the list flag set and document application order**: `--filter`, `--limit`,
   `--page-size`, `--sort-by ~field`, plus `-o name` (gcloud `--uri`). gcloud's help line
   *"applied in this order: --flatten, --sort-by, --filter, --limit"* is worth copying verbatim.
9. **Pick one expression language for `--filter`** and make it gcloud-shaped, not JMESPath:
   `key=value`, `key!=value`, `key:prefix*`, `key~regex`, `<,>`, `AND/OR/NOT`, `labels.env=prod`,
   `createTime>-P2W`. JMESPath (`--query`) is for reshaping output, and Admiral already has
   `-o json | jq`; don't add both.
10. **Use `--quiet`/`-q` semantics for "no prompts, take defaults", keep `--force/-f` only for
    the destructive-confirmation skip** (Admiral's current rule). Non-tty stdin without `-f`
    must fail with gcloud's exact hint: *"This prompt could not be answered because you are not
    in an interactive session. Re-run with --force."* (az downgrades to a WARNING + exit 1, which
    is worse for CI). Write prompts to stderr.
11. **Field mutation quartet**: `--set-labels`/`--update-labels`/`--remove-labels`/`--clear-labels`
    (gcloud) or at minimum `--label k=v` + `--remove-label k` + `--clear-labels`. "Set to empty"
    = `--clear-X` for maps/lists and `--X=""` for scalars, documented in the flag help
    (*"To reset this field to its default, pass an empty string."*). For arbitrary fields,
    az's generic `--set path=value` / `--remove path` is a good escape hatch on `update`.
12. **Long-running ops: block by default with a staged tracker on stderr, `--async`/`--no-wait`
    to return, and a `wait` verb with a documented contract** (gcloud stages + aws/az wait).
    `admiral run apply` should render `✓ Planning... Done.` / `✓ Applying (3/5 components)...`
    to stderr; `admiral run wait RUN --for=succeeded|finished --timeout 30m --interval 5s`
    should say in its help *"polls every 5s, exits 1 on failure, 124 on timeout"* (aws states
    *"poll every 15 seconds ... exit with a return code of 255 after 40 failed checks"*).
13. **Logs**: `admiral run logs RUN [--follow] [--since 10m|RFC3339] [--tail N] [--phase plan|
    apply] [--format text|json]`. Copy aws `logs tail` (`--since` single-unit relative, `--format
    short|detailed|json`, default lookback 10m) rather than gcloud's separate beta `tail` command
    or az's stream-only `log tail`. `--follow` is the flag; the verb stays `logs`.
14. **Error format**: `ERROR: (admiral.env.get) environment [prod] not found in application
    [shop].` — gcloud's command-path prefix makes CI logs greppable; add a hint line
    (`Did you mean 'list'?` / `Examples from command's help:` from az, `Maybe you meant:` from
    gcloud). Exit **2** for usage errors, **1** for runtime errors (all three agree on 2 for
    usage); consider aws-style specific codes only for auth (`253`) and timeout, and document
    them in `admiral help exit-codes` (aws `help return-codes`).
15. **Config introspection with provenance**: `admiral config list` should show the source of
    each value (az `az config get` → `"source": "AZURE_CORE_OUTPUT"`; aws `configure list` →
    `Location` column). Cheap and it kills a whole class of "why is it using that app?" tickets.
    Name env vars `ADMIRAL_<SECTION>_<KEY>` / `ADMIRAL_<KEY>` mechanically (gcloud `CLOUDSDK_*`,
    az `AZURE_*`).
16. **TTY rules**: color only when stdout is a tty, honour `NO_COLOR` (none of the three do —
    easy win), never page by default (aws v2's pager is the most complained-about change in
    its history), table width falls back to 80 when not a tty (gcloud), prompts require stdin
    tty. Never emit spinners to a non-tty; print stage lines instead (gcloud's non-tty tracker).
17. **Help text**: imperative, capitalized, period-terminated Short (`List environments.` — az
    and gcloud groups agree); an `Examples:` block on every leaf command with a one-line prose
    intro per example (gcloud `To list available services:`); mark flags `[Required]` and
    commands `[Preview]`/`[Deprecated]` inline (az); list global flags once as a single line
    (`GCLOUD WIDE FLAGS`) rather than repeating the block on every page (aws, az both repeat).
18. **Accept full identifiers as an alternative to name+parent** where Admiral has a canonical
    ID (az `--ids`, gcloud "fully qualified identifier" positional): `admiral run get
    shop/prod/r-123` or `--id`. This makes `list -o name | xargs admiral X delete` work.
19. **`-o none`/`--output off` for secret-returning commands** (az, aws): `admiral runner token
    create --output none` should be documented as the CI pattern, and `-o value(field)` /
    `-o tsv` (gcloud `value()`, az `tsv`) as the variable-capture pattern.
20. **Ship a `--flags-file`/`--cli-input-json` equivalent early for `changeset add` and
    `env create`** (gcloud/aws): `-f file.yaml` on create/update avoids shell-quoting hell for
    variables with commas, JSON, and secrets, and gives `--generate-skeleton` a home.
