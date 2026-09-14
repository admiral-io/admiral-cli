# CLI conventions research: docker (+compose), terraform, helm, stripe

All output below was captured locally on 2026-09-13 (docker 29.7.2, compose 5.5.1, terraform 1.15.7, helm v4.2.4, stripe 1.50.3) unless marked "docs". Snippets are trimmed, not edited.

---

## 1. docker / docker compose

### Grammar
- Root help is grouped: **Common Commands** (run, exec, ps, build, pull, push, images, login…), **Management Commands** (noun groups: `container`, `image`, `network`, `volume`, `context`, `system`, plus plugins marked `*`: `compose*`, `buildx*`, `scout*`), **Swarm Commands**, then the flat legacy **Commands** (attach, cp, events, inspect, logs, rm, stats, wait…), then **Global Options**.
- Two grammars coexist: noun-verb (`docker container ls`) and top-level verb shortcuts (`docker ps`). Each command prints an **Aliases** block:
  ```
  Aliases:
    docker container ls, docker container list, docker container ps, docker ps
  ```
  `ls`/`list`/`ps` are all accepted for list. `logs` aliases `container logs`; `events` aliases `system events`.
- Names/IDs are positional, multi-valued, and interchangeable: `docker inspect [OPTIONS] NAME|ID [NAME|ID...]`, `docker logs [OPTIONS] CONTAINER`, `docker stats [CONTAINER...]`. Prefix-matching of IDs is accepted.
- Compose: `docker compose [OPTIONS] COMMAND`, service names positional: `docker compose logs [SERVICE...]`, `docker compose ps [SERVICE...]`.
- Usage line format: `Usage:  docker ps [OPTIONS]` (two spaces after `Usage:`), `[OPTIONS]` before positionals.

### Hierarchy / scoping
- Daemon scope via global `-c, --context` / `-H, --host`, overriding `DOCKER_HOST` env and the default context set by `docker context use`. Help text spells out precedence: "overrides DOCKER_HOST env var and default context set with 'docker context use'".
- Compose project scope = `-p, --project-name` or `-f compose.yml` / cwd. When neither resolves: `no configuration file provided: not found` (exit 1). `docker compose ls` shows all projects (`NAME STATUS CONFIG FILES`, e.g. `infra  exited(1)  /path/compose.yml`).

### Input
- Filters are repeated `--filter key=value` (`-f`), never comma lists: `docker ps -a --filter status=exited --filter label=com.docker.compose.project`. Unknown key errors: `Error response from daemon: invalid filter 'foo'`.
- Compose `--status` is a repeatable stringArray with an enumerated set in help: `Values: [paused | restarting | removing | running | dead | created | exited]`.
- `-y, --yes` on `compose up`: "Assume "yes" as answer to all prompts and run non-interactively". `docker rm` refuses running containers unless `-f`: `Error response from daemon: cannot remove container "kind-control-plane": container is running: stop the container before removing or force remove`.
- `--dry-run` is a compose global flag ("Execute command in dry run mode").
- Config file location via `--config` / `DOCKER_CONFIG`.

### Output (human)
- `docker ps -a` default columns (UPPER, multi-word with spaces, 3-space gutter, no tabs):
  ```
  CONTAINER ID   IMAGE                       COMMAND                  CREATED        STATUS                    PORTS                                                NAMES
  8c1999a23b94   temporalio/temporal:1.7.0   "temporal server sta…"   5 days ago     Exited (255) 5 days ago   127.0.0.1:7233->7233/tcp, 127.0.0.1:8233->8233/tcp   shannon-temporal
  6b6d2aee1a37   kindest/node:v1.35.0        "/usr/local/bin/entr…"   3 months ago   Up 5 days                 127.0.0.1:49912->6443/tcp                            kind-control-plane
  ```
  - NAMES is the **last** column; ID is first (short 12-char). `--no-trunc` for full.
  - CREATED is relative ("5 days ago"). STATUS is a **sentence fragment combining state + duration + health**: `Up 5 days`, `Up 3 hours (healthy)`, `Exited (255) 5 days ago`, `Restarting (1) 4 seconds ago`. Exit code embedded in parentheses.
  - Truncation uses a single Unicode ellipsis `…` (COMMAND at ~20 chars).
- Empty result: prints **header row only**, exit 0 (no "No containers found" message). `-q` prints nothing.
- `docker compose ps` columns: `NAME IMAGE COMMAND SERVICE CREATED STATUS PORTS` (adds SERVICE). `docker system df`: `TYPE TOTAL ACTIVE SIZE RECLAIMABLE` with `6.826GB (76%)`.
- `docker inspect` = pretty JSON array of full API objects (`[ { "Id": ..., "State": { "Status": "running", "Running": true, ... } } ]`), 4-space indent. Not a table.
- No color in tables at all (docker ps output has zero ANSI codes). Color is reserved for compose's per-service log prefixes.
- Error messages go to stderr with prefix `Error response from daemon: ` (server errors) or `error: ` (client, e.g. `error: no such object: nonexistent-xyz` from inspect) or `docker: unknown command: docker psx`. Lowercase after the prefix.

### Output (machine): `--format`
- One flag, three modes, documented inline in every list command's help:
  ```
  --format string   Format output using a custom template:
                    'table':            Print output in table format with column headers (default)
                    'table TEMPLATE':   Print output in table format using the given Go template
                    'json':             Print in JSON format
                    'TEMPLATE':         Print output using the given Go template.
  ```
- `docker ps --format 'table {{.Names}}\t{{.Status}}'` → custom columns, header names derived from field names:
  ```
  NAMES                STATUS
  shannon-temporal     Exited (255) 5 days ago
  kind-control-plane   Up 5 days
  ```
