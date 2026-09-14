# Cross-resource name denormalization

**Status:** Proposal
**Audience:** Admiral server maintainers
**Origin:** CLI consistency review, 2026-04-25

## Problem

Most resources hold UUID foreign keys to other resources: a `Module` has
`source_id`, a `Component` has `module_id`, a `Source` has
`credential_id`, an `Environment` has a runner ID nested under
`infrastructure_targets`, and so on. Today these reads return only the
UUID — the human-readable *name* of the linked resource is not
included.

The CLI surfaces this in narrow list rows and in post-`update` echoes,
where columns like `SOURCE ID`, `MODULE ID`, `RUNNER` end up rendering
unreadable UUIDs (or 8-char prefixes) that mean nothing to the user
scanning a table:

```
admiral module list
NAME                      TYPE        SOURCE ID                              REF         DESCRIPTION              LABELS        AGE
datalift-hub--bootstrap   TERRAFORM   9856b38a-f6b3-4eea-b723-46c33e30e937   (default)   datalift-hub bootstrap   k1=v1,k2=v2   1m
```

The user can't tell at a glance which source backs that module. The CLI
has two bad options:

1. Drop the column entirely from narrow rows — loses real information.
2. Fan out an extra RPC per row to resolve `id → name` — N extra reads
   per `list`, with all the latency and chatter that implies.

This is a server-side concern. Any client (CLI, UI, agent) that wants
to render human-readable cross-references hits the same wall and would
have to solve it the same way independently.

## Goal

Every read response that carries a foreign-key UUID also carries the
linked resource's current `name` as a sibling field, populated by the
server at read time. The CLI (and any other client) can render
readable rows without an extra RPC.

## Scope (resources & fields)

| Resource (proto) | Existing field | New denormalized field |
|---|---|---|
| `module.v1.Module` | `source_id` | `source_name` |
| `component.v1.Component` | `module_id` | `module_name` |
| `component.v1.Component` | `application_id` | `application_name` |
| `component.v1.ComponentOverride` | `module_id` (override) | `module_name` |
| `component.v1.ComponentOverride` | `component_id` | `component_name` |
| `component.v1.ComponentOverride` | `environment_id` | `environment_name` |
| `source.v1.Source` | `credential_id` (optional) | `credential_name` (optional, mirrors nullability) |
| `environment.v1.Environment` | `application_id` | `application_name` |
| `environment.v1.Environment` → `InfrastructureTarget.Terraform.runner_id` | (nested) | `runner_name` (sibling inside `Terraform`) |
| `runner.v1.AccessToken` | `runner_id` | `runner_name` |

Where a foreign key is optional (e.g. `Source.credential_id`), the
denormalized name field is also optional and populated only when the
ID is set.

## Behavior

- All `Get*` and `List*` RPCs populate the new fields.
- Server reads the linked resource's current `name` at the time of the
  response. Stale reads are acceptable in the same way labels or other
  fields are — eventually consistent is fine.
- If the linked resource has been deleted but the foreign key still
  exists (e.g. dangling override pointing at a deleted module), the
  name field is left empty and the ID is preserved as-is. Clients
  render an empty cell rather than a fabricated value.

### What the server does *not* need to do

- No changes to write RPCs (`Create*`, `Update*`). Callers still pass
  IDs in; server still resolves them. The denormalization is read-side
  only.
- No nesting of the full linked resource. Just the name. If a client
  needs more, it can issue a follow-up `Get*`. We are not building
  field expansion.
- No back-population on rename. If a `Source` is renamed, the next read
  of dependent `Module`s naturally returns the new `source_name`. We
  don't store the name; we resolve it at read time.

## Non-goals

- General-purpose field expansion (`?expand=source`-style query
  parameters). Out of scope; revisit if the use cases stack up.
- Breaking changes to existing fields. The new fields are additive
  siblings.
- A registry of every cross-resource pointer in the API. The table
  above is the concrete set the CLI exercises today; new pointers
  added in the future should follow the same convention.

## Edge cases

- **Linked resource deleted (dangling reference).** Return the existing
  ID, leave the name empty. Don't fail the read.
- **Permissions: caller can read the parent but not the linked
  resource.** Return the ID (which the caller can already see in the
  parent), leave the name empty. Do not fail the read with a
  permission error — the caller is reading the parent, not the linked
  resource.
- **Performance.** A `List*` of N items now triggers N name lookups.
  Implement a single bulk lookup per response (one query, not N) so
  list latency stays flat. This is the only meaningful implementation
  cost and worth getting right up front.
- **Caching.** Names rarely change; a short-TTL in-memory cache keyed
  on resource ID is acceptable but not required for correctness.

## Acceptance criteria

- For each resource in the scope table, `Get*` and `List*` responses
  include the new sibling name field.
- A `List*` response containing N items issues O(1) — not O(N) —
  resolution queries for the linked names.
- Deleting a linked resource leaves dangling references readable: ID
  preserved, name empty.
- Renaming a linked resource is reflected in the next read of any
  dependent (no client-visible cache lag beyond the server's own).
- No write RPC behavior changes.

## CLI follow-up (out of scope here)

Once the server returns `source_name`, `module_name`, `runner_name`,
etc., the CLI swaps the unreadable `SOURCE ID` / `MODULE ID` / `RUNNER`
(8-char prefix) columns in narrow rows for the readable names. Wide
rows can continue to expose both `*_id` and `*_name` for scripting.
