# Admiral CLI

[![Release](https://img.shields.io/github/v/release/admiral-io/admiral-cli)](https://github.com/admiral-io/admiral-cli/releases/latest)
[![build](https://github.com/admiral-io/admiral-cli/actions/workflows/release.yaml/badge.svg)](https://github.com/admiral-io/admiral-cli/actions/workflows/release.yaml)
[![CodeQL](https://github.com/admiral-io/admiral-cli/actions/workflows/codeql.yaml/badge.svg)](https://github.com/admiral-io/admiral-cli/actions/workflows/codeql.yaml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](https://github.com/admiral-io/admiral-cli/blob/master/LICENSE)

The official command-line interface for [Admiral](https://admiral.io), the platform orchestrator.

Admiral manages infrastructure provisioning and application deployment as a single, dependency-aware control plane. It orchestrates the tools you already use (Terraform, Helm, Kustomize, any CI/CD system) and maintains the dependency graph across your full stack so changes happen in the right order.

No proprietary formats, no lock-in. If you stop using Admiral, you keep all your manifests and modules.

The CLI gives you direct access to the Admiral API from your terminal or CI/CD pipelines. This release covers authentication, applications, environments, and the component registry; more of the platform lands in each release.

## Installation

### Homebrew (macOS/Linux)

```bash
brew install admiral-io/tap/admiral
```

### Scoop (Windows)

```powershell
scoop bucket add admiral-io https://github.com/admiral-io/scoop-bucket
scoop install admiral-io/admiral
```

### Docker

```bash
docker run --rm ghcr.io/admiral-io/admiral:latest
```

Pre-built binaries for Linux, macOS, and Windows, plus `.deb`, `.rpm`, and Arch Linux packages, are available on the [Releases](https://github.com/admiral-io/admiral-cli/releases) page.

## Quick Start

### Authentication

Sign in:

```bash
# Interactive: opens your browser and stores a session that refreshes itself
admiral auth login

# Same, but limit the session to read-only scopes (e.g. on a shared machine)
admiral auth login --scope app:read,env:read

# Over SSH? Print the sign-in URL instead of launching a browser
admiral auth login --no-browser

# No browser at all? Store an API key instead (prompted, not echoed).
# Create one in the Admiral UI.
admiral auth login --with-token

# Or store a 1Password reference; the key is fetched via `op` on each run
echo "op://Engineering/admiral/api-key" | admiral auth login --with-token

# Check what the CLI will use and who the server says it is
admiral auth status

# Only what is stored: no network, no secret-store prompt
admiral auth status --no-verify

# Remove the stored credential (and revoke the session, if any)
admiral auth logout
```

For CI and automation, pass an API key through the environment. It takes
precedence over anything stored by `auth login`:

```bash
export ADMIRAL_API_KEY=<key>
```

### Configuration

```bash
# List all configuration values
admiral config list

# Get a specific value
admiral config get server

# Set a value (omit value to be prompted interactively)
admiral config set <key> [value]

# Remove a value
admiral config unset <key>
```

Available keys: `server`, `output`, `insecure`, `plaintext`

### Usage

Applications and environments share the same verbs: `list`, `get`,
`describe`, `create`, `update`, `delete`. Commands take the resource name or
ID. An environment is addressed by its path, `app/env`, or by name with
`--app` for the scope — never both at once. The component registry has its
own verbs, below.

```bash
# Applications
admiral app list
admiral app create billing-api --description "Handles billing" --label team=platform
admiral app describe billing-api

# Environments of an application
admiral env create billing-api/staging
admiral env list billing-api
admiral env describe billing-api/staging

# Components: Terraform modules, Helm charts, or manifests, published as
# immutable revisions and named by tag
admiral component publish ./modules/cloud-sql --tag v1.2.0
admiral component list
admiral component get cloud-sql

# Machine-readable output for scripts
admiral app list -o json
admiral env list billing-api -o name

# Skip confirmation prompts in automation
admiral env delete billing-api/staging --force
```

Run `admiral <command> --help` for every flag and more examples.

### Proxies

The CLI honors the standard `HTTPS_PROXY` and `NO_PROXY` environment
variables for both API calls and browser login. (`HTTP_PROXY` applies only to
plain-HTTP requests, which the CLI does not make against a production server.)
No extra configuration is needed. If your proxy intercepts TLS, install its CA
certificate in the operating system trust store, as you would for a browser.

## Documentation

Full documentation is available at [admiral.io/docs](https://admiral.io/docs).