- `docker ps --format json` → **JSON Lines, one object per row, not an array**, and a *curated, pre-formatted* view model, not the API object: `{"Command":"\"temporal server sta…\"","CreatedAt":"2026-09-08 10:53:35 -0400 EDT","HealthStatus":"starting","ID":"8c1999a23b94","Image":"temporalio/temporal:1.7.0","Labels":"k=v,k2=v2","Names":"shannon-temporal","RunningFor":"5 days ago","State":"exited","Status":"Exited (255) 5 days ago", ...}`. Note the JSON contains *display strings* (`"RunningFor":"5 days ago"`, truncated `Command`, labels flattened to a CSV string). Separate `State` (enum) vs `Status` (human sentence) fields.
- By contrast `docker compose ls --format json` returns a bare array `[{"Name":"infra","Status":"exited(1)","ConfigFiles":"..."}]`. Inconsistent across commands.
- `docker inspect -f '{{.State.Status}}'` for Go-template extraction; missing key is a hard error: `template parsing error: template: :1:26: executing "" at <.State.Health.Status>: map has no entry for key "Health"`.
- `-q, --quiet`: IDs only, one per line — the universal "pipe into xargs" convention (`docker ps -q`, `docker compose ps -q`).
- `docker events --format json` streams JSONL: `{"Type":"container","Action":"create","Actor":{"ID":"...","Attributes":{...}},"scope":"local","time":1789349792,"timeNano":...}`. `--since/--until` accept timestamps or relative durations.

### TTY awareness
- Tables render identically piped or not (no color to strip). `docker stats` **without** `--no-stream` still emits cursor-home/clear sequences (`^[[H`, `^[[K`, `^[[J`) when piped — a wart; `--no-stream` is the fix. Compose has `--ansi never|always|auto` (default auto) and `--progress auto|tty|plain|json|quiet`.
- `docker ps` reflows nothing on width; long rows wrap.

### Streaming & long-running
- `docker logs [OPTIONS] CONTAINER`:
  ```
  -f, --follow         Follow log output
      --since string   Show logs since timestamp (e.g. "2013-01-02T13:23:37Z") or relative (e.g. "42m" for 42 minutes)
  -n, --tail string    Number of lines to show from the end of the logs (default "all")
  -t, --timestamps     Show timestamps
      --until string   Show logs before a timestamp (...) or relative (e.g. "42m" ...)
      --details        Show extra details provided to logs
  ```
  `--tail` is a string so `all` is a legal value. Timestamps are RFC3339Nano prefixed: `2026-09-08T20:10:23.083654045Z [  OK  ] Reached target ...`. Container stdout→stdout, container stderr→stderr (preserves streams).
- `docker compose logs` adds `--no-log-prefix`, `--no-color`, `--index int` (replica), and prefixes lines with colored `service-1  | `.
- `docker compose up` waits: `--wait` ("Wait for services to be running|healthy. Implies detached mode"), `--wait-timeout int` seconds, `--exit-code-from SERVICE` ("Return the exit code of the selected service container"), `--abort-on-container-exit/-failure`, `-t, --timeout int` for shutdown. `docker wait CONTAINER` "Block until one or more containers stop, then print their exit codes".
- `docker events` = a first-class server-side event stream with `--filter` and `--since/--until`.

### Status / health / metrics
- `docker stats --no-stream`:
  ```
  CONTAINER ID   NAME                 CPU %     MEM USAGE / LIMIT    MEM %     NET I/O           BLOCK I/O         PIDS
  6b6d2aee1a37   kind-control-plane   39.20%    2.007GiB / 15.6GiB   12.87%    75.8MB / 1.64MB   24.4TB / 9.69GB   412
  ```
  Units embedded in values (`GiB`, `MB`, `%`), compound columns `MEM USAGE / LIMIT` and `NET I/O` as `in / out`. Live mode redraws in place (~1s). `--format` works here too (`table`/`json`/template).
- Health: `HealthStatus` field (`starting|healthy|unhealthy|none`) surfaces in STATUS as `(healthy)` suffix.
- `docker system df` for quotas/usage; `docker top CONTAINER` for processes.

### Errors & exit codes
- Exit 1 for runtime/daemon errors and unknown command; **exit 125** for CLI usage errors (`unknown flag: --bogus`, then `Usage:` + `Run 'docker ps --help' for more information`). `docker run` reserves 125 (daemon error), 126 (not executable), 127 (not found); otherwise the container's exit code.
- Not found: `Error response from daemon: No such container: nonexistent-xyz` (logs) vs `error: no such object: nonexistent-xyz` (inspect). No "did you mean".
- `-D, --debug` global; `-l, --log-level debug|info|warn|error|fatal`.

### Pagination & filtering
- No pagination; `-a/--all` (default hides non-running), `-n, --last int`, `-l, --latest`. Filtering is server-side `--filter key=value` with a fixed key vocabulary per command (`status=`, `name=`, `label=`, `health=`, `ancestor=`, `before=/since=`).

### Help text style
- Short descriptions: capitalized imperative, no trailing period ("List containers", "Fetch the logs of a container", "Display a live stream of container(s) resource usage statistics").
- Flag descriptions: capitalized sentence fragment, no period, defaults appended by pflag `(default "all")`. Long flag help wraps at ~80 cols. Enumerations are written inline: `("never"|"always"|"auto")`.
- Root help groups commands by audience with plugins marked `*`. Footer: `Run 'docker COMMAND --help' for more information on a command.`
- No Examples blocks in docker help (docs site has them). Compose help likewise.

### Config & auth
- `~/.docker/config.json` (`--config`, `DOCKER_CONFIG`); contexts (`docker context ls` shows `NAME DESCRIPTION DOCKER ENDPOINT ERROR`, current marked `*`). Precedence: `--context` flag > `DOCKER_CONTEXT` > `DOCKER_HOST` > `docker context use` default.

### Steal / avoid
- Steal: `--format` trio (`table`/`table TEMPLATE`/`json`/`TEMPLATE`); `-q` IDs-only; STATUS sentence with `(healthy)` suffix and exit code; `--since/--until` accepting RFC3339 *or* `42m`; separate `State` (enum) and `Status` (human) in JSON; `compose up --wait --wait-timeout --exit-code-from`; `--status` enum filter; empty list prints just the header.
- Avoid: JSON that contains pre-rendered display strings and flattened label CSV; JSONL in `docker ps --format json` vs array in `compose ls --format json`; `docker stats` leaking cursor control codes to pipes; exit 125 being undocumented in `--help`.

---

## 2. terraform

