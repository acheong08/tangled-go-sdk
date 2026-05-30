package tangled

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
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

// httpClient is the shared HTTP client with sensible timeouts.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// Client is a Tangled API client that authenticates via AT Protocol
// and provides operations for issues, pull requests, and branches.
type Client struct {
	xrpc      *xrpc.Client
	did       string
	handle    string
	pdsHost   string
	accessJWT string
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
		xrpc:      xrpcClient,
		did:       session.Did,
		handle:    session.Handle,
		pdsHost:   pdsHost,
		accessJWT: session.AccessJwt,
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

// requireAuth returns an error if the client is not authenticated.
func (c *Client) requireAuth() error {
	if !c.IsAuthenticated() {
		return fmt.Errorf("authentication required")
	}
	return nil
}

// getServiceToken obtains a service auth token for authenticating with Tangled knots.
func (c *Client) getServiceToken(ctx context.Context) (string, error) {
	if err := c.requireAuth(); err != nil {
		return "", err
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

	ownerDID, err := c.resolveDID(ctx, owner)
	if err != nil {
		return nil, err
	}

	pdsURL, err := resolvePDS(ctx, ownerDID)
	if err != nil {
		return nil, err
	}

	// Always use raw HTTP — avoids indigo's LexiconTypeDecoder which requires
	// registered types that Tangled collections don't have.
	records, err := pdsListRecords(ctx, pdsURL, ownerDID, CollectionRepo, 100, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list repos for %q: %w", owner, err)
	}

	// When there are multiple repo records with the same name (e.g., a newer
	// rkey-based one and an older TID-based one), prefer the one with an explicit
	// "name" field — it's the original record that issues/PRs reference via AT-URI.
	var fallback *RepoInfo
	var altURIs []string

	for _, rec := range records {
		name, _ := rec.Value["name"].(string)

		// Newer Tangled repos use the rkey as the repo name and omit the "name" field.
		// Older repos use a TID rkey with an explicit "name" field.
		if name == "" {
			rkey, _ := extractRkey(rec.URI)
			name = rkey
		}

		if name != repoName {
			continue
		}

		info := buildRepoInfo(rec.URI, rec.CID, rec.Value, ownerDID, repoName)
		altURIs = append(altURIs, rec.URI)

		// If this record has an explicit "name" field, it's the original/canonical record.
		if _, hasName := rec.Value["name"]; hasName {
			info.AltURIs = altURIs // might include self but that's ok
			return info, nil
		}

		// Otherwise, keep it as fallback (newer rkey-based record without name field).
		if fallback == nil {
			fallback = info
		}
	}

	if fallback != nil {
		fallback.AltURIs = altURIs
		return fallback, nil
	}

	return nil, fmt.Errorf("repo %q not found for owner %q", repoName, owner)
}

// ListMyRepos lists the authenticated user's Tangled repositories.
func (c *Client) ListMyRepos(ctx context.Context) ([]*RepoInfo, error) {
	if err := c.requireAuth(); err != nil {
		return nil, err
	}

	records, err := c.pdsListRecords(ctx, CollectionRepo, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list repos: %w", err)
	}

	var repos []*RepoInfo
	// Deduplicate: prefer records with an explicit "name" field (canonical).
	seen := make(map[string]int) // name -> index into repos

	for _, rec := range records {
		name, _ := rec.Value["name"].(string)
		knot, _ := rec.Value["knot"].(string)

		// Newer Tangled repos use the rkey as the repo name and omit the "name" field.
		if name == "" {
			rkey, _ := extractRkey(rec.URI)
			name = rkey
		}

		if name == "" || knot == "" {
			continue
		}

		info := buildRepoInfo(rec.URI, rec.CID, rec.Value, c.did, name)

		if idx, exists := seen[name]; exists {
			// Prefer the record with an explicit "name" field (canonical/older)
			if _, hasName := rec.Value["name"]; hasName {
				repos[idx] = info
			}
		} else {
			seen[name] = len(repos)
			repos = append(repos, info)
		}
	}
	return repos, nil
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
	return resolveHandlePublic(ctx, owner)
}

// ---- Raw PDS HTTP helpers ----
// These bypass indigo's xrpc type system entirely, since Tangled's custom
// lexicon types (sh.tangled.*) aren't registered with indigo's LexiconTypeDecoder.

// pdsRecord represents a single record from the PDS listRecords API.
type pdsRecord struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid"`
	Value map[string]any `json:"value"`
}

// pdsListRecordsAuthenticated lists records from the authenticated user's PDS.
func (c *Client) pdsListRecords(ctx context.Context, collection string, limit int) ([]pdsRecord, error) {
	pdsURL, err := resolvePDS(ctx, c.did)
	if err != nil {
		return nil, err
	}
	return pdsListRecords(ctx, pdsURL, c.did, collection, limit, c.accessJWT)
}

// pdsListRecords lists records via raw HTTP to a PDS endpoint.
// If accessToken is empty, the request is made without authentication.
func pdsListRecords(ctx context.Context, pdsURL, repo, collection string, limit int, accessToken string) ([]pdsRecord, error) {
	url := fmt.Sprintf("%s/xrpc/com.atproto.repo.listRecords?repo=%s&collection=%s&limit=%d",
		pdsURL, repo, collection, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PDS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PDS returned HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Records []pdsRecord `json:"records"`
		Cursor  *string     `json:"cursor,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode PDS response: %w", err)
	}
	return result.Records, nil
}

// pdsPutRecord creates or replaces a record via raw HTTP to a PDS endpoint.
func (c *Client) pdsPutRecord(ctx context.Context, collection, rkey string, record map[string]any, swapRecord *string) (uri, cid string, err error) {
	if err := c.requireAuth(); err != nil {
		return "", "", err
	}

	pdsURL, err := resolvePDS(ctx, c.did)
	if err != nil {
		return "", "", err
	}

	body := map[string]any{
		"repo":       c.did,
		"collection":  collection,
		"rkey":        rkey,
		"record":      record,
	}
	if swapRecord != nil {
		body["swapRecord"] = *swapRecord
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := pdsURL + "/xrpc/com.atproto.repo.putRecord"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessJWT)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("PDS putRecord failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("PDS putRecord returned HTTP %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode putRecord response: %w", err)
	}
	return result.URI, result.CID, nil
}

// pdsDeleteRecord deletes a record via raw HTTP to a PDS endpoint.
func (c *Client) pdsDeleteRecord(ctx context.Context, collection, rkey string) error {
	if err := c.requireAuth(); err != nil {
		return err
	}

	pdsURL, err := resolvePDS(ctx, c.did)
	if err != nil {
		return err
	}

	body := map[string]any{
		"repo":       c.did,
		"collection":  collection,
		"rkey":        rkey,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := pdsURL + "/xrpc/com.atproto.repo.deleteRecord"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessJWT)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PDS deleteRecord failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PDS deleteRecord returned HTTP %d: %s", resp.StatusCode, respBody)
	}
	return nil
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

// discoverPDS resolves a handle to its PDS host by: handle → DID → PLC directory.
func discoverPDS(ctx context.Context, handle string) (string, error) {
	did, err := resolveHandlePublic(ctx, handle)
	if err != nil {
		return DefaultPDS, fmt.Errorf("resolve handle: %w", err)
	}
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
	resp, err := httpClient.Do(req)
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
	if !strings.HasPrefix(did, "did:plc:") && !strings.HasPrefix(did, "did:web:") {
		return DefaultPDS, nil
	}

	if strings.HasPrefix(did, "did:web:") {
		domain := strings.TrimPrefix(did, "did:web:")
		url := "https://" + domain + "/.well-known/did.json"
		return resolvePDSFromDIDDocument(ctx, url)
	}

	// did:plc — use PLC directory
	url := "https://plc.directory/" + did
	return resolvePDSFromDIDDocument(ctx, url)
}

// resolvePDSFromDIDDocument fetches a DID document and extracts the PDS service endpoint.
func resolvePDSFromDIDDocument(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return DefaultPDS, nil
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return DefaultPDS, nil
	}
	defer resp.Body.Close()

	var doc struct {
		Service []struct {
			ID              string `json:"id"`
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

// extractRkey extracts the record key from an AT-URI.
// AT-URI format: at://did:plc:.../collection/rkey
func extractRkey(uri string) (string, error) {
	parts := strings.SplitN(uri, "/", 5)
	if len(parts) < 5 {
		return "", fmt.Errorf("invalid AT-URI: %q", uri)
	}
	return parts[4], nil
}

// repoRefMatches checks if the given repo reference (from an issue/PR record) matches
// the resolved RepoInfo. The repo field can be a DID, a RepoDID, the primary AT-URI,
// or any alternate AT-URI for the same repo.
func repoRefMatches(ref string, repoInfo *RepoInfo) bool {
	if ref == repoInfo.DID || ref == repoInfo.RepoDID || ref == repoInfo.ATURI {
		return true
	}
	for _, alt := range repoInfo.AltURIs {
		if ref == alt {
			return true
		}
	}
	return false
}
