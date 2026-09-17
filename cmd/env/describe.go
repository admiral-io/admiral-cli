package env

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/filter"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	sdkclient "go.admiral.io/sdk/client"
	applicationv1 "go.admiral.io/sdk/proto/admiral/api/application/v1"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
	variablev1 "go.admiral.io/sdk/proto/admiral/api/variable/v1"
)

const recentRunsLimit = 5

func newDescribeCmd(opts *client.Options) *cobra.Command {
	var appName string

	cmd := &cobra.Command{
		Use:   "describe <name>",
		Short: "Show everything about an environment",
		Long: `Render the operator view of an environment: identity, health, every
component with its module and last run, open change sets, variables and
recent runs. A section the active credential cannot read is shown as
unavailable.

Environment health is the worst of its components, in the order Healthy,
Progressing, Degraded, Unknown.

describe is a human view. Use 'env get -o json' for the raw record.`,
		Example: `  # By path
  admiral env describe shop/prod

  # By name, scoped with --app
  admiral env describe prod --app shop

  # By ID
  admiral env describe <uuid>`,
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
			var sec envSections
			compResp, err := c.Environment().ListEnvironmentComponents(ctx, &environmentv1.ListEnvironmentComponentsRequest{
				EnvironmentId: envID,
			})
			if err != nil {
				sec.compsErr = err
			} else {
				sec.comps = compResp.Components
			}
			sec.runs, sec.runsErr = listRecentRuns(ctx, c, e.ApplicationId, envID)
			sec.openCS, sec.openCSErr = listOpenChangeSets(ctx, c, e.ApplicationId, envID)
			sec.vars, sec.varsErr = listAllVariables(ctx, c, envID)
			if sec.openCSErr == nil {
				sec.diffs = loadPendingDiffs(ctx, c, sec.openCS)
			}

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
	openCS    []*changesetv1.ChangeSet
	openCSErr error
	diffs     map[string]*changesetv1.ChangeSetDiff
	vars      []*variablev1.Variable
	varsErr   error
}

// permissionDenied reports whether any section read was refused for scope.
func (s envSections) permissionDenied() bool {
	for _, err := range []error{s.compsErr, s.runsErr, s.openCSErr, s.varsErr} {
		if status.Code(err) == codes.PermissionDenied {
			return true
		}
	}
	return false
}

// describeEnv assembles the §2.5 operator view from the pieces describe
// fetched. Sections the server does not yet report (agent presence,
// conditions, per-component health messages, metrics) are left out rather
// than faked; see SERVER_FOLLOWUPS.md.
func describeEnv(e *environmentv1.Environment, app *applicationv1.Application, sec envSections) *output.Describe {
	comps, openCS, diffs, runs, vars := sec.comps, sec.openCS, sec.diffs, sec.runs, sec.vars
	d := output.NewDescribe()
	d.Field("Name", e.Name)
	d.Field("Application", app.Name)
	d.Field("ID", e.Id)
	d.Field("Description", e.Description)
	d.Fields("Labels", output.LabelLines(e.Labels))
	if sec.compsErr != nil {
		d.Field("Health", unavailable)
	} else {
		d.Field("Health", envHealth(comps))
	}
	d.Field("Pending Changes", strconv.FormatBool(e.HasPendingChanges))
	switch {
	case sec.runsErr != nil:
		d.Field("Last Run", unavailable)
	case len(runs) > 0:
		r := runs[0]
		d.Field("Last Run", fmt.Sprintf("%s %s %s ago", runDisplay(r), output.FormatEnum(r.Status), output.FormatAge(r.CreatedAt)))
	default:
		d.Field("Last Run", "")
	}
	d.Field("Created", output.FormatDescribeTime(e.CreatedAt))
	d.Field("Created By", output.FormatActor(e.CreatedBy))

	if sec.compsErr != nil {
		d.Unavailable("Components", cmderr.Format(sec.compsErr))
	} else {
		d.Section("Components", func(b *output.Block) {
			if len(comps) == 0 {
				b.Field("Count", "0")
				return
			}
			for _, c := range comps {
				b.Block(c.Name, func(b *output.Block) {
					b.Field("Kind", output.FormatEnumKebab(c.CatalogItemType))
					b.Field("Ref", c.Ref)
					st := output.FormatEnum(c.LastRevisionStatus)
					if c.LastDeployedAt != nil {
						st = fmt.Sprintf("%s (%s ago)", st, output.FormatAge(c.LastDeployedAt))
					}
					b.Field("Status", st)
					b.Field("Health", componentHealth(c))
					b.Field("Last Revision", c.LastRevisionId)
				})
			}
		})
	}

	if sec.varsErr != nil {
		d.Unavailable("Variables", cmderr.Format(sec.varsErr))
	} else {
		d.Section("Variables", func(b *output.Block) {
			if len(vars) == 0 {
				b.Field("Count", "0")
				return
			}
			rows := make([][]string, 0, len(vars))
			for _, v := range vars {
				value := v.Value
				if v.Sensitive {
					value = "<redacted>"
				}
				rows = append(rows, []string{v.Key, output.FormatEnumKebab(v.Type), output.FormatEnumKebab(v.Source), strconv.FormatBool(v.Sensitive), output.Truncate(value, 60)})
			}
			b.Table([]string{"Key", "Type", "Source", "Sensitive", "Value"}, rows)
		})
	}

	if sec.openCSErr != nil {
		d.Unavailable("Open Change Sets", cmderr.Format(sec.openCSErr))
	} else {
		d.Section("Open Change Sets", func(b *output.Block) {
			if len(openCS) == 0 {
				b.Field("Count", "0")
				return
			}
			for _, cs := range openCS {
				id := cs.DisplayId
				if id == "" {
					id = truncateUUID(cs.Id)
				}
				b.Block(id, func(b *output.Block) {
					b.Field("Title", cs.Title)
					b.Field("Created By", output.FormatActor(cs.CreatedBy))
					b.Field("Age", output.FormatAge(cs.CreatedAt))
					df := diffs[cs.Id]
					if df == nil {
						return
					}
					if entries := df.GetEntries(); len(entries) > 0 {
						rows := make([][]string, 0, len(entries))
						for _, en := range entries {
							mod, ver := pendingEntryModuleVersion(en)
							rows = append(rows, []string{en.GetComponentName(), output.FormatEnum(en.GetChangeType()), mod, ver})
						}
						b.Table([]string{"Component", "Change", "Module", "Ref"}, rows)
					}
					if variables := df.GetVariables(); len(variables) > 0 {
						rows := make([][]string, 0, len(variables))
						for _, v := range variables {
							oldStr, newStr := pendingVariableSides(v)
							rows = append(rows, []string{v.GetKey(), output.FormatEnum(v.GetChangeType()), oldStr, newStr})
						}
						b.Table([]string{"Key", "Change", "Old", "New"}, rows)
					}
				})
			}
		})
	}

	if sec.runsErr != nil {
		d.Unavailable("Recent Runs", cmderr.Format(sec.runsErr))
	} else {
		rows := make([][]string, 0, len(runs))
		for _, r := range runs {
			rows = append(rows, []string{runDisplay(r), output.FormatEnum(r.Status), changeSetDisplay(r), output.FormatActor(r.TriggeredBy), output.FormatAge(r.CreatedAt)})
		}
		d.Table("Recent Runs", []string{"ID", "Status", "Change Set", "Triggered By", "Age"}, rows)
	}

	if len(runs) > 0 {
		d.Hint("To see the last run's transcript, run: admiral run logs " + runDisplay(runs[0]))
	}
	if sec.permissionDenied() {
		d.Hint(cmderr.ScopeHint)
	}
	return d
}

// unavailable marks a top-block field whose backing read failed.
const unavailable = "<unavailable>"

// healthRank orders health words from best to worst so the environment's
// health can be the worst of its components (argocd's rule).
var healthRank = map[string]int{"Healthy": 0, "Progressing": 1, "Degraded": 2, "Unknown": 3}

// componentHealth derives a health word from the component's last revision
// status until the server reports health directly.
func componentHealth(c *environmentv1.EnvironmentComponent) string {
	switch c.LastRevisionStatus {
	case runv1.RevisionStatus_REVISION_STATUS_SUCCEEDED:
		return "Healthy"
	case runv1.RevisionStatus_REVISION_STATUS_PENDING, runv1.RevisionStatus_REVISION_STATUS_QUEUED,
		runv1.RevisionStatus_REVISION_STATUS_PLANNING, runv1.RevisionStatus_REVISION_STATUS_PLANNED,
		runv1.RevisionStatus_REVISION_STATUS_APPLYING, runv1.RevisionStatus_REVISION_STATUS_DEFERRED:
		return "Progressing"
	case runv1.RevisionStatus_REVISION_STATUS_FAILED, runv1.RevisionStatus_REVISION_STATUS_BLOCKED:
		return "Degraded"
	default:
		return "Unknown"
	}
}

// envHealth is the worst component health, naming the culprits.
func envHealth(comps []*environmentv1.EnvironmentComponent) string {
	if len(comps) == 0 {
		return "Unknown (no components)"
	}
	worst := "Healthy"
	var culprits []string
	for _, c := range comps {
		h := componentHealth(c)
		if healthRank[h] > healthRank[worst] {
			worst, culprits = h, []string{c.Name}
		} else if h == worst && h != "Healthy" {
			culprits = append(culprits, c.Name)
		}
	}
	if len(culprits) == 0 {
		return worst
	}
	sort.Strings(culprits)
	return fmt.Sprintf("%s (%s)", worst, joinMax(culprits, 3))
}

func joinMax(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:n], ", ") + fmt.Sprintf(", +%d", len(items)-n)
}

