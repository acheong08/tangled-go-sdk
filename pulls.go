package tangled

import (
	"context"
	"fmt"
	"strconv"
)

// ListPulls lists pull requests for the specified repository.
// With an authenticated client, it queries the user's PDS for PRs they created.
// With a public client, it falls back to ListPublicPulls (queries repo owner's PDS).
//
// Note: PDS-based PR listing is inherently limited — PRs are stored on the
// creator's PDS, so listing the owner's PDS only shows PRs created by that
// account. For repos with many external contributors, most PRs won't appear.
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

	var pulls []*Pull
	for _, rec := range records {
		if !pullMatchesRepo(rec.Value, repoInfo) {
			continue
		}
		pulls = append(pulls, rawRecordToPull(rec))
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

		return rawRecordToPull(rec), nil
	}

	return nil, fmt.Errorf("pull request with rkey %q not found", rkey)
}

// ListPublicPulls lists pull requests on a repository using public PDS queries.
// This queries the repo owner's PDS, which only contains PRs created by the repo owner.
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

	var accessToken string
	if c.IsAuthenticated() {
		accessToken = c.accessJWT
	}

	records, err := pdsListRecords(ctx, pdsURL, repoInfo.DID, CollectionPull, limit, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to query PDS: %w", err)
	}

	var pulls []*Pull
	for _, rec := range records {
		if !pullMatchesRepo(rec.Value, repoInfo) {
			continue
		}
		pulls = append(pulls, rawRecordToPull(rec))
	}

	return pulls, nil
}

// pullMatchesRepo checks if a PR record targets the given repo.
// It handles both the nested format (target.repo, target.branch) from the lexicon
// and the flat format (targetRepo, targetBranch) found in older/production records.
func pullMatchesRepo(m map[string]any, repoInfo *RepoInfo) bool {
	// Nested format (lexicon): target: { repo: "did:...", branch: "..." }
	if targetMap, ok := m["target"].(map[string]any); ok {
		targetRepo, _ := targetMap["repo"].(string)
		return repoRefMatches(targetRepo, repoInfo)
	}

	// Flat format (production): targetRepo: "at://...", targetBranch: "..."
	if targetRepo, ok := m["targetRepo"].(string); ok {
		return repoRefMatches(targetRepo, repoInfo)
	}

	return false
}

// rawRecordToPull converts a raw pdsRecord to a Pull.
// It handles both the nested format (target/source objects) from the lexicon
// and the flat format (targetRepo/targetBranch/source.branch) found in production.
func rawRecordToPull(rec pdsRecord) *Pull {
	pull := &Pull{
		URI:       rec.URI,
		CID:       rec.CID,
		Title:     jsonStr(rec.Value, "title"),
		Body:      jsonStr(rec.Value, "body"),
		CreatedAt: jsonStr(rec.Value, "createdAt"),
	}

	// Pull ID (flat format uses "pullId")
	if pullID := jsonFloat(rec.Value, "pullId"); pullID > 0 {
		pull.ID = int(pullID)
	}

	// Source: nested format (source: { branch: "...", sha: "..." })
	if sourceMap, ok := rec.Value["source"].(map[string]any); ok {
		pull.Source.Branch, _ = sourceMap["branch"].(string)
		pull.Source.SHA, _ = sourceMap["sha"].(string)
		pull.Source.Repo, _ = sourceMap["repo"].(string)
	}

	// Target: nested format (target: { repo: "...", branch: "..." })
	if targetMap, ok := rec.Value["target"].(map[string]any); ok {
		pull.Target.Repo, _ = targetMap["repo"].(string)
		pull.Target.Branch, _ = targetMap["branch"].(string)
		if rd, ok := targetMap["repoDid"].(string); ok {
			pull.Target.RepoDID = rd
		}
	} else {
		// Target: flat format (targetRepo, targetBranch as top-level strings)
		if tr, ok := rec.Value["targetRepo"].(string); ok {
			pull.Target.Repo = tr
		}
		if tb, ok := rec.Value["targetBranch"].(string); ok {
			pull.Target.Branch = tb
		}
	}

	return pull
}

// formatPullID returns a display string for a pull request ID.
func formatPullID(pull *Pull) string {
	if pull.ID > 0 {
		return strconv.Itoa(pull.ID)
	}
	rkey, err := extractRkey(pull.URI)
	if err != nil {
		return "?"
	}
	return rkey
}
