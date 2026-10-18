package component

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/complete"
	"go.admiral.io/cli/internal/flags"
	"go.admiral.io/cli/internal/manifest"
	"go.admiral.io/cli/internal/output"
	"go.admiral.io/cli/internal/resolve"
	"go.admiral.io/cli/internal/valuesfile"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

func newImportCmd(opts *client.Options) *cobra.Command {
	var (
		repo        string
		version     string
		ref         string
		path        string
		name        string
		tags        []string
		description string
		labelStrs   []string
		credentials []string
		baseline    string
	)

	cmd := &cobra.Command{
		Use:   "import <source>",
		Short: "Publish a copy of an artifact you do not own",
		Long: `Publish a copy of an artifact you do not own.

The registry fetches the source, closes it the way publish does, and
publishes the result as a version with the source recorded as its
provenance. What is published is a copy: the bytes reviewed, whatever
upstream does to the tag later. Importing the same thing twice yields the
same version.

The source says what it is:

  oci://HOST/PATH/CHART            a Helm chart in an OCI registry; --version
  CHART --repo URL                 a Helm chart in an HTTP repository; --version
  NAMESPACE/NAME/SYSTEM[//DIR]     a module in a Terraform registry, host-
                                   qualified for a private one; --version is
                                   a constraint, unset means the latest
  URL ending in .git, ssh://, git@ a git repository (github.com/OWNER/REPO
                                   needs no .git); --ref names a branch,
                                   tag or commit, --path one directory of it
  URL ending in .tgz, .zip, ...    an archive

The component is named after the artifact: the chart, the module, the
repository or the directory. --name says otherwise. A chart's version and
a module's resolved version become tags whether or not named.

Nothing is fetched with ambient credentials. --credential attaches one the
tenant holds; it is presented only where its type fits the protocol and its
allowed hosts permit, and a fetch that reaches a host none covers fails
naming the host.

--baseline-values sets the organization's defaults for the artifact, a
YAML or JSON object laid over its own defaults (a chart's values.yaml, a
module's variable defaults) before any environment's values. They are part
of the version: importing with different baseline values is a new version.`,
		Example: `  # A chart from a repository, as cert-manager
  admiral component import cert-manager --repo https://charts.jetstack.io --version v1.16.2

  # A chart from an OCI registry, as argo-cd
  admiral component import oci://ghcr.io/argoproj/argo-helm/argo-cd --version 7.7.0

  # A registry module at the newest 8.x, as cloud-armor
  admiral component import GoogleCloudPlatform/cloud-armor/google --version '~> 8.0'

  # One directory of a private repository, over SSH with a deploy key
  admiral component import git@github.com:acme/infra.git --ref v2.1.0 --path modules/vpc --credential infra-deploy

  # A private chart, under a name of your own, tagged
  admiral component import oci://ghcr.io/acme/charts/base --version 1.4.0 --name base-upstream --tag stable --credential github

  # A chart with the organization's defaults laid over its own
  admiral component import cert-manager --repo https://charts.jetstack.io --version v1.16.2 --baseline-values cert-manager.yaml`,
		Args: flags.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			labels, err := flags.ParseLabels(labelStrs)
			if err != nil {
				return err
			}
			if name != "" && !manifest.ValidName(name) {
				return cmderr.Usage("component name %q must be lowercase letters, digits and hyphens", name)
			}
			src, err := parseSource(args[0], sourceFlags{repo: repo, version: version, ref: ref, path: path})
			if err != nil {
				return err
			}
			var baselineJSON string
			if baseline != "" {
				if baselineJSON, err = readBaseline(baseline); err != nil {
					return err
				}
			}

			c, err := client.CreateClient(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer c.Close() //nolint:errcheck

			ids := make([]string, 0, len(credentials))
			for _, cred := range credentials {
				id, err := resolve.Credential(cmd.Context(), c.Credential(), cred)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}

			p := output.NewPrinter(cmd, opts.OutputFormat)
			output.Writef(p.Err(), "Importing %s\n", args[0])
			resp, err := c.Registry().ImportComponent(cmd.Context(), &registryv1.ImportComponentRequest{
				Name:               name,
				Source:             src,
				CredentialIds:      ids,
				Tags:               tags,
				Description:        description,
				Labels:             labels,
				BaselineValuesJson: baselineJSON,
			})
			if err != nil {
				return err
			}
			ver := resp.Version
			// The source's own digest is in the version's provenance; the
			// digest to pin is the version's, which the next line prints.
			if prov := ver.Provenance; prov != nil {
				output.Writef(p.Err(), "Imported %s%s\n", prov.Uri, atRef(prov.Ref))
			}
			reportPublished(cmd, p, resp.Component, ver, resp.Unchanged)
			return p.PrintOne(ver, resp.Component.Name+"@"+ver.Digest, versionTable.Render(p, ver))
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "the Helm repository the chart is in; the source is then the chart's name")
	cmd.Flags().StringVar(&version, "version", "", "the chart's version, or a module's version constraint")
	cmd.Flags().StringVar(&ref, "ref", "", "the branch, tag or commit of a git repository (default: its default branch)")
	cmd.Flags().StringVar(&path, "path", "", "one directory of a git repository")
	cmd.Flags().StringVar(&name, "name", "", "component name (default: the artifact's own)")
	cmd.Flags().StringArrayVarP(&tags, "tag", "t", nil, "tag to set on the version (repeatable)")
	cmd.Flags().StringVar(&description, "description", "", "component description, applied when the component is created")
	flags.Label(cmd, &labelStrs, "label to attach when the component is created (key=value, repeatable)")
	cmd.Flags().StringArrayVar(&credentials, "credential", nil, "credential the import may present, by name or ID (repeatable)")
	complete.Flag(cmd, "credential", complete.Credentials(opts))
	cmd.Flags().StringVar(&baseline, "baseline-values", "", "file of default values (YAML or JSON object) laid over the artifact's own")
	_ = cmd.MarkFlagFilename("baseline-values", "yaml", "yml", "json")

	return cmd
}

// readBaseline reads a baseline values file as the JSON object the API
// takes. YAML is read the way a values file is, so JSON reads too.
func readBaseline(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", cmderr.Usage("cannot read --baseline-values: %v", err)
	}
	tree, _, err := valuesfile.Parse(data)
	if err != nil {
		return "", cmderr.Usage("--baseline-values %s: %v", file, err)
	}
	s, err := valuesfile.EncodeJSON(tree)
	if err != nil {
		return "", fmt.Errorf("--baseline-values %s: %w", file, err)
	}
	return s, nil
}

