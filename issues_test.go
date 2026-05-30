package tangled

import (
	"context"
	"os"
	"testing"
)

func TestListIssues(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	issues, err := client.ListIssues(ctx, "zzstoatzz.io/tangled-mcp", 10)
	if err != nil {
		t.Fatalf("failed to list issues: %v", err)
	}

	t.Logf("Found %d issues", len(issues))
	for _, issue := range issues {
		t.Logf("  #%d: %s", issue.ID, issue.Title)
		if len(issue.Labels) > 0 {
			t.Logf("    labels: %v", issue.Labels)
		}
	}
}

func TestCreateAndGetAndDeleteIssue(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	handle := os.Getenv("TANGLED_HANDLE")
	if handle == "" {
		t.Skip("TANGLED_HANDLE must be set for issue CRUD tests")
	}

	// Find a repo that the authenticated user owns
	repos, err := client.ListMyRepos(ctx)
	if err != nil {
		t.Skipf("failed to list user repos: %v", err)
	}
	if len(repos) == 0 {
		t.Skip("no repos found for authenticated user")
	}

	repo := repos[0]
	repoID := handle + "/" + repo.Name
	t.Logf("Testing issue CRUD on repo: %s", repoID)

	// Create an issue
	created, err := client.CreateIssue(ctx, repoID, CreateIssueParams{
		Title: "Test issue from Go client",
		Body:  "This is a test issue created by the Go tangled client library.",
	})
	if err != nil {
		t.Fatalf("failed to create issue: %v", err)
	}

	t.Logf("Created issue #%d: URI=%s CID=%s", created.ID, created.URI[:50]+"...", created.CID[:20]+"...")

	if created.Title != "Test issue from Go client" {
		t.Errorf("expected title 'Test issue from Go client', got %q", created.Title)
	}

	// Get the issue
	got, err := client.GetIssue(ctx, repoID, created.ID)
	if err != nil {
		t.Fatalf("failed to get issue: %v", err)
	}

	if got.Title != created.Title {
		t.Errorf("expected title %q, got %q", created.Title, got.Title)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %d, got %d", created.ID, got.ID)
	}

	t.Logf("Retrieved issue #%d: %s", got.ID, got.Title)

	// Update the issue
	updated, err := client.UpdateIssue(ctx, repoID, UpdateIssueParams{
		IssueID: created.ID,
		Title:   "Updated test issue from Go client",
		Body:    "Updated body text.",
	})
	if err != nil {
		t.Fatalf("failed to update issue: %v", err)
	}

	t.Logf("Updated issue #%d: %s", updated.ID, updated.Title)

	if updated.Title != "Updated test issue from Go client" {
		t.Errorf("expected updated title, got %q", updated.Title)
	}

	// Delete the issue
	err = client.DeleteIssue(ctx, repoID, created.ID)
	if err != nil {
		t.Fatalf("failed to delete issue: %v", err)
	}

	t.Logf("Deleted issue #%d", created.ID)

	// Verify it's gone
	_, err = client.GetIssue(ctx, repoID, created.ID)
	if err == nil {
		t.Error("expected error getting deleted issue")
	}
}

func TestListLabels(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	labels, err := client.ListLabels(ctx, "zzstoatzz.io/tangled-mcp")
	if err != nil {
		t.Fatalf("failed to list labels: %v", err)
	}

	t.Logf("Available labels: %v", labels)
}