### Grammar
- Verb-first, flat, single-dash long flags (`-out=path`, `-var 'foo=bar'`, `-json`, `-no-color`). Root help: **Main commands** (init, validate, plan, apply, destroy) then **All other commands** alphabetically, then **Global options (use these before the subcommand, if any)**: `-chdir=DIR`, `-help`, `-version`.
- Usage line: `Usage: terraform [global options] plan [options]`, `terraform [global options] apply [options] [PLAN]`, `terraform [global options] output [options] [NAME]`, `terraform state list [options] [address...]`.
- Sub-noun groups exist only for `state` (`state list/show/mv/rm/pull/push`) and `workspace`.
- Flag values: `-flag=value` form is canonical (`-input=false`, `-refresh=false`, `-lock-timeout=0s`); `-var 'foo=bar'` and `-target=resource` are repeatable ("Use this option more than once").

### Hierarchy / scoping
- Scope = working directory (`-chdir`) + workspace (`terraform workspace select`), never a flag on plan/apply. Backend/remote config comes from files. Env `TF_WORKSPACE`, `TF_DATA_DIR`, `TF_CLI_ARGS` / `TF_CLI_ARGS_plan` (per-subcommand default args), `TF_VAR_name`, `TF_INPUT=0`, `TF_IN_AUTOMATION`, `TF_LOG`, `TF_LOG_PATH`.

### Input
- Variables: `-var 'name=store'` (repeatable), `-var-file=filename` (repeatable, "in addition to the default files terraform.tfvars and *.auto.tfvars"), `TF_VAR_name` env. Precedence: later `-var/-var-file` wins.
- Interactive prompts: `-input=true` default "Ask for input for variables if not directly set"; `-input=false` for automation. `-json` **implies** `-input=false` (docs).
- Approval: without `-auto-approve` apply prompts:
  ```
  Do you want to perform these actions?
    Terraform will perform the actions described above.
    Only 'yes' will be accepted to approve.

    Enter a value:
  ```
  Anything but `yes` → `Apply cancelled.` (exit 1). With stdin closed/non-TTY: `Error: error asking for approval: EOF`. `-auto-approve` skips; passing a saved plan file (`terraform apply tfplan`) also skips the prompt ("Terraform performs the operations in the saved plan without prompting").
- Plan file handoff: `-out=path` → `Saved the plan to: tfplan` + `To perform exactly these actions, run the following command to apply:\n    terraform apply "tfplan"`. Without `-out`, plan ends with `Note: You didn't use the -out option to save this plan, so Terraform can't guarantee to take exactly these actions if you run "terraform apply" now.`
- Targeting/replacement: `-target=resource`, `-replace=resource` (repeatable). Targeting emits `Warning: Resource targeting is in effect` explaining the plan may be partial.
- Locking: `-lock=false`, `-lock-timeout=0s` (Go duration).

### Output (human) — the plan
- Legend precedes the diff, listing only symbols actually used:
  ```
  Terraform used the selected providers to generate the following execution
  plan. Resource actions are indicated with the following symbols:
    + create
    ~ update in-place

  Terraform will perform the following actions:

    # terraform_data.a will be updated in-place
    ~ resource "terraform_data" "a" {
          id     = "086f5be6-d4ba-5e30-9535-431b4b49d4aa"
        ~ input  = "shop" -> "store"
        ~ output = "shop" -> (known after apply)
      }

  Plan: 0 to add, 1 to change, 0 to destroy.

  Changes to Outputs:
    ~ name = "shop" -> (known after apply)
  ```
  Symbols: `+` create, `-` destroy, `~` update in-place, `-/+` replace (destroy then create), `+/-` create then destroy, `<=` read. Unchanged attrs are printed unprefixed and aligned; changed values as `old -> new`; unknowns as `(known after apply)`; sensitive as `(sensitive value)`. Each resource is preceded by a `# addr will be <verb>` comment line. A horizontal rule `─────` separates the plan from trailing notes.
- Summary line is fixed-shape and grep-able: `Plan: 2 to add, 0 to change, 0 to destroy.`; apply: `Apply complete! Resources: 2 added, 0 changed, 0 destroyed.`; no-op: `No changes. Your infrastructure matches the configuration.` followed by a two-line explanation.
- Apply progress is per-resource, address-prefixed, with elapsed time and id:
  ```
  terraform_data.a: Refreshing state... [id=086f5be6-...]
  terraform_data.a: Creating...
  terraform_data.a: Creation complete after 0s [id=086f5be6-...]
  terraform_data.a: Modifying... [id=...]
  terraform_data.a: Modifications complete after 0s [id=...]
  ```
  Long operations print `Still creating... [10s elapsed]` every 10s.
- `terraform show` (no args) renders state as HCL-like (`# terraform_data.a:\nresource "terraform_data" "a" {\n    id = ...`). `terraform output` prints `name = "shop"`; `-raw name` prints bare `shop` (no newline) for shell capture; `-json` gives `{"name":{"sensitive":false,"type":"string","value":"shop"}}`.
- `terraform state list` prints bare addresses one per line (a `-o name` equivalent by default).
- Diagnostics are boxed, colored, with severity prefix, location, and a paragraph of guidance:
  ```
  ╷
  │ Error: Output "nope" not found
  │
  │ The output variable requested could not be found in the state file. If you
  │ recently added this to your configuration, be sure to run `terraform
  │ apply`, since the state won't be updated with new output variables until
  │ that command is run.
  ╵
  ```
  and for config errors: `Error: Invalid character` / `  on main.tf line 1, in variable "name":` / `   1: variable "name" { ... }` / explanation. Warnings share the shape (`Warning: ...`). `-compact-warnings` collapses warnings to their summary line.

