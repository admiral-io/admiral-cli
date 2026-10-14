package changeset

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

// renderedOptions are the flags of get --rendered.
type renderedOptions struct {
	rendered  bool
	component string
	revision  int32
	outputDir string
	force     bool
}

func newGetCmd(opts *client.Options) *cobra.Command {
	var ro renderedOptions

	cmd := &cobra.Command{
		Use:   "get <changeset>",
		Short: "Get a changeset",
		Long: `Get a changeset.

With --rendered, print a prepared revision's manifests and hooks instead: for
each component a header naming it and its namespace, its findings as
comments, its manifests, then its hooks. Secret values are masked. The
revision is the one the changeset's newest run planned unless --revision
names another; get never prepares one, 'admiral changeset plan' does.
--output-dir unpacks the whole artifact into a directory.`,
		Example: `  admiral changeset get cs-7f2a1c9d0e3b

  # The full object
  admiral changeset get cs-7f2a1c9d0e3b -o yaml

  # What the last plan rendered
  admiral changeset get cs-7f2a1c9d0e3b --rendered

  # One component of revision 3
  admiral changeset get cs-7f2a1c9d0e3b --rendered --revision 3 --component api

  # The artifact as files
  admiral changeset get cs-7f2a1c9d0e3b --rendered --output-dir ./rendered`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			csID, err := changeSetID(args[0])
			if err != nil {
				return err
			}
			if err := ro.check(cmd); err != nil {
				return err
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			if ro.rendered {
				return getRendered(cmd, c, csID, ro)
			}
			resp, err := c.ChangeSet().GetChangeSet(cmd.Context(), &changesetv1.GetChangeSetRequest{ChangeSetId: csID})
			if err != nil {
				return err
			}
			p := output.NewPrinter(cmd, opts.OutputFormat)
			return p.PrintOne(resp.ChangeSet, resp.ChangeSet.Id, changeSetTable.Render(p, resp.ChangeSet))
		},
	}

	cmd.Flags().BoolVar(&ro.rendered, "rendered", false, "print a prepared revision's manifests and hooks")
	cmd.Flags().StringVar(&ro.component, "component", "", "with --rendered, only this component")
	cmd.Flags().Int32Var(&ro.revision, "revision", 0, "with --rendered, this revision number instead of the one last planned")
	cmd.Flags().StringVar(&ro.outputDir, "output-dir", "", "with --rendered, unpack the artifact into this directory")
	cmd.Flags().BoolVarP(&ro.force, "force", "f", false, "with --output-dir, write into a directory that is not empty")

	return cmd
}

// check refuses what it can before any sign-in, including a directory
// that is not empty.
func (ro renderedOptions) check(cmd *cobra.Command) error {
	if !ro.rendered {
		for _, name := range []string{"component", "revision", "output-dir", "force"} {
			if cmd.Flags().Changed(name) {
				return cmderr.Usage("--%s needs --rendered", name)
			}
		}
		return nil
	}
	if cmd.Flags().Changed("revision") && ro.revision < 1 {
		return cmderr.Usage("--revision must be 1 or more")
	}
	if ro.component != "" {
		if err := componentName(ro.component); err != nil {
			return err
		}
	}
	if ro.outputDir == "" {
		if ro.force {
			return cmderr.Usage("--force applies to --output-dir")
		}
		return nil
	}
	if ro.component != "" {
		return cmderr.Usage("--component cannot be combined with --output-dir, which unpacks the whole artifact")
	}
	return checkOutputDir(ro.outputDir, ro.force)
}

// getRendered reads an artifact; it never asks for one to be made.
func getRendered(cmd *cobra.Command, c sdkclient.AdmiralClient, csID string, ro renderedOptions) error {
	ctx := cmd.Context()
	rev := strconv.Itoa(int(ro.revision))
	if !cmd.Flags().Changed("revision") {
		var err error
		if rev, err = lastPlanned(cmd, c, csID); err != nil {
			return err
		}
	}

	resp, err := c.ChangeSet().GetArtifact(ctx, &changesetv1.GetArtifactRequest{ChangeSetId: csID, Revision: rev})
	if err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			return cmderr.WithHint(err, fmt.Sprintf("Run 'admiral changeset plan %s' to prepare the head.", csID))
		}
		return err
	}
	files, err := readArtifact(resp.Artifact)
	if err != nil {
		return err
	}
	if ro.outputDir != "" {
		if err := unpack(ro.outputDir, files); err != nil {
			return err
		}
		output.Writef(cmd.ErrOrStderr(), "Unpacked %s revision %s (%s) into %s\n",
			csID, rev, shortArtifact(resp.ArtifactDigest), ro.outputDir)
		return nil
	}

	comps, err := components(files)
	if err != nil {
		return err
	}
	if ro.component != "" {
		comps, err = onlyComponent(comps, ro.component, csID, rev)
		if err != nil {
			return err
		}
	}
	writeRendered(cmd.OutOrStdout(), comps)
	return nil
}

// lastPlanned is the revision of the changeset's newest run, warning when
// the head has moved past it.
func lastPlanned(cmd *cobra.Command, c sdkclient.AdmiralClient, csID string) (string, error) {
	ctx := cmd.Context()
	resp, err := c.ChangeSet().GetChangeSet(ctx, &changesetv1.GetChangeSetRequest{ChangeSetId: csID})
	if err != nil {
		return "", err
	}
	runID := resp.ChangeSet.GetLatestRunId()
	if runID == "" {
		return "", cmderr.WithHint(fmt.Errorf("%s has not been planned", csID),
			fmt.Sprintf("Run 'admiral changeset plan %s' to prepare the head.", csID))
	}
	run, err := c.Run().GetRun(ctx, &runv1.GetRunRequest{RunId: runID})
	if err != nil {
		return "", err
	}
	rev := run.Run.GetRevisionId()
	if head := resp.Head; head != nil && head.Id != rev {
		output.Writef(cmd.ErrOrStderr(), "Warning: this is %s, last planned by %s; the head is revision %d and not yet planned\n",
			rev, runID, head.Number)
	}
	return rev, nil
}

func onlyComponent(comps []*renderedComponent, name, csID, rev string) ([]*renderedComponent, error) {
	names := make([]string, 0, len(comps))
	for _, c := range comps {
		if c.name == name {
			return []*renderedComponent{c}, nil
		}
		names = append(names, c.name)
	}
	return nil, fmt.Errorf("component %q is not in %s revision %s, which renders: %s",
		name, csID, rev, strings.Join(names, ", "))
}
