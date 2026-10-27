package run

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/output"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	environmentv1 "go.admiral.io/sdk/proto/admiral/api/environment/v1"
	runv1 "go.admiral.io/sdk/proto/admiral/api/run/v1"
)

const (
	csID      = "cs-7f2a1c9d0e3b"
	testRunID = "run-1nnsyzr305q7"
)

func render(t *testing.T, printer func(*output.Printer, *runv1.Run, string) error, r *runv1.Run) string {
	t.Helper()
	return renderTitled(t, printer, r, "")
}

func renderTitled(t *testing.T, printer func(*output.Printer, *runv1.Run, string) error, r *runv1.Run, title string) string {
	t.Helper()
	cmd := &cobra.Command{}
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	require.NoError(t, printer(output.NewPrinter(cmd, output.FormatTable), r, title))
	return out.String() + errOut.String()
}

func change(kind, name string, a agentv1.ResourceChange_Action) *agentv1.ResourceChange {
	return &agentv1.ResourceChange{Action: a, Resource: k8sRef(kind, name)}
}

func k8sRef(kind, name string) *agentv1.ResourceRef {
	return &agentv1.ResourceRef{Ref: &agentv1.ResourceRef_Kubernetes{Kubernetes: &agentv1.KubernetesResourceRef{
		ApiVersion: "v1", Kind: kind, Namespace: "podinfo-dev", Name: name,
	}}}
}

func planned(rule *environmentv1.ApprovalRule) *runv1.Run {
	return &runv1.Run{
		Id: testRunID, ChangeSetId: csID, Source: &runv1.Run_RevisionId{RevisionId: "rev-q8tx5359s1nk"},
		Status: runv1.Run_STATUS_PLANNED, RequiredApproval: rule,
		Plan: &runv1.PlanPhase{PlanDigest: "sha256:4965", Components: []*agentv1.ComponentPlan{{
			Component: "podinfo", Changes: []*agentv1.ResourceChange{change("Deployment", "podinfo", agentv1.ResourceChange_ACTION_CREATE)},
		}}},
	}
}

// A planned run points at the changeset, and at approving only when the
// environment asks for approvals.
func TestPlannedRunHints(t *testing.T) {
	out := render(t, Print, planned(nil))
	assert.Contains(t, out, "To apply it, run: admiral changeset apply "+csID)
	assert.Contains(t, out, "admiral run diff "+testRunID)
	assert.NotContains(t, out, "approve")
	assert.NotContains(t, out, "admiral run apply")

	withDiff := func(p *output.Printer, r *runv1.Run, title string) error {
		return PrintWithDiff(p, r, title, func(w io.Writer) { output.Writef(w, "\n+ podinfo: the diff\n") })
	}
	out = render(t, withDiff, planned(&environmentv1.ApprovalRule{RequiredApprovals: 1}))
	assert.NotContains(t, out, "run diff", "the diff is shown")
	hint := "It needs 1 approval(s) from someone other than the changeset's author: admiral changeset approve " + csID
	assert.Contains(t, out, hint)
	assert.Contains(t, out, "\n\n+ podinfo: the diff\n\n"+hint, "the diff, set apart once, then the next steps")

	groups := planned(&environmentv1.ApprovalRule{Groups: []*environmentv1.ApprovalRule_GroupApproval{{GroupId: "grp-1", Approvals: 2}}})
	assert.Contains(t, render(t, Print, groups), "It needs 2 approval(s)")

	desired := planned(nil)
	desired.ChangeSetId, desired.Source = "", &runv1.Run_Desired{Desired: &runv1.Desired{Mode: runv1.Desired_MODE_REAPPLY}}
	assert.Contains(t, render(t, Print, desired), "To apply it, run: admiral run apply "+testRunID, "a run without a changeset is acted on directly")
}

func TestApplyShowsWhatEachComponentChanged(t *testing.T) {
	r := planned(nil)
	r.Status = runv1.Run_STATUS_APPLIED
	r.Apply = &runv1.ApplyPhase{Components: []*runv1.ComponentApply{
		{Component: "podinfo", Status: runv1.ComponentApply_STATUS_SUCCEEDED},
	}}
	out := render(t, Print, r)
	assert.Regexp(t, `podinfo\s+succeeded\s+\+1 ~0 ±0 -0`, out)
	assert.Contains(t, out, "To see the changes, run: admiral run diff "+testRunID, "an applied run points at what changed")
}