### Output (machine)
- `-json` on plan/apply = **streaming JSON Lines UI** (documented as "machine-readable UI", schema version in first message):
  ```
  {"@level":"info","@message":"Terraform 1.15.7","@module":"terraform.ui","@timestamp":"2026-09-13T21:46:18.597015-04:00","terraform":"1.15.7","type":"version","ui":"1.3"}
  {"@level":"warn","@message":"Warning: Provider development overrides are in effect","@module":"terraform.ui","@timestamp":"...","diagnostic":{"severity":"warning","summary":"...","detail":"..."},"type":"diagnostic"}
  {"@level":"info","@message":"terraform_data.a: Plan to create","@module":"terraform.ui","@timestamp":"...","change":{"resource":{"addr":"terraform_data.a","module":"","resource":"terraform_data.a","implied_provider":"terraform","resource_type":"terraform_data","resource_name":"a","resource_key":null},"action":"create"},"type":"planned_change"}
  {"@level":"info","@message":"Plan: 2 to add, 0 to change, 0 to destroy.","@module":"terraform.ui","@timestamp":"...","changes":{"add":2,"change":0,"import":0,"remove":0,"action_invocation":0,"operation":"plan"},"type":"change_summary"}
  {"@level":"info","@message":"terraform_data.a: Creating...","...","hook":{"resource":{...},"action":"create"},"type":"apply_start"}
  {"@level":"info","@message":"terraform_data.a: Creation complete after 0s [id=...]","hook":{...,"elapsed_seconds":0},"type":"apply_complete"}
  {"@level":"info","@message":"Outputs: 1","outputs":{"name":{"sensitive":false,"type":"string","value":"store"}},"type":"outputs"}
  ```
  Envelope: `@level`, `@message` (the exact human line), `@module`, `@timestamp`, `type`. Types: `version`, `log`, `diagnostic`, `resource_drift`, `planned_change`, `change_summary`, `outputs`, `apply_start/progress/complete/errored`, `refresh_start/complete`, `provision_*`, test_*. Docs: "Clients … should handle unexpected message types by presenting at least the `@message` field." Versioned `ui` field ("1.3"; minor = additive).
- `terraform show -json tfplan` = full plan document (`format_version`, `planned_values`, `resource_changes[].change.{actions,before,after,after_unknown}`, `configuration`). `terraform show -json` = state document. These are the "full object" forms; `-json` streaming is the "curated event" form.
- No `-o table|json` unification: `-json` is a boolean per command. Only `output -raw` exists for scalars.

### TTY awareness
- Plan/apply body → **stdout**; diagnostics (warnings, errors) → **stderr**. Verified: `terraform plan 2>/dev/null` shows the plan without the warning box; `2>&1 >/dev/null` shows only diagnostics.
- Color: stdout auto-disables when not a TTY, but **stderr diagnostics stayed colored when piped and even with `NO_COLOR=1`** — only `-no-color` (or `TF_CLI_ARGS=-no-color`) suppresses it. Widely considered a bug; do not copy.
- Non-TTY stdin with a prompt pending does not silently default; it errors (`error asking for approval: EOF`).

### Streaming & long-running
- No follow/watch flags; the operation itself streams progress lines (`Creating...`, `Still creating... [Ns elapsed]`). `-parallelism=n` (default 10). Interrupt (Ctrl-C) triggers graceful stop, second Ctrl-C forces.

### Errors & exit codes
- Default: 0 success, 1 any error (including `Apply cancelled.`). `plan -detailed-exitcode`: **0 = no changes, 1 = error, 2 = changes present** — verified (`exit=2` with diff, `exit=0` after apply). This is the canonical "diff as exit code" contract used by CI gates.
- Error format: `Error: <Title Case summary>` + blank + wrapped detail paragraph, boxed. Not-found for output: `Error: Output "nope" not found` with remediation. Unknown `-target` address silently produces no-change plan (surprising).
- Apply does not roll back on failure; state is saved for completed resources and the error is reported (docs).
- `TF_LOG=debug|trace` for verbosity, never a `-v` flag.

### Pagination & filtering
- None; `state list [address...]` filters by address prefix and `-id=ID`.

### Help text style
- Long-form prose help; each command starts with a paragraph, then `Plan Customization Options:` / `Other Options:` groups. Flag lines are `-flag=default   Description sentence(s) with periods.` Descriptions are full sentences, capitalized, with periods, sometimes multi-sentence with rationale ("This is for exceptional use only."). No Examples blocks in `-help`. Short descriptions in root help: capitalized imperative, no period ("Show changes required by the current configuration").

### Config & auth
- `~/.terraformrc` / `terraform.rc` (credentials blocks, `dev_overrides`), `TF_TOKEN_<host>` env, `terraform login HOST`. Precedence per subcommand: flags > `TF_CLI_ARGS_<cmd>` > `TF_CLI_ARGS` > config.

### Steal / avoid
- Steal: the diff legend (print only symbols used) + `# addr will be <verb>` header + aligned `old -> new`; the fixed-shape summary sentence `Plan: N to add, N to change, N to destroy.`; `-detailed-exitcode` 0/1/2; `-out` + "run this to apply exactly these actions" hand-off; `yes`-only approval with `Only 'yes' will be accepted`; JSONL UI stream with `@message` carrying the identical human line; `Still creating... [10s elapsed]` heartbeat; boxed diagnostics with a detail paragraph telling you what to do; `output -raw` for shell capture; `-compact-warnings`.
- Avoid: single-dash long flags; ignoring `NO_COLOR`; coloring stderr when piped; `-json` implying `-input=false` silently; no `-o` unification.

---

## 3. helm

### Grammar
- Verb-first, flat: `helm list`, `helm status RELEASE_NAME`, `helm history RELEASE_NAME`, `helm rollback <RELEASE> [REVISION]`, `helm upgrade [RELEASE] [CHART]`, `helm install [NAME] [CHART]`, `helm uninstall RELEASE`. Only `get` is a noun group: `helm get all|hooks|manifest|metadata|notes|values RELEASE`. Aliases: `list, ls`; `history, hist`.
- Root help is a prose page: "Common actions for Helm" (4 bullets), a Markdown table of every `$HELM_*` env var with description, XDG path table, then `Available Commands:` (cobra), then `Flags:` (global). Short descriptions are **lowercase** imperative, no period ("list releases", "fetch release history", "roll back a release to a previous revision").

### Hierarchy / scoping
- Namespace is the only parent: `-n, --namespace` global flag, `HELM_NAMESPACE` env, else kubeconfig context namespace. `-A, --all-namespaces` on list. Cluster via `--kube-context`/`--kubeconfig`/`HELM_KUBECONTEXT`.
- Omitting `-n` for a release in another namespace → `Error: release: not found` (exit 1) — it does **not** say which namespace it looked in nor suggest `-A`. Widely complained about.