func atRef(ref string) string {
	if ref == "" {
		return ""
	}
	return " at " + ref
}

type sourceFlags struct {
	repo, version, ref, path string
}

// archiveExtensions are what marks a URL as an archive rather than a
// repository; the same list the registry unpacks.
var archiveExtensions = []string{".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz", ".tar.zst", ".tzst", ".zip", ".tar"}

// parseSource reads the kind off the source's form, then checks the flags
// against it: a flag that belongs to another kind is a usage error rather
// than silently ignored.
func parseSource(arg string, f sourceFlags) (*registryv1.ImportSource, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil, cmderr.Usage("source is required")
	}
	gitOnly := func(kind string) error {
		if f.ref != "" || f.path != "" {
			return cmderr.Usage("--ref and --path apply to a git repository, not %s", kind)
		}
		return nil
	}
	notRepo := func(kind string) error {
		if f.repo != "" {
			return cmderr.Usage("--repo names a Helm repository, and %s is not a chart in one", kind)
		}
		return nil
	}

	switch {
	case f.repo != "":
		if strings.Contains(arg, "/") || strings.Contains(arg, ":") {
			return nil, cmderr.Usage("with --repo the source is the chart's name, not %q", arg)
		}
		if f.version == "" {
			return nil, cmderr.Usage("--version is required for a chart")
		}
		if err := gitOnly("a chart"); err != nil {
			return nil, err
		}
		return &registryv1.ImportSource{Source: &registryv1.ImportSource_HelmChart_{HelmChart: &registryv1.ImportSource_HelmChart{
			Repository: f.repo, Chart: arg, Version: f.version,
		}}}, nil

	case strings.HasPrefix(arg, "oci://"):
		if f.version == "" {
			return nil, cmderr.Usage("--version is required for a chart")
		}
		if err := gitOnly("a chart"); err != nil {
			return nil, err
		}
		return &registryv1.ImportSource{Source: &registryv1.ImportSource_OciChart{OciChart: &registryv1.ImportSource_OCIChart{
			Reference: arg, Version: f.version,
		}}}, nil

	case isGit(arg):
		if err := notRepo("a git repository"); err != nil {
			return nil, err
		}
		if f.version != "" {
			return nil, cmderr.Usage("a git repository is pinned with --ref, not --version")
		}
		return &registryv1.ImportSource{Source: &registryv1.ImportSource_GitTree_{GitTree: &registryv1.ImportSource_GitTree{
			Url: gitURL(arg), Ref: f.ref, Path: f.path,
		}}}, nil

	case isArchive(arg):
		if err := notRepo("an archive"); err != nil {
			return nil, err
		}
		if err := gitOnly("an archive"); err != nil {
			return nil, err
		}
		if f.version != "" {
			return nil, cmderr.Usage("an archive has no --version; it is what the URL names")
		}
		return &registryv1.ImportSource{Source: &registryv1.ImportSource_Archive_{Archive: &registryv1.ImportSource_Archive{Url: arg}}}, nil

	case isRegistryModule(arg):
		if err := notRepo("a registry module"); err != nil {
			return nil, err
		}
		if err := gitOnly("a registry module"); err != nil {
			return nil, err
		}
		return &registryv1.ImportSource{Source: &registryv1.ImportSource_RegistryModule_{RegistryModule: &registryv1.ImportSource_RegistryModule{
			Address: arg, Version: f.version,
		}}}, nil
	}
	return nil, cmderr.UsageHint(
		"A chart is oci://... or NAME --repo URL; a module is NAMESPACE/NAME/SYSTEM; a repository ends in .git or is ssh://; an archive ends in .tgz, .zip, ...",
		"cannot tell what %q is", arg)
}