func TestWriteDiffPrintsADeleteAsOneLine(t *testing.T) {
	plan := &runv1.PlanPhase{Components: []*agentv1.ComponentPlan{
		{Component: "podinfo", Changes: []*agentv1.ResourceChange{
			change("Deployment", "podinfo", agentv1.ResourceChange_ACTION_DESTROY),
			change("Service", "podinfo", agentv1.ResourceChange_ACTION_DESTROY),
		}},
		{Component: "web", Changes: []*agentv1.ResourceChange{
			change("ConfigMap", "web", agentv1.ResourceChange_ACTION_UPDATE),
			change("Secret", "old", agentv1.ResourceChange_ACTION_DESTROY),
		}},
	}}
	d := &agentv1.PlanDiff{Resources: []*agentv1.ResourceDiff{
		{Component: "podinfo", Resource: k8sRef("Deployment", "podinfo"), Action: agentv1.ResourceChange_ACTION_DESTROY,
			Before: obj(t, `{"kind":"Deployment","spec":{"replicas":1}}`)},
		{Component: "podinfo", Resource: k8sRef("Service", "podinfo"), Action: agentv1.ResourceChange_ACTION_DESTROY},
		{Component: "web", Resource: k8sRef("ConfigMap", "web"), Action: agentv1.ResourceChange_ACTION_UPDATE,
			Before: obj(t, `{"data":{"a":"1"}}`), After: obj(t, `{"data":{"a":"2"}}`)},
		{Component: "web", Resource: k8sRef("Secret", "old"), Action: agentv1.ResourceChange_ACTION_DESTROY},
	}}
	out := writeDiff(d, plan)
	assert.Contains(t, out, "- podinfo: v1 Deployment podinfo-dev/podinfo (destroy)\n")
	assert.Contains(t, out, "- podinfo: v1 Service podinfo-dev/podinfo (destroy)\n")
	assert.NotContains(t, out, "replicas", "a delete is not printed whole")
	assert.Contains(t, out, "~ web: v1 ConfigMap podinfo-dev/web (update)\n    data.a: \"1\" -> \"2\"\n")
	assert.Equal(t, 1, strings.Count(out, "The namespace podinfo-dev stays"),
		"said once, and only for a component removed outright")
}

// An update shows only what changes, inside lists too, and never a
// sensitive value: only that it changed.
func TestWriteDiffShowsEachChangedValueAtItsPath(t *testing.T) {
	d := &agentv1.PlanDiff{Resources: []*agentv1.ResourceDiff{{
		Component: "api", Resource: k8sRef("Deployment", "api"), Action: agentv1.ResourceChange_ACTION_UPDATE,
		Before: obj(t, `{"metadata":{"labels":{"app.kubernetes.io/version":"1"}},"spec":{"replicas":2,"template":{"spec":{"containers":[{"name":"api","image":"api:1","env":[{"name":"TOKEN","value":null}]}]}}}}`),
		After:  obj(t, `{"metadata":{"labels":{"app.kubernetes.io/version":"2"}},"spec":{"replicas":2,"template":{"spec":{"containers":[{"name":"api","image":"api:2","env":[{"name":"TOKEN","value":null}]}]}}}}`),
		SensitivePaths: []*agentv1.SensitivePath{{
			Path: path("spec", "template", "spec", "containers", 0, "env", 0, "value"), Changed: true,
		}},
	}}}
	out := writeDiff(d, nil)
	assert.Contains(t, out, `    spec.template.spec.containers[0].image: "api:1" -> "api:2"`)
	assert.Contains(t, out, `    metadata.labels."app.kubernetes.io/version": "1" -> "2"`)
	assert.Contains(t, out, "    spec.template.spec.containers[0].env[0].value: <sensitive, changed>\n")
	assert.NotContains(t, out, "replicas", "an unchanged value is not shown")
	assert.NotContains(t, out, "null -> null")
}

// A takeover names every field it takes from another manager, so the
// approver sees what Helm or kubectl stops owning.
func TestWriteDiffNamesWhatATakeoverTakes(t *testing.T) {
	d := &agentv1.PlanDiff{Resources: []*agentv1.ResourceDiff{{
		Component: "web", Resource: k8sRef("Deployment", "web"), Action: agentv1.ResourceChange_ACTION_TAKE_OVER,
		Before: obj(t, `{"metadata":{"labels":{}},"spec":{"replicas":3}}`),
		After:  obj(t, `{"metadata":{"labels":{"admiral.io/environment":"env-1"}},"spec":{"replicas":3}}`),
		Change: &agentv1.ResourceDiff_Kubernetes{Kubernetes: &agentv1.KubernetesChange{Taken: []*agentv1.TakenField{
			{Manager: "helm", Path: path("spec", "replicas")},
		}}},
	}}}
	out := writeDiff(d, nil)
	assert.Contains(t, out, "~ web: v1 Deployment podinfo-dev/web (take over)\n")
	assert.Contains(t, out, `    metadata.labels."admiral.io/environment": <absent> -> "env-1"`)
	assert.Contains(t, out, "    takes spec.replicas from helm\n")
}

