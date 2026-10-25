package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.admiral.io/cli/internal/credentials"
)

// EnvStateHost overrides where hosted state is read from, for installations
// whose state host is not the API host with `api.` swapped for `state.`.
const EnvStateHost = "ADMIRAL_STATE_HOST"

// StateClient reads hosted Terraform state's history from admiral-state. It
// is plain HTTP at its own host, not the Admiral API, and takes the same
// credential as a Bearer token.
type StateClient struct {
	base      string
	http      *http.Client
	configDir string
	token     string
	session   bool
}

// StateVersion is one stored version of a state.
type StateVersion struct {
	ID        string      `json:"id"`
	Serial    int64       `json:"serial"`
	Writer    StateWriter `json:"writer"`
	SizeBytes int64       `json:"size_bytes"`
	Force     bool        `json:"force"`
	CreatedAt time.Time   `json:"created_at"`
}

// StateWriter is a person or a job's attempt.
type StateWriter struct {
	Person *struct {
		Kind    string `json:"kind"`
		Subject string `json:"subject"`
	} `json:"person,omitempty"`
	Job *struct {
		JobID   string `json:"job_id"`
		Attempt int32  `json:"attempt"`
	} `json:"job,omitempty"`
}

type stateVersionPage struct {
	Versions      []*StateVersion `json:"versions"`
	NextPageToken string          `json:"next_page_token"`
}

// NewStateClient resolves the credential the way CreateClient does.
func NewStateClient(ctx context.Context, opts *Options) (*StateClient, error) {
	base, err := StateBaseURL(opts)
	if err != nil {
		return nil, err
	}
	cred, err := credentials.ResolveToken(ctx, opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &StateClient{
		base:      base,
		http:      &http.Client{Timeout: timeout, Transport: &http.Transport{TLSClientConfig: modeFor(opts).tlsConfig()}},
		configDir: opts.ConfigDir,
		token:     cred.Token,
		session:   cred.Source == credentials.SourceSession,
	}, nil
}

// StateBaseURL is the state host for the configured server: the override,
// or the server's host with a leading `api.` replaced by `state.`.
func StateBaseURL(opts *Options) (string, error) {
	scheme := "https"
	if modeFor(opts).plaintext() {
		scheme = "http"
	}
	if h := os.Getenv(EnvStateHost); h != "" {
		return scheme + "://" + h, nil
	}
	host, _, err := net.SplitHostPort(opts.ServerAddr)
	if err != nil {
		host = opts.ServerAddr
	}
	rest, ok := strings.CutPrefix(host, "api.")
	if !ok {
		return "", fmt.Errorf("cannot tell the state host from server %q; set %s", opts.ServerAddr, EnvStateHost)
	}
	return scheme + "://state." + rest, nil
}

func workspacePath(org, workspace string) string {
	return "/v1/organizations/" + url.PathEscape(org) + "/workspaces/" + url.PathEscape(workspace)
}

// ListStateVersions is one page of a workspace's versions, newest first.
func (c *StateClient) ListStateVersions(ctx context.Context, org, workspace string, pageSize int32, pageToken string) ([]*StateVersion, string, error) {
	q := url.Values{}
	if pageSize > 0 {
		q.Set("page_size", strconv.Itoa(int(pageSize)))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	resp, err := c.get(ctx, workspacePath(org, workspace)+"/state-versions?"+q.Encode())
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	var page stateVersionPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, "", fmt.Errorf("reading state versions: %w", err)
	}
	return page.Versions, page.NextPageToken, nil
}

// DownloadStateVersion is the version's raw state. The caller closes it.
func (c *StateClient) DownloadStateVersion(ctx context.Context, org, workspace, versionID string) (io.ReadCloser, error) {
	resp, err := c.get(ctx, workspacePath(org, workspace)+"/state-versions/"+url.PathEscape(versionID)+"/download")
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// get sends the request, refreshing a session once on 401 as CreateClient's
// interceptor does.
func (c *StateClient) get(ctx context.Context, path string) (*http.Response, error) {
	resp, err := c.do(ctx, path)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && c.session {
		_ = resp.Body.Close()
		fresh, rerr := credentials.ForceRefresh(ctx, c.configDir)
		if rerr == nil {
			c.token = fresh.Token
			if resp, err = c.do(ctx, path); err != nil {
				return nil, err
			}
		}
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close() //nolint:errcheck
		return nil, stateError(resp)
	}
	return resp, nil
}

func (c *StateClient) do(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.http.Do(req)
}

// ErrStateNotFound is a workspace or version that does not exist, or that
// the caller may not read.
var ErrStateNotFound = errors.New("not found")

// stateError reads admiral-state's JSON:API error body.
func stateError(resp *http.Response) error {
	var body struct {
		Errors []struct {
			Detail string `json:"detail"`
			Title  string `json:"title"`
		} `json:"errors"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body)
	msg := http.StatusText(resp.StatusCode)
	if len(body.Errors) > 0 {
		if body.Errors[0].Detail != "" {
			msg = body.Errors[0].Detail
		} else if body.Errors[0].Title != "" {
			msg = body.Errors[0].Title
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", ErrStateNotFound, msg)
	}
	return fmt.Errorf("admiral-state: %s", msg)
}
