# Credential Providers

**Status:** Proposal
**Audience:** Admiral CLI maintainers

## Motivation

The Admiral CLI today stores its API bearer token as plaintext JSON in
`~/.config/admiral/config.json` (mode `0600`). That is acceptable as a floor
but it is the single most sensitive artifact a user has on their machine,
and it sits in cleartext on disk for the lifetime of the install.

Security-conscious users (and their security teams) expect the option to
keep that token in a vault that requires an explicit unlock — an OS
keychain, a password manager like 1Password, or a secrets broker.
Automation environments expect the opposite: a scriptable, non-interactive
path that pulls the token from an environment variable or a
short-lived OIDC exchange.

This doc proposes a pluggable **credential provider** layer that resolves
the CLI's API token through a common reference syntax, with a plaintext
file as the default and additional providers opt-in.

### Scope note: this is about the CLI's own token

This design is strictly about the one secret the CLI keeps *resident on
the user's box* between invocations — the Admiral API bearer token. That
is where the disk-exposure risk actually lives.

`admiral credential create` is explicitly **not** in scope. That command
is a one-shot ingestion: a user enters a secret (password, token, SSH
key) once, the CLI ships it to the Admiral server over gRPC, the server
encrypts it at rest, and the CLI forgets it. Nothing persists locally, so
there is nothing to protect with a provider.

## Goals

1. **Keep plaintext-on-disk as the default.** Zero external dependencies
   out of the box. No new install steps for users who don't care.
2. **Support secret *references* for the stored API token.** A reference
   looks like a URI (`op://...`, `keychain://...`) and is resolved at
   use-time by a provider.
3. **Extensible via a provider interface**, not a hard-coded list.
   Adding a new backend should be additive, not a refactor.
4. **First-class CI path.** Env vars and short-lived tokens must work
   without a TTY and without a config file.
5. **Preserve the current UX.** `admiral config set token <PAT>` must
   keep working exactly as it does today.

## Non-goals

- Becoming a general-purpose secret manager. We resolve references; we
  don't store, rotate, or broker secrets.
- Encrypting the existing config file at rest. If a user wants
  encryption, they use a provider that gives them encryption (keychain,
  1Password). We don't invent a bespoke crypto scheme.
- Guaranteeing availability of every listed provider on every platform.
  Some are opt-in packages; some are platform-specific.

## Current state

The seam already exists:

- `internal/credentials/credentials.go` — `ResolveToken(configDir)` is
  the single chokepoint that every RPC goes through via
  `internal/client/client.go`. It resolves in order:
  1. `ADMIRAL_TOKEN` environment variable
  2. `token` key in `config.json`

This is the right place to plug in a provider resolver. It does not need
to be rewritten — the change is additive.

## Design

### Reference syntax

A secret reference is a URI. The scheme picks the provider.

```
<scheme>://<provider-specific-path>[?<options>]
```

Examples:

```
op://Personal/admiral-prod/token         # 1Password
keychain://admiral/token                  # OS keychain
file:///etc/admiral/token                 # file (for CI images)
env://ADMIRAL_TOKEN                       # env var (explicit form)
```

Any stored value that does **not** match `<scheme>://...` is treated as a
literal secret — this keeps the current plaintext behavior intact.

### Provider interface

```go
// internal/credentials/provider.go
type Provider interface {
    // Scheme returns the URI scheme this provider handles (e.g. "op").
    Scheme() string

    // Resolve returns the secret value for the given reference.
    // ctx carries the CLI's cancellation/timeout.
    Resolve(ctx context.Context, ref Reference) (string, error)
}

type Reference struct {
    Scheme  string
    Path    string            // host + path portion
    Options map[string]string // parsed query string
}
```

A small registry keyed by scheme dispatches to the right provider.
Providers register themselves via `init()` in their subpackages, and the
binary can be built with or without any of them using build tags:

```go
// internal/credentials/providers/onepassword/provider.go
//go:build onepassword

func init() { credentials.Register(&provider{}) }
```

