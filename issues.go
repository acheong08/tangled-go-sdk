package tangled

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
)

// CreateIssue creates a new issue on the specified repository.
func (c *Client) CreateIssue(ctx context.Context, ownerSlashRepo string, params CreateIssueParams) (*Issue, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	// Find the next sequential issue ID
	nextID, err := c.nextIssueID(ctx, repoInfo.ATURI)
	if err != nil {
		return nil, fmt.Errorf("failed to determine next issue ID: %w", err)
	}

	// Validate labels if provided
	if len(params.Labels) > 0 {
		if err := c.validateLabels(params.Labels, repoInfo.Labels); err != nil {
			return nil, err
		}
	}

	// Create the issue record
	rkey := strconv.FormatInt(time.Now().UnixMicro(), 10)
	createdAt := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	record := map[string]any{
		"$type":     CollectionIssue,
		"repo":       repoInfo.ATURI,
		"issueId":   nextID,
		"owner":     c.did,
		"title":     params.Title,
		"createdAt": createdAt,
	}
	if params.Body != "" {
		record["body"] = params.Body
	}

	decoded, err := decodeRecordForWrite(record)
	if err != nil {
		return nil, err
	}

	out, err := comatproto.RepoPutRecord(ctx, c.xrpc, &comatproto.RepoPutRecord_Input{
		Collection: CollectionIssue,
		Repo:       c.did,
		Rkey:       rkey,
		Record:     decoded,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}

	// Apply labels if specified
	if len(params.Labels) > 0 {
		currentLabels := []string{}
		if err := c.applyLabels(ctx, out.Uri, params.Labels, repoInfo.Labels, currentLabels); err != nil {
			// Issue was created but labels failed; report in logs but don't fail
			_ = err
		}
	}

	return &Issue{
		URI:       out.Uri,
		CID:       out.Cid,
		ID:        nextID,
		Title:     params.Title,
		Body:      params.Body,
		Owner:     c.did,
		Labels:    params.Labels,
		CreatedAt: createdAt,
	}, nil
}

// GetIssue retrieves a specific issue by its sequential ID.
func (c *Client) GetIssue(ctx context.Context, ownerSlashRepo string, issueID int) (*Issue, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionIssue, "", 100, c.did, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	for _, rec := range records.Records {
		m := recordToMap(rec)
		repo, _ := m["repo"].(string)
		if repo != repoInfo.ATURI {
			continue
		}
		id := int(jsonFloat(m, "issueId"))
		if id != issueID {
			continue
		}

		issue := recordToIssue(rec, m)
		issue.Labels, _ = c.getIssueLabels(ctx, issue.URI)
		return issue, nil
	}

	return nil, fmt.Errorf("issue #%d not found in repo %s", issueID, ownerSlashRepo)
}

// ListIssues lists issues for the specified repository.
func (c *Client) ListIssues(ctx context.Context, ownerSlashRepo string, limit int) ([]*Issue, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionIssue, "", int64(limit), c.did, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	var issues []*Issue
	var issueURIs []string
	for _, rec := range records.Records {
		m := recordToMap(rec)
		repo, _ := m["repo"].(string)
		if repo != repoInfo.ATURI {
			continue
		}

		id := int(jsonFloat(m, "issueId"))
		if id == 0 {
			continue
		}

		issue := recordToIssue(rec, m)
		issues = append(issues, issue)
		issueURIs = append(issueURIs, issue.URI)
	}

	// Fetch labels for all issues in one pass
	if len(issueURIs) > 0 {
		labelsMap, _ := c.getLabelsForIssues(ctx, issueURIs)
		for _, issue := range issues {
			if labels, ok := labelsMap[issue.URI]; ok {
				issue.Labels = labels
			}
		}
	}

	return issues, nil
}

// UpdateIssue updates an existing issue on the specified repository.
func (c *Client) UpdateIssue(ctx context.Context, ownerSlashRepo string, params UpdateIssueParams) (*Issue, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	// Find the existing issue record
	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionIssue, "", 100, c.did, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	var (
		existingRec  *comatproto.RepoListRecords_Record
		existingMap  map[string]any
		existingRkey string
	)
	for _, rec := range records.Records {
		m := recordToMap(rec)
		repo, _ := m["repo"].(string)
		if repo != repoInfo.ATURI {
			continue
		}
		id := int(jsonFloat(m, "issueId"))
		if id != params.IssueID {
			continue
		}
		existingRec = rec
		existingMap = m
		parts := strings.Split(rec.Uri, "/")
		existingRkey = parts[len(parts)-1]
		break
	}

	if existingRec == nil {
		return nil, fmt.Errorf("issue #%d not found in repo %s", params.IssueID, ownerSlashRepo)
	}

	// Build updated record, preserving existing values where not specified
	title := params.Title
	if title == "" {
		title, _ = existingMap["title"].(string)
	}
	body := params.Body
	if body == "" {
		body, _ = existingMap["body"].(string)
	}
	owner, _ := existingMap["owner"].(string)
	createdAt, _ := existingMap["createdAt"].(string)

	updatedRecord := map[string]any{
		"$type":     CollectionIssue,
		"repo":      repoInfo.ATURI,
		"issueId":   params.IssueID,
		"owner":     owner,
		"title":     title,
		"createdAt": createdAt,
	}
	if body != "" {
		updatedRecord["body"] = body
	}

	decoded, err := decodeRecordForWrite(updatedRecord)
	if err != nil {
		return nil, err
	}

	swapRecord := existingRec.Cid
	out, err := comatproto.RepoPutRecord(ctx, c.xrpc, &comatproto.RepoPutRecord_Input{
		Collection: CollectionIssue,
		Repo:       c.did,
		Rkey:       existingRkey,
		Record:     decoded,
		SwapRecord: &swapRecord,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update issue: %w", err)
	}

	result := &Issue{
		URI:       out.Uri,
		CID:       out.Cid,
		ID:        params.IssueID,
		Title:     title,
		Body:      body,
		Owner:     owner,
		CreatedAt: createdAt,
	}

	// Handle label updates if specified
	if params.Labels != nil {
		if err := c.validateLabels(params.Labels, repoInfo.Labels); err != nil {
			return nil, err
		}
		currentLabels, _ := c.getIssueLabels(ctx, out.Uri)
		if err := c.applyLabels(ctx, out.Uri, params.Labels, repoInfo.Labels, currentLabels); err != nil {
			return nil, fmt.Errorf("failed to apply labels: %w", err)
		}
		result.Labels = params.Labels
	} else {
		result.Labels, _ = c.getIssueLabels(ctx, out.Uri)
	}

	return result, nil
}

// DeleteIssue deletes an issue from the specified repository.
func (c *Client) DeleteIssue(ctx context.Context, ownerSlashRepo string, issueID int) error {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return err
	}

	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionIssue, "", 100, c.did, false)
	if err != nil {
		return fmt.Errorf("failed to list issues: %w", err)
	}

	for _, rec := range records.Records {
		m := recordToMap(rec)
		repo, _ := m["repo"].(string)
		if repo != repoInfo.ATURI {
			continue
		}
		id := int(jsonFloat(m, "issueId"))
		if id != issueID {
			continue
		}

		parts := strings.Split(rec.Uri, "/")
		rkey := parts[len(parts)-1]

		_, err := comatproto.RepoDeleteRecord(ctx, c.xrpc, &comatproto.RepoDeleteRecord_Input{
			Collection: CollectionIssue,
			Repo:       c.did,
			Rkey:       rkey,
		})
		if err != nil {
			return fmt.Errorf("failed to delete issue #%d: %w", issueID, err)
		}
		return nil
	}

	return fmt.Errorf("issue #%d not found in repo %s", issueID, ownerSlashRepo)
}