// A create lists what it sets; a value known only after the apply, and one
// forcing a replace, say so.
func TestWriteDiffCreateAndTerraform(t *testing.T) {
	tf := func(addr string) *agentv1.ResourceRef {
		return &agentv1.ResourceRef{Ref: &agentv1.ResourceRef_Terraform{Terraform: &agentv1.TerraformResourceRef{Address: addr}}}
	}
	d := &agentv1.PlanDiff{
		Resources: []*agentv1.ResourceDiff{
			{Component: "net", Resource: tf(`aws_subnet.private["a"]`), Action: agentv1.ResourceChange_ACTION_CREATE,
				After: obj(t, `{"cidr_block":"10.0.1.0/24","id":null}`), UnknownPaths: []*agentv1.DiffPath{path("id")}},
			{Component: "net", Resource: tf("aws_vpc.main"), Action: agentv1.ResourceChange_ACTION_REPLACE,
				Before: obj(t, `{"cidr_block":"10.0.0.0/16"}`), After: obj(t, `{"cidr_block":"10.1.0.0/16"}`),
				ReplacePaths: []*agentv1.DiffPath{path("cidr_block")},
				Change:       &agentv1.ResourceDiff_Terraform{Terraform: &agentv1.TerraformChange{PreviousAddress: "aws_vpc.this"}}},
		},
		Outputs: []*agentv1.OutputChange{
			{Component: "net", Name: "vpc_id", Action: agentv1.ResourceChange_ACTION_CREATE, Unknown: true},
			{Component: "net", Name: "token", Action: agentv1.ResourceChange_ACTION_UPDATE, Sensitive: true},
		},
	}
	out := writeDiff(d, nil)
	assert.Contains(t, out, "+ net: aws_subnet.private[\"a\"] (create)\n    cidr_block: \"10.0.1.0/24\"\n    id: <known after apply>\n")
	assert.Contains(t, out, "± net: aws_vpc.main (replace)  moved from aws_vpc.this\n")
	assert.Contains(t, out, `    cidr_block: "10.0.0.0/16" -> "10.1.0.0/16"  (forces replacement)`)
	assert.Contains(t, out, "  + net.vpc_id: <absent> -> <known after apply>\n")
	assert.Contains(t, out, "  ~ net.token: <sensitive> -> <sensitive>\n")
}

// A failed plan's diff says why, every reason, where the engine points.
func TestWriteDiffListsAFailedPlansErrors(t *testing.T) {
	d := &agentv1.PlanDiff{Errors: []*agentv1.PlanError{
		{Component: "web", Resource: k8sRef("Deployment", "web"), Summary: "the agent may not patch deployments"},
		{Component: "net", Summary: "Unsupported argument", Location: "main.tf:12", Detail: "An argument named \"cidr\" is not expected here."},
	}}
	out := writeDiff(d, nil)
	assert.Contains(t, out, "Errors:\n  ! web: v1 Deployment podinfo-dev/web: the agent may not patch deployments\n")
	assert.Contains(t, out, "  ! net: Unsupported argument\n      at main.tf:12\n      An argument named \"cidr\" is not expected here.\n")
}

func writeDiff(d *agentv1.PlanDiff, plan *runv1.PlanPhase) string {
	var b bytes.Buffer
	WriteDiff(&b, d, plan)
	return b.String()
}

func obj(t *testing.T, js string) *structpb.Value {
	t.Helper()
	var v structpb.Value
	require.NoError(t, protojson.Unmarshal([]byte(js), &v))
	return &v
}

func path(segs ...any) *agentv1.DiffPath {
	p := &agentv1.DiffPath{}
	for _, s := range segs {
		switch s := s.(type) {
		case string:
			p.Segments = append(p.Segments, &agentv1.DiffPathSegment{Segment: &agentv1.DiffPathSegment_Key{Key: s}})
		case int:
			p.Segments = append(p.Segments, &agentv1.DiffPathSegment{Segment: &agentv1.DiffPathSegment_Index{Index: int32(s)}})
		}
	}
	return p
}