### Input
- Values layering, documented precedence in help: `-f, --values strings` (repeatable; "priority will be given to the last (right-most) file"), `--set key=val,key2=val2` (repeatable, comma-separable), `--set-string`, `--set-file key=path`, `--set-json key=jsonval`, `--set-literal`. On upgrade: `--reuse-values`, `--reset-values`, `--reset-then-reuse-values`.
- Labels: `-l, --labels stringToString` "Should be separated by comma… **You can unset label using null**" (`-l foo=null`) — the "clear a field" idiom.
- `--dry-run string[="unset"]` with enum `none|client|server` ("--dry-run=client simulates client-side only and avoids cluster connections"). Helm 4 replaced `--atomic` with `--rollback-on-failure` and the boolean `--wait` with `--wait WaitStrategy[=watcher]` (`watcher|hookOnly|legacy`), plus `--wait-for-jobs`, `--timeout duration (default 5m0s)`.
- `helm install -g, --generate-name`, `--name-template`. `upgrade -i, --install` (upsert). `--description string` "add a custom description" (free-text on a revision — shows in history).
- Rollback: `helm rollback <RELEASE> [REVISION]` — "If this argument is omitted or set to 0, it will roll back to the previous release." `--cleanup-on-fail`, `--history-max int (default 10)`.
- Destructive ops (`uninstall`, `rollback`) have **no confirmation prompt**; `--dry-run` is the safety net.

### Output (human)
- `helm list -A` (real):
  ```
  NAME             	NAMESPACE        	REVISION	UPDATED                             	STATUS  	CHART                  	APP VERSION
  admiral-app      	admiral-app      	12      	2026-09-08 17:21:13.36733 -0400 EDT 	deployed	admiral-app-0.1.0      	latest
  ```
  Columns `NAME NAMESPACE REVISION UPDATED STATUS CHART APP VERSION`. Cells are padded **and tab-separated** (gopkg.in/gookit uitable) — looks aligned in a terminal, breaks when pasted. UPDATED is a raw Go `time.String()` with microseconds and zone; `--time-format "2006-01-02T15:04:05Z07:00"` fixes it (`2026-09-08T17:21:13-04:00`). No AGE column, no relative time. STATUS words: `deployed|failed|superseded|uninstalled|uninstalling|pending-install|pending-upgrade|pending-rollback|unknown`. `--no-headers`; `-q, --short` prints names only.
- Empty list: header row only, exit 0 (`helm list --filter zzz`); with `-o json` → `[]`. Help text states this explicitly: "If no results are found, 'helm list' will exit 0, but with no output (or … only headers)."
- `helm history` (real):
  ```
  REVISION	UPDATED                 	STATUS    	CHART            	APP VERSION	DESCRIPTION
  10      	Sun Aug 23 13:01:31 2026	superseded	admiral-app-0.1.0	latest     	Upgrade complete
  11      	Tue Sep  8 17:20:15 2026	failed    	admiral-app-0.1.0	latest     	Upgrade "admiral-app" failed: conflict occurred while applying object ... .spec.replicas
  12      	Tue Sep  8 17:21:13 2026	deployed  	admiral-app-0.1.0	latest     	Upgrade complete
  ```
  Oldest→newest. DESCRIPTION carries the completion/error message and is **not truncated** (a 300-char failure line blows out the table). Descriptions are fixed phrases: `Install complete`, `Upgrade complete`, `Rolled back to 2`, `Upgrade "x" failed: ...`. Uses a different date format from `list` (ANSIC `Tue Sep  8 17:21:13 2026`).
- `helm status RELEASE` = key/value header block then resource tables then free-text NOTES:
  ```
  NAME: admiral-app
  LAST DEPLOYED: Tue Sep  8 17:21:13 2026
  NAMESPACE: admiral-app
  STATUS: deployed
  REVISION: 12
  DESCRIPTION: Upgrade complete
  RESOURCES:
  ==> v1/ConfigMap
  NAME                 DATA   AGE
  admiral-app-config   1      67d
  ...
  NOTES:
  <chart notes, free text>
  ```
  `--revision int` shows a historical revision. `helm get metadata` is the same KEY: value layout (`NAME: CHART: VERSION: APP_VERSION: ANNOTATIONS: LABELS: DEPENDENCIES: NAMESPACE: REVISION: STATUS: DEPLOYED_AT: APPLY_METHOD:`), with `-o yaml` giving camelCase keys (`appVersion`, `applyMethod: ssa`).
- Color: `--color never|auto|always` (also `--colour`), `HELM_COLOR`, honors `NO_COLOR` ("overrides $HELM_COLOR"). With color: headers bold, NAMESPACE cyan, STATUS green/red by state.

### Output (machine)
- `-o, --output format` "Allowed values: table, json, yaml (default table)" on list/status/history/get metadata/install/upgrade. `-o json` for list is a **bare array of curated snake_case rows** matching the table: `[{"name":"admiral-app","namespace":"admiral-app","revision":"12","updated":"2026-09-08 17:21:13.36733 -0400 EDT","status":"deployed","chart":"admiral-app-0.1.0","app_version":"latest"}]` (note `revision` is a **string** in list JSON but an int in history JSON; `updated` is RFC3339 in history JSON but Go-string in list JSON).
- `helm status -o json` is the full release object: keys `name, info{first_deployed,last_deployed,description,status,notes,resources}, config, manifest, hooks, version, namespace, apply_method`.
- No jsonpath/go-template/custom-columns; pipe to jq/yq.

### TTY awareness
- Tables identical when piped (tabs + padding). Color auto-off when not a TTY. No pager. No prompts anywhere, so nothing to suppress.

### Streaming & long-running
- `--wait` (strategy) + `--timeout` (Go duration, default `5m0s`) + `--wait-for-jobs`; `--rollback-on-failure` (ex-`--atomic`) auto-reverts on failed upgrade and defaults `--wait` on. Install/upgrade print the same `helm status` block on success (`NAME/LAST DEPLOYED/NAMESPACE/STATUS/REVISION/NOTES`). No spinner, no per-resource progress; `--debug` streams slog lines (`level=DEBUG msg="getting release history" name=nope`).

### Status / health / metrics
- Release-level status word only; resource health is delegated to `helm status` resource tables (kubectl-style `NAME READY STATUS AGE`). No metrics.

