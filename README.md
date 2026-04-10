> :warning: This project is currently **under heavy development and is not considered stable yet**. This means that there may be bugs or unexpected behavior, and we don't recommend using it in production.

# Admiral CLI

[![Release](https://img.shields.io/github/v/release/admiral-io/admiral-cli)](https://github.com/admiral-io/admiral-cli/releases/latest)
[![build](https://github.com/admiral-io/admiral-cli/actions/workflows/release.yaml/badge.svg)](https://github.com/admiral-io/admiral-cli/actions/workflows/release.yaml)
[![CodeQL](https://github.com/admiral-io/admiral-cli/actions/workflows/codeql.yaml/badge.svg)](https://github.com/admiral-io/admiral-cli/actions/workflows/codeql.yaml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](https://github.com/admiral-io/admiral-cli/blob/master/LICENSE)
[![Go Report Card](https://goreportcard.com/badge/go.admiral.io/cli)](https://goreportcard.com/report/go.admiral.io/cli)

The official command-line interface for [Admiral](https://admiral.io), the deployment orchestrator by [Admiral](https://github.com/admiral-io).

Admiral manages infrastructure provisioning and application deployment as a single, dependency-aware control plane. It orchestrates the tools you already use (Terraform, Helm, Kustomize, any CI/CD system) and maintains the dependency graph across your full stack so changes happen in the right order.

No proprietary formats, no lock-in. If you stop using Admiral, you keep all your manifests and modules.

The CLI gives you direct access to the Admiral API for managing clusters, runners, and deployments from your terminal or CI/CD pipelines.

## Installation

### Homebrew (macOS/Linux)

```bash
brew install admiral-io/tap/admiral
```

### Scoop (Windows)

```powershell
scoop bucket add admiral https://github.com/admiral-io/scoop-bucket
scoop install admiral
```

### Docker

```bash
docker run --rm ghcr.io/admiral-io/admiral:latest
```

Pre-built binaries for Linux, macOS, and Windows are available on the [Releases](https://github.com/admiral-io/admiral-cli/releases) page.

## Quick Start

### Authentication

Create a Personal Access Token in the Admiral UI, then configure the CLI:

```bash
# Set your API server
admiral config set server https://admiral.example.com

# Set your token (prompted securely, not echoed)
admiral config set token

# Or pass it directly (visible in shell history)
admiral config set token <your-token>

# Or use an environment variable (useful for CI/CD)
export ADMIRAL_TOKEN=<your-token>
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

Available keys: `server`, `token`, `output`, `insecure`, `plaintext`

### Usage

```bash
# List your applications
admiral app list
```

## Documentation

Full documentation is available at [admiral.io/docs](https://admiral.io/docs).

## Community & Feedback

- [GitHub Issues](https://github.com/admiral-io/admiral-cli/issues) — Bug reports and feature requests
- [GitHub Discussions](https://github.com/admiral-io/admiral-cli/discussions) — Questions and community conversation
- [Admiral Community](https://github.com/admiral-io/admiral-community) — Join the broader Admiral community
