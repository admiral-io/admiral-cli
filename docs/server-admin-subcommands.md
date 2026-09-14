# Server Admin Subcommands

**Status:** Proposal
**Audience:** Admiral Server maintainers (doc drafted from admiral-cli side; will be
moved or mirrored into admiral-server when the work is picked up)

## Motivation

The admiral-cli repository wants a proper end-to-end test suite that drives
the compiled `admiral` binary against a real Admiral server instance. The
test harness needs two things from the server that the server does not
currently expose outside the web flow:

1. A way to **mint a personal access token** for a synthetic user
   deterministically, without going through the Keycloak authorization-code
   flow and the profile-page UI.
2. A way to **reset domain state** between test scripts so each test starts
   from a known baseline.

The obvious shortcut — a gated HTTP endpoint like `POST /test/mint-pat`
behind a feature flag — was rejected. Gated endpoints have a way of
outlasting their gates, and the blast radius if one ever ships to prod is
a remote unauthenticated PAT mint. Not worth the risk.

The admiral-server binary already has a `migrate` subcommand for ops work
on the host. That surface is the right home for these operations: it runs
only where an operator already has shell access to the server host, so the
threat model is equivalent to direct database access. No new remote attack
surface, no feature flags to leak.

### Scope note

This proposal is about **server-binary subcommands** — the same kind of
surface as `admiral-server migrate`. It is explicitly **not** about adding
admin HTTP endpoints to the running server. Anything described here is
invoked by running the server binary on the host with `exec`, not by
sending a request to the running service.

## Goals

1. **Enable deterministic E2E setup** for admiral-cli's test harness
   without modifying the running server's HTTP surface.
2. **Stay decoupled from storage internals.** Callers should never need
   to know how PATs are hashed, how user rows are shaped, or what the
   session table looks like. The server's own code paths handle that.
3. **Be ops-useful beyond tests.** The same commands should be
   legitimately valuable for on-call engineers: "rotate a PAT from the
   host during an outage," "seed an admin user in a fresh install," etc.
   If the only justification is testing, the command is probably wrong.
4. **Match the `migrate` pattern.** Same binary, same flag conventions,
   same logging, same config-loading behavior. An operator who knows
   `migrate` should be able to guess how the new commands work.

## Non-goals

- Becoming a general admin/management CLI. This is a small set of
  targeted subcommands, not `admiral-server admin *` with twenty verbs.
- Replacing Keycloak for the web login flow. Human users still go
  through the OIDC authorization-code path. These subcommands only
  bypass it for operators and automated tests.
- Providing a remote API for any of these operations. If an operator
  cannot `ssh` to the server host, they cannot run these commands. That
  is intentional.
- Implementing a full "test mode" for the server. No feature flags, no
  runtime toggles, no alternate code paths. The subcommands are always
  available; their safety comes from requiring host access, not from
  being conditionally compiled.

## Proposed commands

Two new top-level subcommands on the `admiral-server` binary, alongside
the existing `migrate`:

### `admiral-server user create`

Creates a user record directly in the database using the server's own
user-creation code path (same code the OIDC callback handler uses when a
Keycloak user logs in for the first time).

```
admiral-server user create \
  --email e2e@admiral.local \
  --name "E2E Test User" \
  [--id <uuid>]              # optional; generated if omitted
```

Prints the created user's ID to stdout. Idempotent on `--email`: if a
user with that email already exists, returns the existing ID with exit 0
rather than erroring. (Makes test harness setup scripts simpler.)

### `admiral-server pat create`

Mints a personal access token for a given user using the server's own
PAT-creation code path — same hashing, same storage layout, same
validation rules as a token created through the profile page.

```
admiral-server pat create \
  --user <email-or-id> \
  [--name "e2e harness"] \
  [--token <literal-value>]   # optional; random if omitted
  [--expires-in 24h]          # optional; server default if omitted
```

Prints the plaintext token value to stdout on the first (and only) line
so it can be captured cleanly (`TOKEN=$(admiral-server pat create ...)`).

The `--token` flag lets test harnesses pin a known value so the same
token survives across database resets without the harness needing to
re-capture stdout each time. **This flag must be rejected in production
builds** — see Security considerations below.

### Extending `migrate` (no change proposed)

The existing `admiral-server migrate up` / `migrate down` are sufficient
for schema reset between test scripts. The test harness will run
`migrate down --all && migrate up` between scripts to get a clean
database, then re-run `user create` and `pat create` to re-seed.

This is slower than a SQL `TRUNCATE` loop but has zero schema coupling.
If this turns out to be too slow in practice (say, several seconds per
script), a later proposal can add `admiral-server db reset` as a
faster alternative. Out of scope for this round.

## Security considerations

The core question: does exposing "mint a PAT for any user" as a
subcommand materially widen the attack surface?