This lets us ship a lean default binary and a
`admiral-everything` build with every provider, or let distributors pick.

### Resolution at the chokepoint

`ResolveToken` grows from a 2-step to a 3-step resolver:

1. `ADMIRAL_TOKEN` env var (unchanged).
2. Config `token` value — if it parses as a `<scheme>://...` URI, hand
   off to the matching provider. Otherwise return as literal (unchanged).
3. Error (unchanged).

The reference never leaves the user's machine: the provider resolves
locally, and the plaintext token flows to the Admiral server over the
existing bearer-auth gRPC channel exactly as today.

### Caching (we don't)

The natural instinct is to cache resolved secrets in the CLI. That
instinct is wrong here, because each command invocation is a fresh OS
process — an in-process cache dies the moment `admiral app list` exits,
so it cannot smooth the `list` → `get` → `update` flow at all.

The providers we care about already solve this problem at a lower
layer, and they solve it better than we could:

- **1Password (`op://`)** — the `op` CLI talks to the 1Password desktop
  app over a local socket. When the vault is unlocked (biometric, or
  the "keep unlocked for N minutes" setting), `op read op://...` is
  silent and returns in milliseconds. The *desktop app* decides when to
  re-prompt, not us. This is the same shape as `ssh-agent` +
  `SSH_AUTH_SOCK`: the long-lived agent holds the state; the short-lived
  CLI just asks.
- **OS keychain (`keychain://`)** — unlocked with the user's login
  session. `go-keyring` calls return immediately while the keychain is
  unlocked and the binary is trusted (ACL prompts are handled once, per
  OS UX).
- **`env://`, `file://`, literal** — no resident state to cache.

So V1 does **no caching**. Every invocation calls the provider. That is
cheap (local IPC) for all V1 and V2 providers.

**Cloud providers split cleanly into two use cases**, and the answer is
different for each.

**Case A: automation / CI.** A script fetches the token from Vault /
AWS Secrets Manager / etc. once at the top, exports it, runs a batch of
admiral commands, and exits. The credential lives exactly as long as
the script does. This is already solved today with no new code:

```bash
export ADMIRAL_TOKEN=$(vault read -field=token secret/admiral/ci)
admiral app list
admiral app get my-app
# ...
```

A `vault://` scheme here would be pure sugar — the behavior is already
correct, because the *script lifetime is the cache*. We may or may not
want to add a dedicated provider; it does not unlock anything
fundamentally new.

**Case B: interactive use, cloud-backed.** A developer sits at a
terminal and wants each `admiral` invocation to pull from a corporate
Vault. This is where the round-trip-per-command problem bites — and
it is not clear the use case is real. Interactive developers typically
reach for 1Password or the OS keychain; Vault-for-interactive-CLI is
unusual. **Before building for this, we should confirm anyone actually
wants it.**

If Case B turns out to be a real need, the least-bad approach is
probably the `admiral auth login` model (a la `gcloud`, `aws sso
login`, `doctl`): one cloud round-trip exchanges a long-lived vault
credential for a short-lived Admiral token, which gets stored via a
V1/V2 provider (1Password, keychain) for the rest of the session.
That reduces Case B to a solved problem. We are deliberately not
committing to it here.

## Providers

Versioned by when we build them.

### V1 — MVP (ship with the first release of this feature)

| Scheme       | Backend                     | Notes                                                                                 |
|--------------|-----------------------------|---------------------------------------------------------------------------------------|
| *(literal)*  | Config value as-is          | Default. No change from today.                                                        |
| `env://`     | Environment variable        | Explicit form of today's `ADMIRAL_TOKEN` fallback.                                    |
| `file://`    | File on disk                | Standard CI pattern. Must enforce file mode `<= 0600`.                                |
| `op://`      | 1Password (via `op` CLI)    | Shell out to `op read op://...`. Zero new Go deps; user installs `op` themselves.     |

