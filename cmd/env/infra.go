package env

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/structpb"

	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/output"
	changesetv1 "go.admiral.io/sdk/proto/admiral/api/changeset/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	registryv1 "go.admiral.io/sdk/proto/admiral/api/registry/v1"
)

var engines = map[string]registryv1.InfraEngine{
	"terraform": registryv1.InfraEngine_INFRA_ENGINE_TERRAFORM,
	"opentofu":  registryv1.InfraEngine_INFRA_ENGINE_OPENTOFU,
}

// infraFlags are where an environment's Terraform components keep their
// state and which engine runs them.
type infraFlags struct {
	engine        string
	backend       string
	backendConfig []string
}

func (f *infraFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.engine, "engine", "", "the engine and exact version Terraform components run, as terraform@1.10.6 or opentofu@1.10.6")
	cmd.Flags().StringVar(&f.backend, "backend", "", "where Terraform components keep state, as Terraform names the backend: gcs, s3, azurerm, http")
	cmd.Flags().StringArrayVar(&f.backendConfig, "backend-config", nil,
		"a backend argument, key=value (repeatable); Admiral sets the key or prefix per component")
}

func (f *infraFlags) changed(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("engine") || cmd.Flags().Changed("backend") || cmd.Flags().Changed("backend-config")
}

// apply sets what the flags name on e and returns the update mask paths.
// --backend-config replaces the backend's arguments and needs --backend.
func (f *infraFlags) apply(cmd *cobra.Command, e *environmentv1.Environment) ([]string, error) {
	var paths []string
	if cmd.Flags().Changed("engine") {
		v, err := parseEngine(f.engine)
		if err != nil {
			return nil, err
		}
		e.InfraEngine = v
		paths = append(paths, "infra_engine")
	}
	if cmd.Flags().Changed("backend-config") && !cmd.Flags().Changed("backend") {
		return nil, cmderr.Usage("--backend-config needs --backend, which it configures")
	}
	if cmd.Flags().Changed("backend") {
		b, err := parseBackend(f.backend, f.backendConfig)
		if err != nil {
			return nil, err
		}
		e.InfraBackend = b
		paths = append(paths, "infra_backend")
	}
	return paths, nil
}

func parseEngine(s string) (*registryv1.InfraEngineVersion, error) {
	name, version, ok := strings.Cut(s, "@")
	engine, known := engines[name]
	if !ok || !known || version == "" {
		return nil, cmderr.Usage("--engine %q is not terraform@<version> or opentofu@<version>", s)
	}
	return &registryv1.InfraEngineVersion{Engine: engine, Version: version}, nil
}

func parseBackend(kind string, args []string) (*changesetv1.InfraBackend, error) {
	if kind == "" {
		return nil, cmderr.Usage("--backend names the backend type, such as gcs")
	}
	cfg := make(map[string]any, len(args))
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" {
			return nil, cmderr.Usage("--backend-config %q is not key=value", a)
		}
		cfg[k] = v
	}
	s, err := structpb.NewStruct(cfg)
	if err != nil {
		return nil, err
	}
	return &changesetv1.InfraBackend{Type: kind, Config: s}, nil
}

func engineName(v *registryv1.InfraEngineVersion) string {
	if v == nil {
		return ""
	}
	return output.FormatEnumKebab(v.GetEngine()) + "@" + v.GetVersion()
}

// backendName is the backend's type and its arguments, key=value, sorted.
// Absent is hosted state.
func backendName(b *changesetv1.InfraBackend) string {
	if b == nil {
		return "hosted"
	}
	fields := b.GetConfig().GetFields()
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{b.GetType()}
	for _, k := range keys {
		parts = append(parts, k+"="+fields[k].GetStringValue())
	}
	return strings.Join(parts, " ")
}
