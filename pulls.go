package tangled

import (
	"context"
	"fmt"
	"strings"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
)

// ListPulls lists pull requests created by the authenticated user for the
// specified repository. Note: Tangled stores PRs in the creator's PDS, so
// this only returns PRs that the authenticated user created.
func (c *Client) ListPulls(ctx context.Context, ownerSlashRepo string, limit int) ([]*Pull, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionPull, "", int64(limit), c.did, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list pulls: %w", err)
	}

	var pulls []*Pull
	for _, rec := range records.Records {
		m := recordToMap(rec)

		// Check if this pull targets the requested repo
		targetMap, _ := m["target"].(map[string]any)
		if targetMap == nil {
			continue
		}

		targetRepo, _ := targetMap["repo"].(string)
		// The target repo can be either the AT-URI or the owner's DID
		// depending on the record format version
		matchesRepo := targetRepo == repoInfo.ATURI || targetRepo == repoInfo.DID
		if !matchesRepo {
			continue
		}

		pull := &Pull{
			URI:       rec.Uri,
			CID:       rec.Cid,
			Title:     jsonStr(m, "title"),
			Body:      jsonStr(m, "body"),
			CreatedAt: jsonStr(m, "createdAt"),
		}

		// Parse source
		if sourceMap, ok := m["source"].(map[string]any); ok {
			pull.Source.Branch, _ = sourceMap["branch"].(string)
			pull.Source.SHA, _ = sourceMap["sha"].(string)
			pull.Source.Repo, _ = sourceMap["repo"].(string)
		}

		// Parse target
		pull.Target.Repo = targetRepo
		pull.Target.Branch, _ = targetMap["branch"].(string)
		pull.Target.RepoDID, _ = targetMap["repoDid"].(string)

		pulls = append(pulls, pull)
	}

	return pulls, nil
}

// CreatePull creates a new pull request on the specified repository.
// The source branch must already be pushed to the repo (or your fork).
//
// Note: Creating a PR on Tangled requires:
//  1. The source branch to already exist on a knot
//  2. The record to be written to the creator's PDS
//
// This method creates the PR record. The first round's patch must be provided
// or the PR will be created with an empty rounds list (to be submitted later).
func (c *Client) CreatePull(ctx context.Context, ownerSlashRepo string, params CreatePullParams) (*Pull, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	if params.TargetBranch == "" {
		params.TargetBranch = "main"
	}

	// Build the target object based on the newer repoDID format if available
	targetRepoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		targetRepoRef = repoInfo.RepoDID
	}

	target := map[string]any{
		"repo":   targetRepoRef,
		"branch": params.TargetBranch,
	}
	if repoInfo.RepoDID != "" {
		target["repoDid"] = repoInfo.RepoDID
	}

	source := map[string]any{
		"branch": params.SourceBranch,
	}

	rkey := generateTID()
	createdAt := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	record := map[string]any{
		"$type":     CollectionPull,
		"title":     params.Title,
		"target":    target,
		"source":    source,
		"rounds":    []any{}, // Empty rounds; patch can be submitted later
		"createdAt": createdAt,
	}
	if params.Body != "" {
		record["body"] = params.Body
	}

	decoded, err := decodeRecordForWrite(record)
	if err != nil {
		return nil, err
	}

	out, err := comatproto.RepoPutRecord(ctx, c.xrpc, &comatproto.RepoPutRecord_Input{
		Collection: CollectionPull,
		Repo:       c.did,
		Rkey:       rkey,
		Record:     decoded,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	pull := &Pull{
		URI:       out.Uri,
		CID:       out.Cid,
		Title:     params.Title,
		Body:      params.Body,
		Source:    PullSource{Branch: params.SourceBranch},
		Target:    PullTarget{Repo: targetRepoRef, Branch: params.TargetBranch, RepoDID: repoInfo.RepoDID},
		CreatedAt: createdAt,
	}

	return pull, nil
}

// GetPull retrieves a specific pull request by its AT-URI rkey.
func (c *Client) GetPull(ctx context.Context, rkey string) (*Pull, error) {
	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionPull, "", 100, c.did, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list pulls: %w", err)
	}

	for _, rec := range records.Records {
		// Check if this record matches the requested rkey
		parts := strings.Split(rec.Uri, "/")
		if parts[len(parts)-1] != rkey {
			continue
		}

		m := recordToMap(rec)
		pull := &Pull{
			URI:       rec.Uri,
			CID:       rec.Cid,
			Title:     jsonStr(m, "title"),
			Body:      jsonStr(m, "body"),
			CreatedAt: jsonStr(m, "createdAt"),
		}

		if sourceMap, ok := m["source"].(map[string]any); ok {
			pull.Source.Branch, _ = sourceMap["branch"].(string)
			pull.Source.SHA, _ = sourceMap["sha"].(string)
		}

		if targetMap, ok := m["target"].(map[string]any); ok {
			pull.Target.Repo, _ = targetMap["repo"].(string)
			pull.Target.Branch, _ = targetMap["branch"].(string)
			pull.Target.RepoDID, _ = targetMap["repoDid"].(string)
		}

		return pull, nil
	}

	return nil, fmt.Errorf("pull request with rkey %q not found", rkey)
}