**No, for this reason:** anyone who can execute `admiral-server` on the
host already has filesystem access to the database credentials (they're
in the same config file the binary reads to connect) and network access
to the database. They can already read, modify, or forge any row in any
table, including the PAT table. A `pat create` subcommand is strictly
less powerful than what they already have — it just makes the legitimate
operation (minting a correctly-hashed PAT) easy, without making the
illegitimate ones any easier.

The threat model is "operator with host access," same as `migrate`. That
threat model is already trusted by the existence of `migrate drop` (or
equivalent), which can destroy the entire database.

### `--token <literal-value>` is the one real risk

Letting the caller pin the token value is a convenience for testing, but
in a prod context it would let a host-access attacker mint a predictable
token and then use it remotely to impersonate the user over the API,
without needing continued host access.

Mitigations:

- **Build-tag gate.** `--token` is only compiled in under a `dev` or
  `e2e` build tag. Release builds reject the flag with an error. The
  rest of `pat create` (with a random token) stays available in release
  builds because it's no more dangerous than direct DB access.
- **Alternative: drop `--token` entirely.** Test harnesses capture the
  random token from stdout and re-export it after each reset. Slightly
  more harness code, zero risk. Acceptable fallback if the build-tag
  approach is contentious.

Recommendation: start with the build-tag approach. It's the ergonomic
path and the risk is bounded.

### Compensating controls for synthetic user creation

`user create` is the more interesting risk than `pat create`, because it
introduces a new identity into the system that has no Keycloak backing.
A synthetic user is, by construction, an identity that SSO never
approved. Several things can go wrong if this surface is not constrained:

- **Prod drift.** A synthetic user gets created on a staging box, the
  database gets cloned into production during a disaster-recovery drill,
  and now prod has a user that bypasses SSO.
- **Identity collision.** A synthetic user is created with email
  `alice@company.com`, then the real Alice signs in via Keycloak and the
  OIDC callback tries to reconcile — either the synthetic user gets
  adopted (account takeover), or Alice's login fails in a confusing way.
