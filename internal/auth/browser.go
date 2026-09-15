package auth

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// openBrowser launches the user's browser at url. It honors the BROWSER
// environment variable, then falls back to the platform opener. It does not
// wait for the browser to exit: openers on macOS and Windows return at once,
// and xdg-open may block for the lifetime of a newly started browser.
func openBrowser(url string) error {
	if b := os.Getenv("BROWSER"); b != "" {
		// BROWSER may carry arguments, e.g. "firefox --private-window".
		parts := strings.Fields(b)
		return start(parts[0], append(parts[1:], url)...)
	}

	switch runtime.GOOS {
	case "darwin":
		return start("open", url)
	case "windows":
		return start("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return errors.New("no browser opener found (set BROWSER)")
		}
		return start("xdg-open", url)
	}
}

func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Start()
}
