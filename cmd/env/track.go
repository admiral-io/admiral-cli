package env

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

// trackFlag is --track: how the environment follows publishes of the
// components it runs. A matching publish opens or updates the environment's
// draft changeset for that component and plans it; nothing applies.
type trackFlag struct {
	value string
}

const trackUsage = "follow publishes: branch=main, branch=release/*, tag=v*, range='>=1.4.0 <2.0.0', or none to pin"

func (f *trackFlag) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.value, "track", "", trackUsage)
}

func (f *trackFlag) changed(cmd *cobra.Command) bool { return cmd.Flags().Changed("track") }

// apply sets the environment's tracking from the flag and returns the mask
// path it changed, none when the flag was not given.
func (f *trackFlag) apply(cmd *cobra.Command, e *environmentv1.Environment) ([]string, error) {
	if !f.changed(cmd) {
		return nil, nil
	}
	t, err := parseTrack(f.value)
	if err != nil {
		return nil, err
	}
	e.Tracking = t
	return []string{"tracking"}, nil
}

var trackKinds = map[string]environmentv1.Tracking_Kind{
	"branch": environmentv1.Tracking_KIND_BRANCH,
	"tag":    environmentv1.Tracking_KIND_TAG,
	"range":  environmentv1.Tracking_KIND_RANGE,
}

// parseTrack reads kind=pattern, or none for pinned.
func parseTrack(s string) (*environmentv1.Tracking, error) {
	if s == "none" {
		return nil, nil
	}
	kind, pattern, ok := strings.Cut(s, "=")
	k, known := trackKinds[kind]
	if !ok || !known || pattern == "" {
		return nil, cmderr.Usage("--track takes branch=<name or glob>, tag=<glob>, range=<semver range>, or none, not %q", s)
	}
	return &environmentv1.Tracking{Kind: k, Pattern: pattern}, nil
}

// trackingName is how the policy reads back: branch=main, or pinned.
func trackingName(t *environmentv1.Tracking) string {
	if t == nil {
		return "pinned"
	}
	for name, k := range trackKinds {
		if k == t.GetKind() {
			return name + "=" + t.GetPattern()
		}
	}
	return t.GetPattern()
}
