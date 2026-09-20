package e2e

import (
	"strings"
	"testing"
)

// Exit codes are the contract scripts depend on: 2 for a usage mistake,
// 4 when a sign-in is needed, 1 for everything else that failed.
const (
	exitUsage = 2
	exitAuth  = 4
)

// A syntactically valid key (prefix, length, checksum) that no server
// would accept; enough for the local half of --with-token.
const wellFormedKey = "admp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq1ZqpkG"

func TestVersion(t *testing.T) {
	cli := newCLI(t)
	cli.Run("version").Succeeds().NoStderr()
	cli.Run("--version").Succeeds().NoStderr()
}

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		msg  string
		hint string
	}{
		{"unknown command", []string{"bogus"}, `unknown command "bogus" for "admiral"`, "Run 'admiral --help' for usage."},
		{"unknown subcommand", []string{"app", "bogus"}, `unknown command "bogus" for "admiral app"`, "Run 'admiral app --help' for usage."},
		{"unknown flag", []string{"--bogus", "version"}, "unknown flag: --bogus", "--help' for usage."},
		{"missing positional", []string{"app", "get"}, "missing argument: get <name>", "Run 'admiral app get --help' for usage."},
		{"extra positional", []string{"app", "list", "extra"}, `unknown argument "extra"`, "Run 'admiral app list --help' for usage."},
		{"bad output format", []string{"-o", "xml", "version"}, `invalid output format "xml"`, ""},
		{"bad timeout flag", []string{"--timeout", "soon", "version"}, `invalid argument "soon" for "--timeout"`, ""},
		{"bad shell", []string{"completion", "tcsh"}, `unknown shell "tcsh"`, ""},
		{"unknown config key", []string{"config", "get", "bogus"}, `unknown config key "bogus"`, ""},
		{"mutually exclusive login flags", []string{"auth", "login", "--with-token", "--no-browser"}, "--with-token and --no-browser are mutually exclusive", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newCLI(t).Run(tc.args...).Exits(exitUsage).StderrContains("Error: " + tc.msg)
			if tc.hint != "" {
				r.StderrContains(tc.hint)
			}
			if r.Stdout != "" {
				t.Fatalf("usage error must not write to stdout, got:\n%s", r.Stdout)
			}
		})
	}

	t.Run("bad timeout env", func(t *testing.T) {
		c := newCLI(t)
		c.env = append(c.env, "ADMIRAL_TIMEOUT=soon")
		c.Run("version").Exits(exitUsage).StderrContains(`invalid ADMIRAL_TIMEOUT "soon"`)
	})
}

// Every command that needs the API refuses with exit 4 and the sign-in
// remedy when nothing is stored, before any network use.
func TestNotSignedIn(t *testing.T) {
	for _, args := range [][]string{
		{"app", "list"},
		{"app", "list", "-o", "json"},
		{"app", "get", "shop"},
		{"env", "list", "--app", "shop"},
		{"component", "list"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := newCLI(t).Run(args...).Exits(exitAuth).StderrContains("not signed in")
			r.StderrContains("Run 'admiral auth login' to sign in, or set ADMIRAL_API_KEY.")
		})
	}
}

// --with-token stores a key from a pipe; auth status reports it without
// contacting anything under --no-verify; logout removes it.
func TestAuthWithTokenLifecycle(t *testing.T) {
	cli := newCLI(t)

	cli.Run("auth", "status", "--no-verify").Exits(exitAuth).
		StdoutContains("Authenticated:  no").
		StdoutContains("Run 'admiral auth login' to sign in, or set ADMIRAL_API_KEY.")

	cli.WithStdin(wellFormedKey+"\n").Run("auth", "login", "--with-token").Succeeds()
	cli.Run("auth", "status", "--no-verify").Succeeds().
		StdoutContains("Authenticated:  yes").
		StdoutContains("api-key").
		StdoutNotContains(wellFormedKey)

	cli.Run("auth", "logout").Succeeds()
	cli.Run("auth", "status", "--no-verify").Exits(exitAuth).StdoutContains("Authenticated:  no")

	t.Run("malformed key", func(t *testing.T) {
		newCLI(t).WithStdin("not-a-key\n").Run("auth", "login", "--with-token").Exits(1).
			StderrContains("invalid API key")
	})
	t.Run("empty pipe", func(t *testing.T) {
		newCLI(t).WithStdin("").Run("auth", "login", "--with-token").Exits(exitUsage).
			StderrContains("Error: no API key on stdin")
	})
	t.Run("delete without --force fails before sign-in", func(t *testing.T) {
		newCLI(t).Run("app", "delete", "shop").Exits(exitUsage).
			StderrContains("--force required when not running interactively")
	})
	t.Run("env key wins and refuses login", func(t *testing.T) {
		c := newCLI(t)
		c.env = append(c.env, "ADMIRAL_API_KEY="+wellFormedKey)
		c.WithStdin(wellFormedKey+"\n").Run("auth", "login", "--with-token").Exits(1).
			StderrContains("ADMIRAL_API_KEY environment variable is being used")
	})
}

func TestConfigRoundTrip(t *testing.T) {
	cli := newCLI(t)
	cli.Run("config", "set", "server", "localhost:9999").Succeeds()
	cli.Run("config", "get", "server").Succeeds().StdoutContains("localhost:9999")
	cli.Run("config", "list").Succeeds().StdoutContains("localhost:9999")

	// A signed-out run refuses before dialing, whatever server is stored.
	cli.Run("app", "list").Exits(exitAuth)
}

func TestCompletionScripts(t *testing.T) {
	cli := newCLI(t)
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		cli.Run("completion", shell).Succeeds().NoStderr().StdoutContains("admiral")
	}
	cli.Run("completion", "zsh").StdoutContains("#compdef admiral")
}