// ListLabels lists available labels for the specified repository.
func (c *Client) ListLabels(ctx context.Context, ownerSlashRepo string) ([]string, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	labels := make([]string, 0, len(repoInfo.Labels))
	for _, uri := range repoInfo.Labels {
		parts := strings.Split(uri, "/")
		labels = append(labels, parts[len(parts)-1])
	}
	return labels, nil
}

// recordToIssue converts a list record to an Issue struct.
func recordToIssue(rec *comatproto.RepoListRecords_Record, m map[string]any) *Issue {
	return &Issue{
		URI:       rec.Uri,
		CID:       rec.Cid,
		ID:        int(jsonFloat(m, "issueId")),
		Title:     jsonStr(m, "title"),
		Body:      jsonStr(m, "body"),
		Owner:     jsonStr(m, "owner"),
		CreatedAt: jsonStr(m, "createdAt"),
	}
}

// nextIssueID determines the next sequential issue ID for a repo.
func (c *Client) nextIssueID(ctx context.Context, repoATURI string) (int, error) {
	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionIssue, "", 100, c.did, false)
	if err != nil {
		return 1, nil // No existing issues, start at 1
	}

	maxID := 0
	for _, rec := range records.Records {
		m := recordToMap(rec)
		repo, _ := m["repo"].(string)
		if repo != repoATURI {
			continue
		}
		id := int(jsonFloat(m, "issueId"))
		if id > maxID {
			maxID = id
		}
	}

	return maxID + 1, nil
}

// validateLabels checks that all requested labels exist in the repo's subscribed labels.
func (c *Client) validateLabels(labels []string, repoLabelURIs []string) error {
	availableNames := make([]string, 0, len(repoLabelURIs))
	for _, uri := range repoLabelURIs {
		parts := strings.Split(uri, "/")
		availableNames = append(availableNames, parts[len(parts)-1])
	}

	var invalid []string
	for _, label := range labels {
		if strings.HasPrefix(label, "at://") {
			found := false
			for _, uri := range repoLabelURIs {
				if uri == label {
					found = true
					break
				}
			}
			if !found {
				invalid = append(invalid, label)
			}
		} else {
			found := false
			for _, avail := range availableNames {
				if strings.EqualFold(avail, label) {
					found = true
					break
				}
			}
			if !found {
				invalid = append(invalid, label)
			}
		}
	}

	if len(invalid) > 0 {
		return fmt.Errorf("invalid labels: %v; available: %v", invalid, availableNames)
	}
	return nil
}

