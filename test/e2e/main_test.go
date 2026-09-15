// Package e2e runs end-to-end tests of the compiled admiral CLI against a
// live admiral-server stack. See docs/e2e-testing.md for design.
//
// Prereqs (v1 is BYO server + BYO token — compose orchestration and
// programmatic seeding come later):
//
//	export ADMIRAL_SERVER=localhost:8080           # host:port of the API
//	export ADMIRAL_API_KEY=<your-pat>                # from the profile page
//	export ADMIRAL_PLAINTEXT=1                     # optional; for local http
//
// Run:
//
//	go test ./test/e2e/...
package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var (
	cliDir          string
	sharedConfigDir string
)

func TestMain(m *testing.M) {
	if os.Getenv("ADMIRAL_SERVER") == "" || os.Getenv("ADMIRAL_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "e2e: set ADMIRAL_SERVER and ADMIRAL_API_KEY to run; skipping")
		os.Exit(0)
	}

	bin, err := buildCLI()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: failed to build admiral CLI:", err)
		os.Exit(1)
	}
	cliDir = bin

	cfg, err := seedConfigDir(filepath.Join(cliDir, "admiral"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: failed to seed config dir:", err)
		os.Exit(1)
	}
	sharedConfigDir = cfg

	code := m.Run()

	_ = os.RemoveAll(cliDir)
	_ = os.RemoveAll(sharedConfigDir)
	os.Exit(code)
}
