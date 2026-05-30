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
		Handle:  handle,
		Password: password,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

func TestNewClient(t *testing.T) {
	client := testClient(t)

	if client.DID() == "" {
		t.Error("expected non-empty DID")
	}
	if client.Handle() == "" {
		t.Error("expected non-empty Handle")
	}
	t.Logf("Authenticated as %s (DID: %s)", client.Handle(), client.DID())
}

func TestResolveRepo(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	// Test resolving the tangled-mcp repo itself
	repo, err := client.ResolveRepo(ctx, "zzstoatzz.io/tangled-mcp")
	if err != nil {
		t.Fatalf("failed to resolve repo: %v", err)
	}

	t.Logf("Resolved repo: name=%s knot=%s did=%s repoDid=%s", repo.Name, repo.Knot, repo.DID, repo.RepoDID)

	if repo.Name != "tangled-mcp" {
		t.Errorf("expected name 'tangled-mcp', got %q", repo.Name)
	}
	if repo.Knot == "" {
		t.Error("expected non-empty knot")
	}
	if repo.DID == "" {
		t.Error("expected non-empty DID")
	}
}

func TestResolveRepoWithAt(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	repo, err := client.ResolveRepo(ctx, "@zzstoatzz.io/tangled-mcp")
	if err != nil {
		t.Fatalf("failed to resolve repo with @ prefix: %v", err)
	}

	if repo.Name != "tangled-mcp" {
		t.Errorf("expected name 'tangled-mcp', got %q", repo.Name)
	}
}

func TestResolveRepoNotFound(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	_, err := client.ResolveRepo(ctx, "zzstoatzz.io/nonexistent-repo-xyz")
	if err == nil {
		t.Error("expected error for nonexistent repo")
	}
	t.Logf("Got expected error: %v", err)
}

func TestResolveRepoInvalidFormat(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	_, err := client.ResolveRepo(ctx, "invalid-format")
	if err == nil {
		t.Error("expected error for invalid format")
	}
}
