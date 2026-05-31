package tangled

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// CreateComment creates a comment on an issue.
// SubjectRef can be a numeric issue ID or a full AT-URI.
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
	if params.SubjectRef == "" {
		return nil, fmt.Errorf("subject reference is required")
	}

	// Resolve the subject to its AT-URI and CID
	subjectURI, subjectCID, err := c.resolveSubject(ctx, params.OwnerSlashRepo, params.SubjectRef)
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

// ListComments lists comments on a repository, optionally filtered by subject.
// If subjectRef is empty, all comments from the repo owner's PDS are returned.
// subjectRef can be a numeric issue ID, a PR rkey, or a full AT-URI.
//
// Note: This only finds comments from users on the same PDS as the repo owner.
// Comments from other PDS instances are not visible via this method.
func (c *Client) ListComments(ctx context.Context, ownerSlashRepo string, subjectRef string, limit int) ([]*Comment, error) {
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

	// If a subjectRef is given, resolve it to an AT-URI for filtering
	var subjectURI string
	if subjectRef != "" {
		subjectURI, _, err = c.resolveSubject(ctx, ownerSlashRepo, subjectRef)
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

// resolveSubject finds the AT-URI and CID for an issue or PR.
// subjectRef can be:
//   - A numeric ID like "1" (searches issues, then PRs by ID)
//   - A PR rkey like "3mn4h47wpdk22" (fetches the PR record directly)
//   - A full AT-URI like "at://did:.../sh.tangled.repo.issue/rkey"
func (c *Client) resolveSubject(ctx context.Context, ownerSlashRepo string, subjectRef string) (uri, cid string, err error) {
	// Case 1: Full AT-URI
	if strings.HasPrefix(subjectRef, "at://") {
		return c.resolveSubjectByATURI(ctx, subjectRef)
	}

	// Case 2: Numeric ID (issue or PR)
	if id, err := strconv.Atoi(subjectRef); err == nil {
		return c.resolveSubjectByNumericID(ctx, ownerSlashRepo, id)
	}

	// Case 3: Rkey — try as a PR rkey first, then issue rkey
	return c.resolveSubjectByRkey(ctx, ownerSlashRepo, subjectRef)
}

// resolveSubjectByATURI resolves a full AT-URI to its CID by fetching the record.
func (c *Client) resolveSubjectByATURI(ctx context.Context, atURI string) (uri, cid string, err error) {
	// Parse: at://did:plc:.../collection/rkey
	parts := strings.Split(atURI, "/")
	if len(parts) < 5 {
		return "", "", fmt.Errorf("invalid AT-URI: %s", atURI)
	}
	did := parts[2]
	collection := parts[3]
	rkey := parts[4]

	pdsURL, err := resolvePDS(ctx, did)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve PDS: %w", err)
	}

	accessToken := ""
	if c.IsAuthenticated() {
		accessToken = c.accessJWT
	}

	record, err := pdsGetRecord(ctx, pdsURL, did, collection, rkey, accessToken)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch record: %w", err)
	}

	return record.URI, record.CID, nil
}

// resolveSubjectByNumericID finds an issue by its numeric ID.
func (c *Client) resolveSubjectByNumericID(ctx context.Context, ownerSlashRepo string, id int) (uri, cid string, err error) {
	issues, err := c.ListIssues(ctx, ownerSlashRepo, 100)
	if err == nil {
		for _, issue := range issues {
			if issue.ID == id {
				return issue.URI, issue.CID, nil
			}
		}
	}

	return "", "", fmt.Errorf("issue #%d not found on %s", id, ownerSlashRepo)
}

// resolveSubjectByRkey tries to resolve an issue by its record key.
func (c *Client) resolveSubjectByRkey(ctx context.Context, ownerSlashRepo string, rkey string) (uri, cid string, err error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return "", "", err
	}

	pdsURL, err := resolvePDS(ctx, repoInfo.DID)
	if err != nil {
		return "", "", err
	}

	accessToken := ""
	if c.IsAuthenticated() {
		accessToken = c.accessJWT
	}

	record, err := pdsGetRecord(ctx, pdsURL, repoInfo.DID, CollectionIssue, rkey, accessToken)
	if err == nil {
		return record.URI, record.CID, nil
	}

	return "", "", fmt.Errorf("no issue with rkey %q found on %s", rkey, ownerSlashRepo)
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
