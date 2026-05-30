package tangled_test

import (
	"context"
	"fmt"
	"log"

	"github.com/acheong08/tangled-go-sdk"
)

func ExampleNewPublicClient() {
	// Public client for read-only operations (no credentials needed)
	client := tangled.NewPublicClient()
	ctx := context.Background()

	// Resolve a repository
	repo, err := client.ResolveRepo(ctx, "tangled.org/core")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Repo: %s on knot %s\n", repo.Name, repo.Knot)

	// List branches
	branches, err := client.ListBranches(ctx, "tangled.org/core", 5)
	if err != nil {
		log.Fatal(err)
	}
	for _, b := range branches {
		fmt.Printf("  %s\n", b.Name)
	}
}

func ExampleNewClient() {
	// Authenticated client for read-write operations
	ctx := context.Background()
	client, err := tangled.NewClient(ctx, tangled.Config{
		Handle:  "your.handle.bsky.social",
		Password: "xxxx-xxxx-xxxx-xxxx", // App password from bsky.app/settings/app-passwords
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Logged in as %s (%s)\n", client.Handle(), client.DID())

	// List your repositories
	repos, err := client.ListMyRepos(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range repos {
		fmt.Printf("  %s on %s\n", r.Name, r.Knot)
	}
}

func ExampleClient_CreateIssue() {
	ctx := context.Background()
	client, _ := tangled.NewClient(ctx, tangled.Config{
		Handle:   "your.handle",
		Password: "your-app-password",
	})

	// Create an issue
	issue, err := client.CreateIssue(ctx, "owner/repo", tangled.CreateIssueParams{
		Title: "Bug: something is wrong",
		Body:  "Description of the bug.",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created issue #%d: %s\n", issue.ID, issue.Title)

	// List issues
	issues, err := client.ListIssues(ctx, "owner/repo", 20)
	if err != nil {
		log.Fatal(err)
	}
	for _, i := range issues {
		fmt.Printf("#%d: %s\n", i.ID, i.Title)
	}

	// Update the issue
	_, err = client.UpdateIssue(ctx, "owner/repo", tangled.UpdateIssueParams{
		IssueID: issue.ID,
		Title:   "Bug: something is wrong (updated)",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Delete the issue
	err = client.DeleteIssue(ctx, "owner/repo", issue.ID)
	if err != nil {
		log.Fatal(err)
	}
}
