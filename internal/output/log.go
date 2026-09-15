package output

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

type plainHandler struct {
	w     io.Writer
	level slog.Level
	attrs []slog.Attr
}

// NewLogHandler returns a slog handler for non-verbose runs: records at or
// above level render as one line in the CLI's own voice, "Warning: <msg>",
// with attributes appended as key=value, and nothing below level is shown.
// The default text handler's timestamps and level tags look foreign next to
// the rest of stderr.
func NewLogHandler(w io.Writer, level slog.Level) slog.Handler {
	return &plainHandler{w: w, level: level}
}

func (h *plainHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *plainHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	switch {
	case r.Level >= slog.LevelError:
		b.WriteString("Error: ")
	case r.Level >= slog.LevelWarn:
		b.WriteString("Warning: ")
	}
	b.WriteString(r.Message)
	write := func(a slog.Attr) {
		if a.Key == "" {
			return
		}
		b.WriteString(" ")
		b.WriteString(a.Key)
		b.WriteString("=")
		b.WriteString(a.Value.String())
	}
	for _, a := range h.attrs {
		write(a)
	}
	r.Attrs(func(a slog.Attr) bool { write(a); return true })
	b.WriteString("\n")
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *plainHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &plainHandler{w: h.w, level: h.level, attrs: append(append([]slog.Attr{}, h.attrs...), attrs...)}
}

func (h *plainHandler) WithGroup(string) slog.Handler { return h }