`op://` is deliberately in V1, not deferred. The reason is dogfooding:
1Password is the maintainer's daily credential workflow, so shipping it
first produces real usage feedback on the provider interface, the
resolution flow, the error messages, and the UX of a locked vault. That
feedback then shapes `keychain://` and any future cloud work.
Implementation cost is low — `op://` is a thin wrapper around
`exec.Command("op", "read", ref)` with no Go import footprint.

### V2 — Ship soon after

| Scheme         | Backend                        | Notes                                                                                  |
|----------------|--------------------------------|----------------------------------------------------------------------------------------|
| `keychain://`  | OS keychain via `go-keyring`   | macOS Keychain / Windows Credential Manager / Secret Service on Linux.                 |

The keychain provider adds a cgo/platform story (different backends per
OS, ACL prompts, headless-Linux edge cases), so it ships in its own
release informed by what we learned in V1.

### V3 — Future / on-demand

| Scheme         | Backend                              | Notes                                                       |
|----------------|--------------------------------------|-------------------------------------------------------------|
| `vault://`     | HashiCorp Vault                      | Real need in regulated envs.                                |
| `awssm://`     | AWS Secrets Manager                  | Uses default credential chain.                              |
| `gcpsm://`     | Google Secret Manager                |                                                             |
| `azkv://`      | Azure Key Vault                      |                                                             |
| `bw://`        | Bitwarden (via `bw` CLI)             | Mirror of the 1Password pattern.                            |
| `oidc://`      | OIDC exchange (any IdP)              | See CI section. Requires server-side exchange endpoint.     |

We do not commit to any of these upfront. The provider interface is the
contract; cloud ones land when a user asks.

## CI and automation

Two very different worlds to support.

### Static CI secrets (most common today)

GitHub Actions, GitLab CI, etc. inject a secret into the job environment.
Current pattern works unchanged:

```yaml
- run: admiral app list
  env:
    ADMIRAL_TOKEN: ${{ secrets.ADMIRAL_TOKEN }}
```

No design change needed — this is why env-var resolution stays V1.

The `file://` provider covers the other common CI pattern: secrets
mounted as files by Kubernetes, Docker secrets, or systemd
`LoadCredential=`.

### Workload-identity / OIDC exchange (the direction worth investing in)

In CI and other headless environments, the workload already has a
trustworthy identity sitting in its environment — it was put there by
the orchestrator at job start. Examples:

| Environment          | Where the identity token lives                                          |
|----------------------|-------------------------------------------------------------------------|
| Kubernetes pod       | Projected file at `/var/run/secrets/kubernetes.io/serviceaccount/token` |
| GitHub Actions       | Minted on demand from `$ACTIONS_ID_TOKEN_REQUEST_URL` with audience     |
| GitLab CI            | `$CI_JOB_JWT_V2` (or configured `id_tokens`)                            |
| AWS EC2/EKS/Lambda   | IMDSv2 / IRSA token                                                     |
| GCP GCE/GKE          | Metadata server token endpoint                                          |

We can use that identity to obtain a short-lived Admiral token without
a browser, a PAT, or a long-lived secret anywhere in the CI system.
This is strictly better than a static PAT: nothing to rotate, nothing
to leak, token lifetime measured in minutes, audit trail ties back to
the specific workflow run / pod / role.

**Server-side prerequisite** (not yet built on Admiral): a token-exchange
endpoint following OAuth 2.0 Token Exchange (RFC 8693). At a minimum:

- Accept an ID token (JWT) as `subject_token`.
- Verify the issuer and signature against a configured list of trusted
  IdPs, each with its own JWKS URL.
- Match the token's claims (`iss`, `aud`, `sub`, `repository`, etc.)
  against a trust policy that maps them to an Admiral identity.
- Return a short-lived Admiral access token with an expiry.

#### The flow on the CLI side

Concretely, not hand-wavy:

1. **Bootstrap.** Something triggers an exchange — either the user
   running a one-shot `admiral auth exchange --source <idp>` at the top
   of a CI script, or the first command of the session when `token` is
   set to `oidc://<idp>`.