### Errors & exit codes
- Exit 0/1 only. Format: `Error: <lowercase message>` on stderr: `Error: release: not found`, `Error: path "./nope" not found`, `Error: could not convert revision to a number: strconv.Atoi: parsing "b": invalid syntax` (raw Go error leaked). Usage errors print `Error: "helm rollback" requires at least 1 argument` + `Usage:` line. `Error: unknown command "get values" for "helm"` + `Run 'helm --help' for usage.`
- `--debug` = "enable verbose output" (slog debug to stderr).

### Pagination & filtering
- `helm list`: `-m, --max int (default 256)` + `--offset int` ("next release index in the list") = offset paging; `-f, --filter` is a **Perl regex on name**; `-l, --selector` label query (`=`,`==`,`!=`, comma-joined); state flags are booleans that OR together: `--deployed --failed --pending --superseded --uninstalled --uninstalling`; `-d, --date` sort by date, `-r, --reverse`. `history --max`.

### Help text style
- Long description in prose with embedded `$ helm ...` examples and a sample table, then `Usage:`, `Aliases:`, `Flags:`, `Global Flags:`. Flag descriptions lowercase, no period, often long single lines (120+ cols). Short descriptions lowercase. The root help embeds the full env-var table — a good discoverability move.

### Config & auth
- Kube auth entirely from kubeconfig/`--kube-*` flags/`HELM_KUBE*` env; XDG config/cache/data dirs with `HELM_*_HOME` overrides. `helm env` prints the effective values. `HELM_MAX_HISTORY` caps stored revisions.

### Steal / avoid
- Steal: `history` as first-class verb with `REVISION UPDATED STATUS CHART APP VERSION DESCRIPTION` and human `DESCRIPTION` phrases (`Rolled back to 2`); `rollback NAME [REVISION]` defaulting to previous; `--description` free-text on a mutation; `status --revision N`; `-l key=null` to unset; `--wait`/`--timeout`/`--rollback-on-failure`; boolean state filters that OR; `--time-format`; env-var table in root help; explicit doc that empty list = exit 0 + header.
- Avoid: tab-separated tables; raw `time.String()` timestamps; `revision` as string in one JSON and int in another; untruncated DESCRIPTION; `Error: release: not found` with no name/namespace/hint; no confirmation on `uninstall`/`rollback`; leaking `strconv.Atoi` errors.

---

## 4. stripe

### Grammar
- Root help groups by purpose: **Webhook commands** (listen, trigger), **Stripe commands** (logs), **Resource commands** (charges, customers, payment_intents, `...` "run `stripe resources help`", `v2`), **API commands** (get, post, delete), **Other commands** (config, docs, fixtures, keys, login, logout, open, sandbox, switch, whoami…), **Available plugins** (installable on first use). Also `--map [tree|compact|paths|json]` prints the whole command tree, and an `[Agent guidance]` block in every help page (new: instructions aimed at LLM agents — `stripe sandbox create`, `--api-key`/`STRIPE_API_KEY`, `stripe --map`).
- Resource grammar is **noun operation**: `stripe customers <operation> [parameters...]`, operations enumerated from the OpenAPI spec (`create delete list retrieve update search balance_transactions …`). `stripe help customers list` shows per-operation params. Resource names are plural snake_case (`payment_intents`), IDs positional (`stripe customers update cus_9s6XKzkNRiz8i3 -d "metadata[key]=value"`).
- Raw escape hatch: `stripe get /v1/customers`, `stripe post /v1/customers -d email=a@b.c`, `stripe delete /v1/customers/cus_x`.
- Usage lines: `stripe customers list [--param=value] [-d "nested[param]=value"]`, `stripe events resend <event> [--param=value] [-d "nested[param]=value"]`.

### Hierarchy / scoping
- Scope = profile (`-p, --project-name`, default `default`, sections in one `config.toml`) × mode (test/sandbox by default, `--live` per command) × account context (`stripe switch context acct_123 [--live]`, `stripe login list`). `--stripe-account acct_x` header for Connect. No resource nesting in the command tree; parent IDs are params (`--customer cus_x`).

### Input
- Params are generated flags with API types in help: `--email <string>`, `--limit <integer>`, `--cash-balance.settings.reconciliation-mode automatic|manual|merchant_default` (dotted nested path, enum inline), plus generic `-d, --data stringArray` for anything using form syntax `-d "metadata[key]=value"`. `-e, --expand stringArray` repeatable.
- **Clear a field**: `--address=""  (pass empty string to remove this field)` — printed in help for nullable params.
- `--dry-run` prints the exact request it would send (method, url, params, headers with key masked) instead of sending — verified:
  ```
  {
    "dry_run": {
      "method": "GET",
      "url": "https://api.stripe.com/v1/customers",
      "params": { "expand": ["data.default_source"], "limit": 2, "metadata": {"foo": "bar"}, "starting_after": "cus_123" },
      "headers": { "Authorization": "Bearer sk_test_*ogus" }
    }
  }
  ```
- Confirmation: mutating/`--live` commands print a context banner and prompt; `-c, --confirm` skips:
  ```
  This command will be executed on the account with the following details:
  > Mode: Test
  > Account Name: Datalift
  Are you sure you want to perform the command: DELETE?
  ```
- `-i, --idempotency string` for safe retries; `-v, --stripe-version` pins API version; `-s, --show-headers`.
- Fixtures: `stripe fixtures file.json --override customer:email=x --add --remove --skip` with `${resource:json_path}` and `${.env:VAR|default}` references.

### Output (human)
- Resource responses are **pretty-printed JSON of the full API object** (2-space), colorized on TTY (`--dark-style` alternate scheme). There are no tables for resources at all. Lists come back as the API list envelope `{"object":"list","data":[...],"has_more":true,"url":"/v1/customers"}`.
- `stripe whoami` is a right-aligned key/value block:
  ```
  Profile:              default
  Account:              Datalift (acct_1J4pdHHwB1fRuWyJ)
  Device name:          Orion.local
  Sandbox key:          available (expires 2025-04-20)
  Live mode key:        not available
  API version:          2026-07-29.dahlia
  ```
  and `--format json` gives a **documented stable schema** (`authenticated`, `test_mode_key{available,expires_at}`, `live_mode_key{...}`); help lists exit codes explicitly (`0 Authenticated … 1 Not authenticated, or an error occurred`).
