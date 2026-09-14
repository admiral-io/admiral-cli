# kubectl and argocd: CLI conventions research

Sources: local binaries (`kubectl` v1.36.3, `argocd` v3.5.1; help output and offline/dry-run
behaviour only), kubectl SIG-CLI conventions doc, kubernetes.io kubectl reference / kuberc /
quick-reference pages, kubectl source (`pkg/cmd/get/get.go`, `pkg/printers/internalversion/printers.go`,
`apimachinery/pkg/util/duration`, `pkg/cmd/util/helpers.go`, `pkg/cmd/delete/delete.go`,
`polymorphichelpers/rollout_status.go`), argocd source (`cmd/argocd/commands/app.go`, `app_diff.go`,
`util/errors`, `util/cli`, `cmd/argocd/commands/utils/prompt.go`), argocd docs (getting started,
health, environment variables).

---

## Part 1: kubectl

### 1. Grammar

**verb-noun**, one level deep, with the noun being a generic `TYPE` positional:

```
kubectl get pods
kubectl get pod web-pod-13je7
kubectl get rc/web service/frontend pods/web-pod-13je7      # TYPE/NAME, mixed types
kubectl get rc,services                                      # comma list of types
kubectl get deployments.v1.apps                              # TYPE[.VERSION][.GROUP] fully-qualified
```

Usage line for `get` (real):

```
kubectl get [(-o|--output=)json|yaml|kyaml|name|go-template|...|custom-columns|custom-columns-file|wide]
            (TYPE[.VERSION][.GROUP] [NAME | -l label] | TYPE[.VERSION][.GROUP]/NAME ...) [flags] [options]
```

- Resource types accept singular, plural and short aliases (`pod`, `pods`, `po`) — `Aliases:` block is
  printed in help (`kubectl top pod --help` → `Aliases: pod, pods, po`).
- Verbs that only apply to a subset of resources are grouped as a noun-ish parent with sub-verbs:
  `kubectl rollout {status|history|undo|pause|resume|restart} TYPE/NAME`, `kubectl top {node|pod}`,
  `kubectl config {get-contexts|use-context|set-context|view|...}` (hyphenated compound verbs inside
  `config`, widely considered inconsistent).
- SIG-CLI rule: "Command names are all lowercase, and hyphenated if multiple words"; "kubectl VERB NOUNs
  for commands that apply to multiple resource types"; "NOUNs may be specified as `TYPE name1 name2` or
  `TYPE/name1 TYPE/name2` or `TYPE1,TYPE2,TYPE3/name1`".

### 2. Hierarchy / scoping

Only one level of scope: **namespace**. It is never positional; it's `-n/--namespace` (global flag) or
defaulted from the kubeconfig context (`kubectl config set-context --current --namespace=foo`).

- `-A/--all-namespaces` lifts the scope; then the printed table gains a leading `NAMESPACE` column.
  Help text: "Namespace in current context is ignored even if specified with --namespace."
- Omitted namespace → the context's namespace → `default`. Help for `get` says it plainly: "If the
  desired resource type is namespaced you will only see results in the current namespace if you don't
  specify any namespace."
- In-cluster: `POD_NAMESPACE` env var sets the default namespace; `--namespace` flag overrides.
- Cross-resource references inside output are rendered `TYPE/name` (`Controlled By:  ReplicaSet/nginx-deployment-67d4bdd6f5`).
- SIG-CLI principle: "Explicit should always override implicit".

### 3. Input

- Positionals are `TYPE` and `NAME...` only; everything else is a flag.
- `-f FILE` / `-f -` (stdin) / `-f dir/` / `-f URL` / `-k dir/` (kustomize), `-R` recursive.
- Repeated flags AND comma lists both accepted where it matters: `-L label1 -L label2` or
  `-L a,b`; `--from-literal=key=val` repeated; `--from-file=[key=]path`; `--from-env-file`.
