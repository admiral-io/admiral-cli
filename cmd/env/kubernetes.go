package env

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

var namespacePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// kubernetesFlags are an environment's Kubernetes target as flags.
type kubernetesFlags struct {
	namespace        string
	createNamespaces bool
}

func (k *kubernetesFlags) register(cmd *cobra.Command, namespaceUsage string) {
	cmd.Flags().StringVar(&k.namespace, "namespace", "", namespaceUsage)
	cmd.Flags().BoolVar(&k.createNamespaces, "create-namespaces", true,
		"create a missing namespace at apply (--create-namespaces=false to refuse)")
}

// apply writes the flags that were given onto t, creating it when needed,
// and returns the update mask paths they set. A namespace is checked here,
// before any sign-in.
func (k *kubernetesFlags) apply(cmd *cobra.Command, t **environmentv1.KubernetesTarget) ([]string, error) {
	var paths []string
	if cmd.Flags().Changed("namespace") {
		if k.namespace != "" && !namespacePattern.MatchString(k.namespace) {
			return nil, cmderr.Usage("invalid namespace %q: at most 63 lowercase letters, digits and hyphens", k.namespace)
		}
		if *t == nil {
			*t = &environmentv1.KubernetesTarget{}
		}
		(*t).Namespace = k.namespace
		paths = append(paths, "kubernetes.namespace")
	}
	if cmd.Flags().Changed("create-namespaces") {
		if *t == nil {
			*t = &environmentv1.KubernetesTarget{}
		}
		v := k.createNamespaces
		(*t).CreateNamespaces = &v
		paths = append(paths, "kubernetes.create_namespaces")
	}
	return paths, nil
}

// apiGroups counts the distinct groups among group/version strings; the
// core group is "v1" alone.
func apiGroups(versions []string) int {
	groups := make(map[string]bool, len(versions))
	for _, v := range versions {
		g, _, found := strings.Cut(v, "/")
		if !found {
			g = ""
		}
		groups[g] = true
	}
	return len(groups)
}

// capabilitiesSummary is one line for a table cell: version, groups, age.
func capabilitiesSummary(c *environmentv1.KubernetesCapabilities) string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("%s, %d API groups, %s ago", c.KubeVersion, apiGroups(c.ApiVersions), output.FormatAge(c.ReportedAt))
}

func createNamespaces(t *environmentv1.KubernetesTarget) string {
	if t == nil || t.CreateNamespaces == nil {
		return ""
	}
	return strconv.FormatBool(*t.CreateNamespaces)
}

// describeKubernetes is the Kubernetes section of describe.
func describeKubernetes(d *output.Describe, t *environmentv1.KubernetesTarget) {
	d.Section("Kubernetes", func(b *output.Block) {
		b.Field("Namespace", t.GetNamespace())
		b.Field("Create Namespaces", createNamespaces(t))
		c := t.GetCapabilities()
		if c == nil {
			b.Field("Capabilities", "<none reported>")
			return
		}
		b.Block("Capabilities", func(b *output.Block) {
			b.Field("Version", c.KubeVersion)
			b.Field("API Groups", strconv.Itoa(apiGroups(c.ApiVersions)))
			b.Field("Reported", fmt.Sprintf("%s (%s ago)", output.FormatDescribeTime(c.ReportedAt), output.FormatAge(c.ReportedAt)))
		})
	})
}
