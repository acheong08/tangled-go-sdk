package tangled

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/xrpc"
)

const (
	// TangledDID is the DID of the Tangled appview used for service auth.
	TangledDID = "did:web:tangled.org"

	// CollectionRepo is the atproto collection for Tangled repo records.
	CollectionRepo = "sh.tangled.repo"

	// CollectionIssue is the atproto collection for Tangled issue records.
	CollectionIssue = "sh.tangled.repo.issue"

	// CollectionPull is the atproto collection for Tangled pull request records.
	CollectionPull = "sh.tangled.repo.pull"

	// CollectionLabelOp is the atproto collection for Tangled label operation records.
	CollectionLabelOp = "sh.tangled.label.op"

	// DefaultPDS is the default PDS host for Bluesky.
	DefaultPDS = "https://bsky.social"
)

// Client is a Tangled API client that authenticates via AT Protocol
// and provides operations for issues, pull requests, and branches.
type Client struct {
	xrpc    *xrpc.Client
	did     string
	handle  string
	pdsHost string
}

// Config holds the configuration for creating a new Client.
type Config struct {
	// Handle is the AT Protocol handle (e.g., "user.bsky.social").
	Handle string
	// Password is the account password or app password.
	Password string
	// PDSHost is the optional PDS host. If empty, the correct PDS is
	// auto-discovered by resolving the handle's DID via the PLC directory.
	PDSHost string
}

// NewClient creates a new Tangled client by authenticating with the given config.
// If PDSHost is not set, it auto-discovers the user's PDS by resolving the handle
// to a DID and looking up the PLC directory.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	pdsHost := cfg.PDSHost
	if pdsHost == "" {
		var err error
		pdsHost, err = discoverPDS(ctx, cfg.Handle)
		if err != nil {
			return nil, fmt.Errorf("failed to discover PDS for handle %q: %w", cfg.Handle, err)
		}
	}

	xrpcClient := &xrpc.Client{
		Host: pdsHost,
	}

	session, err := comatproto.ServerCreateSession(ctx, xrpcClient, &comatproto.ServerCreateSession_Input{
		Identifier: cfg.Handle,
		Password:   cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	xrpcClient.Auth = &xrpc.AuthInfo{
		AccessJwt:  session.AccessJwt,
		RefreshJwt: session.RefreshJwt,
		Handle:     session.Handle,
		Did:        session.Did,
	}

	return &Client{
		xrpc:    xrpcClient,
		did:     session.Did,
		handle:  session.Handle,
		pdsHost: pdsHost,
	}, nil
}

// NewPublicClient creates a client for read-only public operations (no authentication).
// Operations requiring auth (create/update/delete) will fail. Resolution works by
// querying public PDS endpoints directly.
func NewPublicClient() *Client {
	return &Client{}
}

// DID returns the authenticated user's DID.
func (c *Client) DID() string {
	return c.did
}

// Handle returns the authenticated user's handle.
func (c *Client) Handle() string {
	return c.handle
}

// IsAuthenticated returns whether the client has valid authentication.
func (c *Client) IsAuthenticated() bool {
	return c.xrpc != nil && c.xrpc.Auth != nil
}

// getServiceToken obtains a service auth token for authenticating with Tangled knots.
func (c *Client) getServiceToken(ctx context.Context) (string, error) {
	if !c.IsAuthenticated() {
		return "", fmt.Errorf("authentication required")
	}
	out, err := comatproto.ServerGetServiceAuth(ctx, c.xrpc, TangledDID, 0, "")
	if err != nil {
		return "", fmt.Errorf("failed to get service auth token: %w", err)
	}
	return out.Token, nil
}

// ResolveRepo resolves an "owner/repo" identifier to a RepoInfo containing
// the knot hostname and repo details. Works with both authenticated and public clients.
func (c *Client) ResolveRepo(ctx context.Context, ownerSlashRepo string) (*RepoInfo, error) {
	if !strings.Contains(ownerSlashRepo, "/") {
		return nil, fmt.Errorf("invalid repo format: %q, expected 'owner/repo'", ownerSlashRepo)
	}

	owner, repoName, _ := strings.Cut(ownerSlashRepo, "/")
	owner = strings.TrimPrefix(owner, "@")

	// Resolve owner handle to DID
	ownerDID, err := c.resolveDID(ctx, owner)
	if err != nil {
		return nil, err
	}

	// Query the owner's PDS for repo records
	if c.IsAuthenticated() {
		return c.resolveRepoAuthenticated(ctx, ownerDID, repoName)
	}
	return c.resolveRepoPublic(ctx, ownerDID, repoName)
}

// resolveRepoAuthenticated resolves a repo using an authenticated atproto client.
func (c *Client) resolveRepoAuthenticated(ctx context.Context, ownerDID, repoName string) (*RepoInfo, error) {
	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionRepo, "", 100, ownerDID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list repos: %w", err)
	}

	for _, rec := range records.Records {
		m := recordToMap(rec)
		name, _ := m["name"].(string)
		if name != repoName {
			continue
		}
		return buildRepoInfo(rec.Uri, rec.Cid, m, ownerDID, repoName), nil
	}

	return nil, fmt.Errorf("repo %q not found for owner DID %s", repoName, ownerDID)
}