2. **Acquire the subject JWT.** Per-IdP logic, small lookup table: read
   the projected file for k8s, call the `ACTIONS_ID_TOKEN_REQUEST_URL`
   for GitHub, hit the metadata server for AWS/GCP, etc.
3. **Exchange.** POST the JWT to Admiral's exchange endpoint; receive a
   short-lived Admiral token and its `expires_at`.
4. **Cache the result to the config dir** — e.g.
   `~/.config/admiral/token_cache.json` with `{token, expires_at,
   source}`. This is the one legitimate on-disk cache this design
   endorses: the cached value is short-lived by design, the underlying
   identity is already sitting unencrypted on the box, and losing the
   cache just triggers a transparent re-exchange.
5. **On subsequent commands**, `ResolveToken` reads `token_cache.json`
   first. If `expires_at` is in the future (with a small safety
   margin), use it. If not, goto step 2 and re-exchange. All silent.

This is the same loop as AWS SDK credential refresh, `gcloud`'s
application-default cache, and `kubectl`'s exec-credential plugins. Not
novel; just needs to be built.

#### What this means for the CLI commands

Two surfaces, sharing one implementation:

1. **Explicit bootstrap:** `admiral auth exchange --source k8s-sa`
   (or `--source github-actions`, etc.). Runs steps 1–4 once. Useful
   when a script author wants the exchange to be visible and
   deterministic, and wants it to fail loudly at the top of the job if
   misconfigured.
2. **Transparent reference:** `admiral config set token
   oidc://github-actions`. Steps 1–4 run lazily on first `ResolveToken`
   call; step 5 runs on every subsequent call. Useful for baked-in
   Dockerfiles and anywhere the user wants set-and-forget.

Both are V3 and strictly gated on the server-side exchange endpoint
existing. The per-IdP JWT-acquisition logic is the only IdP-specific
code; everything else (exchange call, on-disk cache, expiry handling)
is shared.

#### Identity mapping: whom does the exchange authorize as?

The RFC 8693 flow above answers "is this JWT trustworthy?" It does
**not** answer the harder question: *trustworthy to act as which
Admiral principal?* Admiral identities today are tightly bound — a
runner has a runner ID, a user has a user ID, and a SAT/PAT encodes
which one. The OIDC exchange only works if there is a server-side
mapping from verified JWT claims to an Admiral principal.

Every major system that has built this has converged on the same
shape: **federated identity bindings**, configured explicitly per
principal. Examples:

- **AWS IRSA** — an IAM role has a trust policy: "allow
  `AssumeRoleWithWebIdentity` if `iss == <cluster-oidc-issuer>` AND
  `sub == system:serviceaccount:<ns>:<sa-name>` AND `aud ==
  sts.amazonaws.com`."
- **GCP Workload Identity Federation** — a workload identity pool +
  provider + attribute mapping + IAM binding to a service account.
- **GitHub → any cloud** — the cloud's trust policy pattern-matches on
  the `sub` claim (e.g. `repo:org/repo:environment:prod`).

Admiral needs an analogous concept. Sketch, not a commitment:

```text
FederatedIdentityBinding
  target_principal: runner:abc-123        # the Admiral identity to grant
  issuer:           https://kubernetes.default.svc.cluster.local
  audience:         admiral.example.com
  claim_match:
    sub: system:serviceaccount:prod:admiral-runner
    # optionally: namespace, labels, repository, environment, ...
```

The exchange endpoint, after verifying the JWT, looks up the first
binding whose issuer + audience + claim patterns match and issues a
short-lived token for that `target_principal`. No match → exchange
fails. This is the piece of server-side work that *has to exist* before
the CLI story is useful.

#### Implications for existing resource-create commands

Today `admiral runner create` issues a SAT by default — the runner
can't authenticate without one. If workload identity is the mechanism,
that baked-in SAT is unnecessary and actively undesirable: it is
another secret to protect and an alternative credential path you may
not want to leave open.

So the `runner create` (and similar) surfaces should grow a federated
mode. Possible shape:

