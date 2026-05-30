package tangled

import (
	"context"
	"testing"
)

func TestListBranches(t *testing.T) {
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

func TestListBranchesWithRepoDID(t *testing.T) {
	client := NewPublicClient()
	ctx := context.Background()

	// Resolve the repo first to see its repoDid
	repo, err := client.ResolveRepo(ctx, "tangled.org/core")
	if err != nil {
		t.Fatalf("failed to resolve repo: %v", err)
	}
	t.Logf("Repo DID: %s, RepoDID: %s", repo.DID, repo.RepoDID)

	// ListBranches should automatically use repoDid if available
	branches, err := client.ListBranches(ctx, "tangled.org/core", 5)
	if err != nil {
		t.Fatalf("failed to list branches: %v", err)
	}

	t.Logf("Found %d branches", len(branches))
	for _, b := range branches {
		t.Logf("  %s -> %s", b.Name, truncate(b.SHA, 8))
	}
}