func TestProgressStatusSaysWhyARunWaits(t *testing.T) {
	q := func(w agentv1.WaitReason) *runv1.Run {
		return &runv1.Run{Status: runv1.Run_STATUS_QUEUED, WaitReason: w}
	}
	assert.Equal(t, "waiting for an agent", progressStatus(q(agentv1.WaitReason_WAIT_REASON_BACKOFF)))
	assert.Equal(t, "waiting for an agent", progressStatus(q(agentv1.WaitReason_WAIT_REASON_UNSPECIFIED)))
	assert.Equal(t, "waiting: the agent is offline", progressStatus(q(agentv1.WaitReason_WAIT_REASON_AGENT_OFFLINE)))
	assert.Equal(t, "planning", progressStatus(&runv1.Run{Status: runv1.Run_STATUS_PLANNING}))
}

// The status can say planned a moment before the plan is recorded; the wait
// goes on until the plan is readable, or the summary prints empty.
func TestAPlannedRunIsSettledOnceItsPlanIsRecorded(t *testing.T) {
	assert.False(t, settled(&runv1.Run{Status: runv1.Run_STATUS_PLANNED}))
	assert.True(t, settled(&runv1.Run{Status: runv1.Run_STATUS_PLANNED, Plan: &runv1.PlanPhase{PlanDigest: "sha256:1"}}))
	assert.True(t, settled(&runv1.Run{Status: runv1.Run_STATUS_NO_CHANGES}))
	assert.False(t, settled(&runv1.Run{Status: runv1.Run_STATUS_APPLYING}))
}

func TestRefusalSaysWhatUnblocksIt(t *testing.T) {
	author := status.Error(codes.PermissionDenied, "you revised change set "+csID+"; the author never counts toward an approval")
	err := Refusal(author, "admiral changeset", csID)
	assert.Equal(t, "The changeset's author never counts toward its approvals; someone else must run 'admiral changeset approve "+csID+"'.", cmderr.Hint(err))

	needs := status.Error(codes.FailedPrecondition, "the plan needs 1 approval(s) and has 0")
	assert.Contains(t, cmderr.Hint(Refusal(needs, "admiral run", testRunID)), "must run 'admiral run approve "+testRunID+"' first")

	held := status.Error(codes.FailedPrecondition, "change set "+csID+"'s partial apply holds the environment; revert it or accept it first")
	assert.Equal(t, "Run 'admiral changeset revert "+csID+"' to undo it, or 'admiral run accept <run> --reason ...' to accept what landed.",
		cmderr.Hint(Refusal(held, "admiral changeset", "cs-other0000000")))

	scope := status.Error(codes.PermissionDenied, "missing scope run:write")
	assert.Empty(t, cmderr.Hint(Refusal(scope, "admiral run", testRunID)), "a real scope refusal keeps the root's scope hint")
}

func TestPartialOutcomeNamesRevertAndAccept(t *testing.T) {
	err := Outcome(&runv1.Run{Id: testRunID, ChangeSetId: csID, Status: runv1.Run_STATUS_PARTIAL})
	assert.Equal(t, "Run 'admiral changeset revert "+csID+"' to undo it, or 'admiral run accept "+testRunID+" --reason ...' to accept what landed.",
		cmderr.Hint(err))
}

func execRun(t *testing.T, args ...string) error {
	t.Helper()
	t.Setenv(credentials.EnvAPIKey, "")
	root := NewRunCmd(&client.Options{ConfigDir: t.TempDir()})
	var out bytes.Buffer
	root.Cmd.SetOut(&out)
	root.Cmd.SetErr(&out)
	root.Cmd.SetArgs(args)
	return root.Cmd.Execute()
}

