package config

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	admiralclient "go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/config"
	"go.admiral.io/cli/internal/output"
)

func newSetCmd(opts *admiralclient.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> [value]",
		Short: "Set a configuration value",
		Long: fmt.Sprintf(
			"Set a configuration value.\n\nValid keys: %v\n\nOmit the value to be prompted interactively. Sensitive keys (e.g. token) are read without echoing.",
			config.ValidKeys,
		),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !config.IsValidKey(key) {
				return fmt.Errorf("unknown config key %q (valid keys: %v)", key, config.ValidKeys)
			}

			var value string
			if len(args) == 2 {
				value = args[1]
			} else {
				v, err := readInput(cmd, config.IsSensitive(key))
				if err != nil {
					return err
				}
				value = v
			}

			if err := config.Set(opts.ConfigDir, key, value); err != nil {
				return err
			}

			output.Writef(cmd.OutOrStdout(), "%s: %s\n", key, config.DisplayValue(key, value))
			return nil
		},
	}
}

func readInput(cmd *cobra.Command, sensitive bool) (string, error) {
	in := cmd.InOrStdin()

	f, ok := in.(interface{ Fd() uintptr })
	isTTY := ok && term.IsTerminal(int(f.Fd()))

	if isTTY {
		fmt.Fprint(cmd.ErrOrStderr(), "Enter value: ")
		if sensitive {
			b, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(cmd.ErrOrStderr()) // newline after hidden input
			if err != nil {
				return "", fmt.Errorf("failed to read input: %w", err)
			}
			return requireNonEmpty(string(b))
		}
		scanner := bufio.NewScanner(in)
		if scanner.Scan() {
			return requireNonEmpty(scanner.Text())
		}
		return "", fmt.Errorf("no input provided")
	}

	// Piped input: read first line.
	scanner := bufio.NewScanner(in)
	if scanner.Scan() {
		return requireNonEmpty(scanner.Text())
	}
	return "", fmt.Errorf("no input provided")
}

func requireNonEmpty(s string) (string, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return "", fmt.Errorf("value cannot be empty")
	}
	return v, nil
}
