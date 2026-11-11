package env

import (
	"strings"

	"github.com/spf13/cobra"

	"go.admiral.io/cli/internal/cmderr"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
)

// trackFlag is --track: how the environment follows publishes of the
// components it runs. A matching publish opens or updates the environment's
// draft changeset for that component and plans it; nothing applies, except
// a pull request's approved plan once its merge publishes the same bytes,
// which --after-merge turns off.
type trackFlag struct {
	value      string
	afterMerge string
}

const (
	trackUsage      = "follow publishes: branch=main, branch=release/*, tag=v*, range='>=1.4.0 <2.0.0', or none to pin"
	afterMergeUsage = "what a pull request's approved plan does when its merge publishes the same bytes: apply (default) or wait for a person"
)

func (f *trackFlag) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.value, "track", "", trackUsage)
	cmd.Flags().StringVar(&f.afterMerge, "after-merge", "", afterMergeUsage)
}

func (f *trackFlag) changed(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("track") || cmd.Flags().Changed("after-merge")
}

// apply sets the environment's tracking from the flags and returns the mask
// path it changed, none when neither was given. --after-merge alone keeps
// the policy and changes only that; on a pinned environment it is refused,
// since nothing merges into a pin.
func (f *trackFlag) apply(cmd *cobra.Command, e *environmentv1.Environment) ([]string, error) {
	if !f.changed(cmd) {
		return nil, nil
	}
	t := e.GetTracking()
	if cmd.Flags().Changed("track") {
		var err error
		if t, err = parseTrack(f.value); err != nil {
			return nil, err
		}
	}
	if cmd.Flags().Changed("after-merge") {
		after, err := parseAfterMerge(f.afterMerge)
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, cmderr.Usage("--after-merge applies to an environment that tracks a branch; pass --track too")
		}
		t.AfterMerge = after
	}
	e.Tracking = t
	return []string{"tracking"}, nil
}

func parseAfterMerge(s string) (environmentv1.Tracking_AfterMerge, error) {
	switch s {
	case "apply":
		return environmentv1.Tracking_AFTER_MERGE_APPLY, nil
	case "wait":
		return environmentv1.Tracking_AFTER_MERGE_WAIT, nil
	default:
		return 0, cmderr.Usage("--after-merge takes apply or wait, not %q", s)
	}
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

// trackingName is how the policy reads back: branch=main, or pinned, with
// "wait after merge" when a person applies a merged pull request's plan.
func trackingName(t *environmentv1.Tracking) string {
	if t == nil {
		return "pinned"
	}
	name := t.GetPattern()
	for kind, k := range trackKinds {
		if k == t.GetKind() {
			name = kind + "=" + t.GetPattern()
		}
	}
	if t.GetAfterMerge() == environmentv1.Tracking_AFTER_MERGE_WAIT {
		name += " (wait after merge)"
	}
	return name
}