func TestAcceptAsksFirst(t *testing.T) {
	err := execRun(t, "accept", testRunID, "--reason", "fixing forward")
	require.EqualError(t, err, "--yes required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))

	err = execRun(t, "accept", testRunID, "--reason", "fixing forward", "--yes")
	require.ErrorIs(t, err, credentials.ErrNotAuthenticated, "accepted, and stopped only at sign-in")
}

func TestListHasNoStatusFilter(t *testing.T) {
	require.ErrorContains(t, execRun(t, "list", "--status", "failed"), "unknown flag: --status")
}

// A run names what it shipped by its changeset's title, and by id alone when
// the title could not be read.
func TestRunShowsItsChangesetTitle(t *testing.T) {
	assert.Contains(t, renderTitled(t, Print, planned(nil), "admiral-site 9d114fd"), `Changeset:    `+csID+` "admiral-site 9d114fd"`)
	assert.Regexp(t, `Changeset:\s+`+csID+`\n`, render(t, Print, planned(nil)))
}

// A plan that found nothing has no diff to point at.
func TestNoChangesRunHasNoDiffHint(t *testing.T) {
	r := &runv1.Run{Id: testRunID, Status: runv1.Run_STATUS_NO_CHANGES, Plan: &runv1.PlanPhase{PlanDigest: "sha256:1"}}
	assert.NotContains(t, render(t, Print, r), "run diff")
}

// Before approving, a takeover counts what it takes, a Kubernetes create
// counts what it sets, and resources changed the same way share one body.
// The whole diff is one command away.
func TestWriteBriefDiffFoldsWhatRepeats(t *testing.T) {
	helmOff := `{"metadata":{"annotations":{"meta.helm.sh/release-name":"web"},"labels":{}}}`
	admiralOn := `{"metadata":{"annotations":{},"labels":{"admiral.io/environment":"env-1"}}}`
	taken := func(paths ...string) *agentv1.ResourceDiff_Kubernetes {
		k := &agentv1.KubernetesChange{}
		for _, p := range paths {
			k.Taken = append(k.Taken, &agentv1.TakenField{Manager: "helm", Path: path("spec", p)})
		}
		return &agentv1.ResourceDiff_Kubernetes{Kubernetes: k}
	}
	takeOver := agentv1.ResourceChange_ACTION_TAKE_OVER
	d := &agentv1.PlanDiff{Resources: []*agentv1.ResourceDiff{
		{Component: "web", Resource: k8sRef("Deployment", "web"), Action: takeOver,
			Before: obj(t, `{"metadata":{"annotations":{"meta.helm.sh/release-name":"web"},"labels":{}},"spec":{"image":"web:latest"}}`),
			After:  obj(t, `{"metadata":{"annotations":{},"labels":{"admiral.io/environment":"env-1"}},"spec":{"image":"web:abc"}}`),
			Change: taken("image", "replicas", "selector")},
		{Component: "web", Resource: k8sRef("Job", "migrate"), Action: agentv1.ResourceChange_ACTION_CREATE,
			After: obj(t, `{"spec":{"backoffLimit":2,"dnsPolicy":"ClusterFirst"}}`)},
		{Component: "web", Resource: k8sRef("Service", "web"), Action: takeOver,
			Before: obj(t, helmOff), After: obj(t, admiralOn), Change: taken("ports", "selector")},
		{Component: "web", Resource: k8sRef("ConfigMap", "web"), Action: takeOver,
			Before: obj(t, helmOff), After: obj(t, admiralOn), Change: taken("data")},
	}}
	var b bytes.Buffer
	WriteBriefDiff(&b, d, nil, testRunID)
	out := b.String()

	assert.Contains(t, out, "~ web: v1 Deployment podinfo-dev/web (take over)\n")
	assert.Contains(t, out, `    spec.image: "web:latest" -> "web:abc"`)
	assert.Contains(t, out, "    takes 3 fields from helm\n")
	assert.NotContains(t, out, "takes spec.", "no field is named one by one")

	assert.Contains(t, out, "+ web: v1 Job podinfo-dev/migrate (create)\n    sets 2 fields\n")
	assert.NotContains(t, out, "dnsPolicy", "a create's defaults stay out")

	assert.Contains(t, out, "~ web: 2 resources (take over)\n"+
		"    v1 Service podinfo-dev/web\n"+
		"    v1 ConfigMap podinfo-dev/web\n"+
		"  on each:\n")
	assert.Equal(t, 2, strings.Count(out, `metadata.labels."admiral.io/environment": <absent> -> "env-1"`),
		"once for the Deployment, once for the two that share it")
	assert.Contains(t, out, "    takes its fields from helm, 3 in all\n")
	assert.Contains(t, out, "\nEvery field: admiral run diff "+testRunID+"\n")
}

// A diff with nothing left out says nothing about where the rest is.
func TestWriteBriefDiffPointsAtTheWholeDiffOnlyWhenItCutsSomething(t *testing.T) {
	d := &agentv1.PlanDiff{Resources: []*agentv1.ResourceDiff{
		{Component: "web", Resource: k8sRef("ConfigMap", "web"), Action: agentv1.ResourceChange_ACTION_UPDATE,
			Before: obj(t, `{"data":{"a":"1"}}`), After: obj(t, `{"data":{"a":"2"}}`)},
	}}
	var b bytes.Buffer
	WriteBriefDiff(&b, d, nil, testRunID)
	assert.Equal(t, writeDiff(d, nil), b.String())
}
