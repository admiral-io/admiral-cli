package run

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

const (
	phasePlan  = "plan"
	phaseApply = "apply"
)

func newLogsCmd(opts *client.Options) *cobra.Command {
	var (
		componentSlug string
		phase         string
	)

	cmd := &cobra.Command{
		Use:     "logs <run-id>",
		Aliases: []string{"log"},
		Short:   "Print the engine transcript(s) for a run",
		Long: `Stream the captured engine transcript (init, plan or apply, hooks output)
for the components in a run.

When --component is omitted, every component's transcript is rendered, in
alphabetical order, with section headers. Pass --component <name> to scope
the output to a single component.

--phase defaults to the most recent phase each component ran (apply if
available, otherwise plan). When --phase is set explicitly, components
that don't have that phase are skipped with a stderr note.`,
		Example: `  # All components for the run, default phase per component
  admiral run logs run-vnx81rv3pp4c

  # Single component
  admiral run logs run-vnx81rv3pp4c --component network

  # Single component, specific phase
  admiral run logs run-vnx81rv3pp4c --component network --phase plan

  # All components, apply phase only
  admiral run logs run-vnx81rv3pp4c --phase apply`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			runResp, err := c.Run().GetRun(cmd.Context(), &runv1.GetRunRequest{RunId: args[0]})
			if err != nil {
				return err
			}
			revResp, err := c.Run().ListRevisions(cmd.Context(), &runv1.ListRevisionsRequest{RunId: args[0]})
			if err != nil {
				return err
			}
			if len(revResp.Revisions) == 0 {
				return fmt.Errorf("run %s has no revisions", RunID(runResp.Run))
			}

			// Single-component path: existing behavior, scoped error messages.
			if componentSlug != "" {
				rev := findRevisionByName(revResp.Revisions, componentSlug)
				if rev == nil {
					names := collectComponentNames(revResp.Revisions)
					return fmt.Errorf("component %q not in run %s; available: %s",
						componentSlug, RunID(runResp.Run), strings.Join(names, ", "))
				}
				phases := phasesAsStrings(rev.AvailablePhases)
				if len(phases) == 0 {
					return fmt.Errorf("component %q has no transcript yet (status: %s)",
						componentSlug, output.FormatEnum(rev.Status))
				}
				selected := phase
				if selected == "" {
					selected = defaultPhase(phases)
				}
				if !slices.Contains(phases, selected) {
					return fmt.Errorf("component %q has no %s transcript (available: %s)",
						componentSlug, selected, strings.Join(phases, ", "))
				}
				return streamPhase(cmd, opts, runResp.Run.Id, rev.Id, selected, "")
			}

			// All-components path: alphabetical, one section per component, skip
			// components with no matching transcript and surface a stderr note so
			// the operator knows what was filtered out.
			revs := append([]*runv1.Revision(nil), revResp.Revisions...)
			sort.Slice(revs, func(i, j int) bool { return revs[i].ComponentName < revs[j].ComponentName })

			var rendered int
			for _, rev := range revs {
				phases := phasesAsStrings(rev.AvailablePhases)
				if len(phases) == 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "skipped %s: no transcript yet (status: %s)\n",
						rev.ComponentName, output.FormatEnum(rev.Status))
					continue
				}
				selected := phase
				if selected == "" {
					selected = defaultPhase(phases)
				}
				if !slices.Contains(phases, selected) {
					fmt.Fprintf(cmd.ErrOrStderr(), "skipped %s: no %s transcript (available: %s)\n",
						rev.ComponentName, selected, strings.Join(phases, ", "))
					continue
				}
				if rendered > 0 {
					fmt.Fprintln(cmd.OutOrStdout())
				}
				if err := streamPhase(cmd, opts, runResp.Run.Id, rev.Id, selected, rev.ComponentName); err != nil {
					return err
				}
				rendered++
			}
			if rendered == 0 {
				return fmt.Errorf("run %s has no transcripts available", RunID(runResp.Run))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&componentSlug, "component", "", "component name to scope to; omit for all components")
	flags.Enum(cmd, &phase, "phase", "", "phase to fetch (default: most recent per component)", phasePlan, phaseApply)
	return cmd
}

// streamPhase fetches one (revision, phase) transcript and writes it to the
// command's stdout. If headerSlug is non-empty, prints a section header
// first; that path is used by the all-components view to delineate
// per-component sections.
func streamPhase(cmd *cobra.Command, opts *client.Options, runID, revisionID, phase, headerSlug string) error {
	url := phaseOutputURL(opts, runID, revisionID, phase)
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	addAuth(req, opts)

	resp, err := httpClient(opts).Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s transcript: %w", phase, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("transcript fetch failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	if headerSlug != "" {
		writeSectionHeader(cmd.OutOrStdout(), headerSlug, phase)
	}
	if _, err := io.Copy(cmd.OutOrStdout(), resp.Body); err != nil {
		return fmt.Errorf("stream transcript: %w", err)
	}
	return nil
}

// writeSectionHeader prints a 3-line bar that delineates per-component
// transcripts in the all-components view. 80 columns wide; matches typical
// terminal width and stands out clearly when scrolling through a long run.
func writeSectionHeader(w io.Writer, name, phase string) {
	const bar = "================================================================================"
	fmt.Fprintln(w, bar)
	fmt.Fprintf(w, "  %s · %s\n", name, phase)
	fmt.Fprintln(w, bar)
}

func findRevisionByName(revs []*runv1.Revision, name string) *runv1.Revision {
	for _, rev := range revs {
		if rev.ComponentName == name {
			return rev
		}
	}
	return nil
}

func collectComponentNames(revs []*runv1.Revision) []string {
	out := make([]string, 0, len(revs))
	for _, rev := range revs {
		out = append(out, rev.ComponentName)
	}
	return out
}

// defaultPhase prefers apply when both phases exist (operators usually want
// the most-recent transcript), otherwise the only available phase. Caller
// must guarantee available is non-empty.
func defaultPhase(available []string) string {
	if slices.Contains(available, phaseApply) {
		return phaseApply
	}
	return available[0]
}
