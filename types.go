// Package tangled provides a Go client library for the Tangled git collaboration
// platform built on the AT Protocol. It supports managing issues, pull requests,
// and branches on Tangled repositories.
package tangled

import (
	"fmt"
	"time"
)

// RepoInfo contains metadata about a Tangled repository resolved from atproto records.
type RepoInfo struct {
	// Name is the repository name.
	Name string `json:"name"`
	// Knot is the hostname of the knot server hosting this repo.
	Knot string `json:"knot"`
	// ATURI is the primary AT-URI of the repo record (the canonical/older one with name field).
	ATURI string `json:"atUri"`
	// AltURIs are additional AT-URIs for the same repo (e.g., newer rkey-based records).
	// Issues/PRs may reference any of these AT-URIs in their "repo" field.
	AltURIs []string `json:"altUris,omitempty"`
	// DID is the owner's DID.
	DID string `json:"did"`
	// Labels is a list of AT-URIs of label definitions this repo subscribes to.
	Labels []string `json:"labels,omitempty"`
	// Description is the optional repo description.
	Description string `json:"description,omitempty"`
	// RepoDID is the DID of the repository itself (for stable identity across renames).
	RepoDID string `json:"repoDid,omitempty"`
}

// Issue represents a Tangled issue on a repository.
type Issue struct {
	// URI is the AT-URI of the issue record.
	URI string `json:"uri"`
	// CID is the content identifier of the record.
	CID string `json:"cid"`
	// ID is the sequential issue number within the repo.
	ID int `json:"issueId"`
	// Title is the issue title.
	Title string `json:"title"`
	// Body is the optional issue body/description.
	Body string `json:"body,omitempty"`
	// Owner is the DID of the issue creator.
	Owner string `json:"owner,omitempty"`
	// Labels is a list of label names applied to the issue.
	Labels []string `json:"labels,omitempty"`
	// CreatedAt is the ISO 8601 timestamp of when the issue was created.
	CreatedAt string `json:"createdAt"`
}

// PullSource describes the source branch of a pull request.
type PullSource struct {
	// Branch is the source branch name.
	Branch string `json:"branch"`
	// SHA is the commit hash of the source branch tip (may be empty for older records).
	SHA string `json:"sha,omitempty"`
	// Repo is the AT-URI of the source repo (for cross-repo PRs).
	Repo string `json:"repo,omitempty"`
}

// PullTarget describes the target branch of a pull request.
type PullTarget struct {
	// Repo is the DID of the repo being targeted.
	Repo string `json:"repo"`
	// Branch is the target branch name.
	Branch string `json:"branch"`
	// RepoDID is the DID of the target repository itself (newer format).
	RepoDID string `json:"repoDid,omitempty"`
}

// Pull represents a Tangled pull request.
type Pull struct {
	// URI is the AT-URI of the pull record.
	URI string `json:"uri"`
	// CID is the content identifier of the record.
	CID string `json:"cid"`
	// ID is the sequential pull request number.
	ID int `json:"pullId"`
	// Title is the pull request title.
	Title string `json:"title"`
	// Body is the optional pull request description.
	Body string `json:"body,omitempty"`
	// Source describes the source branch.
	Source PullSource `json:"source"`
	// Target describes the target branch.
	Target PullTarget `json:"target"`
	// CreatedAt is the ISO 8601 timestamp of when the PR was created.
	CreatedAt string `json:"createdAt"`
}

// Branch represents a git branch on a Tangled repository.
type Branch struct {
	// Name is the branch name.
	Name string `json:"name"`
	// SHA is the commit hash the branch points to.
	SHA string `json:"sha"`
}

// CreateIssueParams contains the parameters for creating an issue.
type CreateIssueParams struct {
	// Title is the issue title (required).
	Title string
	// Body is the optional issue body/description.
	Body string
	// Labels is an optional list of label names to apply.
	Labels []string
}

// UpdateIssueParams contains the parameters for updating an issue.
// Use *string fields to distinguish "don't change" (nil) from "set to empty" (pointer to "").
type UpdateIssueParams struct {
	// IssueID is the sequential issue number to update (required).
	IssueID int
	// Title is the new title. nil means don't change; pointer to "" means clear.
	Title *string
	// Body is the new body. nil means don't change; pointer to "" means clear.
	Body *string
	// Labels replaces all existing labels. Use empty slice to clear all labels.
	// Nil means don't change labels.
	Labels []string
}

// StringPtr is a helper to create a *string from a string literal.
func StringPtr(s string) *string { return &s }

// CreatePullParams contains the parameters for creating a pull request.
type CreatePullParams struct {
	// Title is the PR title (required).
	Title string
	// Body is the optional PR description.
	Body string
	// SourceBranch is the branch containing changes (required).
	SourceBranch string
	// TargetBranch is the branch to merge into (default: "main").
	TargetBranch string
}

// Comment represents a Tangled comment on an issue or pull request.
type Comment struct {
	// URI is the AT-URI of the comment record.
	URI string `json:"uri"`
	// CID is the content identifier of the record.
	CID string `json:"cid"`
	// Body is the comment text.
	Body string `json:"body"`
	// SubjectURI is the AT-URI of the record this comment is on (issue/PR/comment).
	SubjectURI string `json:"subjectUri"`
	// SubjectCID is the CID of the record this comment is on.
	SubjectCID string `json:"subjectCid"`
	// ReplyToURI is the AT-URI of the parent comment (for threaded replies). Empty for top-level comments.
	ReplyToURI string `json:"replyToUri,omitempty"`
	// ReplyToCID is the CID of the parent comment.
	ReplyToCID string `json:"replyToCid,omitempty"`
	// CreatedAt is the ISO 8601 timestamp of when the comment was created.
	CreatedAt string `json:"createdAt"`
}

// CreateCommentParams contains the parameters for creating a comment.
type CreateCommentParams struct {
	// Body is the comment text (required).
	Body string
	// OwnerSlashRepo is the repository in "owner/repo" format (required).
	OwnerSlashRepo string
	// IssueID is the issue or PR number to comment on (required).
	IssueID int
	// ReplyToURI is the AT-URI of the parent comment for threaded replies (optional).
	ReplyToURI string
	// ReplyToCID is the CID of the parent comment (required if ReplyToURI is set).
	ReplyToCID string
}

// generateTID returns a Timestamp ID string suitable for use as an atproto record key.
func generateTID() string {
	return fmt.Sprintf("%d", time.Now().UnixMicro())
}

// nowISO returns the current UTC time in ISO 8601 format with Z suffix.
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}
