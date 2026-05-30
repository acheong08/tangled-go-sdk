package tangled

import (
	"context"
	"fmt"
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
	// PDSHost is the optional PDS host (defaults to https://bsky.social).
	PDSHost string
}

// NewClient creates a new Tangled client by authenticating with the given config.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	pdsHost := cfg.PDSHost
	if pdsHost == "" {
		pdsHost = DefaultPDS
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

// DID returns the authenticated user's DID.
func (c *Client) DID() string {
	return c.did
}

// Handle returns the authenticated user's handle.
func (c *Client) Handle() string {
	return c.handle
}

// getServiceToken obtains a service auth token for authenticating with Tangled knots.
func (c *Client) getServiceToken(ctx context.Context) (string, error) {
	out, err := comatproto.ServerGetServiceAuth(ctx, c.xrpc, TangledDID, 0, "")
	if err != nil {
		return "", fmt.Errorf("failed to get service auth token: %w", err)
	}
	return out.Token, nil
}

// ResolveRepo resolves an "owner/repo" identifier to a RepoInfo containing
// the knot hostname and repo details.
func (c *Client) ResolveRepo(ctx context.Context, ownerSlashRepo string) (*RepoInfo, error) {
	if !strings.Contains(ownerSlashRepo, "/") {
		return nil, fmt.Errorf("invalid repo format: %q, expected 'owner/repo'", ownerSlashRepo)
	}

	owner, repoName, _ := strings.Cut(ownerSlashRepo, "/")
	owner = strings.TrimPrefix(owner, "@")

	// Resolve owner handle to DID
	var ownerDID string
	if strings.HasPrefix(owner, "did:") {
		ownerDID = owner
	} else {
		resp, err := comatproto.IdentityResolveHandle(ctx, c.xrpc, owner)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve handle %q: %w", owner, err)
		}
		ownerDID = resp.Did
	}

	// List repo records to find the matching repo
	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionRepo, "", 100, ownerDID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list repos for %q: %w", owner, err)
	}

	for _, rec := range records.Records {
		valMap := recordToMap(rec)
		name, _ := valMap["name"].(string)
		if name != repoName {
			continue
		}

		knot, _ := valMap["knot"].(string)
		if knot == "" {
			return nil, fmt.Errorf("repo %q has no knot information", repoName)
		}

		var labels []string
		if raw, ok := valMap["labels"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					labels = append(labels, s)
				}
			}
		}

		desc, _ := valMap["description"].(string)
		repoDID, _ := valMap["repoDid"].(string)

		info := &RepoInfo{
			Name:        repoName,
			Knot:        knot,
			ATURI:        rec.Uri,
			DID:         ownerDID,
			Labels:      labels,
			Description: desc,
			RepoDID:     repoDID,
		}
		return info, nil
	}

	return nil, fmt.Errorf("repo %q not found for owner %q", repoName, owner)
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
	if err := jsonUnmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// decodeRecordForWrite marshals a map to JSON and wraps it in a LexiconTypeDecoder
// suitable for use with RepoPutRecord.
func decodeRecordForWrite(record map[string]any) (*lexutil.LexiconTypeDecoder, error) {
	recordBytes, err := jsonMarshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}

	var decoded lexutil.LexiconTypeDecoder
	if err := decoded.UnmarshalJSON(recordBytes); err != nil {
		return nil, fmt.Errorf("failed to decode record: %w", err)
	}
	return &decoded, nil
}
