# Contributing

## Build, test, lint

```sh
make build      # binary for this platform, under dist/
make test       # go test -race ./...
make lint       # golangci-lint, same version CI runs
make verify     # go.mod and go.sum are tidy
```

CI runs `make lint`, `make verify` and `make test` on every pull request.
`go test ./test/e2e/...` builds the binary and runs it as a user would;
the scenarios that need a server skip unless `ADMIRAL_SERVER` and
`ADMIRAL_API_KEY` name one (see `test/e2e/main_test.go`).

## Style guide

Code comments cite "style guide §1.5" and similar. The guide is the
[Admiral CLI Style Guide](../admiral-cli-next/docs/cli-style-guide.md) in
the `admiral-cli-next` repository, checked out beside this one; the
command tree it governs is
[`COMMAND_TREE.md`](../admiral-cli-next/COMMAND_TREE.md) there. Read it
before adding a command or changing what one prints; the short version:

- stdout is the answer, stderr is everything else (prompts, hints,
  "No applications found.", next-page tokens).
- Usage mistakes exit 2 with a one-line hint; a missing sign-in exits 4;
  Ctrl-C exits 130; anything else that failed exits 1. Route usage errors
  through `cmderr.Usage` and positional checks through `internal/flags`.
- A name positional, parent flag or enum flag registers shell completion
  when it is created (`internal/complete`, `flags.Enum`).
- Anything destructive prompts; `--force`/`-f` skips the prompt, and a run
  that cannot prompt fails as a usage error before touching the network
  (`input.RequireInteractiveOrForce`).
- Row formats live in `cmd/<noun>/output.go`, one table definition shared
  by list, get and the create/update echo.

## Layout

- `cmd/` — one package per noun (`app`, `env`, `component`, `auth`,
  `config`), the root command and its tests.
- `internal/` — shared machinery: `client` (gRPC client and interceptors),
  `credentials` (API keys, sessions, references), `auth` (browser login
  and logout), `flags`, `complete`, `input`, `iostreams`, `output`,
  `cmderr`.
- `test/e2e/` — black-box scenarios against the compiled binary.
