package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
)

// State can hold secrets: the file is the owner's alone, and an existing one
// is not overwritten by accident.
func TestADownloadedStateIsTheOwnersAndKeepsAnExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "net.tfstate")
	require.NoError(t, writeState(path, false, strings.NewReader("one")))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	err = writeState(path, false, strings.NewReader("two"))
	assert.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	got, _ := os.ReadFile(path)
	assert.Equal(t, "one", string(got))

	require.NoError(t, writeState(path, true, strings.NewReader("two")))
	got, _ = os.ReadFile(path)
	assert.Equal(t, "two", string(got))
}

func TestSizeIsReadable(t *testing.T) {
	assert.Equal(t, "512 B", size(512))
	assert.Equal(t, "2.0 KiB", size(2048))
	assert.Equal(t, "1.5 MiB", size(3<<19))
}
