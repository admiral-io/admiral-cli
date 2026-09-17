# Admiral CLI — Command Tree & Consistency Audit

## Root

```
admiral [persistent flags]
```

Persistent flags (apply to every subcommand):

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--config-dir` | | string | `~/.config/admiral` | Path to config directory |
| `--server` | `-s` | string | | host:port of the API server |
| `--plaintext` | | bool | false | Disable TLS |
| `--insecure` | `-i` | bool | false | Skip server certificate and domain verification |
| `--output` | `-o` | string | `table` | Output format: table, json, yaml, wide |
| `--verbose` | `-v` | bool | false | Enable verbose mode |
| `--help` | `-h` | bool | | Help for admiral |

---

## app  *(alias: application)*

| Command | Positional | Flags |
|---------|-----------|-------|
| `app list` | — | `--page-size`, `--page-token`, `--label` |
| `app create` | `<name>` | `--label`, `--description` |
| `app get` | `[app]` | `--id` |
| `app update` | `[app]` | `--id`, `--name`, `--label`, `--description` |
| `app delete` | `[app]` | `--id`, `--force`/`-f` *(prompts to type the app name; cascades to envs, components, runs)* |

---

## credential  *(aliases: cred, credentials)*

| Command | Positional | Flags |
|---------|-----------|-------|
| `credential create basic-auth` | `<name>` | `--label`, `--description`, `--username`, `--password`, `--password-stdin` |
| `credential create bearer-token` | `<name>` | `--label`, `--description`, `--token`, `--token-stdin` |
| `credential create ssh-key` | `<name>` | `--label`, `--description`, `--private-key`, `--private-key-file`, `--passphrase` |
| `credential list` | — | `--page-size`, `--page-token`, `--label` |
| `credential get` | `[cred]` | `--id` |
| `credential update` | `[cred]` | `--id`, `--name`, `--description`, `--label`, `--username`, `--password`, `--password-stdin`, `--token`, `--token-stdin`, `--private-key`, `--private-key-file`, `--passphrase` |
| `credential delete` | `[cred]` | `--id`, `--force`/`-f` |

---

## run  *(alias: runs)*

| Command | Positional | Flags |
|---------|-----------|-------|
| `run plan` | — | `--app`, `--app-id`, `--env`, `--env-id`, `--message`/`-m` *(drift correction path; for change-set-driven plans use `changeset plan`)* |
| `run apply` | `[id]` | `--id` |
| `run rollback` | `<id>` | `--app`, `--app-id`, `--env`, `--env-id`, `--message`/`-m`, `--force`/`-f` |
| `run cancel` | `[id]` | `--id`, `--force`/`-f` |
| `run list` | — | `--app` *(required)*, `--app-id`, `--env`, `--env-id`, `--page-size`, `--page-token` |
| `run get` | `<id>` | — |

---

## env  *(aliases: environment, environments)*

| Command | Positional | Flags |
|---------|-----------|-------|
| `env create` | `<name>` | `--app`, `--app-id`, `--description`, `--runner`, `--runner-id`, `--label` |
| `env list` | — | `--app`, `--app-id`, `--page-size`, `--page-token` |
| `env get` | `[name]` | `--app`, `--app-id`, `--id` |
| `env describe` | `[name]` | `--app`, `--app-id`, `--id` *(alias: `desc`; kubectl-style summary including variables; table-only)* |
| `env update` | `[env]` | `--app`, `--app-id`, `--id`, `--name`, `--description`, `--runner`, `--runner-id`, `--label` |
| `env delete` | `[name]` | `--app`, `--app-id`, `--id`, `--force`/`-f` |

---

## changeset  *(aliases: cs, changesets)*

The unit of work for proposing component and variable changes against an
(application, environment). Replaced the former `infra` tree in V2.

| Command | Positional | Flags |
|---------|-----------|-------|
| `changeset create` | — | `--app`, `--app-id`, `--env`, `--env-id`, `--title`, `--description` |
| `changeset get` | `<id>` | — |
| `changeset list` | — | `--app`, `--app-id`, `--env`, `--env-id`, `--status`, `--page-size`, `--page-token` |
| `changeset discard` | `<id>` | `--force`/`-f` |
| `changeset copy` | `<id>` | `--env`, `--env-id`, `--title`, `--description` |
| `changeset add` | `<slug>` | `--changeset` *(req)*, `--module`, `--module-id`, `--version`, `--values`, `--depends-on`, `--description` |
| `changeset update` | `<slug>` | `--changeset` *(req)*, `--module`, `--module-id`, `--version`, `--values`, `--depends-on`, `--description` |
| `changeset destroy` | `<slug>` | `--changeset` *(req)* |
| `changeset orphan` | `<slug>` | `--changeset` *(req)* |
| `changeset remove-entry` | `<slug>` | `--changeset` *(req)*, `--force`/`-f` |
| `changeset set-var` | `<KEY> <VALUE>` | `--changeset` *(req)*, `--type`, `--sensitive` |
| `changeset remove-var` | `<KEY>` | `--changeset` *(req)* |

---

## module  *(aliases: mod, modules)*

Every `module create <type>` subcommand shares the same flag set:
`--description`, `--source`, `--source-id`, `--ref`, `--root`, `--path`, `--label`.

| Command | Positional | Flags |
|---------|-----------|-------|
| `module create terraform` | `<name>` | *(shared create flags)* |
| `module create helm` | `<name>` | *(shared create flags)* |
| `module create kustomize` | `<name>` | *(shared create flags)* |
| `module create manifest` | `<name>` | *(shared create flags)* |
| `module list` | — | `--page-size`, `--page-token`, `--label` |
| `module get` | `[mod]` | `--id` |
| `module update` | `[mod]` | `--id`, `--name`, `--description`, `--source`, `--source-id`, `--ref`, `--root`, `--path`, `--label` |
| `module delete` | `[mod]` | `--id`, `--force`/`-f` |
| `module resolve` | `[mod]` | `--id`, `--ref` |

---

## runner  *(alias: runners)*

| Command | Positional | Flags |
|---------|-----------|-------|
| `runner create` | `<name>` | `--description`, `--label` |
| `runner list` | — | `--page-size`, `--page-token`, `--label` |
| `runner get` | `[runner]` | `--id` |
| `runner update` | `[runner]` | `--id`, `--name`, `--description`, `--label` |
| `runner delete` | `[runner]` | `--id`, `--force`/`-f` |
| `runner status` | `[runner]` | `--id` |
| `runner jobs` | `[runner]` | `--id`, `--status`, `--type`, `--run`, `--page-size`, `--page-token` |
| `runner token create` | `<name>` | `--runner`, `--runner-id`, `--expires-in` |
| `runner token list` | `[runner]` | `--id`, `--page-size`, `--page-token` |
| `runner token get` | `[runner]` | `--id`, `--token`, `--token-id` |
| `runner token revoke` | `[runner]` | `--id`, `--token`, `--token-id`, `--force`/`-f` |

---

## source  *(aliases: src, sources)*

Shared `source create <type>` flags: `--description`, `--url`, `--credential`, `--credential-id`, `--catalog`, `--label`.

| Command | Positional | Type-specific flags |
|---------|-----------|---------------------|
| `source create git` | `<name>` | — |
| `source create terraform` | `<name>` | `--tf-namespace`, `--tf-module-name`, `--tf-system` |
| `source create helm` | `<name>` | `--chart-name` |
| `source create oci` | `<name>` | — |
| `source create http` | `<name>` | — |

| Command | Positional | Flags |
|---------|-----------|-------|
| `source list` | — | `--page-size`, `--page-token`, `--label` |
| `source get` | `[src]` | `--id` |
| `source update` | `[src]` | `--id`, `--name`, `--description`, `--url`, `--credential`, `--credential-id`, `--clear-credential`, `--catalog`, `--label` |
| `source delete` | `[src]` | `--id`, `--force`/`-f` |
| `source test` | `[src]` | `--id` |
| `source versions` | `[src]` | `--id`, `--page-size`, `--page-token` |

---

## state

| Command | Positional | Flags |
|---------|-----------|-------|
| `state pull` | — | *(temporarily disabled in V2 -- pending slug-based component lookup)* |
| `state push` | — | *(temporarily disabled in V2 -- pending slug-based component lookup)* |

---

## config

| Command | Positional | Flags |
|---------|-----------|-------|
| `config set` | `<key> [value]` | — |
| `config get` | `<key>` | — |
| `config list` | — | — |
| `config unset` | `<key>` | — |

---

## Utility

| Command | Positional | Flags |
|---------|-----------|-------|
| `completion` | `[bash\|zsh\|fish\|powershell]` | — |
| `version` | — | — |
| `whoami` | — | — |

---

# Consistency Findings

Issues to review for cross-command consistency.

## 1. ~~Destructive-action confirmation is uneven~~ *(resolved 2026-04-18)*

Standardized on **prompt-by-default + `--force`/`-f` to skip**. Matches gcloud/gh/terraform style; replaces the old, backwards `--confirm` flag.

**Pattern:**
- y/n prompt by default on: `credential delete`, `env delete`, `infra delete`, `infra override delete`, `module delete`, `runner delete`, `source delete`, `run cancel`, `run rollback`, `runner token revoke`.
- **Type-the-name** prompt on `app delete` (cascades to envs, components, runs).
- Non-TTY stdin + no `--force` → fails with "refusing to proceed without confirmation; pass --force to skip the prompt". Does not hang.

**Shared helpers:** `util.Confirm` and `util.ConfirmName` in `internal/util/confirm.go`.

## 2. ~~Alias `rm` only on `runner delete`~~ *(resolved 2026-04-18)*
- Dropped the `rm` alias from `runner delete`. `delete` is now the single canonical verb everywhere, matching gcloud/kubectl/gh conventions.

## 3. ~~Short-flag collisions in `state`~~ *(resolved 2026-04-18)*
- `state pull --output`/`-o` and `state push --input`/`-i` both collided with root persistent flags.
- Renamed both to `--file` with **no short form**. `-f` is reserved for `--force` (see finding #1), which `state push` is a likely future candidate for since it replaces state.

## 4. Duplicate flag names
- `infra override set` exposes both `--disabled` and `--disable`.
- **Fix:** pick one and drop the other (or make one a hidden alias).

## 5. Label update semantics differ
- Most `update` commands describe `--label` as "set" (additive/merge).
- `env update` and `runner update` describe it as "replace all labels; empty list clears".
- **Decide on one semantic** and align wording and behavior across all resources.

## 6. Positional argument pattern is inconsistent
- Some `get`/`update`/`delete` accept a positional name **and** `--id` (e.g. `env get [name]`).
- Others only take `[resource]` with `--id` (e.g. `app get [app]`).
- `run get` takes a required `<id>` positional with **no** `--id` flag.
- **Decide:** adopt a single rule — e.g. positional is always a name, `--id` is always the UUID alternative.

## 7. App scoping is optional in some lists, required in others
- `run list --app` is required.
- `env list --app` is optional.
- `infra list --app` is required.
- **Decide:** should child-resource lists always require parent scoping?

## 8. `config` uses bare positional args while the rest of the CLI prefers flags
- `config set <key> [value]`, `config get <key>`, `config unset <key>`.
- Fine for a settings tool, but worth noting: no `--help`-example parity, no flag-driven form.

## 9. ~~`runner create` kind is now fixed to infrastructure~~ *(resolved 2026-04-19)*
- `kind` was dropped from the proto (`Runner` message and `CreateRunnerRequest`); the hidden `--kind` flag, the `parseRunnerKind`/`formatRunnerKind` helpers, and the KIND column were all removed.
- **Revisit** if/when workflow runners ship: reintroduce as typed subcommands (`runner create infrastructure`, `runner create workflow`) to match `module`/`source`/`credential` rather than re-adding the flag.

## 10. ~~Pagination flags are missing on several list commands~~ *(resolved 2026-04-19)*

All list commands now expose `--page-size` and `--page-token` and print `NEXT PAGE TOKEN` to stderr when the response has more pages (skipped for `json`/`yaml`, which already include `next_page_token` in the payload).

Covered: `app list`, `credential list`, `run list`, `env list`, `infra list`, `infra override list`, `module list`, `runner list`, `runner token list`, `source list`, `source versions`.

## 11. Type-prefixed flags in `source create` are inconsistent
- `terraform` uses `--tf-namespace`, `--tf-module-name`, `--tf-system` (prefixed).
- `helm` uses `--chart-name` (unprefixed).
- **Decide:** either prefix all type-specific flags (`--helm-chart-name`) or drop prefixes (`--namespace`, `--module-name`, `--system`).

## 12. `--message`/`-m` only on run subset
- `run plan` and `run rollback` have `--message`/`-m`.
- `run apply` and `run cancel` do not.
- **Decide:** do `apply` and `cancel` need a message? If yes, add it; if no, document the asymmetry.

## 13. `credential update` carries flags for every credential type
- `--username`, `--password`, `--token`, `--private-key`, etc. are all on one command.
- Consider mirroring the `create` pattern: `credential update basic-auth`, `credential update bearer-token`, `credential update ssh-key`.

## 14. `source update --clear-credential` has no analogue elsewhere
- Other resources don't expose "clear X" flags.
- **Decide on a convention** for nulling optional references (e.g. `--clear-<field>` everywhere, or passing an empty value).

## 15. Short flags are rare and irregular
- Current short flags: `-m` (run message), `-f` (`--force` on destructive commands), plus root's `-s/-i/-o/-v/-h`.
- State's `-o`/`-i` were dropped in finding #3.
- **Decide:** either commit to short flags for common operations or drop the few remaining for a cleaner surface.

## 16. `runner token get` / `revoke` runner scope is now optional *(resolved 2026-04-19)*

Today:
```
admiral runner token get    [runner] --token <name>       # runner scope required for name lookup
admiral runner token get             --token-id <uuid>    # no runner scope needed
admiral runner token revoke [runner] --token <name>
admiral runner token revoke          --token-id <uuid>
```

`--token` (name) needs runner scope to resolve via `ListRunnerTokens`. `--token-id` (UUID) does not — the server resolves the parent runner from the token ID.

**Proto change:** `runner_id` was removed entirely from `GetRunnerTokenRequest` and `RevokeRunnerTokenRequest` (field 1 reserved). REST routes moved from `/api/v1/runners/{runner_id}/tokens/{token_id}` to `/api/v1/runners/tokens/{token_id}`. Runner-ID is UUID-validated, so the new literal `tokens/` segment can't collide with the existing `/api/v1/runners/{runner_id}` routes.

**Still open:** whether to promote token-id to a positional (`admiral runner token get <token-id>`) to match the *child = positional, parent = flag* convention. Holding off until the flag form has had some use.

## 17. Server-name completion is only wired for `app` *(rule: style guide §1.5)*

Every positional and flag that goes through `resolve.X` must offer names via `complete.X` (`internal/complete`). Done: `app get|describe|update|delete <name>` and every `--app` (through `flags.App`). Remaining, by resolver:

| Resolver | Positionals | Flags | Notes |
|---|---|---|---|
| `Environment` | `env get\|describe\|update\|delete`, `changeset copy` | `--env` on `changeset create\|list\|copy`, `run list\|rollback` | bare name scoped by `--app` on the line; path form `app/env` completes segment by segment (style guide §1.5); `flags.Env` registers it like `flags.App` does |
| `Source` | `source get\|describe\|update\|delete\|test\|versions` | `--source` on `catalog create\|update` | |
| `Credential` | `credential get\|describe\|update\|delete` | `--credential` on `source create\|update` | |
| `CatalogItem` | `catalog get\|describe\|update\|delete\|resolve`, `changeset entry` | | |
| `Agent` | `agent get\|describe\|update\|delete\|status\|jobs`, `agent token create\|list` | `--agent` on `agent token get\|revoke` | |
| `AgentToken` | `agent token get\|revoke` | | scope from `--agent` |
| `PersonalAccessToken` | `auth key get\|revoke` | | |
| enums | | `--phase`, `--status`, `--type`, … | static lists, `RegisterFlagCompletionFunc`; `--scope` on `auth login` is the model |
| display IDs | `run get\|describe\|…`, `changeset get\|…` | | `run-…`/`cs-…` IDs are typed, not named; complete from `list` in the current scope if it proves useful |

Tick rows here as they land; new commands do not get a row because §13 of the style guide makes completion part of the definition.
