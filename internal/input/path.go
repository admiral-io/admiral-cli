package input

import (
	"fmt"
	"os"
	"strings"
)

// ExpandPath expands a leading ~ or ~/ to the user's home directory. Quoted
// shell arguments don't get tilde expansion from the shell, so the CLI does it
// itself for any flag that accepts a filesystem path. ~user/ is not supported.
func ExpandPath(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return home + path[1:], nil
}
