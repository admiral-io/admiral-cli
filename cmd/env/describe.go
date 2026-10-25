package env

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runcmd "go.admiral.io/cli/cmd/run"
	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

const recentRunsLimit = 5

func newDescribeCmd(opts *client.Options) *cobra.Command {
	var appName string

	cmd := &cobra.Command{
		Use:   "describe <name>",
		Short: "Show everything about an environment",
		Long: `Render the operator view of an environment: identity, the agents it
deploys through, what an apply needs, any hold, every component with its
version and last apply, open changesets and recent runs. A section the
active credential cannot read is shown as unavailable.

describe is a human view. Use 'env get -o json' for the raw record.`,
		Example: `  # By path
  admiral env describe shop/prod

  # By name, scoped with --app
  admiral env describe prod --app shop

  # By ID
  admiral env describe env-3k9x2m7qpl0a`,
		Aliases:           []string{"desc"},
		Args:              flags.ExactArgs(1),
		ValidArgsFunction: complete.First(complete.Envs(opts)),
		RunE: func(cmd *cobra.Command, args []string) error {
			appScope, name, err := flags.EnvTarget(cmd, appName, args[0])
			if err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			ctx := cmd.Context()

			envID, err := resolve.Environment(ctx, c.Environment(), c.Application(), appScope, name)
			if err != nil {
				return err
			}

			envResp, err := c.Environment().GetEnvironment(ctx, &environmentv1.GetEnvironmentRequest{
				EnvironmentId: envID,
			})
			if err != nil {
				return err
			}
			e := envResp.Environment

			appResp, err := c.Application().GetApplication(ctx, &applicationv1.GetApplicationRequest{ApplicationId: e.ApplicationId})
			if err != nil {
				return fmt.Errorf("fetching application: %w", err)
			}
			app := appResp.Application

			// The environment and its application are the view; the
			// sections below are assembled from further reads. A section
			// whose read fails (missing scope, endpoint not served) is
			// rendered as unavailable rather than failing the describe.
			// The reads are independent. Every section keeps its own
			// error, so nothing here returns one and the group is only a way
			// to wait.
			var sec envSections
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				compResp, err := c.Environment().ListEnvironmentComponents(gctx, &environmentv1.ListEnvironmentComponentsRequest{
					EnvironmentId: &envID,
					PageSize:      100,
				})
				if err != nil {
					sec.compsErr = err
				} else {
					sec.comps = compResp.Components
				}
				return nil
			})
			g.Go(func() error {
				sec.runs, sec.moreRuns, sec.runsErr = listRecentRuns(gctx, c, envID)
				ids := make([]string, 0, len(sec.runs))
				for _, r := range sec.runs {
					ids = append(ids, r.ChangeSetId)
				}
				sec.runTitles = runcmd.ChangeSetTitles(gctx, c.ChangeSet(), ids...)
				return nil
			})
			g.Go(func() error {
				sec.openCS, sec.openCSErr = listOpenChangeSets(gctx, c, envID)
				return nil
			})
			g.Go(func() error {
				sec.agentNames = agentNames(gctx, c, e.Agents)
				return nil
			})
			if e.LastApply != nil {
				g.Go(func() error {
					sec.lastApplyRun = latestRun(gctx, c, e.LastApply.ChangeSetId)
					return nil
				})
			}
			if e.Hold != nil {
				g.Go(func() error {
					sec.holdRun = latestRun(gctx, c, e.Hold.ChangeSetId)
					return nil
				})
			}
			_ = g.Wait() // nothing returns an error
			// Live components whose reference the platform did not record are
			// named by the registry's tags instead.
			refs := make([]*changesetv1.VersionRef, 0, len(sec.comps))
			for _, comp := range sec.comps {
				refs = append(refs, comp.GetVersion())
			}
			sec.tags = resolve.VersionTags(ctx, c.Registry(), refs)

			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintDescribe(describeEnv(e, app, sec),
				fmt.Sprintf("admiral env get %s/%s", app.Name, e.Name))
		},
	}

	flags.App(cmd, &appName, opts)
	return cmd
}

// envSections holds the reads describe assembles around the environment
// record. Each read carries its own error so a failed section renders as
// unavailable without hiding the others.
type envSections struct {
	comps     []*environmentv1.EnvironmentComponent
	compsErr  error
	runs      []*runv1.Run
	runsErr   error
	runTitles map[string]string
	// moreRuns is whether runs beyond recentRunsLimit exist.
	moreRuns   bool
	openCS     []*changesetv1.ChangeSet
	openCSErr  error
	agentNames map[string]string
	// The runs behind the last apply and the hold, nil when unreadable.
	lastApplyRun *runv1.Run
	holdRun      *runv1.Run
	// tags is each version's tags by version ID, for components whose
	// reference is not a tag.
	tags map[string][]string
}

