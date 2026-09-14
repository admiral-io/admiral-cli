package e2e

import "testing"

// TestRunnerCRUD exercises create / list / get / update / delete for a
// single runner. Gives a yes/no signal that each verb works against the
// live server; not exhaustive flag coverage.
func TestRunnerCRUD(t *testing.T) {
	cli := newCLI()
	const name = "e2e-runner"

	// Best-effort cleanup: if the test bails mid-flow, this removes the
	// leftover runner so the next run starts clean. Also runs on the
	// happy path, where the runner is already gone — the unchained
	// Run() tolerates the non-zero exit.
	t.Cleanup(func() { cli.Run(t, "agent", "delete", name, "--force") })

	cli.Run(t, "agent", "create", name, "--description", "e2e test runner").
		NoStderr()

	cli.Run(t, "agent", "list").
		StdoutContains(name)

	var got struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	cli.Run(t, "agent", "get", name, "-o", "json").DecodeJSON(&got)
	if got.Name != name {
		t.Errorf("name = %q, want %q", got.Name, name)
	}
	if got.Description != "e2e test runner" {
		t.Errorf("description = %q, want %q", got.Description, "e2e test runner")
	}

	cli.Run(t, "agent", "update", name, "--description", "updated by e2e").
		NoStderr()

	cli.Run(t, "agent", "get", name, "-o", "json").DecodeJSON(&got)
	if got.Description != "updated by e2e" {
		t.Errorf("description after update = %q, want %q", got.Description, "updated by e2e")
	}

	cli.Run(t, "agent", "delete", name, "--force").StderrContains("deleted")

	cli.Run(t, "agent", "list").StdoutNotContains(name)
}
