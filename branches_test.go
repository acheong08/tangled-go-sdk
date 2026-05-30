package tangled

import (
	"context"
	"os"
	"testing"
)

func TestListBranches(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	branches, err := client.ListBranches(ctx, "zzstoatzz.io/tangled-mcp", 10)
	if err != nil {
		t.Fatalf("failed to list branches: %v", err)
	}

	t.Logf("Found %d branches", len(branches))
	for _, b := range branches {
		t.Logf("  %s -> %s", b.Name, b.SHA[:min(8, len(b.SHA))])
	}

	// Should have at least a main branch
	found := false
	for _, b := range branches {
		if b.Name == "main" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'main' branch")
	}
}

func TestListMyBranches(t *testing.T) {
	handle := os.Getenv("TANGLED_HANDLE")
	password := os.Getenv("TANGLED_PASSWORD")
	if handle == "" || password == "" {
		t.Skip("TANGLED_HANDLE and TANGLED_PASSWORD must be set for integration tests")
	}

	// Try listing branches on a repo the user owns
	// This test uses the user's own handle to find repos
	client := testClient(t)
	ctx := context.Background()

	// List the authenticated user's repos first
	repos, err := client.ListMyRepos(ctx)
	if err != nil {
		t.Skipf("failed to list user repos: %v", err)
	}
	if len(repos) == 0 {
		t.Skip("no repos found for authenticated user")
	}

	// Pick the first repo
	repo := repos[0]
	t.Logf("Testing branches on repo: %s/%s", client.Handle(), repo.Name)

	branches, err := client.ListBranches(ctx, client.Handle()+"/"+repo.Name, 10)
	if err != nil {
		t.Fatalf("failed to list branches: %v", err)
	}

	for _, b := range branches {
		t.Logf("  %s -> %s", b.Name, b.SHA[:min(8, len(b.SHA))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
