package complete

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

// Positions completes each positional with the function at its index, and
// offers nothing past the last one or where the function is nil.
func Positions(fs ...Func) Func {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) >= len(fs) || fs[len(args)] == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return fs[len(args)](cmd, args, toComplete)
	}
}

// ThenEach completes the first positional with first and every later one
// with rest, for a command such as `changeset set <cs> <path>=<value>...`.
func ThenEach(first, rest Func) Func {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return first(cmd, args, toComplete)
		}
		return rest(cmd, args, toComplete)
	}
}

// ComponentsOf offers the components of the changeset or environment named by the
// first positional, an app/env or a cs- ID: those live in its environment
// and those its head revision adds. With dotted set, a component is offered
// as "api." with no trailing space, so the path is typed next; once a dot
// is typed nothing more is offered, since value paths are the chart's.
func ComponentsOf(opts *client.Options, dotted bool) Func {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 || (dotted && strings.Contains(toComplete, ".")) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		out, directive := withClient(cmd, opts, toComplete, func(ctx context.Context, c sdkclient.AdmiralClient) ([]Candidate, error) {
			return changeSetComponents(ctx, c, args[0])
		})
		if dotted {
			return withSuffix(out, "."), noSpace(directive)
		}
		return out, directive
	}
}

func changeSetComponents(ctx context.Context, c sdkclient.AdmiralClient, ref string) ([]Candidate, error) {
	var envID string
	var added []string
	if app, env, isPath := strings.Cut(ref, "/"); isPath {
		id, err := resolve.Environment(ctx, c.Environment(), c.Application(), app, env)
		if err != nil {
			return nil, err
		}
		envID = id
	} else {
		resp, err := c.ChangeSet().GetChangeSet(ctx, &changesetv1.GetChangeSetRequest{ChangeSetId: ref})
		if err != nil {
			return nil, err
		}
		envID = resp.GetChangeSet().GetEnvironmentId()
		for _, e := range resp.GetHead().GetEntries() {
			added = append(added, e.GetComponent())
		}
	}
	resp, err := c.Environment().ListEnvironmentComponents(ctx, &environmentv1.ListEnvironmentComponentsRequest{
		EnvironmentId: &envID,
		PageSize:      pageSize,
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []Candidate
	for _, comp := range resp.GetComponents() {
		if !seen[comp.GetName()] {
			seen[comp.GetName()] = true
			out = append(out, Candidate{Name: comp.GetName()})
		}
	}
	for _, name := range added {
		if !seen[name] {
			seen[name] = true
			out = append(out, Candidate{Name: name})
		}
	}
	return out, nil
}

// Scopes offers what an agent may be granted to: "org", or "app:", "env:"
// and "group:" with no trailing space, then application names after
// "app:" and app/env paths after "env:", completed segment by segment as
// Envs does.
func Scopes(opts *client.Options) Func {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		kind, rest, hasKind := strings.Cut(toComplete, ":")
		if !hasKind {
			var out []string
			for _, s := range []string{"org", "app:", "env:", "group:"} {
				if strings.HasPrefix(s, toComplete) {
					out = append(out, s)
				}
			}
			if len(out) == 1 && out[0] == "org" {
				return out, cobra.ShellCompDirectiveNoFileComp
			}
			return out, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
		}
		var out []string
		var directive cobra.ShellCompDirective
		switch kind {
		case "app":
			out, directive = Apps(opts)(cmd, args, rest)
		case "env":
			out, directive = Envs(opts)(cmd, args, rest)
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		for i, s := range out {
			out[i] = kind + ":" + s
		}
		return out, directive
	}
}