- key=value positionals for `label`/`annotate`: `kubectl label pods foo unhealthy=true`.
  **Clear a field**: trailing minus — `kubectl label pods foo bar-` ("Does not require the --overwrite
  flag"). `--overwrite` is required to change an existing label value.
- `--dry-run=none|client|server` (an enum, not a bool; the old bool form was deprecated as a mistake).
  Output with dry-run: `configmap/x created (dry run)`.
- Prompts: kubectl has essentially **no interactive prompts** by default. `delete` gained
  `-i/--interactive` (opt-in, can be defaulted on via kuberc). The prompt text (from source):

  ```
  You are about to delete the following 2 resource(s):
  pod/foo
  deployment.apps/bar
  Do you want to continue? (y/N):
  ```
- `--force` in kubectl means "force delete, bypass graceful deletion" — NOT "skip confirmation".
- Flag descriptions: "start with an uppercase letter and not have a period at the end" (SIG-CLI);
  kubectl help renders flags as `    -o, --output='':` on one line with description indented below.
- Boolean flags print their default: `--watch=false`, `--wait=true`. Durations use Go syntax
  (`--since=1h`, `--timeout=30s`, `--pod-running-timeout=20s`); RFC3339 for absolute
  (`--since-time=2024-08-30T06:00:00Z`).

### 4. Output (human)

Default `get` table (real):

```
NAME                                READY   STATUS    RESTARTS   AGE
nginx-deployment-67d4bdd6f5-cx2nz   1/1     Running   0          13s
nginx-deployment-67d4bdd6f5-w6kd7   1/1     Running   0          13s
```

Column rules (SIG-CLI + source):
- Headers are **UPPERCASE, no spaces** ("Column titles and values should not contain spaces");
  multi-word headers are hyphenated: `UP-TO-DATE`, `CHANGE-CAUSE`, `LAST SEEN` (events, one exception).
- "The first column should be the resource name, titled `NAME`"; "The last default column should be time
  since creation, titled `AGE`"; `NAMESPACE` first when `-A`.
- Column separators: three spaces (tabwriter, padding 3). No borders, no colour, ever.
- Hidden "priority 1" columns appear only with `-o wide` (pods: `IP NODE NOMINATED NODE READINESS GATES`;
  deployments: `CONTAINERS IMAGES SELECTOR`).
- Deployment: `NAME READY UP-TO-DATE AVAILABLE AGE`; Job: `NAME STATUS COMPLETIONS DURATION AGE`;
  Events: `LAST SEEN TYPE REASON OBJECT MESSAGE`.
- `READY` is a ratio `1/1`; `RESTARTS` is `3 (5m ago)` (source: `fmt.Sprintf("%d (%s ago)", restarts, since)`).
- `STATUS` is a single CamelCase word/reason: `Running`, `Pending`, `Completed`, `Terminating`,
  `CrashLoopBackOff`, `Init:1/2`, `ExitCode:137`, `NotReady`, `Unknown`.
- Placeholder values are angle-bracketed: `<none>`, `<unset>`, `<unknown>`, `<pending>`, `<invalid>`.

**AGE format** (`duration.HumanDuration`, exact):
- `<2m` → `NNs`; `<10m` → `NmNs` (or `Nm` if 0s); `<3h` → `Nm`; `<8h` → `NhNm` (or `Nh`);
  `<48h` → `Nh`; `<8d` → `NdNh` (or `Nd`); `<2y` → `Nd`; `<8y` → `NyNd`; else `Ny`.
  Negative up to -1s → `0s`; beyond → `<invalid>`. Never "ago", never two units past 8d.
- `describe` uses absolute local timestamps instead: `Start Time:   Thu, 17 Feb 2022 16:51:01 -0500`.

**Single `get` vs list**: identical table, one row. There is no special single-object view; that's what
`describe` is for.

**`describe` layout** (real, trimmed):

```
Name:         nginx-deployment-67d4bdd6f5-w6kd7
Namespace:    default
Node:         kube-worker-1/192.168.0.113
Start Time:   Thu, 17 Feb 2022 16:51:01 -0500
Labels:       app=nginx
              pod-template-hash=67d4bdd6f5
Annotations:  <none>
Status:       Running
Controlled By:  ReplicaSet/nginx-deployment-67d4bdd6f5
Containers:
  nginx:
    Image:          nginx
    State:          Running
      Started:      Thu, 17 Feb 2022 16:51:05 -0500
    Ready:          True
    Restart Count:  0
Conditions:
  Type              Status
  Initialized       True
  Ready             True
Events:
  Type    Reason     Age   From               Message
  ----    ------     ----  ----               -------
  Normal  Scheduled  34s   default-scheduler  Successfully assigned default/... to kube-worker-1
  Normal  Pulled     30s   kubelet            Successfully pulled image "nginx" in 1.146417389s
```

Rules: `Key:` Title-Case with colon, values column-aligned per block, 2-space nesting, multi-value
fields continue on indented lines, embedded tables use Title-Case headers with a dashes underline,
`Events:` is always the last section (and `--show-events` defaults true for single object, false for
many). `describe TYPE NAME_PREFIX` does prefix matching if no exact match.

**Mutation output**: `TYPE/name verbed` — `configmap/x created`, `deployment.apps/web configured`,
`deployment.apps/web unchanged`, `pod/foo scaled`, `deployment.apps/abc rolled back`,
`deployment.apps/abc skipped rollback (current template already matches revision 3)`.
`delete` is the odd one: `pod "foo" deleted from default namespace` (`force deleted` when grace=0;
`(dry run)` / `(server dry run)` suffix). `-o name` on any mutation prints just `pod/foo`.

**stderr**: "Only errors should be directed to stderr". Empty list is the notable exception — source:

```go
fmt.Fprintf(o.ErrOut, "No resources found in %s namespace.\n", o.Namespace)   // namespaced
fmt.Fprintln(o.ErrOut, "No resources found")                                   // cluster-scoped
```

Exit code 0, stdout empty. So `kubectl get pods | wc -l` is 0 and scripts stay clean. Server warnings
(deprecations) go to stderr prefixed `Warning:`; `--warnings-as-errors` turns them into exit 1.

**Colour**: none. kubectl never emits ANSI (users install `kubecolor`). **Pager**: none.

### 5. Output (machine)

`-o` accepts: `json, yaml, kyaml, name, go-template, go-template-file, template, templatefile, jsonpath,
jsonpath-as-json, jsonpath-file, custom-columns, custom-columns-file, wide`.

- `-o json/yaml` is the **full API object**, unmodified. A multi-object result is wrapped:
  `{"kind":"List","apiVersion":"v1","metadata":{},"items":[...]}` (source `get.go` builds it). A single
  object is the bare object. `--show-managed-fields=false` strips noise by default.
- `-o name` → `pod/foo` per line (pipeable into another kubectl).
- `-o jsonpath='{.items[*].metadata.name}'`, `-o jsonpath-as-json`, `-o go-template='{{.status.phase}}'`,
  `--template=` companion flag, `--allow-missing-template-keys=true`.
- `-o custom-columns=NAME:.metadata.name,IMAGE:.spec.containers[0].image` — user-defined table with
  user-chosen UPPER headers; `--no-headers` strips the header row.
- `-o wide` is human-only (adds priority-1 columns).
- `-L key1,key2` / `--show-labels` add label columns to the default table.
- Table output is itself server-rendered (`--server-print=true`); the server returns a `Table` object so
  new CRDs get columns without a client upgrade.
- Scripts guidance (kubernetes.io conventions page): "Request one of the machine-oriented output forms,
  such as `-o name`, `-o json`, `-o yaml`, `-o go-template`, or `-o jsonpath`... Don't rely on context,
  preferences, or other implicit states."

### 6. TTY awareness

Practically none. Tables, `No resources found`, and mutation messages are identical on a pipe. There is
no colour to disable, no `--no-color`, no `NO_COLOR` handling, no pager. `kubectl exec/attach` are the
only TTY-sensitive commands (`-t` allocates a TTY; warns "Unable to use a TTY" when stdin isn't one).
`kubectl edit` uses `$KUBE_EDITOR`/`$EDITOR`. `delete -i` prompt reads stdin regardless of TTY.

### 7. Streaming and long-running

`kubectl logs` (usage: `kubectl logs [-f] [-p] (POD | TYPE/NAME) [-c CONTAINER] [options]`):

| flag | default | note |
|---|---|---|
| `-f, --follow` | false | stream |
| `--tail=N` | `-1` (all) — but **10 when a selector is used** | |
| `--since=1h` / `--since-time=RFC3339` | all | mutually exclusive |
| `--timestamps` | false | prepend RFC3339 timestamp to each line |
| `-p, --previous` | false | previous instance of the container |
| `--prefix` | false | `[pod/name/container] ` prefix per line; auto-on with `--all-pods` |
| `-l selector` + `--max-log-requests=5` | | fan-out across pods |
| `--limit-bytes` | 0 | |
| `--ignore-errors` | false | keep following past errors |
| `--pod-running-timeout=20s` | | wait for the thing to exist first |

`logs deployment/nginx` resolves the parent to its pods (first container unless `-c`) — polymorphic
`TYPE/NAME` on a log command.

`kubectl get -w/--watch`: prints the table header once, then appends a row each time an object changes;
`--watch-only` skips the initial list; `--output-watch-events` adds an `EVENT` column
(`ADDED|MODIFIED|DELETED`). Works with `-o json` (one object per event, newline-delimited).

`kubectl rollout status TYPE/NAME` **watches by default** (`-w, --watch=true`, `--watch=false` for a
one-shot), `--timeout=0s` (never), `--revision=N` pins. Progress lines (exact):

```
Waiting for deployment "nginx" rollout to finish: 1 out of 3 new replicas have been updated...
Waiting for deployment "nginx" rollout to finish: 1 old replicas are pending termination...
Waiting for deployment "nginx" rollout to finish: 2 of 3 updated replicas are available...
deployment "nginx" successfully rolled out
```

Exit 1 with `error: deployment "nginx" exceeded its progress deadline` or
`error: timed out waiting for the condition` — the watched thing's failure becomes the CLI's exit status.

`kubectl wait --for=condition=Ready pod/busybox1 --timeout=30s`; `--for=delete`, `--for=create`,
`--for=jsonpath='{.status.phase}'=Running`; multiple `--for` AND'ed. Default timeout 30s; `0` = check
once; negative = a week. Prints `pod/busybox1 condition met` on success; exit 1 on timeout with
`error: timed out waiting for the condition on pods/busybox1`.

`kubectl delete --wait=true` (default) blocks until finalizers finish; `--timeout`.

`kubectl rollout history deployment/abc`:

```
REVISION  CHANGE-CAUSE
1         <none>
2         kubectl set image deployment/abc nginx=nginx:1.16.1
```

`--revision=3` shows that revision's pod template. `rollout undo deployment/abc [--to-revision=3]`
→ `deployment.apps/abc rolled back`. `--dry-run=server` supported on undo.

No spinners, no progress bars, no `--web`.

### 8. Status / health / metrics

- `kubectl top pod [NAME | -l label]` → `NAME  CPU(cores)  MEMORY(bytes)`; `top node` →
  `NAME  CPU(cores)  CPU(%)  MEMORY(bytes)  MEMORY(%)`. Units in the header, values like `250m`,
  `128Mi`. `--containers`, `--sum`, `--sort-by=cpu|memory`, `--no-headers`. One-shot; no refresh loop
  (users wrap it in `watch`). Help text sets expectations explicitly: "not intended to be a replacement
  for full-featured monitoring solutions".
- Health is not a separate command; it's the `STATUS`/`READY` columns plus `describe` → `Conditions:`
  table (`Type/Status`) and `Events:`.
- `kubectl events --for pod/web --watch --types=Warning` is the audit trail; `kubectl get events`
  legacy.

### 9. Errors and exit codes

Observed (real):

```
$ kubectl gett
error: unknown command "gett" for "kubectl"

Did you mean this?
	set
	get
$ kubectl get pods --bogus
error: unknown flag: --bogus
See 'kubectl get --help' for usage.
$ kubectl get
error: Required resource not specified.
Use "kubectl explain <resource>" for a detailed description of that resource (e.g. kubectl explain pods).
See 'kubectl get -h' for help and examples
$ kubectl logs
error: expected 'logs [-f] [-p] (POD | TYPE/NAME) [-c CONTAINER]'.
POD or TYPE/NAME is a required argument for the logs command
See 'kubectl logs -h' for help and examples
$ kubectl get pods --server=https://127.0.0.1:1
The connection to the server 127.0.0.1:1 was refused - did you specify the right host or port?
```

Server errors (source `helpers.go`): `Error from server (NotFound): pods "foo" not found`,
`Error from server (Forbidden): ...`, and Unauthorized becomes
`error: You must be logged in to the server (Unauthorized)`.

Rules: lowercase `error: ` prefix for client-side errors, message sentence-case-ish, trailing hint line
`See 'kubectl <cmd> -h' for help and examples`. All exit codes are **1** (`DefaultErrorExitCode = 1`;
bad flag, unknown command, connection refused, not found — all 1). `--help` exits 0. Only `kubectl diff`
differs: "Exit status: 0 No differences were found. 1 Differences were found. >1 Kubectl or diff failed
with an error." SIG-CLI also reserves `3` for "success but no changes" behind `--error-unchanged`.

`--ignore-not-found` turns a NotFound `get`/`delete` into silent exit 0 (auto-true for `delete --all`).
Ambiguity: `describe` prefix matching returns all matches instead of erroring.

Verbosity: `-v/--v=N` klog levels: 6 = show requested resources, 7 = HTTP request headers,
8 = request contents, 9 = full contents untruncated. Unhandled client errors leak as klog lines
(`E0913 21:41:41.089091   79140 memcache.go:265] "Unhandled Error" err=...`) before the friendly line —
a well-known wart.

### 10. Pagination and filtering

- **No user-visible pagination**. `--chunk-size=500` (default) makes the client fetch in server pages
  using `continue` tokens transparently and print one combined table. `0` disables.
- `-l/--selector 'key=val,key2!=v,key3 in (a,b),!key4'` — server-side label selector; same flag and
  syntax on `get/describe/delete/logs/top/wait/rollout`.
- `--field-selector metadata.name=foo,status.phase!=Running` — server-side, "limited number of field
  queries per type".
- `--sort-by='{.metadata.creationTimestamp}'` — client-side JSONPath; string or int fields only.
- `--all` (delete/wait/label: every object of the type in scope), `-A`.
- No `--limit`, no `--status` flag; status filtering is `--field-selector status.phase=Running`.

### 11. Help text style

Root help groups commands under bold-ish headings with a one-line Short each:

```
Basic Commands (Beginner):
  create          Create a resource from a file or from stdin
  ...
Deploy Commands:
  rollout         Manage the rollout of a resource
Troubleshooting and Debugging Commands:
  describe        Show details of a specific resource or group of resources
  logs            Print the logs for a container in a pod
...
Use "kubectl <command> --help" for more information about a given command.
Use "kubectl options" for a list of global command-line options (applies to all commands).
```

- Short: imperative verb, capitalized, **no trailing period** ("Display one or many resources",
  "Print the logs for a container in a pod").
- Long: full sentences with periods.
- Examples: each preceded by a `# comment` line, real command, blank line between; long lists (get has
  16). Placeholders in usage are UPPER (`TYPE`, `NAME`, `FILENAME`, `POD`, `CONTAINER`).
- Global flags are hidden from per-command help and live under `kubectl options` — keeps per-command
  help short.
- Flags block prints as `    -f, --follow=false:` then a tab-indented description; shows defaults inline.

### 12. Config and auth

- `$HOME/.kube/config`, overridable by `KUBECONFIG` (path list, merged) then `--kubeconfig` (single file,
  no merge). Contexts = cluster + user + namespace; `kubectl config use-context`, `get-contexts`
  (`CURRENT NAME CLUSTER AUTHINFO NAMESPACE`, `*` marks current), `--context`, `--cluster`, `--user`,
  `-n` override individual pieces.
- Precedence: flag > env (`KUBECONFIG`, `POD_NAMESPACE` in-cluster) > current-context > built-in
  default (`default` namespace, `localhost:8080`).
- **kuberc** (new, `~/.kube/kuberc`, `kubectl.config.k8s.io/v1beta1 kind: Preference`): separates
  *preferences* from *credentials*. `aliases:` (name → command + default option values + prepend/append
  args) and `defaults:` (per-command default flag values, e.g. make `delete` `--interactive=true`).
  "Explicit CLI flags override kuberc defaults." Disable with `KUBECTL_KUBERC=false`.
- Auth is entirely kubeconfig-driven (`--token`, `--username/--password`, client certs, exec plugins).

### 13. Notable / mistakes

Steal: `TYPE/NAME` addressing everywhere; `-o name` as a pipe-friendly format; server-rendered tables;
`<none>`-style placeholders; `No resources found` on stderr; `rollout status` watching by default and
turning the target's failure into an exit code; the `label foo-` clear syntax; `--dry-run=client|server`
enum; kuberc preferences-vs-credentials split; `Did you mean this?` suggestions.

Mistakes (widely acknowledged): `--force` meaning "skip graceful deletion" rather than "yes";
`kubectl config set-context` hyphenated verb soup; klog noise leaking before friendly errors;
`--sort-by` being client-side JSONPath instead of a column name; exit code 1 for everything (scripts
can't distinguish not-found from auth failure without parsing stderr); AGE hiding two-unit precision
inconsistently (`5d3h` but never `9d3h`).

---

## Part 2: argocd

### 1. Grammar

**noun-verb**, two levels: `argocd app get my-app`, `argocd app sync my-app`, `argocd proj create`,
`argocd repo list`, `argocd cluster add`. Root nouns: `account admin app appset cert cluster configure
context gpg login logout proj relogin repo repocreds version`. Aliases exist on some (`context, ctx`).

`app` sub-verbs (real, 24 of them): `actions add-source confirm-deletion create delete delete-resource
diff edit get get-resource history list logs manifests patch patch-resource remove-source resources
rollback set sync terminate-op unset wait`. Hyphenated compound verbs when the target is a child
(`delete-resource`, `patch-resource`, `add-source`, `terminate-op`).

Positional: `APPNAME` (usage lines use `APPNAME`, `APPNAME [ID]`, `[APPNAME... | -l selector]`).
Multi-target verbs (`sync`, `wait`, `delete`) take multiple names OR `-l selector` OR `--project`.

### 2. Hierarchy / scoping

App is the root object; everything below it is addressed via flags on an `app` verb, not via nesting:
- Child resources: `--resource GROUP:KIND:NAME` (repeatable, `!` negation, `*` wildcards,
  `NAMESPACE/NAME` disambiguation): `argocd app sync my-app --resource apps:Deployment:my-service --resource '!*:Service:*'`.
- Logs target: `--group`, `--kind`, `--namespace`, `--name`, `-c container`.
- Multi-source: `--source-position N` (1-based) or `--source-name`.
- App namespace: `-N/--app-namespace`; qualified name form `ns/app` also accepted (`QualifiedName()`).
- Project as filter: `-p/--project` on `list`, `--project` on `sync`, and on `resources` "specifying
  this allows the command to report "not found" instead of "permission denied"".
- Server/context scope: `--server`, `--argocd-context`, `ARGOCD_SERVER`, else current context in
  `~/.config/argocd/config`. Omitted → whatever `current-context` says; if none, the default
  `port-forward` pseudo-server (observed: `Failed to establish connection to port-forward:443`).

### 3. Input

- `app create NAME --repo ... --path ... --dest-server ... --dest-namespace ...` (flags, many), or
  `-f/--file manifest.yaml`. `-l/--label` repeatable, `--annotations` repeatable.
- `-p/--parameter key=value` (stringArray, repeated; `--helm-set k=v`, `--kustomize-image name=img`).
- **Clear a field**: dedicated `app unset` verb mirroring `set`: `argocd app unset my-app -p COMPONENT=PARAM`,
  `--namesuffix` (bool = remove), `--kustomize-image alpine`. `unset` prompts:
  `Are you sure you want to unset the parameters? [y/n]`.
- `app patch --patch '...' --type merge|json`, `app edit` opens `$EDITOR`.
- Prompts: `app delete` prompts **only when stdout is a TTY** and cascade is on
  (`utils.NewPrompt(cascade && isTerminal && !noPrompt)`), skipped with `-y/--yes`:
  `Are you sure you want to delete 'my-app' and all its resources? [y/n] ` and for many:
  `... [y/n/a] where 'a' is to delete all specified apps and their resources without prompting `.
  Cancel prints `The command to delete 'my-app' was cancelled.`
- Other optional prompts are gated by a global `--prompts-enabled` flag / local config
  ("false by default"). `sync` has `--assumeYes` (camelCase — a mistake) and `--preview-changes`
  (shows diff, asks to proceed). Secrets: `login --password`, `--auth-token` or `ARGOCD_AUTH_TOKEN`;
  `login` prompts for username/password interactively if omitted; `--sso` opens a browser.

### 4. Output (human)

`app list` default `-o wide` (yes, the *default* is called `wide`; there is no narrower table):

```
NAME       CLUSTER                         NAMESPACE  PROJECT  STATUS     HEALTH   SYNCPOLICY  CONDITIONS  REPO  PATH  TARGET
guestbook  https://kubernetes.default.svc  default    default  OutOfSync  Missing  Manual      <none>      https://github.com/argoproj/argocd-example-apps.git  guestbook  HEAD
```

Headers UPPER, no spaces (`SYNCPOLICY`), tabwriter padding 2. **No AGE column**. Empty list prints
just the header row (no "No resources found" message).

`app get my-app` (default `wide`) = key/value summary + optional conditions table + resources table:

```
Name:               guestbook
Project:            default
Server:             https://kubernetes.default.svc
Namespace:          default
URL:                https://10.97.164.88/applications/guestbook
Source:
- Repo:             https://github.com/argoproj/argocd-example-apps.git
  Target:           HEAD
  Path:             guestbook
SyncWindow:         Sync Allowed
Sync Policy:        Manual
Sync Status:        OutOfSync from HEAD (1ff8a67)
Health Status:      Missing

CONDITION          MESSAGE                     LAST TRANSITION
SharedResourceWarning  ...                     2024-01-01 12:00:00 +0000 UTC

GROUP  KIND        NAMESPACE  NAME          STATUS     HEALTH   HOOK  MESSAGE
apps   Deployment  default    guestbook-ui  OutOfSync  Missing
       Service     default    guestbook-ui  OutOfSync  Missing
```

Format string is literally `"%-20s%s\n"` (`printOpFmtStr`): Title-Case `Key:` padded to 20 chars.
`Sync Status` composes status + revision: `Synced to HEAD (1ff8a67)` / `OutOfSync from HEAD (1ff8a67)`.
`--show-operation` appends an `Operation:` block; `--show-params` appends parameters;
`--refresh` / `--hard-refresh` force a server-side reconcile before printing.

Status vocab: Sync = `Synced | OutOfSync | Unknown`; Health = `Healthy | Progressing | Degraded |
Suspended | Missing | Unknown` ("The App health will be the worst health of its immediate child
resources, based on the following priority (from most to least healthy): Healthy, Suspended,
Progressing, Missing, Degraded, Unknown"). Operation phase = `Running | Succeeded | Failed | Error |
Terminating`. All CamelCase single tokens. `URL:` is printed in `get` so users can jump to the UI.

`app history my-app`:

```
SOURCE  https://github.com/argoproj/argocd-example-apps.git
ID  DATE                                  REVISION
0   2024-01-01 12:00:00 +0000 UTC         HEAD (1ff8a67)
1   2024-01-02 09:30:00 +0000 UTC         v1.2.0 (5e3c1a2)
```

`-o id` prints bare IDs, one per line (feeds `rollback`). Dates are absolute Go `time.String()`
(ugly but sortable); SHAs truncated to 7.

`app resources my-app` → `GROUP KIND NAMESPACE NAME ORPHANED`; `--output tree` →
`KIND/NAME STATUS HEALTH MESSAGE`; `tree=detailed` adds `AGE HEALTH REASON`.

`app diff my-app` prints per-resource sections through the external `diff -u`
(`KUBECTL_EXTERNAL_DIFF` honoured):

```
===== apps/Deployment default/guestbook-ui ======
--- /tmp/argocd-diff/guestbook-ui-live.yaml
+++ /tmp/argocd-diff/guestbook-ui
@@ -12,7 +12,7 @@
-        image: gcr.io/heptio-images/ks-guestbook-demo:0.1
+        image: gcr.io/heptio-images/ks-guestbook-demo:0.2
```

Mutation messages: `application 'guestbook' created`, `application 'guestbook' deleted`,
`Application 'guestbook' deleted` (wait path — inconsistent casing), `set` prints nothing.

Colour: none. Pager: none.

### 5. Output (machine)

- `-o json|yaml` = the full `Application` API object (spec + status + operationState), single object;
  `app list -o json` = bare JSON **array** (not wrapped). `-o name` on `list` = qualified names one per
  line. `history -o id`. No jsonpath / template / custom-columns / jq — users pipe to `jq`.
- `wait`, `sync`, `rollback` accept `-o json|yaml` and then **suppress** the progress table
  ("We don't want to print these when output type is json or yaml, as the output would become
  unparsable") and print only the final object.
- `app resources` / `tree` have their own `--output` (no `-o` short) — inconsistency.

### 6. TTY awareness

Only the `delete` prompt checks `isatty(stdout)`. Tables/logs identical on pipes. No colour.
`--logformat json|text` (default **json**) governs logrus output — so a fatal error on a pipe or a
terminal is a JSON line (observed):

```
{"level":"fatal","msg":"Failed to establish connection to 127.0.0.1:1: error dial proxy: dial tcp 127.0.0.1:1: connect: connection refused","time":"2026-09-13T21:42:40-04:00"}
```

with `--logformat text`: `time="..." level=fatal msg="Failed to establish connection ..."`. This is
argocd's most-complained-about UX wart.

### 7. Streaming and long-running

`app logs APPNAME`: `-f/--follow`, `--tail N`, `--since-seconds N` (integer seconds, not a duration —
worse than kubectl), `--until-time RFC3339`, `-p/--previous`, `-c container`, `--filter "error"`
`-m/--match-case` (server-side grep), scoping via `--group/--kind/--namespace/--name`. Lines are
printed raw (`fmt.Println(msg.GetContent())`); on `Unavailable` during follow it silently reconnects
with `since-seconds=1`.

`app sync` is **synchronous by default** (blocks until operation completes; `--async` returns
immediately; `--timeout N` seconds, uint). While running it streams a change-log table of resource
state transitions (only rows whose state changed are printed):

```
TIMESTAMP                  GROUP        KIND   NAMESPACE                  NAME    STATUS    HEALTH        HOOK  MESSAGE
2019-08-21T21:45:38-07:00   apps  Deployment     default          guestbook-ui  OutOfSync  Missing
2019-08-21T21:45:39-07:00            Service     default          guestbook-ui    Synced  Healthy
2019-08-21T21:45:40-07:00   apps  Deployment     default          guestbook-ui    Synced  Progressing              deployment.apps/guestbook-ui created

Name:               guestbook
...
Sync Status:        Synced to HEAD (1ff8a67)
Health Status:      Progressing

Operation:          Sync
Sync Revision:      1ff8a67...
Phase:              Succeeded
Start:              2019-08-21 21:45:38 -0700 PDT
Finished:           2019-08-21 21:45:40 -0700 PDT
Duration:           2s
Message:            successfully synced (all tasks run)

GROUP  KIND        NAMESPACE  NAME          STATUS  HEALTH       HOOK  MESSAGE
apps   Deployment  default    guestbook-ui  Synced  Progressing        deployment.apps/guestbook-ui created
       Service     default    guestbook-ui  Synced  Healthy            service/guestbook-ui created
```

If the operation ends non-successful: `log.Fatalf("Operation has completed with phase: %s", phase)`
→ non-zero. `--dry-run` previews, `--prune`, `--force`, `--replace`, `--server-side`, `--strategy
apply|hook`, `--retry-limit/--retry-backoff-*`, `--revision`, `--apply-out-of-sync-only`,
`--preview-changes` (diff + confirm), `--info k=v` (annotate the operation).

`app wait [APPNAME.. | -l selector]`: default waits for sync AND health AND no pending operation;
narrow with `--sync`, `--health`, `--operation`, `--suspended`, `--degraded`, `--delete`, `--hydrated`;
`--resource GROUP:KIND:NAME` to wait on a subset; `--timeout N` seconds (0 = forever). Same streaming
table as sync, then the final summary. On timeout prints the current state with
`This is the state of the app after wait timed out:` / `The command timed out waiting for the
conditions to be met.` and errors `timed out (Ns) waiting for app "x" match desired state`. If health
transitions to Degraded while `--health` waiting → immediate error
`application 'x' health state has transitioned from Progressing to Degraded`.

`app rollback APPNAME [ID]` (ID from history; omitted = previous) → runs a sync op to that revision and
waits like `sync` (`--timeout`, `--prune`, `-o`). `app terminate-op APPNAME` cancels the running op.
`app delete --wait` blocks until gone.

No spinners; no `--web` (but `URL:` is in `get` output). `--refresh`/`--hard-refresh` on `get`/`diff`.

### 8. Status / health / metrics

No metrics command. State is the `STATUS` (sync) + `HEALTH` columns on every table plus the
`Sync Status:` / `Health Status:` lines and the per-resource table. `app get --show-operation` shows
the last operation's `Phase/Start/Finished/Duration/Message`. `CONDITIONS` column summarises
warnings/errors (`<none>` or e.g. `ComparisonError(1)`).

### 9. Errors and exit codes

Observed (real):

```
$ argocd app list --bogus
Error: unknown flag: --bogus              # cobra default, capital E, then full usage dump; exit 1
$ argocd app get                          # missing arg: prints the command's help, exit 1
$ argocd app foo                          # unknown subcommand: prints parent help, exit 1
$ argocd app list --server 127.0.0.1:1 --plaintext
{"level":"fatal","msg":"Failed to establish connection to 127.0.0.1:1: ...","time":"..."}   # exit 1
```

Two exit paths coexist: `log.Fatal*` (logrus → exit **1**) and `errors.CheckError` → `Fatal(ErrorGeneric=20)`
(exit **20**, the documented "generic exit code (20) returned by all CLI commands"). So API errors
(`rpc error: code = NotFound desc = applications.argoproj.io "foo" not found`) exit 20 while client-side
validation and connection failures exit 1. `app diff`: "Returns the following exit codes: 2 on general
errors, 1 when a diff is found, and 0 when no diff is found"; `--diff-exit-code N` and `--exit-code=false`
to tune. Errors are logrus-formatted (JSON by default) — no `error:` prefix, no hints, no
did-you-mean. `--loglevel debug|info|warn|error`.

### 10. Pagination and filtering

None server-side. `app list` filters: `-l selector` (full k8s selector syntax including `exists` /
`!key` / `notin`), `-p/--project` (repeatable), `-r/--repo URL`, `-c/--cluster`, `-P/--path`,
`-N/--app-namespace`. No `--limit`, no `--sort-by`, no `--status`/`--health` filter (users `| grep`).

### 11. Help text style

Standard cobra: `Usage:` / `Examples:` / `Available Commands:` / `Flags:` / `Global Flags:` — and the
global flags block (~30 lines of `--redis-name`, `--grpc-web-root-path`, …) is repeated in **every**
command's help, burying the command's own flags. Shorts are imperative, capitalized, mostly no period
but inconsistent ("Perform a diff against the target and live state." has one; `rollback`'s Short is
a 90-char sentence). Examples use `# comment` + command, usually with `my-app`. Flag descriptions are
inconsistent in style ("Specify if the logs should be streamed" vs "Wait for health"). Usage
placeholder `APPNAME`.

### 12. Config and auth

`~/.config/argocd/config` (or `ARGOCD_CONFIG_DIR`, `--config`), YAML shape observed:

```yaml
contexts:
- name: autopilot
  server: port-forward
  user: autopilot
current-context: autopilot
servers:
- grpc-web-root-path: ""
  server: port-forward
users:
- auth-token: <redacted>
  name: autopilot
```

Tokens (JWT) live in the same file as contexts (unlike kuberc's split). `argocd login SERVER
[--username --password | --sso | --core] [--name ctx]`, `argocd relogin`, `argocd logout`,
`argocd context [NAME] [--delete]` (lists as `CURRENT NAME SERVER` with `*`). Env:
`ARGOCD_SERVER` ("instead of specifying --server for every command"), `ARGOCD_AUTH_TOKEN`
("set this or the --auth-token flag"), `ARGOCD_OPTS` ("command-line options to pass to argocd CLI eg.
ARGOCD_OPTS="--grpc-web""). Precedence: flag > env > config current-context.

### 13. Notable / mistakes

Steal: single-token `STATUS` + `HEALTH` columns on every table; worst-of health aggregation with an
explicit priority order; `Sync Status: Synced to HEAD (1ff8a67)` composing state + target + resolved
revision; `URL:` in `get`; `sync` blocking by default with `--async` opt-out; the timestamped
transition table during waits (prints only changed rows, then a final full snapshot); `wait` with
composable `--sync/--health/--operation` predicates; `history -o id` feeding `rollback ID`; `set`/`unset`
pairing for clearing fields; `--refresh` vs `--hard-refresh`; `diff` exit 1 on differences.

Mistakes: JSON-formatted fatal errors on a terminal; exit 1 vs 20 inconsistency; global flags
dumped in every help; `--assumeYes` camelCase; `--since-seconds` int instead of duration; default
output named `wide` with no narrow table; `list` has no AGE; absolute `time.String()` dates in history;
`--output` without `-o` on `resources`; empty list prints a lonely header row.

---

## Lessons for Admiral

1. **Keep `NAME` first and `AGE` last on every list table; UPPERCASE, no-space headers, three-space
   tabwriter padding, no borders, no colour in tables** (kubectl SIG-CLI rules; argocd follows too).
   Add a leading scope column (e.g. `APP` / `ENV`) only when a list spans parents, exactly like
   kubectl's `NAMESPACE` under `-A`.

2. **Adopt kubectl's `HumanDuration` for AGE verbatim** (`13s`, `5m12s`, `2h`, `5d3h`, `41d`, `2y`),
   and use absolute RFC3339/local timestamps only in `describe` and `history`. Don't do argocd's
   `2024-01-01 12:00:00 +0000 UTC` in tables.

3. **Status is one CamelCase token per column** — `Running | Succeeded | Failed | Cancelled` for runs,
   `Healthy | Progressing | Degraded | Unknown` for health — and app/env health is **worst-of children**
   with a documented priority order (argocd). Put `STATUS` and, where it exists, `HEALTH` as adjacent
   columns on every table (argocd `app list`).

4. **Empty list → `No <resources> found[ in <scope>].` on stderr, exit 0, stdout empty** (kubectl
   `get.go`). Never print a bare header row like argocd. Already Admiral's rule; keep the kubectl
   wording incl. scope: `No runs found for env prod.`

5. **Mutations print `type/name verbed` on stdout**: `app/shop created`, `env/prod updated`,
   `run/1234 cancelled`, `changeset/abc discarded` (kubectl). Support `-o name` on mutations so
   `admiral run apply ... -o name | xargs admiral run logs -f` works.

6. **`describe` = kubectl layout**: `Key:` Title-Case padded columns, 2-space nesting, `<none>` for
   empties, embedded tables with Title-Case headers and a dashes underline, and a chronological
   `Events:`/`History:` table last. Add argocd's touches: a `URL:` line pointing at the web UI, and
   composed status lines like `Sync Status: Synced to HEAD (1ff8a67)` → e.g.
   `Status:  Succeeded (revision 42, applied 3m ago)`.

7. **`-o json|yaml` returns the full API object; lists are wrapped** (`{"items":[...], "nextPageToken": ...}`)
   rather than argocd's bare array — kubectl wraps in a `List` so metadata (continue token) has a home.
   Single `get` returns the bare object. Add `-o jsonpath=` or `-o go-template=` (kubectl) before
   anything fancier; skip custom-columns unless the server renders tables.

8. **Placeholders are `<none>` / `<unknown>` / `<unset>`**, never blank cells or `-`, so columns stay
   parseable with `awk` (kubectl printers).

9. **`logs`: `-f/--follow`, `--tail N` (default all, 10 when fanned out), `--since 1h` and
   `--since-time RFC3339` (mutually exclusive), `--timestamps`, `-p/--previous`, `--prefix` when
   multiplexing phases/components** (kubectl). Use Go durations, not argocd's `--since-seconds`.
   Accept the parent noun on logs (`admiral run logs 1234` and `admiral env logs prod --app shop` →
   latest run), mirroring `kubectl logs deployment/nginx`.

10. **Long-running verbs block by default and expose `--async`** (argocd `sync`; kubectl `rollout
    status --watch=true`). While blocking, stream a timestamped **transition table** that prints only
    rows whose state changed (`TIMESTAMP COMPONENT PHASE STATUS MESSAGE`), then a final full snapshot.
    Suppress the stream when `-o json|yaml` is set and emit only the final object (argocd `wait`).

11. **The watched thing's outcome is the exit code**: `run apply --wait` / `run wait` exit 1 when the
    run fails or times out, with kubectl-style text `error: run 1234 failed: <server message>` and
    argocd-style timeout text `timed out (300s) waiting for run 1234`. Provide `admiral run wait ID
    [--for=succeeded|finished] [--timeout 5m]` modelled on `kubectl wait --for=... --timeout` /
    `argocd app wait --health --sync --operation`.

12. **`history` and `rollback` pair via an ID column**: `admiral run history --env prod --app shop`
    prints `REVISION  STATUS  DEPLOYED  CHANGE-CAUSE` (kubectl `REVISION CHANGE-CAUSE` + argocd
    `ID DATE REVISION`), `-o id` prints bare IDs, and `run rollback [--to-revision N]` defaults to the
    previous one (`kubectl rollout undo` / `argocd app rollback [ID]`). Print
    `run/1235 rolled back to revision 41` or `skipped rollback (current already matches revision 41)`.

13. **Errors: lowercase `error: ` prefix, one line, then a hint line** — `See 'admiral env get --help'
    for usage.` and `Did you mean this?` for typos (kubectl). Never argocd's JSON-log fatal lines, and
    never cobra's full-usage dump on a missing argument. Server errors: `error from server (NotFound):
    env "prod" not found in app "shop"`.

14. **Exit codes: 0 ok, 1 error, and reserve 2 for usage errors** (Go/cobra norm); optionally `3` for
    "no changes" (SIG-CLI). Don't do argocd's 1-vs-20 split. Make `changeset diff`/`run plan` exit `1`
    when there are differences and `>1` on failure (kubectl diff / argocd diff), with
    `--exit-code=false` to opt out.

15. **`--force/-f` should mean "skip confirmation" (argocd `-y/--yes`), and never overload it with a
    semantic like kubectl's "bypass graceful deletion"**. If a hard/immediate variant is needed later,
    name it explicitly (`--now`, `--cascade`). Prompt only when stdin is a TTY (argocd checks isatty),
    and print `cancelled` on decline.

16. **Clearing a field**: adopt kubectl's trailing-minus on key=value flags (`--label team-`,
    `--var FOO-`) for map-like fields and argocd's `unset` verb only if a field is a scalar with no
    natural key. Don't invent `--clear-x` flags per field.

17. **Filtering: `-l/--selector` with the k8s selector grammar (`=`, `!=`, `in (a,b)`, `!key`),
    `--field-selector status=Failed` for server-side field filters, `--sort-by COLUMN`** (name a
    column, not a JSONPath — kubectl's JSONPath sort is a known pain). Keep `--page-size/--page-token`
    but consider kubectl's `--chunk-size` model: auto-follow pages by default and print
    `NEXT PAGE TOKEN` only when `--page-size` is given explicitly.

18. **Help style: Short = capitalized imperative, no period; Long = sentences with periods; Examples =
    `# comment` + command pairs; hide global flags behind `admiral options`** (kubectl) instead of
    repeating them in every subcommand (argocd's biggest help wart). Group root commands under headings
    (`Resource Commands`, `Run Commands`, `Settings Commands`).

19. **Config: split preferences from credentials** (kubectl kubeconfig vs kuberc; argocd keeps the
    token in the context file — Admiral already has `credentials.json` separate). Support contexts
    with a `CURRENT NAME SERVER` list marked `*`, `--context` override, `ADMIRAL_SERVER` /
    `ADMIRAL_TOKEN` / `ADMIRAL_OPTS`-style env passthrough (argocd), precedence flag > env > context.

20. **Offer `--refresh` on `get`/`describe`/`diff` for "force the server to re-evaluate before
    answering"** (argocd `--refresh` / `--hard-refresh`) — cheap to add, avoids stale-status
    confusion in a control plane — and add `--web`/`URL:` so users can jump to the UI from any
    single-resource view.
