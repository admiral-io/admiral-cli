package e2e

import (
	"fmt"
	"testing"
	"time"
)

// uniqueName is a resource name no earlier run left behind.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()%1_000_000_000)
}

// The application lifecycle through every output format a script might
// use. Runs only against a live stack (see serverStack).
func TestApp_RoundTrip(t *testing.T) {
	cli := newServerCLI(t)
	name := uniqueName("e2e-app")
	t.Cleanup(func() { cli.Run("app", "delete", name, "--force") })

	cli.Run("auth", "status").Succeeds().StdoutContains("Authenticated:  yes")

	// create echoes the narrow row; -o name is just the name.
	cli.Run("app", "create", name, "--description", "e2e round trip", "--label", "suite=e2e").
		Succeeds().StdoutContains("NAME").StdoutContains(name)

	var app struct {
		ID          string            `json:"id"`
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Labels      map[string]string `json:"labels"`
	}
	cli.Run("app", "get", name, "-o", "json").DecodeJSON(&app)
	if app.Name != name || app.Description != "e2e round trip" || app.Labels["suite"] != "e2e" || app.ID == "" {
		t.Fatalf("app get -o json returned %+v", app)
	}
	cli.Run("app", "get", name, "-o", "name").Succeeds().StdoutIs(name + "\n")

	// The ID addresses the same resource.
	cli.Run("app", "get", app.ID, "-o", "name").Succeeds().StdoutIs(name + "\n")

	// list: table, -o name one per line, -o json a bare array.
	cli.Run("app", "list").Succeeds().StdoutContains(name)
	cli.Run("app", "list", "-o", "name").Succeeds().StdoutContains(name + "\n")
	cli.Run("app", "list", "--label", "suite=e2e", "-o", "name").Succeeds().StdoutContains(name + "\n")
	var listed []struct {
		Name string `json:"name"`
	}
	cli.Run("app", "list", "-o", "json").DecodeJSON(&listed)
	found := false
	for _, a := range listed {
		found = found || a.Name == name
	}
	if !found {
		t.Fatalf("app list -o json does not include %s: %+v", name, listed)
	}

	cli.Run("app", "update", name, "--description", "updated").Succeeds()
	cli.Run("app", "get", name, "-o", "json").DecodeJSON(&app)
	if app.Description != "updated" {
		t.Fatalf("description after update: %q", app.Description)
	}

	// A second create with the same name is a runtime failure, not usage.
	cli.Run("app", "create", name).Exits(1)

	// delete without --force cannot prompt off a pipe: usage error, and
	// the application is still there.
	cli.Run("app", "delete", name).Exits(exitUsage).StderrContains("--force")
	cli.Run("app", "get", name, "-o", "name").Succeeds().StdoutIs(name + "\n")

	cli.Run("app", "delete", name, "--force").Succeeds().
		StderrContains(fmt.Sprintf("application %q deleted", name))
	cli.Run("app", "get", name).Exits(1).StderrContains("not found")
}

// A stack that answers but rejects the key: exit 4 with the remedy.
func TestApp_RejectedKey(t *testing.T) {
	cli := newServerCLIWithKey(t, wellFormedKey)
	cli.Run("app", "list").Exits(exitAuth).StderrContains("Run 'admiral auth login'")
}
