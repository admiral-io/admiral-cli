// Package e2e runs the compiled admiral binary the way a user would and
// checks what comes back: exit codes, stdout shapes, stderr messages.
//
// Two tiers share the harness. Offline scenarios need nothing but the
// binary and always run, so `go test ./test/e2e/...` is a real check in
// CI. Server scenarios talk to a live admiral-server and skip unless the
// stack is named (v1 is BYO server + BYO token; compose orchestration and
// programmatic seeding come later):
//
//	export ADMIRAL_SERVER=localhost:8080   # host:port of the API
//	export ADMIRAL_API_KEY=<your-key>      # from the profile page
//	export ADMIRAL_PLAINTEXT=1             # optional; for local http
//
// Run:
//
//	go test ./test/e2e/...
package e2e

import (
	"fmt"
	"os"
	"testing"
)

// binary is the freshly built admiral, so every scenario runs this
// checkout rather than whatever is on the developer's PATH.
var binary string

func TestMain(m *testing.M) {
	dir, err := buildCLI()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: failed to build admiral CLI:", err)
		os.Exit(1)
	}
	binary = dir + "/admiral"

	code := m.Run()

	_ = os.RemoveAll(dir)
	os.Exit(code)
}
