package e2e

import "testing"

// TestRunnerTokenBasics exercises create / list on the runner-token
// surface. Token get / revoke are deferred until v2 — they need the
// token UUID, which means parsing it from create's output and feeding
// it forward. See docs/e2e-testing.md.
func TestRunnerTokenBasics(t *testing.T) {
	cli := newCLI()
	const runner = "e2e-token-runner"

	t.Cleanup(func() { cli.Run(t, "agent", "delete", runner, "--force") })

	cli.Run(t, "agent", "create", runner).NoStderr()

	cli.Run(t, "agent", "token", "create", "primary",
		"--agent", runner, "--expires-in", "1h").NoStderr()

	cli.Run(t, "agent", "token", "list", runner).StdoutContains("primary")
}