func runDisplay(r *runv1.Run) string {
	if r.DisplayId != "" {
		return r.DisplayId
	}
	return truncateUUID(r.Id)
}

func changeSetDisplay(r *runv1.Run) string {
	if r.ChangeSetDisplayId != "" {
		return r.ChangeSetDisplayId
	}
	if r.ChangeSetId != "" {
		return truncateUUID(r.ChangeSetId)
	}
	return ""
}

// pendingEntryModuleVersion picks the "after" side of a module/version diff
// for the rendered table. CREATE entries only have the new side; UPDATEs
// either keep or replace the module — operators care about what the entry
// will produce, not the prior pin.
func pendingEntryModuleVersion(e *changesetv1.EntryDiff) (string, string) {
	var mod, ver string
	if m := e.GetCatalogItem(); m != nil {
		if m.CatalogItemNameNew != nil && *m.CatalogItemNameNew != "" {
			mod = *m.CatalogItemNameNew
		} else if m.CatalogItemIdNew != nil && *m.CatalogItemIdNew != "" {
			mod = *m.CatalogItemIdNew
		}
		if m.RefNew != nil && *m.RefNew != "" {
			ver = *m.RefNew
		}
	}
	return mod, ver
}

func pendingVariableSides(v *changesetv1.VariableDiff) (string, string) {
	if v.GetSensitive() {
		return "<redacted>", "<redacted>"
	}
	var oldStr, newStr string
	if v.Old != nil {
		oldStr = *v.Old
	}
	if v.New != nil {
		newStr = *v.New
	}
	return output.Truncate(oldStr, 40), output.Truncate(newStr, 40)
}

