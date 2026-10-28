package agent

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/cmderr"
	"go.admiral.io/cli/internal/credentials"
	"go.admiral.io/cli/internal/output"
	sdkclient "go.admiral.io/sdk/client"
	agentv1 "go.admiral.io/sdk/proto/admiral/api/agent/v1"
	serviceaccountv1 "go.admiral.io/sdk/proto/admiral/api/serviceaccount/v1"
)

const (
	agentID = "agt-4m8q2x7k1p9c"
	saID    = "sa-7f2a1c9d0e3b"
	keyText = "adme_3k9x2m7qpl0a5f2c"
)

// run executes the agent tree with no stored credential, so a command that
// gets past its own checks fails at client creation.
func run(t *testing.T, args ...string) error {
	t.Helper()
	os.Unsetenv(credentials.EnvAPIKey)
	root := NewAgentCmd(&client.Options{ConfigDir: t.TempDir()})
	root.Cmd.SetOut(&bytes.Buffer{})
	root.Cmd.SetErr(&bytes.Buffer{})
	root.Cmd.SetArgs(args)
	return root.Cmd.Execute()
}

type fakeClient struct {
	sdkclient.AdmiralClient
	agents *agentClient
	sas    *saClient
}

func (f *fakeClient) Agent() agentv1.AgentAPIClient                            { return f.agents }
func (f *fakeClient) ServiceAccount() serviceaccountv1.ServiceAccountAPIClient { return f.sas }

type agentClient struct {
	agentv1.AgentAPIClient
	req  *agentv1.CreateAgentRequest
	resp *agentv1.CreateAgentResponse
}

func (m *agentClient) CreateAgent(_ context.Context, req *agentv1.CreateAgentRequest, _ ...grpc.CallOption) (*agentv1.CreateAgentResponse, error) {
	m.req = req
	return m.resp, nil
}

type saClient struct {
	serviceaccountv1.ServiceAccountAPIClient
	sa  *serviceaccountv1.ServiceAccount
	err error
}

func (m *saClient) GetServiceAccount(_ context.Context, req *serviceaccountv1.GetServiceAccountRequest, _ ...grpc.CallOption) (*serviceaccountv1.GetServiceAccountResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if req.GetServiceAccountId() != m.sa.GetId() {
		return nil, assert.AnError
	}
	return &serviceaccountv1.GetServiceAccountResponse{ServiceAccount: m.sa}, nil
}

// The kind is what the agent deploys and cannot be changed later, so it is
// never defaulted.
func TestCreateRequiresKind(t *testing.T) {
	err := run(t, "create", "dev")
	require.EqualError(t, err, `required flag(s) "kind" not set`)

	err = run(t, "create", "dev", "--kind", "terraform")
	require.ErrorContains(t, err, "must be one of kubernetes, infra")

	for _, gone := range []string{"--provider", "--subject"} {
		err = run(t, "create", "dev", "--kind", "kubernetes", gone, "x")
		require.ErrorContains(t, err, "unknown flag: "+gone)
	}

	for _, kind := range []string{"kubernetes", "infra"} {
		err = run(t, "create", "dev", "--kind", kind)
		require.ErrorIs(t, err, credentials.ErrNotAuthenticated, kind)
	}
}

func TestProviderCommandIsGone(t *testing.T) {
	err := run(t, "provider", "list")
	require.Error(t, err)
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
}

func create(t *testing.T, kind agentv1.Agent_Kind, expires time.Time) (string, *agentClient) {
	t.Helper()
	agents := &agentClient{resp: &agentv1.CreateAgentResponse{
		Agent:         &agentv1.Agent{Id: agentID, Name: "dev", Kind: kind, ServiceAccountId: saID, Health: agentv1.Agent_HEALTH_NEW},
		EnrollmentKey: &agentv1.EnrollmentKey{Secret: keyText, ExpiresAt: timestamppb.New(expires)},
	}}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	req := &agentv1.CreateAgentRequest{Agent: &agentv1.Agent{Name: "dev", Kind: kind}}
	require.NoError(t, createAgent(cmd, &fakeClient{agents: agents}, output.FormatTable, req))
	return out.String(), agents
}

