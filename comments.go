package tangled

import (
	"context"
	"fmt"
)

// CreateComment creates a comment on an issue or pull request.
// The SubjectURI and SubjectCID should reference the issue or PR record
// (obtained from Issue.URI + Issue.CID, or Pull.URI + Pull.CID).
func (c *Client) CreateComment(ctx context.Context, params CreateCommentParams) (*Comment, error) {
	if err := c.requireAuth(); err != nil {
		return nil, err
	}
	if params.Body == "" {
		return nil, fmt.Errorf("body is required")
	}
	if params.SubjectURI == "" || params.SubjectCID == "" {
		return nil, fmt.Errorf("subject URI and CID are required")
	}

	rkey := generateTID()

	record := map[string]any{
		"$type": CollectionComment,
		"body": map[string]any{
			"$type":     "sh.tangled.markup.markdown",
			"text":      params.Body,
			"original":  params.Body,
		},
		"subject": map[string]any{
			"uri": params.SubjectURI,
			"cid": params.SubjectCID,
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
		URI:         uri,
		CID:         cid,
		Body:        params.Body,
		SubjectURI:  params.SubjectURI,
		SubjectCID:  params.SubjectCID,
		ReplyToURI:  params.ReplyToURI,
		ReplyToCID:  params.ReplyToCID,
		CreatedAt:   record["createdAt"].(string),
	}, nil
}

// ListComments lists comments on an issue or pull request.
// It queries the repo owner's PDS for records in the sh.tangled.feed.comment collection
// that reference the given subject URI.
//
// Note: This only finds comments from users on the same PDS as the repo owner.
// Comments from other PDS instances are not visible via this method.
func (c *Client) ListComments(ctx context.Context, ownerSlashRepo string, subjectURI string, limit int) ([]*Comment, error) {
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
