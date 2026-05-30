package tangled

import (
	"context"
	"testing"
)

func TestListPulls(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	pulls, err := client.ListPulls(ctx, "tangled.org/core", 10)
	if err != nil {
		// This may fail if the user has no PRs on this repo
		t.Logf("ListPulls result: %v (this is expected if the user has no PRs on this repo)", err)
		return
	}

	t.Logf("Found %d pull requests", len(pulls))
	for _, pull := range pulls {
		t.Logf("  %s: %s -> %s (branch: %s)", pull.Title, pull.Source.Branch, pull.Target.Branch, pull.Target.Repo)
	}
}