// The answer is the agent's id, the key with the expiry the server set (not
// one the CLI assumes), and an install that leaves the service account name,
// the server address and the image to the chart.
func TestCreatePrintsTheKeyAndTheInstall(t *testing.T) {
	expires := time.Date(2026, 10, 14, 18, 7, 0, 0, time.UTC)
	out, agents := create(t, agentv1.Agent_KIND_KUBERNETES, expires)

	assert.Equal(t, agentv1.Agent_KIND_KUBERNETES, agents.req.GetAgent().GetKind())
	assert.Contains(t, out, "Agent "+agentID+" created.")
	assert.Contains(t, out, "expires "+output.FormatDescribeTime(timestamppb.New(expires))+"):\n"+keyText+"\n")
	assert.Contains(t, out, "helm install admiral-k8s-agent-dev oci://ghcr.io/admiral-io/admiral-helm/admiral-k8s-agent \\\n")
	assert.Contains(t, out, "kubectl create namespace admiral-system\n")
	assert.Contains(t, out, "kubectl -n admiral-system create secret generic admiral-k8s-agent-dev-enrollment \\\n    --from-literal=key="+keyText+"\n")
	assert.Contains(t, out, "--namespace admiral-system \\\n")
	assert.Contains(t, out, "--set admiral.serviceAccountId="+saID+" \\\n")
	assert.Contains(t, out, "--set enrollment.existingSecret=admiral-k8s-agent-dev-enrollment\n")
	assert.NotContains(t, out, "enrollment.key", "the key never goes into release values")
	for _, never := range []string{"serviceAccount.name", "admiral.server", "image.tag", "--version"} {
		assert.NotContains(t, out, never)
	}

	later := expires.Add(3 * time.Hour)
	out, _ = create(t, agentv1.Agent_KIND_KUBERNETES, later)
	assert.Contains(t, out, "expires "+output.FormatDescribeTime(timestamppb.New(later)))
}

// An infra agent installs from its own chart, under a release whose name
// makes the subject it enrolls with.
func TestCreateInfraPrintsItsOwnChart(t *testing.T) {
	out, _ := create(t, agentv1.Agent_KIND_INFRA, time.Now().Add(time.Hour))
	assert.Contains(t, out, "Agent "+agentID+" created.")
	assert.Contains(t, out, "helm install admiral-infra-agent-dev oci://ghcr.io/admiral-io/admiral-helm/admiral-infra-agent \\\n")
	assert.Contains(t, out, "create secret generic admiral-infra-agent-dev-enrollment \\\n    --from-literal=key="+keyText+"\n")
	assert.NotContains(t, out, "admiral-k8s-agent")
}

func describeErr(sas *saClient) (string, error) {
	var w bytes.Buffer
	a := &agentv1.Agent{Id: agentID, ServiceAccountId: saID}
	d := output.NewDescribe()
	if _, err := addServiceAccount(context.Background(), d, &fakeClient{sas: sas}, a); err != nil {
		return "", err
	}
	err := d.Render(&w)
	return w.String(), err
}

func describe(t *testing.T, sa *serviceaccountv1.ServiceAccount) string {
	t.Helper()
	out, err := describeErr(&saClient{sa: sa})
	require.NoError(t, err)
	return out
}

// Someone who may read agents but not service accounts still gets the
// agent; any other failure is still an error.
func TestGetWithoutServiceAccountRead(t *testing.T) {
	out, err := describeErr(&saClient{err: status.Error(codes.PermissionDenied, "missing scope")})
	require.NoError(t, err)
	assert.Regexp(t, `Service Account:\n\s+ID:\s+`+saID, out)
	assert.Regexp(t, `Enrolled:\s+needs serviceaccount:read`, out)
	assert.NotContains(t, out, "Subject")

	_, err = describeErr(&saClient{err: status.Error(codes.Unavailable, "down")})
	require.Equal(t, codes.Unavailable, status.Code(err))
}

func TestGetShowsEnrollment(t *testing.T) {
	out := describe(t, &serviceaccountv1.ServiceAccount{Id: saID, AgentId: agentID})
	assert.Contains(t, out, saID)
	assert.Regexp(t, `Enrolled:\s+not enrolled`, out)
	assert.NotContains(t, out, "Subject")

	subject := "system:serviceaccount:admiral-system:admiral-k8s-agent-dev"
	out = describe(t, &serviceaccountv1.ServiceAccount{
		Id: saID, AgentId: agentID, IssuerId: "iss-9c2k4m7x1q0p", Subject: subject,
		EnrolledAt: timestamppb.New(time.Date(2026, 10, 14, 17, 10, 0, 0, time.UTC)),
	})
	assert.NotContains(t, out, "not enrolled")
	assert.Regexp(t, `Subject:\s+`+subject, out)
	assert.Regexp(t, `Issuer:\s+iss-9c2k4m7x1q0p`, out)
}

// An offline agent says how long it has been gone; one that never reported
// says so rather than showing nothing.
func TestLastSeen(t *testing.T) {
	assert.Equal(t, "never", lastSeen(&agentv1.Agent{}))
	assert.Equal(t, "never", lastSeen(&agentv1.Agent{Report: &agentv1.AgentReport{AgentVersion: "0.4.2"}}))
	a := &agentv1.Agent{Report: &agentv1.AgentReport{ReportedAt: timestamppb.New(time.Now().Add(-3 * time.Hour))}}
	assert.Equal(t, "3h ago", lastSeen(a))
}
