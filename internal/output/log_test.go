package output

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogHandler(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewLogHandler(&buf, slog.LevelWarn))

	log.Debug("hidden")
	log.Info("hidden too")
	log.Warn("credentials file has insecure permissions", "path", "/x/credentials.json", "mode", "0644")
	log.Error("boom", "code", 7)
	log.With("scope", "s").Warn("grouped")

	require.Equal(t,
		"Warning: credentials file has insecure permissions path=/x/credentials.json mode=0644\n"+
			"Error: boom code=7\n"+
			"Warning: grouped scope=s\n",
		buf.String())
}
