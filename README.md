# tangled-go-sdk

A Go client library for the [Tangled](https://tangled.org) git collaboration
platform, built on the [AT Protocol](https://atproto.com).

> [!CAUTION]
> DO NOT USE. THIS IS A VIBE CODING EXPERIMENT. WHILE I DID HAVE INPUT OVER DESIGN AND TESTING, I DID NOT REVIEW EVERY LINE OF CODE.

## Install

```bash
go get github.com/acheong08/tangled-go-sdk
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    tangled "github.com/acheong08/tangled-go-sdk"
)

func main() {
    ctx := context.Background()

    // Public, read-only client — no credentials needed
    client := tangled.NewPublicClient()

    // List branches on a public repo
    branches, _ := client.ListBranches(ctx, "tangled.org/core", 10)
    for _, b := range branches {
        fmt.Printf("  %s -> %s\n", b.Name, b.SHA)
    }

    // Resolve a repo to get its metadata
    repo, _ := client.ResolveRepo(ctx, "tangled.org/core")
    fmt.Printf("knot=%s repoDid=%s\n", repo.Knot, repo.RepoDID)
}
```

### Authenticated Client

```go
client, err := tangled.NewClient(ctx, tangled.Config{
    Handle:   "user.bsky.social",
    Password: "xxxx-xxxx-xxxx-xxxx", // app password from bsky.app/settings/app-passwords
})
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Authenticated as %s (%s)\n", client.Handle(), client.DID())

// List your repos
repos, _ := client.ListMyRepos(ctx)

// Create an issue
issue, _ := client.CreateIssue(ctx, "owner/repo", tangled.CreateIssueParams{
    Title:  "Bug report",
    Body:   "Something is broken",
    Labels: []string{"bug"},
})

// Close an issue (creates a sh.tangled.repo.issue.state record)
client.CloseIssue(ctx, "owner/repo", issue.ID)

// Reopen an issue
client.ReopenIssue(ctx, "owner/repo", issue.ID)

// Comment on the issue
comment, _ := client.CreateComment(ctx, tangled.CreateCommentParams{
    Body:           "Confirmed, I can reproduce this.",
    OwnerSlashRepo: "owner/repo",
    SubjectRef:     "1", // issue ID as a string
})

// Update an issue (use StringPtr for fields you want to change)
updated, _ := client.UpdateIssue(ctx, "owner/repo", tangled.UpdateIssueParams{
    IssueID: issue.ID,
    Title:   tangled.StringPtr("Updated title"),
    Body:    tangled.StringPtr("Updated body"),
    State:   tangled.IssueStateClosed, // can also change state via UpdateIssue
})

// Delete an issue (permanently removes the PDS record)
client.DeleteIssue(ctx, "owner/repo", issue.ID)
```

## API Reference

### Client Construction

| Method                   | Description                                       |
| ------------------------ | ------------------------------------------------- |
| `NewClient(ctx, Config)` | Authenticated client (requires handle + password) |
| `NewPublicClient()`      | Read-only client (no credentials)                 |

### Repository Operations

| Method                            | Auth     | Description                                        |
| --------------------------------- | -------- | -------------------------------------------------- |
| `ResolveRepo(ctx, "owner/repo")`  | Either   | Resolve repo name to metadata (knot, DID, RepoDID) |
| `ListMyRepos(ctx)`                | Required | List the authenticated user's repositories          |

### Issue Operations

| Method                                   | Auth     | Description                           |
| ---------------------------------------- | -------- | ------------------------------------- |
| `ListIssues(ctx, "owner/repo", limit)`   | Either   | List issues for a repo (includes state) |
| `GetIssue(ctx, "owner/repo", id)`        | Required | Get a specific issue by ID (includes state) |
| `CreateIssue(ctx, "owner/repo", params)` | Required | Create a new issue                    |
| `UpdateIssue(ctx, "owner/repo", params)` | Required | Update title/body/state/labels       |
| `CloseIssue(ctx, "owner/repo", id)`      | Required | Close an issue (creates state record) |
| `ReopenIssue(ctx, "owner/repo", id)`      | Required | Reopen a closed issue                |
| `DeleteIssue(ctx, "owner/repo", id)`      | Required | Permanently delete an issue record    |
| `ListLabels(ctx, "owner/repo")`          | Either   | List available label names for a repo |

### Comment Operations

| Method                                           | Auth     | Description                                                                 |
| ------------------------------------------------ | -------- | --------------------------------------------------------------------------- |
| `CreateComment(ctx, params)`                     | Required | Create a comment on an issue                                                |
| `ListComments(ctx, "owner/repo", subjectRef, limit)` | Either   | List comments, optionally filtered by subject                               |

`CreateCommentParams.SubjectRef` can be:
- A numeric issue ID like `"1"` — resolved via the issue list
- A full AT-URI like `"at://did:plc:.../sh.tangled.repo.issue/rkey"` — fetched directly

### Branch Operations

| Method                                   | Auth   | Description                       |
| ---------------------------------------- | ------ | --------------------------------- |
| `ListBranches(ctx, "owner/repo", limit)` | Either | List branches via the knot server |

## CLI

An example CLI is included at `examples/cli/`:

```bash
# Build
cd examples/cli && go build -o tangled .

# Login (saves credentials to ~/.config/tangled/config.toml)
tangled login
tangled login user.bsky.social xxxx-xxxx-xxxx-xxxx

# Identity
tangled whoami

# Repos
tangled repo list

# Issues
tangled issue list owner/repo
tangled issue create owner/repo -t "Bug report" -b "Description" -l bug
tangled issue show owner/repo 1
tangled issue close owner/repo 1
tangled issue reopen owner/repo 1
tangled issue comment owner/repo 1 -m "Confirmed"

# Branches
tangled branch list owner/repo

# Labels
tangled label list owner/repo
```

## Architecture

Tangled is built on the AT Protocol. Each piece of data lives on the creator's
PDS (Personal Data Server), not on a central server:

- **Repositories** are records (`sh.tangled.repo`) on the owner's PDS
- **Issues** are records (`sh.tangled.repo.issue`) on the **creator's** PDS
- **Issue state** uses event-sourcing via separate records (`sh.tangled.repo.issue.state`) on the **closer's** PDS — closed/open state is not stored on the issue record itself
- **Comments** are records (`sh.tangled.feed.comment`) on the **commenter's** PDS
- **Branches** are queried from the **knot server** (the git host)

This library authenticates via `com.atproto.server.createSession`, then uses raw
HTTP calls to PDS endpoints for all repo record operations. It does **not** use
indigo's xrpc type decoder for Tangled-specific collections (since
`sh.tangled.*` types are not registered with indigo), keeping xrpc only for
standard AT Protocol auth operations.

## Known Limitations

### Issue listings are incomplete

The AT Protocol stores issues on the **creator's** PDS, not on the repo owner's
PDS. When you call `ListIssues`, the library queries a single PDS at a time:

- **Authenticated `ListIssues`**: queries the **authenticated user's** PDS →
  only shows issues **you** created on that repo.
- **Public `ListIssues`**: queries the **repo owner's** PDS → only shows issues
  the **repo owner** created.

For a popular repo with many contributors, querying any single PDS will only
return a small fraction of the total. Issues from other contributors are
invisible because they live on their own PDS instances.

The Tangled website (tangled.org) shows complete listings because its
**appview** subscribes to the AT Protocol firehose and aggregates all records
from all PDS instances into a single database. However, the appview **does not
expose a public JSON/XRPC query API** — it only renders HTML server-side.

**Impact**: You cannot build a complete issue listing through this library (or
any PDS-based approach). Only the Tangled appview has the full picture.

### No pull request support

Pull request records created via the Tangled web UI do not include a `pullId`
field — the PR number is assigned by the appview, not stored in the PDS record.
Without a public appview API, there is no reliable way to map PR numbers to
PDS records or list PRs from PDS queries alone. PR support has been removed
until the appview exposes a query API.

### Issue ID collisions

`CreateIssue` determines the next sequential issue ID by scanning existing
issues on the authenticated user's PDS and the repo owner's PDS. Since issues
from other PDS instances are invisible, two people creating issues
simultaneously might both get the same ID.

The Tangled appview manages a definitive ID sequence, but there is no API to
query it.

### Comment listings are incomplete

`ListComments` queries the repo owner's PDS, so it only finds comments from
users on that same PDS. Comments from contributors on other PDS instances are
not visible.

### ListLabels resolution

Labels with TID-format record keys (e.g., `3lzmajxaipo22`) are resolved to their
human-readable names (e.g., `area`) by fetching the label definition records
from the label owner's PDS. If the PDS is unreachable, the raw TID rkey is
returned as a fallback.

### Pagination

All list operations use a fixed limit (default 50, max 100). Cursor-based
pagination from the PDS `listRecords` response is not yet implemented. For repos
with more than 100 records in a collection, older records will be truncated.

### Repository record duality

Some Tangled repositories have two `sh.tangled.repo` records on the owner's PDS
— an older one with a TID rkey and explicit `name` field, and a newer one with a
human-readable rkey and no `name` field. This library prefers the older
(canonical) record, which is the one referenced by existing issues.

## Development

```bash
# Run public tests (no credentials needed)
go test -v -run TestPublic ./...

# Run authenticated tests (requires credentials)
TANGLED_HANDLE=your.handle TANGLED_PASSWORD=xxxx go test -v ./...
```