```bash
# Current behavior (unchanged): creates the runner and prints a SAT.
admiral runner create my-runner

# Federated: no SAT issued; registers a trust binding instead.
admiral runner create my-runner \
  --federate-from k8s \
  --issuer https://kubernetes.default.svc.cluster.local \
  --sub system:serviceaccount:prod:admiral-runner \
  --audience admiral.example.com
```

The `--federate-from` variant does not print a SAT — there is nothing
secret to hand off. At deploy time, the workload simply runs where its
OIDC identity matches the binding, and `admiral auth exchange` (or the
`oidc://` scheme) does the rest.

This is the end-state vision: **no pre-shared secrets in a workload
deployment**. The pod comes up with only its Kubernetes SA token (or
equivalent), which the orchestrator already provisioned, and that is
enough — Admiral recognizes it via the federated binding and issues
short-lived access from there. The user deploys by placement, not by
credential handoff.

We are not building this now. It is captured here so the OIDC exchange
endpoint — when it is designed — accounts for the mapping problem and
the `runner create` (and related) commands reserve room for
`--federate-from`-style flags without breaking changes.

**Not in scope here:** interactive human SSO login (browser popup,
device code flow, authorization code flow). That is a different
feature for a different audience and does not belong in this CI-focused
section. If we ever build it, it reuses the same token-exchange
endpoint — the subject token just comes from a browser flow instead of
the workload environment.

## Security considerations

- **Memory hygiene.** Resolved secrets should be held in byte slices
  that are zeroed after use where practical. Go's GC makes this
  imperfect but we can avoid gratuitous string copies of the token.
- **File-mode enforcement** for `file://`. Refuse to read a file with
  mode wider than `0600` unless `?insecure=true` is explicitly set.
- **Logging.** No provider should ever log a resolved secret, not even
  at debug level. Logging a *reference* is fine and useful.
- **Prompt channel.** Interactive prompts from providers (biometric,
  passphrase) must go to stderr, never stdout, so pipelines don't see
  them.
- **Failure mode.** If a provider can't resolve (locked vault, offline,
  missing CLI binary), return a clear error that names the provider and
  the reference; do not silently fall through to a different source.

## Rollout

1. Extract the URI detection + provider registry into
   `internal/credentials`. Keep `ResolveToken` semantically identical:
   literal values still win.
2. Land `env://` and `file://` providers. No user-facing behavior
   change beyond the new explicit-form schemes.
3. Land `op://`. This completes V1 and begins dogfooding. Document the
   workflow in the README (`op` install, storing a PAT as an op item,
   setting `admiral config set token op://...`).
4. Collect feedback from V1 usage. Adjust the provider interface, error
   messages, and docs as needed *before* committing more backends to it.
5. Land `keychain://` (V2) via `github.com/zalando/go-keyring`. This
   carries the cgo / platform story, hence its own release.
6. Revisit V3 based on user demand and the cloud-caching research item.

Each step is independently shippable and each one is additive.

## Open questions

1. Do we want `admiral config set token` to accept a reference directly
   (`admiral config set token op://...`), or only a literal? Leaning
   toward *yes*, with a confirmation prompt if the value looks like a
   reference to confirm the user didn't paste a secret by accident.
2. Cloud providers split into two cases (see Caching section).
   Automation is already solved today via `ADMIRAL_TOKEN=$(vault
   read ...)`; a dedicated `vault://` scheme would be sugar, not
   capability. Interactive cloud-backed use is the open question —
   both whether the demand exists *at all*, and if so whether
   `admiral auth login` → short-lived token → local provider is the
   right shape. Validate demand before designing further.
3. Workload identity requires a **federated identity binding** concept
   on the Admiral server (see OIDC → Identity mapping). This is the
   piece that maps verified JWT claims to a specific Admiral principal
   (runner, user, etc.). The OIDC exchange endpoint cannot be
   meaningfully designed without also designing this. Implication for
   CLI: `admiral runner create` (and similar commands that issue
   identities today) will eventually need a `--federate-from` variant
   that registers a binding instead of minting a SAT. Plan the flag
   surface now; don't ship it until the server side is real.