// scpLike is the shorthand GitHub hands out, git@github.com:acme/infra.git.
var scpLike = regexp.MustCompile(`^([^@/:]+@[^:/]+):(.+)$`)

// gitURL is the source as the URI the API takes: the go-getter prefix
// dropped, the scp shorthand as ssh://.
func gitURL(s string) string {
	s = strings.TrimPrefix(s, "git::")
	if m := scpLike.FindStringSubmatch(s); m != nil {
		return "ssh://" + m[1] + "/" + strings.TrimLeft(m[2], "/")
	}
	return s
}

func isGit(s string) bool {
	if strings.HasPrefix(s, "git::") || strings.HasPrefix(s, "ssh://") || scpLike.MatchString(s) {
		return true
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	path := strings.TrimSuffix(u.Path, "/")
	if strings.HasSuffix(path, ".git") {
		return true
	}
	// The forges everyone types without .git: owner/repo and nothing after.
	switch strings.ToLower(u.Host) {
	case "github.com", "gitlab.com", "bitbucket.org":
		return len(strings.Split(strings.Trim(path, "/"), "/")) == 2
	}
	return false
}

func isArchive(s string) bool {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	for _, ext := range archiveExtensions {
		if strings.HasSuffix(u.Path, ext) {
			return true
		}
	}
	return false
}

// isRegistryModule is namespace/name/system, host-qualified or not, with
// an optional //subdirectory: no scheme, no scp-like colon.
func isRegistryModule(s string) bool {
	if strings.Contains(s, "://") || strings.Contains(s, "@") {
		return false
	}
	addr, _, _ := strings.Cut(s, "//")
	parts := strings.Split(addr, "/")
	if len(parts) == 4 {
		if !strings.Contains(parts[0], ".") && !strings.Contains(parts[0], ":") {
			return false // a host has a dot or a port; four bare parts is nothing
		}
		parts = parts[1:]
	}
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" || strings.Contains(p, ":") {
			return false
		}
	}
	return true
}