// permissionDenied reports whether any section read was refused for scope.
func (s envSections) permissionDenied() bool {
	for _, err := range []error{s.compsErr, s.runsErr, s.openCSErr} {
		if status.Code(err) == codes.PermissionDenied {
			return true
		}
	}
	return false
}

// describeEnv assembles the operator view from the pieces describe fetched.
func describeEnv(e *environmentv1.Environment, app *applicationv1.Application, sec envSections) *output.Describe {
	comps, openCS, runs := sec.comps, sec.openCS, sec.runs
	d := output.NewDescribe()
	d.Field("Name", e.Name)
	d.Field("Application", app.Name)
	d.Field("ID", e.Id)
	d.Field("Description", e.Description)
	d.Fields("Labels", output.LabelLines(e.Labels))
	d.Field("Generation", strconv.FormatInt(e.Generation, 10))
	d.Field("Hold", holdLine(e.Hold, sec.holdRun))
	d.Field("Last Apply", lastApplyLine(e.LastApply, sec.lastApplyRun))
	if r := applying(runs); r != nil {
		d.Field("Applying", fmt.Sprintf("%s (%s) since %s ago", r.Id, r.ChangeSetId, output.FormatAge(r.GetApply().GetStartedAt())))
	}
	switch {
	case sec.runsErr != nil:
		d.Field("Last Run", unavailable)
	case len(runs) > 0:
		r := runs[0]
		d.Field("Last Run", fmt.Sprintf("%s %s %s ago", r.Id, output.FormatEnum(r.Status), output.FormatAge(r.CreatedAt)))
	default:
		d.Field("Last Run", "")
	}
	d.Field("Created", output.FormatDescribeTime(e.CreatedAt))
	d.Field("Created By", output.FormatActorWithID(e.CreatedBy))

	d.Section("Agents", func(b *output.Block) {
		if len(e.Agents) == 0 {
			b.Field("Count", "0")
			return
		}
		rows := make([][]string, 0, len(e.Agents))
		for _, a := range e.Agents {
			name := sec.agentNames[a.AgentId]
			if name == "" {
				name = a.AgentId
			}
			k := a.GetKubernetes()
			rows = append(rows, []string{name, a.Id,
				k.GetDefaultNamespace(), optionalBool(k.CreateNamespaces), optionalBool(k.Prune)})
		}
		b.Table([]string{"Agent", "Assignment", "Namespace", "Create Namespaces", "Prune"}, rows)
	})

	d.Section("Terraform", func(b *output.Block) {
		b.Field("Engine", engineName(e.GetInfraEngine()))
		b.Field("Backend", backendName(e.GetInfraBackend()))
	})

	d.Section("Approval", func(b *output.Block) {
		b.Field("Required Approvals", approvals(e))
		groups := make([]string, 0, len(e.GetApproval().GetGroups()))
		for _, g := range e.GetApproval().GetGroups() {
			groups = append(groups, fmt.Sprintf("%s: %d", g.GroupId, g.Approvals))
		}
		b.Fields("Groups", groups)
	})

	if sec.compsErr != nil {
		d.Unavailable("Components", cmderr.Format(sec.compsErr))
	} else {
		d.Section("Components", func(b *output.Block) {
			if len(comps) == 0 {
				b.Field("Count", "0")
				return
			}
			rows := make([][]string, 0, len(comps))
			for _, c := range comps {
				applied := ""
				if c.LastApply != nil {
					applied = output.FormatAge(c.LastApply.AppliedAt) + " ago"
				}
				rows = append(rows, []string{c.Name, output.FormatEnumKebab(c.Kind), resolve.VersionLabel(c.Version, sec.tags), applied})
			}
			b.Table([]string{"Name", "Kind", "Version", "Applied"}, rows)
		})
	}

	if sec.openCSErr != nil {
		d.Unavailable("Open Changesets", cmderr.Format(sec.openCSErr))
	} else {
		d.Section("Open Changesets", func(b *output.Block) {
			if len(openCS) == 0 {
				b.Field("Count", "0")
				return
			}
			rows := make([][]string, 0, len(openCS))
			for _, cs := range openCS {
				rows = append(rows, []string{cs.Id, cs.Title, strconv.Itoa(int(cs.HeadRevision)),
					output.FormatActor(cs.CreatedBy), output.FormatAge(cs.CreatedAt)})
			}
			b.Table([]string{"ID", "Title", "Revision", "Created By", "Age"}, rows)
		})
	}

	if sec.runsErr != nil {
		d.Unavailable("Recent Runs", cmderr.Format(sec.runsErr))
	} else {
		rows := make([][]string, 0, len(runs))
		for _, r := range runs {
			rows = append(rows, []string{r.Id, output.FormatEnum(r.Status), r.ChangeSetId, sec.runTitles[r.ChangeSetId],
				output.FormatActor(r.CreatedBy), output.FormatAge(r.CreatedAt)})
		}
		d.Table("Recent Runs", []string{"ID", "Status", "Changeset", "Title", "Created By", "Age"}, rows)
		if sec.moreRuns {
			d.Hint(fmt.Sprintf("Showing the %d most recent runs. To list them all, run: admiral run list %s/%s",
				recentRunsLimit, app.Name, e.Name))
		}
	}

	if e.Hold != nil {
		d.Hint(runcmd.HoldHint(e.Hold.ChangeSetId, sec.holdRun.GetId()))
	}
	if sec.permissionDenied() {
		d.Hint(cmderr.ScopeHint)
	}
	return d
}

