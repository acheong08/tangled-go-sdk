package tangled

import (
	"context"
	"fmt"
)

// ListPulls lists pull requests for the specified repository.
// With an authenticated client, it queries the user's PDS for PRs they created.
// With a public client, it falls back to ListPublicPulls (queries repo owner's PDS).
func (c *Client) ListPulls(ctx context.Context, ownerSlashRepo string, limit int) ([]*Pull, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	if c.IsAuthenticated() {
		return c.listPullsAuthenticated(ctx, repoInfo, limit)
	}
	return c.ListPublicPulls(ctx, ownerSlashRepo, limit)
}

func (c *Client) listPullsAuthenticated(ctx context.Context, repoInfo *RepoInfo, limit int) ([]*Pull, error) {
	records, err := c.pdsListRecords(ctx, CollectionPull, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list pulls: %w", err)
	}

	repoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		repoRef = repoInfo.RepoDID
	}

	var pulls []*Pull
	for _, rec := range records {
		targetMap, _ := rec.Value["target"].(map[string]any)
		if targetMap == nil {
			continue
		}

		targetRepo, _ := targetMap["repo"].(string)
		if targetRepo != repoRef && targetRepo != repoInfo.ATURI {
			continue
		}

		pull := rawRecordToPull(rec)
		pulls = append(pulls, pull)
	}

	return pulls, nil
}

// CreatePull creates a new pull request on the specified repository.
func (c *Client) CreatePull(ctx context.Context, ownerSlashRepo string, params CreatePullParams) (*Pull, error) {
	if err := c.requireAuth(); err != nil {
		return nil, err
	}

	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	if params.TargetBranch == "" {
		params.TargetBranch = "main"
	}

	// Use RepoDID for the target repo field (DID format per lexicon)
	targetRepoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		targetRepoRef = repoInfo.RepoDID
	}

	target := map[string]any{
		"repo":   targetRepoRef,
		"branch": params.TargetBranch,
	}

	source := map[string]any{
		"branch": params.SourceBranch,
	}

	rkey := generateTID()
	createdAt := nowISO()

	record := map[string]any{
		"$type":     CollectionPull,
		"title":     params.Title,
		"target":    target,
		"source":    source,
		"rounds":    []any{},
		"createdAt": createdAt,
	}
	if params.Body != "" {
		record["body"] = params.Body
	}

	uri, cid, err := c.pdsPutRecord(ctx, CollectionPull, rkey, record, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	return &Pull{
		URI:       uri,
		CID:       cid,
		Title:     params.Title,
		Body:      params.Body,
		Source:    PullSource{Branch: params.SourceBranch},
		Target:    PullTarget{Repo: targetRepoRef, Branch: params.TargetBranch},
		CreatedAt: createdAt,
	}, nil
}

// GetPull retrieves a specific pull request by its AT-URI rkey.
func (c *Client) GetPull(ctx context.Context, rkey string) (*Pull, error) {
	if err := c.requireAuth(); err != nil {
		return nil, err
	}

	records, err := c.pdsListRecords(ctx, CollectionPull, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list pulls: %w", err)
	}

	for _, rec := range records {
		recRkey, err := extractRkey(rec.URI)
		if err != nil || recRkey != rkey {
			continue
		}

		pull := rawRecordToPull(rec)
		return pull, nil
	}

	return nil, fmt.Errorf("pull request with rkey %q not found", rkey)
}

// ListPublicPulls lists pull requests on a repository using public PDS queries.
func (c *Client) ListPublicPulls(ctx context.Context, ownerSlashRepo string, limit int) ([]*Pull, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	pdsURL, err := resolvePDS(ctx, repoInfo.DID)
	if err != nil {
		return nil, err
	}

	var records []pdsRecord
	if c.IsAuthenticated() {
		// Authenticated: query owner's PDS with our token
		records, err = pdsListRecords(ctx, pdsURL, repoInfo.DID, CollectionPull, limit, c.accessJWT)
	} else {
		records, err = pdsListRecords(ctx, pdsURL, repoInfo.DID, CollectionPull, limit, "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query PDS: %w", err)
	}

	repoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		repoRef = repoInfo.RepoDID
	}

	var pulls []*Pull
	for _, rec := range records {
		targetMap, _ := rec.Value["target"].(map[string]any)
		if targetMap == nil {
			continue
		}

		targetRepo, _ := targetMap["repo"].(string)
		if targetRepo != repoRef && targetRepo != repoInfo.ATURI {
			continue
		}

		pull := rawRecordToPull(rec)
		pulls = append(pulls, pull)
	}

	return pulls, nil
}

// rawRecordToPull converts a raw pdsRecord to a Pull.
func rawRecordToPull(rec pdsRecord) *Pull {
	pull := &Pull{
		URI:       rec.URI,
		CID:       rec.CID,
		Title:     jsonStr(rec.Value, "title"),
		Body:      jsonStr(rec.Value, "body"),
		CreatedAt: jsonStr(rec.Value, "createdAt"),
	}

	if sourceMap, ok := rec.Value["source"].(map[string]any); ok {
		pull.Source.Branch, _ = sourceMap["branch"].(string)
		pull.Source.SHA, _ = sourceMap["sha"].(string)
		pull.Source.Repo, _ = sourceMap["repo"].(string)
	}

	if targetMap, ok := rec.Value["target"].(map[string]any); ok {
		pull.Target.Repo, _ = targetMap["repo"].(string)
		pull.Target.Branch, _ = targetMap["branch"].(string)
		// repoDid is a non-standard extension in target; read it if present
		if rd, ok := targetMap["repoDid"].(string); ok {
			pull.Target.RepoDID = rd
		}
	}

	return pull
}
