# Source / credential test-state hygiene

**Status:** Proposal
**Audience:** Admiral server maintainers
**Origin:** CLI consistency review, 2026-04-25

## Problem

A `Source` carries a cached test outcome — `last_test_status`,
`last_test_error`, `last_tested_at`. Today these fields persist as-is
across mutations that invalidate the result. Two cases:

**Case 1 — source mutated.** A user runs `source test`, gets `SUCCESS`.
They later edit the source's `url` or swap `credential_id`. The cached
`SUCCESS` is now stale: it reflects the old URL/credential pair, not the
current one. Anyone looking at the source — `source list`, `source get`,
the UI, the agent — sees `SUCCESS` and trusts it.

**Case 2 — credential rotated.** A user rotates a credential's auth
material (password, bearer token, SSH key). Every source pointing at
that credential just had its underlying secret change. None of those
sources' `last_test_status` reflects the rotated material; the rows show
`SUCCESS` indefinitely until someone re-tests.

This is a server-side correctness issue. The CLI (and any other client)
only displays what the server returns; if the server retains stale
data, every client surfaces stale data.

### Reproducing today

```bash
# Case 1
admiral source test acme-infra              # → SUCCESS
admiral source update acme-infra --url https://example.com/wrong.git
admiral source get acme-infra                # last_test_status still SUCCESS

# Case 2
admiral source test acme-infra              # → SUCCESS, uses cred "acme-pat"
admiral credential update acme-pat --token NEW_VALUE
admiral source get acme-infra                # last_test_status still SUCCESS
```

## Goal

After any mutation that could invalidate a previous test result, the
server clears `last_test_status`, `last_test_error`, and `last_tested_at`
so the source returns to an "untested" state. A subsequent `source test`
re-establishes ground truth.

## Behavior changes

### `UpdateSource`

When the update mask contains `url` or `credential_id` (in any
combination), clear the three fields:

```
last_test_status = unset
last_test_error  = ""
last_tested_at   = unset
```

If neither field is in the mask (e.g. only `description`, `labels`,
`name`, or `catalog` changed), leave test fields untouched.

### `UpdateCredential`

When the update mask contains `auth_config` (a secret rotation —
password, token, or SSH key), clear test state on every source whose
`credential_id` references this credential. Same three fields, same
reset, ideally in the same transaction as the credential write so
clients never observe a window of "rotated credential / stale source
status."

If the mask is name / description / labels only, do **not** fan out.

The CLI already uses the literal mask path `"auth_config"` for
rotations — see `cmd/credential/update.go` in the CLI repo
(`paths = append(paths, "auth_config")`).

### `DeleteCredential`

No change. (Cascades / FK behavior are out of scope here.)

## Non-goals

- Auto-running `source test` after a mutation. Re-testing is the user's
  call; the server only invalidates the cache.
- Doing the same for any analogous "freshness cache" on other resources
  (e.g. runner health). If we add comparable cached state elsewhere we
  apply the same principle then — not now.
- Encrypting, re-validating, or otherwise touching the credential
  material itself.

## Proto fields involved

`admiral.source.v1.Source`:
- `last_test_status` (`SourceTestStatus`, optional, field 21)
- `last_test_error` (string, field 22)
- `last_tested_at` (`google.protobuf.Timestamp`, field 23)
- `url` (string)
- `credential_id` (string, optional)

`admiral.source.v1.UpdateSourceRequest` — `google.protobuf.FieldMask`.

`admiral.credential.v1.UpdateCredentialRequest` —
`google.protobuf.FieldMask`. Mask path for secret rotation is
`auth_config`.

## Edge cases

- **Concurrent test in flight.** If a `TestSource` RPC is in flight when
  the user updates the URL, the test result landing afterward must not
  "win" against the cleared state. Two options: (a) stamp tests with
  the source revision they ran against and drop stale results; (b)
  accept the race and document it. (a) is cleaner; (b) is acceptable
  for v1 if (a) is non-trivial.
- **Credential update with no material change** (e.g. labels only).
  The fan-out must be gated on `auth_config` being in the mask, not on
  `UpdateCredential` being called at all.
- **Idempotent update** (user sends the same URL back). The mask still
  contains `url`, so we clear regardless. Comparing old vs. new value to
  decide whether to clear is more code for no user benefit.

## Acceptance criteria

- `UpdateSource`:
  - mask contains `url` only → test fields cleared
  - mask contains `credential_id` only → test fields cleared
  - mask contains both `url` and `credential_id` → test fields cleared
  - mask contains neither (e.g. `description`, `labels`, `name`,
    `catalog`) → test fields preserved
- `UpdateCredential`:
  - mask contains `auth_config` → every source with
    `credential_id == this credential` has its test fields cleared in
    the same transaction; sources pointing at *other* credentials are
    untouched
  - mask without `auth_config` (labels / description / name only) → no
    fan-out, no source rows touched

## CLI follow-up (out of scope for this work)

Once the server is correct, the CLI's existing `formatTestStatus`
naturally shows `-` (unset) rather than stale `SUCCESS`. The CLI may
later add a display heuristic (`UpdatedAt > LastTestedAt` → "STALE") as
belt-and-suspenders, but it only catches Case 1 and is unnecessary if
the server does the right thing.
