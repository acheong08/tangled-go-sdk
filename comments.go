package tangled

import (
	"context"
	"fmt"
)

// CreateComment creates a comment on an issue or pull request.
// It resolves the issue/PR ID to the AT-URI and CID automatically.
func (c *Client) CreateComment(ctx context.Context, params CreateCommentParams) (*Comment, error) {
	if err := c.requireAuth(); err != nil {
		return nil, err
	}
	if params.Body == "" {
		return nil, fmt.Errorf("body is required")
	}
	if params.OwnerSlashRepo == "" {
		return nil, fmt.Errorf("owner/repo is required")
	}
	if params.IssueID <= 0 {
		return nil, fmt.Errorf("issue ID is required")
	}

	// Resolve the issue or PR to get its AT-URI and CID
	subjectURI, subjectCID, err := c.resolveSubject(ctx, params.OwnerSlashRepo, params.IssueID)
	if err != nil {
		return nil, err
	}

	rkey := generateTID()

	record := map[string]any{
		"$type": CollectionComment,
		"body": map[string]any{
			"$type":    "sh.tangled.markup.markdown",
			"text":     params.Body,
			"original": params.Body,
		},
		"subject": map[string]any{
			"uri": subjectURI,
			"cid": subjectCID,
		},
		"createdAt": nowISO(),
	}

	// If this is a reply to another comment, include the reply-to field
	if params.ReplyToURI != "" {
		replyTo := map[string]any{
			"uri": params.ReplyToURI,
		}
		if params.ReplyToCID != "" {
			replyTo["cid"] = params.ReplyToCID
		}
		record["reply"] = replyTo
	}

	uri, cid, err := c.pdsPutRecord(ctx, CollectionComment, rkey, record, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	return &Comment{
		URI:        uri,
		CID:        cid,
		Body:       params.Body,
		SubjectURI: subjectURI,
		SubjectCID: subjectCID,
		ReplyToURI: params.ReplyToURI,
		ReplyToCID: params.ReplyToCID,
		CreatedAt:  record["createdAt"].(string),
	}, nil
}

// ListComments lists comments on a repository, optionally filtered to a specific issue or PR.
// If issueID is 0, all comments from the repo owner's PDS are returned.
//
// Note: This only finds comments from users on the same PDS as the repo owner.
// Comments from other PDS instances are not visible via this method.
func (c *Client) ListComments(ctx context.Context, ownerSlashRepo string, issueID int, limit int) ([]*Comment, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	pdsURL, err := resolvePDS(ctx, repoInfo.DID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve PDS for %s: %w", repoInfo.DID, err)
	}

	accessToken := ""
	if c.IsAuthenticated() {
		accessToken = c.accessJWT
	}

	if limit <= 0 {
		limit = 50
	}

	records, err := pdsListRecords(ctx, pdsURL, repoInfo.DID, CollectionComment, limit, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to list comments: %w", err)
	}

	// If an issueID is given, resolve the subject AT-URI for filtering
	var subjectURI string
	if issueID > 0 {
		subjectURI, _, err = c.resolveSubject(ctx, ownerSlashRepo, issueID)
		if err != nil {
			return nil, err
		}
	}

	var comments []*Comment
	for _, rec := range records {
		comment := rawRecordToComment(rec)
		if comment == nil {
			continue
		}
		// Filter to only comments on the requested subject
		if subjectURI != "" && comment.SubjectURI != subjectURI {
			continue
		}
		comments = append(comments, comment)
	}

	return comments, nil
}

// resolveSubject finds the AT-URI and CID for an issue or PR by its numeric ID.
// It tries issues first, then falls back to PRs. Works with both public and authenticated clients.
func (c *Client) resolveSubject(ctx context.Context, ownerSlashRepo string, id int) (uri, cid string, err error) {
	// Try issues first — ListIssues works without auth
	issues, err := c.ListIssues(ctx, ownerSlashRepo, 100)
	if err == nil {
		for _, issue := range issues {
			if issue.ID == id {
				return issue.URI, issue.CID, nil
			}
		}
	}

	// Fall back to PRs
	pulls, err := c.ListPulls(ctx, ownerSlashRepo, 100)
	if err == nil {
		for _, p := range pulls {
			if p.ID == id {
				return p.URI, p.CID, nil
			}
		}
	}

	return "", "", fmt.Errorf("issue or PR #%d not found on %s", id, ownerSlashRepo)
}

// rawRecordToComment converts a raw pdsRecord to a Comment.
// Returns nil if the record doesn't have the expected structure.
func rawRecordToComment(rec pdsRecord) *Comment {
	v := rec.Value

	// Extract body text from the markdown object
	body := ""
	if bodyObj, ok := v["body"].(map[string]any); ok {
		body, _ = bodyObj["text"].(string)
	} else if bodyStr, ok := v["body"].(string); ok {
		// Some older records use a plain string body
		body = bodyStr
	}

	// Extract subject
	subjectURI, subjectCID := "", ""
	if subject, ok := v["subject"].(map[string]any); ok {
		subjectURI, _ = subject["uri"].(string)
		subjectCID, _ = subject["cid"].(string)
	}

	// Extract reply-to (optional)
	replyToURI, replyToCID := "", ""
	if reply, ok := v["reply"].(map[string]any); ok {
		replyToURI, _ = reply["uri"].(string)
		replyToCID, _ = reply["cid"].(string)
	}

	return &Comment{
		URI:         rec.URI,
		CID:         rec.CID,
		Body:        body,
		SubjectURI:  subjectURI,
		SubjectCID:  subjectCID,
		ReplyToURI:  replyToURI,
		ReplyToCID:  replyToCID,
		CreatedAt:   jsonStr(v, "createdAt"),
	}
}
