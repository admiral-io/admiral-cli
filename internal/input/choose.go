package input

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/iostreams"
)

// Option is one thing Choose offers: its name, and a short detail shown
// beside it.
type Option struct {
	Name   string
	Detail string
}

// listed is how many options a non-interactive refusal names; shown is how
// many a prompt numbers before saying how many more there are.
const (
	listed = 20
	shown  = 50
)

// Choose lists options on stderr, numbered, and reads one back from stdin
// by name or by number. A name wins, since a tag can be `17`; a number is
// a position only when no option is named by it. Nothing is pre-selected:
// an empty answer cancels, so a choice is always one the person made.
// When the session is not interactive it fails with a usage error naming
// flag and listing the options, so a script learns what it could have
// passed.
//
// what is the noun in the prompt, "version"; flag is how it is passed
// without a prompt, "--version".
func Choose(cmd *cobra.Command, flag, what string, options []Option) (Option, error) {
	if len(options) == 0 {
		return Option{}, cmderr.Usage("no %s to choose from", what)
	}
	ios := iostreams.FromCommand(cmd)
	if !ios.Interactive() {
		return Option{}, cmderr.UsageHint(
			fmt.Sprintf("Pass %s as one of: %s", flag, names(options, listed)),
			"%s required when not running interactively", flag)
	}
	numbered := min(len(options), shown)
	width := len(strconv.Itoa(numbered))
	for i, o := range options[:numbered] {
		if o.Detail != "" {
			fmt.Fprintf(ios.Err, "  %*d. %s  (%s)\n", width, i+1, o.Name, o.Detail)
		} else {
			fmt.Fprintf(ios.Err, "  %*d. %s\n", width, i+1, o.Name)
		}
	}
	if rest := len(options) - numbered; rest > 0 {
		fmt.Fprintf(ios.Err, "  and %d more, by name\n", rest)
	}
	fmt.Fprintf(ios.Err, "Choose the %s [number or name]: ", what)
	reply, err := readLine(ios)
	if err != nil {
		return Option{}, fmt.Errorf("read %s: %w", what, err)
	}
	if reply == "" {
		fmt.Fprintln(ios.Err, "canceled")
		return Option{}, ErrCanceled
	}
	for _, o := range options {
		if o.Name == reply {
			return o, nil
		}
	}
	if n, err := strconv.Atoi(reply); err == nil {
		if n < 1 || n > numbered {
			return Option{}, cmderr.Usage("%d is not between 1 and %d", n, numbered)
		}
		return options[n-1], nil
	}
	return Option{}, cmderr.Usage("%q is not one of the %ss listed", reply, what)
}

// names is the first n options, comma-separated, with a count of the rest.
func names(options []Option, n int) string {
	var b strings.Builder
	for i, o := range options {
		if i == n {
			fmt.Fprintf(&b, ", and %d more", len(options)-n)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(o.Name)
	}
	return b.String()
}