// resolveRepoPublic resolves a repo using public PDS HTTP endpoints.
func (c *Client) resolveRepoPublic(ctx context.Context, ownerDID, repoName string) (*RepoInfo, error) {
	pdsURL, err := resolvePDS(ctx, ownerDID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/xrpc/com.atproto.repo.listRecords?repo=%s&collection=%s&limit=100",
		pdsURL, ownerDID, CollectionRepo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query PDS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PDS returned HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Records []struct {
			URI   string         `json:"uri"`
			CID   string         `json:"cid"`
			Value map[string]any `json:"value"`
		} `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode PDS response: %w", err)
	}

	for _, rec := range result.Records {
		name, _ := rec.Value["name"].(string)
		if name != repoName {
			continue
		}
		return buildRepoInfo(rec.URI, rec.CID, rec.Value, ownerDID, repoName), nil
	}

	return nil, fmt.Errorf("repo %q not found for owner DID %s", repoName, ownerDID)
}

// buildRepoInfo creates a RepoInfo from a record map.
func buildRepoInfo(uri, cid string, m map[string]any, ownerDID, repoName string) *RepoInfo {
	knot, _ := m["knot"].(string)
	desc, _ := m["description"].(string)
	repoDID, _ := m["repoDid"].(string)

	var labels []string
	if raw, ok := m["labels"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				labels = append(labels, s)
			}
		}
	}

	return &RepoInfo{
		Name:        repoName,
		Knot:        knot,
		ATURI:       uri,
		DID:         ownerDID,
		Labels:      labels,
		Description: desc,
		RepoDID:     repoDID,
	}
}

// resolveDID resolves a handle or DID to a DID.
func (c *Client) resolveDID(ctx context.Context, owner string) (string, error) {
	if strings.HasPrefix(owner, "did:") {
		return owner, nil
	}
	if c.IsAuthenticated() {
		resp, err := comatproto.IdentityResolveHandle(ctx, c.xrpc, owner)
		if err != nil {
			return "", fmt.Errorf("failed to resolve handle %q: %w", owner, err)
		}
		return resp.Did, nil
	}
	// Public resolution via PDS
	return resolveHandlePublic(ctx, owner)
}

// ListMyRepos lists the authenticated user's Tangled repositories.
func (c *Client) ListMyRepos(ctx context.Context) ([]*RepoInfo, error) {
	if !c.IsAuthenticated() {
		return nil, fmt.Errorf("authentication required")
	}
	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionRepo, "", 100, c.did, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list repos: %w", err)
	}

	var repos []*RepoInfo
	for _, rec := range records.Records {
		m := recordToMap(rec)
		name, _ := m["name"].(string)
		knot, _ := m["knot"].(string)
		if name == "" || knot == "" {
			continue
		}
		repos = append(repos, buildRepoInfo(rec.Uri, rec.Cid, m, c.did, name))
	}
	return repos, nil
}

// discoverPDS resolves a handle to its PDS host by: handle → DID → PLC directory.
// Falls back to DefaultPDS if any step fails.
func discoverPDS(ctx context.Context, handle string) (string, error) {
	// Step 1: Resolve handle to DID
	did, err := resolveHandlePublic(ctx, handle)
	if err != nil {
		return DefaultPDS, fmt.Errorf("resolve handle: %w", err)
	}

	// Step 2: Look up PLC directory for PDS endpoint
	pdsURL, err := resolvePDS(ctx, did)
	if err != nil {
		return DefaultPDS, fmt.Errorf("resolve PDS from DID: %w", err)
	}
	return pdsURL, nil
}

// resolveHandlePublic resolves a handle to a DID using the public PDS endpoint.
func resolveHandlePublic(ctx context.Context, handle string) (string, error) {
	url := DefaultPDS + "/xrpc/com.atproto.identity.resolveHandle?handle=" + handle
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var result struct {
		DID string `json:"did"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.DID, nil
}

// resolvePDS finds the PDS URL for a given DID by checking the PLC directory.
func resolvePDS(ctx context.Context, did string) (string, error) {
	if !strings.HasPrefix(did, "did:plc:") {
		return DefaultPDS, nil
	}

	url := "https://plc.directory/" + did
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return DefaultPDS, nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return DefaultPDS, nil
	}
	defer resp.Body.Close()

	var doc struct {
		Service []struct {
			ID             string `json:"id"`
			ServiceEndpoint string `json:"serviceEndpoint"`
		} `json:"service"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return DefaultPDS, nil
	}

	for _, svc := range doc.Service {
		if strings.Contains(svc.ID, "atproto_pds") {
			return svc.ServiceEndpoint, nil
		}
	}

	return DefaultPDS, nil
}

// recordToMap converts a RepoListRecords_Record's value to a map[string]any
// by marshaling the LexiconTypeDecoder to JSON and unmarshaling.
func recordToMap(rec *comatproto.RepoListRecords_Record) map[string]any {
	if rec.Value == nil {
		return nil
	}
	b, err := rec.Value.MarshalJSON()
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// decodeRecordForWrite marshals a map to JSON and wraps it in a LexiconTypeDecoder
// suitable for use with RepoPutRecord.
func decodeRecordForWrite(record map[string]any) (*lexutil.LexiconTypeDecoder, error) {
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}

	var decoded lexutil.LexiconTypeDecoder
	if err := decoded.UnmarshalJSON(recordBytes); err != nil {
		return nil, fmt.Errorf("failed to decode record: %w", err)
	}
	return &decoded, nil
}
