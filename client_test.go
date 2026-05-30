package tangled

import (
	"context"
	"os"
	"testing"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	handle := os.Getenv("TANGLED_HANDLE")
	password := os.Getenv("TANGLED_PASSWORD")
	if handle == "" || password == "" {
		t.Skip("TANGLED_HANDLE and TANGLED_PASSWORD must be set for integration tests")
	}

	ctx := context.Background()
	client, err := NewClient(ctx, Config{
		Handle:   handle,
		Password: password,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

func TestAuthenticatedClient(t *testing.T) {
	client := testClient(t)
	t.Logf("Authenticated as %s (DID: %s)", client.Handle(), client.DID())

	if !client.IsAuthenticated() {
		t.Error("expected IsAuthenticated() to be true")
	}

	ctx := context.Background()

	// Resolve repo with authenticated client
	repo, err := client.ResolveRepo(ctx, "tangled.org/core")
	if err != nil {
		t.Fatalf("failed to resolve repo: %v", err)
	}
	t.Logf("Resolved repo: name=%s knot=%s did=%s", repo.Name, repo.Knot, repo.DID)

	// List my repos
	repos, err := client.ListMyRepos(ctx)
	if err != nil {
		t.Logf("ListMyRepos: %v (may be expected)", err)
	} else {
		t.Logf("Found %d repos for authenticated user", len(repos))
	}
}