- `stripe config --list` dumps the TOML with secrets masked: `live_mode_api_key = 'rk_live_****...hR56'` (last 4 visible).
- Errors: blank line, then message, then hint sentence, on stderr, exit 1: `The API key for the default profile has expired. Run \`stripe login\` to re-authenticate.` + `If you recently ran \`stripe login\` and still see this error, it may have authenticated a different profile — run \`stripe whoami\` to confirm.` API errors: `Request failed, status=401, body={ "error": { "message": "Invalid API Key provided: sk_test_*ogus", "type": "invalid_request_error" } }`.

### Output (machine)
- Resources: already JSON. Streams: `--format JSON` on `listen`/`logs tail` (one event per line). `whoami --format json`, `login --non-interactive` (JSON), `--map=json`. No jq/template flags — pipe to jq is the documented pattern.

### TTY awareness
- `--color on|off|auto` (global; persisted with `stripe config --set color off`). `login` auto-switches to `--non-interactive` when stdin is not a TTY and prints JSON instead of opening a browser — verified:
  ```
  {"browser_url": "https://access.stripe.com/stripecli/oauth2/device", "verification_code": "PZHS-HSJZ", "next_step": "stripe login --complete-device"}
  ```
- Confirmation prompt still renders with stdin closed (then fails) — no auto-`--confirm`.

### Streaming & long-running
- `stripe listen [--forward-to localhost:4242/webhook] [--events a,b] [--live] [--latest] [--print-secret] [--format JSON]` — comma-list `strings` flags. Terminal rendering (docs):
  ```
  > Ready! Your webhook signing secret is whsec_abcdefg1234567
  2022-01-28 09:47:46   --> customer.created [evt_abc123]
  2022-01-28 09:48:22  <--  [200] POST http://localhost:4242/webhook [evt_abc123]
  ```
  `-->` inbound event, `<--` forward response with status code; IDs in brackets.
- `stripe logs tail` — same `> Ready!` banner then `2022-01-28 09:47:46 [200] POST /v1/customers [req_abc123]`; filters are typed `--filter-<dimension> strings` (comma lists) with enumerated values in help: `--filter-http-method GET,POST`, `--filter-status-code-type 2XX|4XX|5XX`, `--filter-request-status SUCCEEDED|FAILED`, `--filter-source API|DASHBOARD`, `--filter-request-path`, `--filter-ip-address`, `--filter-account connect_in|connect_out|self`.
- `stripe trigger <event>` fabricates a full event chain; `stripe events resend evt_x [--webhook-endpoint we_x]` replays. `-s, --skip-update` disables the update check on `listen`.

### Status / health
- `stripe whoami` (no API call — reads local creds), `stripe status` (service status page) exists in older versions. No metrics.

### Errors & exit codes
- 0/1 only (documented on `whoami`). Messages are full sentences, capitalized, with a "Run `stripe login`" style remediation. Unknown command: `Unknown command "logs tail" for "stripe".` + `See "stripe --help" for a list of available commands.` (note: multi-word `stripe logs tail --help` fails; must use `stripe help logs tail`).
- `--log-level debug|info|trace|warn|error`.

### Pagination & filtering
- API-native cursor pagination as flags: `--limit <integer>` (1–100, default 10), `--starting-after <id>` / `-b, --ending-before <id>`; response carries `has_more`. Filters are per-resource typed params (`--email`, `--created`), plus `search` operations with a query string.