// loadPendingDiffs fetches DiffChangeSet for each open changeset so describe
// can render staged entries and variables under each cs header. Failures are
// silent: a missing entry in the returned map is rendered as "no pending
// changes" rather than a hard error, since describe is read-only and partial
// info is more useful than no info.
func loadPendingDiffs(ctx context.Context, c sdkclient.AdmiralClient, css []*changesetv1.ChangeSet) map[string]*changesetv1.ChangeSetDiff {
	out := make(map[string]*changesetv1.ChangeSetDiff, len(css))
	for _, cs := range css {
		resp, err := c.ChangeSet().DiffChangeSet(ctx, &changesetv1.DiffChangeSetRequest{ChangeSetId: cs.Id})
		if err != nil {
			continue
		}
		out[cs.Id] = resp.GetDiff()
	}
	return out
}

// scopeFilter builds the application + environment predicate shared by the
// describe sub-queries.
func scopeFilter(appID, envID string) (string, error) {
	byApp, err := filter.Eq("application_id", appID)
	if err != nil {
		return "", err
	}
	byEnv, err := filter.Eq("environment_id", envID)
	if err != nil {
		return "", err
	}
	return filter.And(byApp, byEnv), nil
}

func listRecentRuns(ctx context.Context, c sdkclient.AdmiralClient, appID, envID string) ([]*runv1.Run, error) {
	f, err := scopeFilter(appID, envID)
	if err != nil {
		return nil, err
	}
	resp, err := c.Run().ListRuns(ctx, &runv1.ListRunsRequest{
		Filter:   f,
		PageSize: recentRunsLimit,
	})
	if err != nil {
		return nil, err
	}
	return resp.Runs, nil
}

func listOpenChangeSets(ctx context.Context, c sdkclient.AdmiralClient, appID, envID string) ([]*changesetv1.ChangeSet, error) {
	scope, err := scopeFilter(appID, envID)
	if err != nil {
		return nil, err
	}
	resp, err := c.ChangeSet().ListChangeSets(ctx, &changesetv1.ListChangeSetsRequest{
		Filter:   filter.And(scope, "field['status'] = 'OPEN'"),
		PageSize: 50,
	})
	if err != nil {
		return nil, err
	}
	return resp.ChangeSets, nil
}

func listAllVariables(ctx context.Context, c sdkclient.AdmiralClient, envID string) ([]*variablev1.Variable, error) {
	var all []*variablev1.Variable
	pageToken := ""
	for {
		resp, err := c.Environment().ListEnvironmentVariables(ctx, &environmentv1.ListEnvironmentVariablesRequest{
			EnvironmentId: envID,
			PageSize:      100,
			PageToken:     pageToken,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Variables...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	// Sort by key so describe output is stable + scannable across pages.
	sort.Slice(all, func(i, j int) bool { return all[i].Key < all[j].Key })
	return all, nil
}

func truncateUUID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}