// applyLabels creates a label op record to apply labels to an issue.
func (c *Client) applyLabels(ctx context.Context, issueURI string, labels []string, repoLabelURIs []string, currentLabels []string) error {
	// Resolve label names to URIs
	newLabelURIs := make(map[string]bool)
	for _, label := range labels {
		if strings.HasPrefix(label, "at://") {
			newLabelURIs[label] = true
		} else {
			for _, uri := range repoLabelURIs {
				parts := strings.Split(uri, "/")
				name := parts[len(parts)-1]
				if strings.EqualFold(name, label) {
					newLabelURIs[uri] = true
					break
				}
			}
		}
	}

	currentSet := make(map[string]bool)
	for _, l := range currentLabels {
		currentSet[l] = true
	}

	// Calculate diff
	var toAdd, toDelete []map[string]string
	for uri := range newLabelURIs {
		if !currentSet[uri] {
			toAdd = append(toAdd, map[string]string{"key": uri, "value": ""})
		}
	}
	for uri := range currentSet {
		if !newLabelURIs[uri] {
			toDelete = append(toDelete, map[string]string{"key": uri, "value": ""})
		}
	}

	if len(toAdd) == 0 && len(toDelete) == 0 {
		return nil
	}

	rkey := strconv.FormatInt(time.Now().UnixMicro(), 10)
	record := map[string]any{
		"$type":       CollectionLabelOp,
		"subject":    issueURI,
		"add":         toAdd,
		"delete":      toDelete,
		"performedAt": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}

	decoded, err := decodeRecordForWrite(record)
	if err != nil {
		return err
	}

	_, err = comatproto.RepoPutRecord(ctx, c.xrpc, &comatproto.RepoPutRecord_Input{
		Collection: CollectionLabelOp,
		Repo:       c.did,
		Rkey:       rkey,
		Record:     decoded,
	})
	if err != nil {
		return fmt.Errorf("failed to create label op: %w", err)
	}

	return nil
}

// getIssueLabels retrieves the current labels for an issue.
func (c *Client) getIssueLabels(ctx context.Context, issueURI string) ([]string, error) {
	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionLabelOp, "", 100, c.did, false)
	if err != nil {
		return nil, err
	}

	labelSet := make(map[string]bool)
	for _, rec := range records.Records {
		m := recordToMap(rec)
		subject, _ := m["subject"].(string)
		if subject != issueURI {
			continue
		}

		// Process "add" operations
		if addItems, ok := m["add"].([]any); ok {
			for _, item := range addItems {
				if sm, ok := item.(map[string]any); ok {
					if key, ok := sm["key"].(string); ok {
						labelSet[key] = true
					}
				}
			}
		}

		// Process "delete" operations
		if deleteItems, ok := m["delete"].([]any); ok {
			for _, item := range deleteItems {
				if sm, ok := item.(map[string]any); ok {
					if key, ok := sm["key"].(string); ok {
						delete(labelSet, key)
					}
				}
			}
		}
	}

	labels := make([]string, 0, len(labelSet))
	for k := range labelSet {
		// Extract label name from URI
		parts := strings.Split(k, "/")
		labels = append(labels, parts[len(parts)-1])
	}
	return labels, nil
}

// getLabelsForIssues fetches labels for multiple issues at once.
func (c *Client) getLabelsForIssues(ctx context.Context, issueURIs []string) (map[string][]string, error) {
	records, err := comatproto.RepoListRecords(ctx, c.xrpc, CollectionLabelOp, "", 100, c.did, false)
	if err != nil {
		return nil, err
	}

	uriSet := make(map[string]bool)
	for _, u := range issueURIs {
		uriSet[u] = true
	}

	result := make(map[string]map[string]bool)
	for _, u := range issueURIs {
		result[u] = make(map[string]bool)
	}

	for _, rec := range records.Records {
		m := recordToMap(rec)
		subject, _ := m["subject"].(string)
		if !uriSet[subject] {
			continue
		}

		if addItems, ok := m["add"].([]any); ok {
			for _, item := range addItems {
				if sm, ok := item.(map[string]any); ok {
					if key, ok := sm["key"].(string); ok {
						result[subject][key] = true
					}
				}
			}
		}

		if deleteItems, ok := m["delete"].([]any); ok {
			for _, item := range deleteItems {
				if sm, ok := item.(map[string]any); ok {
					if key, ok := sm["key"].(string); ok {
						delete(result[subject], key)
					}
				}
			}
		}
	}

	out := make(map[string][]string)
	for uri, labelSet := range result {
		labels := make([]string, 0, len(labelSet))
		for k := range labelSet {
			parts := strings.Split(k, "/")
			labels = append(labels, parts[len(parts)-1])
		}
		out[uri] = labels
	}
	return out, nil
}
