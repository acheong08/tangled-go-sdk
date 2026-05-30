package tangled

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// ListBranches lists branches for a repository by querying the knot's XRPC endpoint.
// This works for public repos without authentication.
func (c *Client) ListBranches(ctx context.Context, ownerSlashRepo string, limit int) ([]*Branch, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	// Use repoDid if available (newer format), otherwise fall back to did/repoName
	repoID := repoInfo.DID + "/" + repoInfo.Name
	if repoInfo.RepoDID != "" {
		repoID = repoInfo.RepoDID
	}

	url := fmt.Sprintf("https://%s/xrpc/sh.tangled.repo.branches", repoInfo.Knot)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Try with service token if available, otherwise attempt without auth
	token, tokenErr := c.getServiceToken(ctx)
	if tokenErr == nil {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	q := req.URL.Query()
	q.Set("repo", repoID)
	q.Set("limit", strconv.Itoa(limit))
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list branches: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Branches []struct {
			Reference struct {
				Name string `json:"name"`
				Hash string `json:"hash"`
			} `json:"reference"`
		} `json:"branches"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode branches response: %w", err)
	}

	branches := make([]*Branch, 0, len(result.Branches))
	for _, b := range result.Branches {
		branches = append(branches, &Branch{
			Name: b.Reference.Name,
			SHA:  b.Reference.Hash,
		})
	}

	return branches, nil
}

