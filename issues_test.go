package tangled

import (
	"context"
	"os"
	"testing"
)

func TestAuthenticatedCreateIssue(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	handle := os.Getenv("TANGLED_HANDLE")
	if handle == "" {
		t.Skip("TANGLED_HANDLE must be set")
	}

	// Find a repo the user owns
	repos, err := client.ListMyRepos(ctx)
	if err != nil || len(repos) == 0 {
		t.Skipf("no repos found for user: %v", err)
	}

	repo := repos[0]
	repoID := handle + "/" + repo.Name
	t.Logf("Testing on repo: %s", repoID)

	// Create an issue
	created, err := client.CreateIssue(ctx, repoID, CreateIssueParams{
		Title: "Test issue from Go client",
		Body:  "This is a test issue created by the tangled Go client library.",
	})
	if err != nil {
		t.Fatalf("failed to create issue: %v", err)
	}
	t.Logf("Created issue #%d: %s", created.ID, created.URI)

	// Get the issue
	got, err := client.GetIssue(ctx, repoID, created.ID)
	if err != nil {
		t.Fatalf("failed to get issue: %v", err)
	}
	if got.Title != "Test issue from Go client" {
		t.Errorf("unexpected title: %q", got.Title)
	}

	// Update the issue
	updated, err := client.UpdateIssue(ctx, repoID, UpdateIssueParams{
		IssueID: created.ID,
		Title:   StringPtr("Updated test issue from Go client"),
	})
	if err != nil {
		t.Fatalf("failed to update issue: %v", err)
	}
	if updated.Title != "Updated test issue from Go client" {
		t.Errorf("unexpected updated title: %q", updated.Title)
	}

	// Delete the issue
	err = client.DeleteIssue(ctx, repoID, created.ID)
	if err != nil {
		t.Fatalf("failed to delete issue: %v", err)
	}
	t.Logf("Deleted issue #%d", created.ID)

	// Verify deletion
	_, err = client.GetIssue(ctx, repoID, created.ID)
	if err == nil {
		t.Error("expected error getting deleted issue")
	}
}

func TestAuthenticatedListIssues(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	handle := os.Getenv("TANGLED_HANDLE")
	repos, err := client.ListMyRepos(ctx)
	if err != nil || len(repos) == 0 {
		t.Skip("no repos found")
	}

	repoID := handle + "/" + repos[0].Name
	issues, err := client.ListIssues(ctx, repoID, 10)
	if err != nil {
		t.Logf("ListIssues: %v (may be expected if no issues)", err)
	} else {
		t.Logf("Found %d issues", len(issues))
		for _, i := range issues {
			t.Logf("  #%d: %s", i.ID, i.Title)
		}
	}
}

func TestAuthenticatedListLabels(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	labels, err := client.ListLabels(ctx, "tangled.org/core")
	if err != nil {
		t.Logf("ListLabels: %v", err)
	} else {
		t.Logf("Available labels: %v", labels)
	}
}
