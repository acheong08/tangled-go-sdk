// Package tangled provides a Go client library for the Tangled git collaboration
// platform built on the AT Protocol. It supports managing issues
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

// IssueStateOpen is the state value for an open issue.
const IssueStateOpen = "sh.tangled.repo.issue.state.open"

// IssueStateClosed is the state value for a closed issue.
const IssueStateClosed = "sh.tangled.repo.issue.state.closed"

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
	// State is the current state of the issue (e.g., IssueStateOpen, IssueStateClosed).
	// Open is the default for issues without a state record.
	State string `json:"state,omitempty"`
	// Owner is the DID of the issue creator.
	Owner string `json:"owner,omitempty"`
	// Labels is a list of label names applied to the issue.
	Labels []string `json:"labels,omitempty"`
	// CreatedAt is the ISO 8601 timestamp of when the issue was created.
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
	// State is the new state (e.g., IssueStateOpen, IssueStateClosed). Empty means don't change.
	// Setting this creates or updates a sh.tangled.repo.issue.state record.
	State string
	// Labels replaces all existing labels. Use empty slice to clear all labels.
	// Nil means don't change labels.
	Labels []string
}

// StringPtr is a helper to create a *string from a string literal.
func StringPtr(s string) *string { return &s }

// Comment represents a Tangled comment on an issue.
type Comment struct {
	// URI is the AT-URI of the comment record.
	URI string `json:"uri"`
	// CID is the content identifier of the record.
	CID string `json:"cid"`
	// Body is the comment text.
	Body string `json:"body"`
	// SubjectURI is the AT-URI of the record this comment is on (issue/comment).
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
	// SubjectRef identifies what to comment on. It can be:
	//   - A numeric issue ID like "1" or "42" (resolves via issue list)
	//   - A full AT-URI like "at://did:plc:.../sh.tangled.repo.issue/rkey"
	SubjectRef string
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