### Help text style
- Cobra: `Usage:`, `Examples:` (real, multi-line with `\` continuation), `[Agent guidance]`, `Flags:`, `Global flags:`. Short descriptions capitalized imperative, no period ("Listen for webhook events", "Tail API request logs from your Stripe requests." — inconsistent period). Flag help capitalized, enumerations as indented `Acceptable values:` lists with per-value descriptions. Generated param help shows type in angle brackets `<string>` and the API doc sentence beneath.

### Config & auth
- `$HOME/.config/stripe/config.toml` (`XDG_CONFIG_HOME` aware) with `[profile]` tables; secrets in OS keychain when available. Precedence (docs): env `STRIPE_API_KEY` / `STRIPE_DEVICE_NAME` > `--api-key` flag > `stripe config --set test_mode_api_key` > `stripe login`. Sessions auto-refresh; `stripe login --new-session`, `stripe reauth`, `stripe logout [--all]`. Login: pairing code + browser (`Your pairing code is: word-word-word-word`), `--interactive` to paste a key, `--non-interactive` + `--complete <poll-url>` two-step for agents/CI. `stripe sandbox create` provisions a throwaway account without a browser.

### Steal / avoid
- Steal: `--dry-run` that prints the exact request; `whoami` with documented stable JSON + documented exit codes; `login --non-interactive` JSON hand-off with `next_step`; `--confirm/-c` + banner that names the account and mode before a mutation; `--live` as explicit opt-in to the dangerous mode; `--param=""` to clear a field; `--limit/--starting-after/--ending-before` cursor flags mirroring the API; `> Ready!` banner + `-->`/`<--` stream glyphs with IDs in `[brackets]`; masked secrets in `config --list`; `--map` command tree; remediation sentence in every error; `[Agent guidance]` block.
- Avoid: no tables anywhere (JSON-only is fine for an API wrapper but not for an ops CLI); `stripe logs tail --help` not working; inconsistent trailing periods; enum values in stringArray flags rather than typed flags.

---

## Lessons for Admiral

1. **Adopt docker's `--format`/`-o` trio semantics under Admiral's existing `-o`:** keep `-o table|wide|json|yaml`, add `-o go-template=...` and `-o custom-columns`-style `table {{.Name}}\t{{.Status}}` (docker). Keep JSON as the full API object; never put display strings ("5 days ago", truncated commands) in JSON (docker's mistake).
2. **Make list JSON shape consistent everywhere.** Pick one: bare array (helm, compose ls) or wrapped `{items:[], next_page_token}`. Docker mixes JSONL and arrays; helm mixes `revision` string/int and two date formats. Admiral's pagination token argues for a wrapped object for lists and a bare object for get.
3. **STATUS column = state word + duration + health suffix** (docker): `Running 3h (healthy)`, `Failed (exit 1) 5d ago`, `Applied 2h ago`. Keep a separate machine `state` enum in JSON alongside human `status`.
4. **Add `-q/--quiet` (names/IDs only, one per line) to every list** (docker `ps -q`, helm `list -q`). This is the xargs contract; it pairs with `-o name` from kubectl.
5. **Empty list: header only, exit 0** on table output (docker, helm — helm documents it in help). Admiral's stderr "No X found." is fine as long as stdout stays empty/header and exit is 0; JSON must still emit the full (empty) envelope.
6. **`run plan` output should copy terraform's diff grammar**: legend listing only the symbols used (`+ create`, `~ update in-place`, `- destroy`, `-/+ replace`), a `# <component> will be <verb>` header per item, aligned `old -> new`, `(known after apply)` for unknowns, and a fixed-shape summary line `Plan: N to add, N to change, N to destroy.` Emit the plan to stdout, diagnostics to stderr.
7. **`run plan --detailed-exitcode` → 0 no changes / 1 error / 2 changes** (terraform). CI gates on this. Document it in the flag help exactly as terraform does.
8. **Approval UX (terraform):** `run apply` prompts `Only 'yes' will be accepted to approve.`; `--auto-approve` (or Admiral's `--force/-f`) skips; a non-TTY stdin without `--auto-approve` must **error** ("error asking for approval: EOF"), never default to yes. Print stripe's banner before the prompt: target app/env and whether it's a production env. Add `--input=false` semantics (fail rather than prompt) for automation.
9. **Plan → apply hand-off:** `run plan --out` prints "To perform exactly these actions, run: `admiral run apply <plan-id>`"; `run apply <plan-id>` skips the prompt because the plan was already reviewed (terraform). Without a saved plan, print terraform's "Note: you didn't save this plan…" caveat.
10. **Streaming JSON for long-running verbs (terraform `-json`):** `run apply -o json` and `run logs -o json` should stream JSON Lines with a stable envelope `{ "@timestamp", "@level", "@message", "type", ...payload }` where `@message` is the exact human line, a `version` message first with a `ui` schema version, and typed events (`planned_change`, `change_summary`, `apply_start/complete/errored`, `outputs`, `diagnostic`). Document "unknown types: render `@message`".
11. **Per-resource progress lines with heartbeat** (terraform): `web: Creating...`, `web: Still creating... [10s elapsed]`, `web: Creation complete after 42s [id=...]`. Apply summary `Apply complete! Resources: 2 added, 1 changed, 0 destroyed.`
12. **`run logs` flags = docker logs exactly:** `-f/--follow`, `-n/--tail N` (default all), `--since`/`--until` accepting RFC3339 **or** relative `42m`, `-t/--timestamps` (RFC3339Nano prefix). Add compose's `--no-log-prefix`/`--no-color` for multi-phase interleaved logs with a colored `phase | ` prefix.
13. **Wait semantics (compose/helm):** `--wait` + `--timeout 5m` (Go duration, shown as `5m0s` in help) on `run apply`, `env create`, etc.; `run apply --wait` exits with the run's final status (compose `--exit-code-from`). Consider helm's `--rollback-on-failure`.
14. **Revision history verbs (helm):** `run history <run>` / `env history prod --app shop` with columns `REVISION  UPDATED  STATUS  <what changed>  DESCRIPTION`, oldest→newest, DESCRIPTION truncated at ~40 like Admiral's narrow row (helm's untruncated column is the anti-pattern). `run rollback <run> [REVISION]` defaulting to the previous revision, and `--description` free text recorded on every mutation.
15. **`status` layout (helm/stripe whoami):** a `KEY:   value` header block (`NAME: LAST DEPLOYED: STATUS: REVISION: DESCRIPTION:`), then sub-tables (`RESOURCES:`), then free text. Right-align values in a column like `stripe whoami`. Provide `--revision N` to view a historical status.
16. **Metrics/stats table (docker stats):** `NAME  CPU %  MEM USAGE / LIMIT  MEM %  NET I/O  ...` with units inside values (`2.0GiB / 15.6GiB`, `39.2%`), `--no-stream` for a snapshot (default on non-TTY — docker gets this wrong), live redraw on TTY only.
17. **`--dry-run` that prints the request** (stripe) for every mutating command: method/URL/body with secrets masked. Cheap to implement over the SDK and invaluable for agents and debugging.
18. **Errors: `Error: <summary>` + remediation sentence** (terraform boxed detail, stripe "Run `stripe login` to re-authenticate"). Never `Error: release: not found` (helm) — say `Error: environment "prod" not found in app "shop" (try 'admiral env list --app shop')`. Never leak Go internals (`strconv.Atoi: parsing "b"`). Reserve a distinct exit code for CLI usage errors (docker uses 125; 2 is more conventional) versus 1 for server/runtime errors.
19. **Color/TTY:** honor `NO_COLOR`, provide `--color auto|always|never` (helm names both `--color` and `--colour`; stripe persists it via config), and disable color on **stderr too** when not a TTY — terraform's colored stderr under `NO_COLOR` is the cautionary tale. Auto-degrade prompts to errors and live tables to snapshots when not a TTY.
20. **Auth/whoami:** ship `admiral whoami --format json` with a documented, stable schema and documented exit codes (stripe), `auth login` non-interactive two-step (`--non-interactive` prints `{browser_url, verification_code, next_step}`; `--complete <url>` polls) that activates automatically when stdin is not a TTY, and mask stored secrets in any config dump (`rk_live_***hR56`).
21. **Filtering:** typed `--status` enum flag listing values in help (compose `Values: [running | exited | ...]`) rather than free-form `--filter`; repeatable `--filter key=value` (docker) only for open-ended dimensions; `--time-format` (helm) is unnecessary if AGE + `-o wide` full timestamp exist.
22. **Help text:** capitalized imperative short descriptions with no trailing period (docker, terraform, stripe — helm's lowercase is the outlier); an `Aliases:` block; an `Examples:` block with `\`-continued real commands (stripe); enumerations inline as `(auto|always|never)`; root help grouped by audience (docker: Common / Management / Other); an env-var table somewhere discoverable (helm root help); and stripe's `[Agent guidance]` block or an equivalent `admiral --map` command tree for LLM agents.