- **Quiet privilege creep.** Synthetic users default to whatever the
  "new user" default is. If that default is ever widened (e.g. "all
  users see all environments"), synthetic users silently inherit it.
- **No audit signal.** A real user creation flows through the OIDC
  callback, which logs structured events. A subcommand invocation on a
  host might never reach the application log pipeline.

The following compensating controls should be implemented **together**,
not picked à la carte. Each one is cheap; the defense-in-depth comes
from having all of them.

**1. Reserved email domain.** `user create` rejects any `--email` whose
domain is not in a configured allowlist, defaulting to
`admiral.local` / `admiral.invalid` / `.test`. These are RFC 6761 /
IANA-reserved TLDs that cannot resolve on the public internet and
cannot be registered by a real organization. A synthetic user can
never share an email with a real one, so identity collision is
impossible by construction.

**2. `synthetic = true` marker column.** Add a boolean column to the
users table. `user create` sets it; the OIDC callback does not. The
column makes synthetic users trivially queryable ("show me every user
not backed by SSO") and gives downstream code a discriminator for
policy decisions ("synthetic users cannot be assigned the org-admin
role," "synthetic users are excluded from billing counts," etc.).

**3. No Keycloak subject association.** Synthetic users have
`keycloak_sub = NULL`. The OIDC callback handler must refuse to
reconcile a Keycloak login against a user with `keycloak_sub = NULL` —
if a collision somehow occurs (shouldn't, given the reserved domain,
but belt-and-suspenders), the real SSO user's login errors cleanly
instead of quietly adopting the synthetic account.

**4. Environment affirmation.** `user create` and `pat create` read
the same config the server reads (so they pick up the same `env`
value). If `env == production`, the command refuses to run unless
`--force-production` is passed. No `ADMIRAL_TEST_MODE`-style runtime
flag; the gate is the declared environment of the database the command
is pointed at. This makes accidental "ssh'd into the wrong box" a hard
error instead of a silent mutation.

**5. Structured audit logging.** Every `user create` and `pat create`
invocation writes a structured log entry (same pipeline as the
server's request logs) containing: timestamp, hostname, OS uid/username
of the invoker, full command line (with `--token` values redacted),
exit status, and the resulting user ID / PAT ID. SRE can alert on any
`pat create` against the prod database in the same way they'd alert on
a `migrate down`.

**6. Minimum privilege by default.** Synthetic users are created with
zero roles and zero environment memberships. Granting anything is a
separate, explicit step. This matters because the defaults for real
users are tuned for a human who just completed SSO — those defaults
may reasonably be "read access to the default environment." Synthetic
users are not humans and should not inherit those assumptions.

**7. Synthetic user TTL (optional, recommended).** Add an
`expires_at` column to synthetic users, defaulting to 30 days from
creation. The server rejects authentication against expired synthetic
users. This prevents synthetic users from accumulating indefinitely in
long-lived dev databases. Can be opted out with
`--no-expiry` for known-ops use cases.

**8. `pat create --token <literal>` stays build-tag-gated.** Already
covered above, but listed here for completeness: the pinned-token
convenience flag is never present in release builds.

With all of the above in place, the honest threat model is:

- An operator with host access can create a synthetic user in a
  reserved-domain namespace that cannot collide with real identities,
  marked as synthetic, with no privileges, logged in the audit
  pipeline, optionally auto-expiring, and only possible in non-prod
  environments without an explicit force flag.
- That is strictly less capability than the operator already has via
  direct `psql`, and every action is observable.

### What this does *not* expose

- No remote endpoint. The server's HTTP surface is unchanged.
- No feature flag. There is no runtime switch that could accidentally
  ship enabled.
- No new database schema except the `synthetic` and `expires_at`
  columns on the users table (both nullable / defaulted so existing
  rows are unaffected).
- No bypass of rate limits, authorization, audit logs, or any other
  server-enforced policy that applies once a PAT is in use — the PAT
  created via this command is indistinguishable from one created via
  the UI and is subject to all the same policies.

## Alternatives considered

### HTTP endpoint gated by env var / feature flag

`POST /test/mint-pat` behind `ADMIRAL_TEST_MODE=1`. **Rejected.** Gated
endpoints accumulate: "just this one" becomes five, the flag becomes
load-bearing for dev workflows, and a misconfigured deploy ships with
the flag on. The blast radius is remote unauthenticated PAT mint.

### Direct SQL seeding from the test harness

Test harness connects to Postgres and does `INSERT INTO users ...` +
`INSERT INTO personal_access_tokens ...`. **Rejected.** Couples the
admiral-cli test suite to the admiral-server schema and hashing scheme.
Silent breakage when the server changes its storage layout.

### Importing admiral-server Go packages from admiral-cli tests

Vendor or depend on admiral-server's `pat` package directly and call its
mint function from test code. **Rejected.** Creates a cross-repo Go
module dependency for the CLI, which is otherwise standalone, just to
service test code. Build-time coupling is worse than runtime coupling
via the server binary.

### Driving the real OIDC flow programmatically

Keycloak password-grant → admiral OIDC callback → scrape session cookie
→ call the real PAT creation endpoint. **Rejected.** Three moving parts
of brittleness. Exercises Keycloak and admiral's session-minting in
every test run, which is not what the CLI tests are trying to cover.
Appropriate for a dedicated auth-flow integration test; overkill here.

## Consumer: admiral-cli E2E harness

Illustrative sequence the harness will run (details will land in a
separate admiral-cli design doc):

```
# Bring up the stack using admiral-server's existing compose file
docker compose -f admiral-server/docker-compose.yml up -d --wait

# Schema
admiral-server migrate up

# Seed
USER_ID=$(admiral-server user create --email e2e@admiral.local --name e2e)
export ADMIRAL_TOKEN=$(admiral-server pat create --user e2e@admiral.local \
                                                  --name "e2e harness" \
                                                  --token "$KNOWN_TEST_TOKEN")

# Run testscript suite (each script sees a freshly migrated database)
go test ./test/e2e/...

# Teardown
docker compose -f admiral-server/docker-compose.yml down -v
```

Between individual testscript files, the harness runs
`migrate down --all && migrate up && user create && pat create` to
restore baseline state. The known-token pinning via `--token` means
`ADMIRAL_TOKEN` can be set once in `TestMain` and reused across all
scripts without re-export.

## Open questions

1. **Is there a better home than `user create` / `pat create` at the
   top level?** Options: `admiral-server admin user create`,
   `admiral-server ops user create`, keep it flat. Naming discussion
   should happen at implementation time.
2. **Should `migrate down --all` require `--confirm` or similar?** It
   currently drops user data; if E2E calls it frequently, accidentally
   running it against a dev database would be painful. Possibly orthogonal
   to this proposal.
3. **Do we need `admiral-server pat revoke` and `admiral-server pat list`
   to round out the surface?** Useful for on-call, but not required for
   the E2E use case. Defer until there's a concrete ask.
4. **What exactly does `user create` do about the Keycloak subject ID?**
   Real users have a `sub` claim tying them to their Keycloak identity.
   Synthetic users created via this command don't have a Keycloak
   counterpart. Either the column is nullable, or `user create` accepts
   an optional `--keycloak-sub` flag, or it synthesizes a placeholder.
   Needs a look at the user table before deciding.

## Out of scope (future work)

- `admiral-server db reset` for fast truncation without re-running
  migrations.
- `admiral-server pat rotate` / `pat revoke` / `pat list`.
- Corresponding admin HTTP endpoints if a legitimate remote-ops need
  ever emerges (at which point they should be authenticated and audited,
  not gated by a flag).
- Seeding other domain objects (runners, environments, etc.) via
  subcommand. The test harness can use the CLI itself for that, since
  exercising those commands is the whole point of the E2E suite.