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

// StateVersion is one stored version of a state, from the HCP Terraform
// API's state-versions object.
type StateVersion struct {
	ID        string    `json:"id"`
	Serial    int64     `json:"serial"`
	SizeBytes int64     `json:"size_bytes"`
	Forced    bool      `json:"forced"`
	CreatedAt time.Time `json:"created_at"`
	// CreatedBy is who wrote it: a user, a service account, or a job's
	// attempt.
	CreatedBy StateWriter `json:"created_by"`
}

// StateWriter is the created-by relationship: its type ("users",
// "service-accounts" or "jobs"), its id, and a job's attempt.
type StateWriter struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Attempt int32  `json:"attempt,omitempty"`
}

type stateVersionDoc struct {
	ID         string `json:"id"`
	Attributes struct {
		Serial    int64     `json:"serial"`
		Size      int64     `json:"size"`
		Forced    bool      `json:"forced"`
		CreatedAt time.Time `json:"created-at"`
	} `json:"attributes"`
	Relationships struct {
		CreatedBy struct {
			Data struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"data"`
			Meta struct {
				Attempt int32 `json:"attempt"`
			} `json:"meta"`
		} `json:"created-by"`
	} `json:"relationships"`
}

type stateVersionList struct {
	Data []stateVersionDoc `json:"data"`
	Meta struct {
		Pagination struct {
			NextPage *int `json:"next-page"`
		} `json:"pagination"`
	} `json:"meta"`
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
		base: base,
		http: &http.Client{Timeout: timeout, Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment, TLSClientConfig: modeFor(opts).tlsConfig(),
		}},
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

// ListStateVersions is one page of a workspace's finalized versions, newest
// first, through the documented state-versions list. pageToken is the page
// number the previous call returned; next is "" on the last page.
func (c *StateClient) ListStateVersions(ctx context.Context, org, workspace string, pageSize int32, pageToken string) ([]*StateVersion, string, error) {
	q := url.Values{}
	q.Set("filter[organization][name]", org)
	q.Set("filter[workspace][name]", workspace)
	if pageSize > 0 {
		q.Set("page[size]", strconv.Itoa(int(pageSize)))
	}
	if pageToken != "" {
		q.Set("page[number]", pageToken)
	}
	resp, err := c.get(ctx, "/api/v2/state-versions?"+q.Encode())
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	var list stateVersionList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, "", fmt.Errorf("reading state versions: %w", err)
	}
	out := make([]*StateVersion, len(list.Data))
	for i, d := range list.Data {
		out[i] = &StateVersion{
			ID: d.ID, Serial: d.Attributes.Serial, SizeBytes: d.Attributes.Size,
			Forced: d.Attributes.Forced, CreatedAt: d.Attributes.CreatedAt,
			CreatedBy: StateWriter{
				Type: d.Relationships.CreatedBy.Data.Type, ID: d.Relationships.CreatedBy.Data.ID,
				Attempt: d.Relationships.CreatedBy.Meta.Attempt,
			},
		}
	}
	next := ""
	if n := list.Meta.Pagination.NextPage; n != nil {
		next = strconv.Itoa(*n)
	}
	return out, next, nil
}

// DownloadStateVersion is a version's raw state. The caller closes it.
func (c *StateClient) DownloadStateVersion(ctx context.Context, versionID string) (io.ReadCloser, error) {
	resp, err := c.get(ctx, "/api/v2/state-versions/"+url.PathEscape(versionID)+"/download")
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
