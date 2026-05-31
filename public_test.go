package tangled

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestPublicListBranches(t *testing.T) {
	client := NewPublicClient()
	ctx := context.Background()

	branches, err := client.ListBranches(ctx, "tangled.org/core", 10)
	if err != nil {
		t.Fatalf("failed to list branches: %v", err)
	}

	t.Logf("Found %d branches on tangled.org/core", len(branches))
	for _, b := range branches {
		t.Logf("  %s -> %s", b.Name, truncate(b.SHA, 8))
	}

	// Should have at least a master branch
	found := false
	for _, b := range branches {
		if b.Name == "master" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'master' branch")
	}
}

func TestPublicResolveRepo(t *testing.T) {
	client := NewPublicClient()
	ctx := context.Background()

	repo, err := client.ResolveRepo(ctx, "tangled.org/core")
	if err != nil {
		t.Fatalf("failed to resolve repo: %v", err)
	}

	t.Logf("Resolved: name=%s knot=%s did=%s repoDid=%s", repo.Name, repo.Knot, repo.DID, repo.RepoDID)

	if repo.Knot == "" {
		t.Error("expected non-empty knot")
	}
}

func TestPublicResolveRepoNotFound(t *testing.T) {
	client := NewPublicClient()
	ctx := context.Background()

	_, err := client.ResolveRepo(ctx, "tangled.org/nonexistent-repo-xyz-12345")
	if err == nil {
		t.Error("expected error for nonexistent repo")
	}
	t.Logf("Got expected error: %v", err)
}

func TestPublicListIssues(t *testing.T) {
	client := NewPublicClient()
	ctx := context.Background()

	// We need an authenticated client to list issues from the user's PDS
	// since issues are stored on the creator's PDS.
	// For a public client, we can only resolve repos and list branches.
	// Issues are stored in the creator's repo, so we'd need the creator's DID.
	// Try with the core repo owner's issues
	repo, err := client.ResolveRepo(ctx, "tangled.org/core")
	if err != nil {
		t.Fatalf("failed to resolve repo: %v", err)
	}

	// List issues from the repo owner's PDS
	pdsURL, err := resolvePDS(ctx, repo.DID)
	if err != nil {
		t.Fatalf("failed to resolve PDS: %v", err)
	}

	url := fmt.Sprintf("%s/xrpc/com.atproto.repo.listRecords?repo=%s&collection=%s&limit=5",
		pdsURL, repo.DID, CollectionIssue)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list issues: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("PDS returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Records []struct {
			URI   string         `json:"uri"`
			Value map[string]any `json:"value"`
		} `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	t.Logf("Found %d issue records on the owner's PDS", len(result.Records))
	for _, r := range result.Records {
		title, _ := r.Value["title"].(string)
		t.Logf("  %s", title)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
