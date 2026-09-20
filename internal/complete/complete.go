// Package complete provides shell-completion functions that offer server
// resource names, so `admiral app get bil<TAB>` fills in `billing-api` the
// way kubectl completes deployment names. Every positional or flag that
// resolves a name (see package resolve) registers one of these.
//
// Completion runs while the user is typing, so a function here must never
// prompt, block for long, or write to stdout: on any failure it returns no
// candidates and ShellCompDirectiveError, which the shell shows as nothing.
package complete

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

// Func is cobra's completion signature, shared by ValidArgsFunction and
// RegisterFlagCompletionFunc.
type Func = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)

// timeout bounds the whole completion round trip. A Tab that hangs is
// worse than one that offers nothing.
const timeout = 2 * time.Second

// newClient is swapped in tests.
var newClient = client.CreateClient

// pageSize is the server's maximum; maxCandidates caps how many pages are
// fetched so a huge collection cannot stall the shell.
const (
	pageSize      = 100
	maxCandidates = 500
)

// Candidate is one completion entry. Description, when set, is shown next
// to the name by shells that support it (zsh, fish, powershell).
type Candidate struct {
	Name        string
	Description string
}

// Flag registers f as the completion for the named flag on cmd. The flag
// must already be defined; a missing flag is a programming error.
func Flag(cmd *cobra.Command, name string, f Func) {
	if err := cmd.RegisterFlagCompletionFunc(name, f); err != nil {
		panic(err)
	}
}

// First restricts f to the first positional: once a name is present there
// is nothing more to offer, and file completion must not kick in.
func First(f Func) Func {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return f(cmd, args, toComplete)
	}
}

// Apps offers application names.
func Apps(opts *client.Options) Func {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return withClient(cmd, opts, toComplete, func(ctx context.Context, c sdkclient.AdmiralClient) ([]Candidate, error) {
			var out []Candidate
			err := paged(func(token string) (string, error) {
				resp, err := c.Application().ListApplications(ctx, &applicationv1.ListApplicationsRequest{
					PageSize:  pageSize,
					PageToken: token,
				})
				if err != nil {
					return "", err
				}
				for _, a := range resp.Applications {
					out = append(out, Candidate{Name: a.Name, Description: a.Description})
				}
				return resp.NextPageToken, nil
			}, &out)
			return out, err
		})
	}
}

// Envs offers environment names for a positional or --env, in both forms
// of style guide §1.2. A bare prefix completes inside the --app already on
// the line. With no --app it offers application prefixes ("shop/", no
// trailing space) so the path can be typed segment by segment, and a
// prefix that already contains "/" completes the environment inside that
// application as "shop/prod".
func Envs(opts *client.Options) Func {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		app, _, isPath := strings.Cut(toComplete, "/")
		if !isPath {
			app = flagValue(cmd, "app")
			if app == "" {
				return appPrefixes(cmd, opts, toComplete)
			}
		}
		// withClient filters on toComplete, which in the path form is
		// "shop/pr" against candidates renamed to "shop/prod".
		return withClient(cmd, opts, toComplete, func(ctx context.Context, c sdkclient.AdmiralClient) ([]Candidate, error) {
			items, err := envsIn(ctx, c, app)
			if err != nil {
				return nil, err
			}
			if isPath {
				for i := range items {
					items[i].Name = app + "/" + items[i].Name
				}
			}
			return items, nil
		})
	}
}

// NewEnv offers the application prefix of a new environment's path
// ("shop/") and nothing after the slash, since the name does not exist
// yet. With --app on the line there is nothing to offer.
func NewEnv(opts *client.Options) Func {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if strings.Contains(toComplete, "/") || flagValue(cmd, "app") != "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return appPrefixes(cmd, opts, toComplete)
	}
}

// appPrefixes offers application names with a trailing slash and tells
// the shell not to add a space, so the next Tab completes the environment.
func appPrefixes(cmd *cobra.Command, opts *client.Options, toComplete string) ([]string, cobra.ShellCompDirective) {
	out, directive := Apps(opts)(cmd, nil, toComplete)
	for i, s := range out {
		name, desc, _ := strings.Cut(s, "\t")
		out[i] = name + "/"
		if desc != "" {
			out[i] += "\t" + desc
		}
	}
	if directive == cobra.ShellCompDirectiveNoFileComp {
		directive |= cobra.ShellCompDirectiveNoSpace
	}
	return out, directive
}

// envsIn lists the environments of app (a name or ID).
func envsIn(ctx context.Context, c sdkclient.AdmiralClient, app string) ([]Candidate, error) {
	appID, err := resolve.App(ctx, c.Application(), app)
	if err != nil {
		return nil, err
	}
	byApp, err := filter.Eq("application_id", appID)
	if err != nil {
		return nil, err
	}
	var out []Candidate
	err = paged(func(token string) (string, error) {
		resp, err := c.Environment().ListEnvironments(ctx, &environmentv1.ListEnvironmentsRequest{
			Filter:    byApp,
			PageSize:  pageSize,
			PageToken: token,
		})
		if err != nil {
			return "", err
		}
		for _, e := range resp.Environments {
			out = append(out, Candidate{Name: e.Name, Description: e.Description})
		}
		return resp.NextPageToken, nil
	}, &out)
	return out, err
}

// flagValue returns the named flag's value on cmd, or "" when the flag is
// not defined. During completion cobra has already parsed the line, so
// this is the --app the user typed before pressing Tab.
func flagValue(cmd *cobra.Command, name string) string {
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return ""
	}
	return f.Value.String()
}

// withClient runs list with a bounded context and a fresh client, then
// filters the result by prefix. Any failure, including not being logged
// in, yields no candidates rather than an error the user can see.
func withClient(cmd *cobra.Command, opts *client.Options, toComplete string,
	list func(ctx context.Context, c sdkclient.AdmiralClient) ([]Candidate, error),
) ([]string, cobra.ShellCompDirective) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The shell owns the terminal right now; nothing below may prompt.
	_ = os.Setenv("ADMIRAL_NO_INPUT", "1")

	c, err := newClient(ctx, opts)
	if err != nil {
		slog.Debug("completion: no client", "error", err)
		return nil, cobra.ShellCompDirectiveError
	}
	defer c.Close() //nolint:errcheck // best-effort cleanup

	items, err := list(ctx, c)
	if err != nil {
		slog.Debug("completion: list failed", "error", err)
		return nil, cobra.ShellCompDirectiveError
	}
	return Filter(items, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// paged calls next with successive page tokens until the server returns no
// more or out holds maxCandidates.
func paged(next func(token string) (string, error), out *[]Candidate) error {
	token := ""
	for {
		var err error
		token, err = next(token)
		if err != nil {
			return err
		}
		if token == "" || len(*out) >= maxCandidates {
			return nil
		}
	}
}

// Filter keeps the candidates whose name starts with prefix and renders
// them in cobra's "name\tdescription" form.
func Filter(items []Candidate, prefix string) []string {
	var out []string
	for _, it := range items {
		if !strings.HasPrefix(it.Name, prefix) {
			continue
		}
		if it.Description != "" {
			out = append(out, it.Name+"\t"+it.Description)
		} else {
			out = append(out, it.Name)
		}
	}
	return out
}
