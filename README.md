# Admiral CLI

[![Release](https://img.shields.io/github/v/release/admiral-io/admiral-cli)](https://github.com/admiral-io/admiral-cli/releases/latest)
[![build](https://github.com/admiral-io/admiral-cli/actions/workflows/release.yaml/badge.svg)](https://github.com/admiral-io/admiral-cli/actions/workflows/release.yaml)
[![CodeQL](https://github.com/admiral-io/admiral-cli/actions/workflows/codeql.yaml/badge.svg)](https://github.com/admiral-io/admiral-cli/actions/workflows/codeql.yaml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](https://github.com/admiral-io/admiral-cli/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/go.admiral.io/cli)](https://goreportcard.com/report/go.admiral.io/cli)

The official command-line interface for [Admiral](https://admiral.io), the deployment orchestrator by [Admiral](https://github.com/admiral-io).

Admiral manages infrastructure provisioning and application deployment as a single, dependency-aware control plane. It orchestrates the tools you already use (Terraform, Helm, Kustomize, any CI/CD system) and maintains the dependency graph across your full stack so changes happen in the right order.

No proprietary formats, no lock-in. If you stop using Admiral, you keep all your manifests and modules.

The CLI gives you direct access to the Admiral API from your terminal or CI/CD pipelines. This release covers authentication, applications, and environments; more of the platform lands in each release.

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

Pre-built binaries for Linux, macOS, and Windows are available on the [Releases](https://github.com/admiral-io/admiral-cli/releases) page.

## Quick Start

### Authentication

Sign in:

```bash
# Interactive: opens your browser and stores a session that refreshes itself
admiral auth login

# Same, but limit the session to read-only scopes (e.g. on a shared machine)
admiral auth login --scope app:read,env:read

# No browser handy? Store an API key instead (prompted, not echoed).
# Create one in the Admiral UI.
admiral auth login --with-token

# Or store a 1Password reference; the key is fetched via `op` on each run
echo "op://Engineering/admiral/api-key" | admiral auth login --with-token

# Check what the CLI will use, or verify it against the server
admiral auth status
admiral whoami

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

Every resource has the same verbs: `list`, `get`, `describe`, `create`,
`update`, `delete`. Commands take the resource name or ID; `--app` scopes
environment commands to an application (or set `ADMIRAL_APP`).

```bash
# Applications
admiral app list
admiral app create billing-api --description "Handles billing" --label team=platform
admiral app describe billing-api

# Environments of an application
admiral env create staging --app billing-api
admiral env list --app billing-api
admiral env describe staging --app billing-api

# Machine-readable output for scripts
admiral app list -o json
admiral env list --app billing-api -o name

# Skip confirmation prompts in automation
admiral env delete staging --app billing-api --force
```

Run `admiral <command> --help` for every flag and more examples.

### Proxies

The CLI honors the standard `HTTPS_PROXY`, `HTTP_PROXY`, and `NO_PROXY`
environment variables for both API calls and browser login. No extra
configuration is needed. If your proxy intercepts TLS, install its CA
certificate in the operating system trust store, as you would for a browser.

## Documentation

Full documentation is available at [admiral.io/docs](https://admiral.io/docs).

## Community & Feedback

- [GitHub Issues](https://github.com/admiral-io/admiral-cli/issues) — Bug reports and feature requests
- [GitHub Discussions](https://github.com/admiral-io/admiral-cli/discussions) — Questions and community conversation
- [Admiral Community](https://github.com/admiral-io/admiral-community) — Join the broader Admiral community