// unavailable marks a top-block field whose backing read failed.
const unavailable = "<unavailable>"

func holdLine(h *environmentv1.Hold, r *runv1.Run) string {
	if h == nil {
		return ""
	}
	by := h.ChangeSetId
	if r != nil {
		by += " (" + r.Id + ")"
	}
	return fmt.Sprintf("held by %s since %s ago", by, output.FormatAge(h.HeldAt))
}

func lastApplyLine(l *environmentv1.LastApply, r *runv1.Run) string {
	if l == nil {
		return ""
	}
	line := fmt.Sprintf("%s %s %s ago", l.ChangeSetId, l.RevisionId, output.FormatAge(l.AppliedAt))
	if by := r.GetApply().GetAppliedBy(); by != nil {
		line += " by " + output.FormatActorWithID(by)
	}
	return line
}

// applying is the run applying now, if any.
func applying(runs []*runv1.Run) *runv1.Run {
	for _, r := range runs {
		if r.Status == runv1.Run_STATUS_APPLYING {
			return r
		}
	}
	return nil
}

// latestRun is a changeset's newest run, or nil when it has none or cannot
// be read.
func latestRun(ctx context.Context, c sdkclient.AdmiralClient, changeSetID string) *runv1.Run {
	cs, err := c.ChangeSet().GetChangeSet(ctx, &changesetv1.GetChangeSetRequest{ChangeSetId: changeSetID})
	if err != nil || cs.GetChangeSet().GetLatestRunId() == "" {
		return nil
	}
	r, err := c.Run().GetRun(ctx, &runv1.GetRunRequest{RunId: cs.GetChangeSet().GetLatestRunId()})
	if err != nil {
		return nil
	}
	return r.GetRun()
}

func optionalBool(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

// agentNames reads each assigned agent for its name. An agent the caller
// cannot read is shown by id.
func agentNames(ctx context.Context, c sdkclient.AdmiralClient, assigned []*environmentv1.AgentAssignment) map[string]string {
	names := make(map[string]string, len(assigned))
	for _, a := range assigned {
		resp, err := c.Agent().GetAgent(ctx, &agentv1.GetAgentRequest{AgentId: a.AgentId})
		if err == nil {
			names[a.AgentId] = resp.Agent.GetName()
		}
	}
	return names
}

// listRecentRuns is the environment's newest runs, and whether older ones
// exist.
func listRecentRuns(ctx context.Context, c sdkclient.AdmiralClient, envID string) ([]*runv1.Run, bool, error) {
	f, err := filter.Eq("environment_id", envID)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.Run().ListRuns(ctx, &runv1.ListRunsRequest{
		Filter:   f,
		PageSize: recentRunsLimit,
	})
	if err != nil {
		return nil, false, err
	}
	return resp.Runs, resp.NextPageToken != "", nil
}

func listOpenChangeSets(ctx context.Context, c sdkclient.AdmiralClient, envID string) ([]*changesetv1.ChangeSet, error) {
	f, err := filter.Eq("status", changesetv1.ChangeSet_STATUS_OPEN.String())
	if err != nil {
		return nil, err
	}
	resp, err := c.ChangeSet().ListChangeSets(ctx, &changesetv1.ListChangeSetsRequest{
		EnvironmentId: envID,
		Filter:        f,
		PageSize:      50,
	})
	if err != nil {
		return nil, err
	}
	return resp.ChangeSets, nil
}
