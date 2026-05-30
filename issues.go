package tangled

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CreateIssue creates a new issue on the specified repository.
func (c *Client) CreateIssue(ctx context.Context, ownerSlashRepo string, params CreateIssueParams) (*Issue, error) {
	if err := c.requireAuth(); err != nil {
		return nil, err
	}

	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	nextID, err := c.nextIssueID(ctx, repoInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to determine next issue ID: %w", err)
	}

	if len(params.Labels) > 0 {
		if err := c.validateLabels(params.Labels, repoInfo.Labels); err != nil {
			return nil, err
		}
	}

	rkey := strconv.FormatInt(time.Now().UnixMicro(), 10)
	createdAt := nowISO()

	// Use RepoDID (stable repo identity) for the repo field, falling back to owner DID
	repoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		repoRef = repoInfo.RepoDID
	}

	record := map[string]any{
		"$type":     CollectionIssue,
		"repo":      repoRef,
		"issueId":   nextID,
		"owner":     c.did,
		"title":     params.Title,
		"createdAt": createdAt,
	}
	if params.Body != "" {
		record["body"] = params.Body
	}

	uri, cid, err := c.pdsPutRecord(ctx, CollectionIssue, rkey, record, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}

	// Apply labels if specified
	if len(params.Labels) > 0 {
		if err := c.applyLabels(ctx, uri, params.Labels, repoInfo.Labels, nil); err != nil {
			// Issue was created but labels failed
			_ = err
		}
	}

	return &Issue{
		URI:       uri,
		CID:       cid,
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
	if err := c.requireAuth(); err != nil {
		return nil, err
	}

	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	records, err := c.pdsListRecords(ctx, CollectionIssue, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	repoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		repoRef = repoInfo.RepoDID
	}

	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if repo != repoRef && repo != repoInfo.ATURI {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id != issueID {
			continue
		}

		issue := rawRecordToIssue(rec)
		issue.Labels, _ = c.getIssueLabels(ctx, issue.URI)
		return issue, nil
	}

	return nil, fmt.Errorf("issue #%d not found in repo %s", issueID, ownerSlashRepo)
}

// ListIssues lists issues for the specified repository.
// With an authenticated client, it queries the user's PDS.
// With a public client, it falls back to direct PDS queries against the repo owner.
func (c *Client) ListIssues(ctx context.Context, ownerSlashRepo string, limit int) ([]*Issue, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	if c.IsAuthenticated() {
		return c.listIssuesAuthenticated(ctx, repoInfo, limit)
	}
	return c.listIssuesPublic(ctx, repoInfo, limit)
}

func (c *Client) listIssuesAuthenticated(ctx context.Context, repoInfo *RepoInfo, limit int) ([]*Issue, error) {
	records, err := c.pdsListRecords(ctx, CollectionIssue, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	repoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		repoRef = repoInfo.RepoDID
	}

	var issues []*Issue
	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if repo != repoRef && repo != repoInfo.ATURI {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id == 0 {
			continue
		}
		issues = append(issues, rawRecordToIssue(rec))
	}

	// Fetch labels in one pass
	issueURIs := make([]string, len(issues))
	for i, issue := range issues {
		issueURIs[i] = issue.URI
	}
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

func (c *Client) listIssuesPublic(ctx context.Context, repoInfo *RepoInfo, limit int) ([]*Issue, error) {
	pdsURL, err := resolvePDS(ctx, repoInfo.DID)
	if err != nil {
		return nil, err
	}

	records, err := pdsListRecords(ctx, pdsURL, repoInfo.DID, CollectionIssue, limit, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	repoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		repoRef = repoInfo.RepoDID
	}

	var issues []*Issue
	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if repo != repoRef && repo != repoInfo.ATURI {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id == 0 {
			continue
		}
		issues = append(issues, rawRecordToIssue(rec))
	}
	return issues, nil
}

// UpdateIssue updates an existing issue on the specified repository.
func (c *Client) UpdateIssue(ctx context.Context, ownerSlashRepo string, params UpdateIssueParams) (*Issue, error) {
	if err := c.requireAuth(); err != nil {
		return nil, err
	}

	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	records, err := c.pdsListRecords(ctx, CollectionIssue, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	repoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		repoRef = repoInfo.RepoDID
	}

	var (
		existingMap  map[string]any
		existingCID  string
		existingRkey string
	)
	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if repo != repoRef && repo != repoInfo.ATURI {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id != params.IssueID {
			continue
		}
		existingMap = rec.Value
		existingCID = rec.CID
		existingRkey, err = extractRkey(rec.URI)
		if err != nil {
			return nil, err
		}
		break
	}

	if existingMap == nil {
		return nil, fmt.Errorf("issue #%d not found in repo %s", params.IssueID, ownerSlashRepo)
	}

	// Build updated record — *string nil means don't change, pointer to "" means clear
	title := jsonStr(existingMap, "title")
	if params.Title != nil {
		title = *params.Title
	}
	body := jsonStr(existingMap, "body")
	if params.Body != nil {
		body = *params.Body
	}
	owner := jsonStr(existingMap, "owner")
	createdAt := jsonStr(existingMap, "createdAt")
	repo := jsonStr(existingMap, "repo")

	updatedRecord := map[string]any{
		"$type":     CollectionIssue,
		"repo":      repo,
		"issueId":   params.IssueID,
		"owner":     owner,
		"title":     title,
		"createdAt": createdAt,
	}
	// Always include body when explicitly set (even if empty), skip if unchanged
	if params.Body != nil || body != "" {
		updatedRecord["body"] = body
	}

	uri, cid, err := c.pdsPutRecord(ctx, CollectionIssue, existingRkey, updatedRecord, &existingCID)
	if err != nil {
		return nil, fmt.Errorf("failed to update issue: %w", err)
	}

	result := &Issue{
		URI:       uri,
		CID:       cid,
		ID:        params.IssueID,
		Title:     title,
		Body:      body,
		Owner:     owner,
		CreatedAt: createdAt,
	}

	if params.Labels != nil {
		if err := c.validateLabels(params.Labels, repoInfo.Labels); err != nil {
			return nil, err
		}
		currentLabels, _ := c.getIssueLabels(ctx, uri)
		if err := c.applyLabels(ctx, uri, params.Labels, repoInfo.Labels, currentLabels); err != nil {
			return nil, fmt.Errorf("failed to apply labels: %w", err)
		}
		result.Labels = params.Labels
	} else {
		result.Labels, _ = c.getIssueLabels(ctx, uri)
	}

	return result, nil
}

// DeleteIssue deletes an issue from the specified repository.
func (c *Client) DeleteIssue(ctx context.Context, ownerSlashRepo string, issueID int) error {
	if err := c.requireAuth(); err != nil {
		return err
	}

	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return err
	}

	records, err := c.pdsListRecords(ctx, CollectionIssue, 100)
	if err != nil {
		return fmt.Errorf("failed to list issues: %w", err)
	}

	repoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		repoRef = repoInfo.RepoDID
	}

	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if repo != repoRef && repo != repoInfo.ATURI {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id != issueID {
			continue
		}

		rkey, err := extractRkey(rec.URI)
		if err != nil {
			return err
		}

		if err := c.pdsDeleteRecord(ctx, CollectionIssue, rkey); err != nil {
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

// rawRecordToIssue converts a raw pdsRecord to an Issue.
func rawRecordToIssue(rec pdsRecord) *Issue {
	return &Issue{
		URI:       rec.URI,
		CID:       rec.CID,
		ID:        int(jsonFloat(rec.Value, "issueId")),
		Title:     jsonStr(rec.Value, "title"),
		Body:      jsonStr(rec.Value, "body"),
		Owner:     jsonStr(rec.Value, "owner"),
		CreatedAt: jsonStr(rec.Value, "createdAt"),
	}
}

// nextIssueID determines the next sequential issue ID for a repo.
func (c *Client) nextIssueID(ctx context.Context, repoInfo *RepoInfo) (int, error) {
	records, err := c.pdsListRecords(ctx, CollectionIssue, 100)
	if err != nil {
		return 0, fmt.Errorf("failed to list existing issues: %w", err)
	}

	repoRef := repoInfo.DID
	if repoInfo.RepoDID != "" {
		repoRef = repoInfo.RepoDID
	}

	maxID := 0
	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if repo != repoRef && repo != repoInfo.ATURI {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id > maxID {
			maxID = id
		}
	}

	return maxID + 1, nil
}

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

func (c *Client) applyLabels(ctx context.Context, issueURI string, labels []string, repoLabelURIs []string, currentLabels []string) error {
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
		"performedAt": nowISO(),
	}

	_, _, err := c.pdsPutRecord(ctx, CollectionLabelOp, rkey, record, nil)
	if err != nil {
		return fmt.Errorf("failed to create label op: %w", err)
	}
	return nil
}

func (c *Client) getIssueLabels(ctx context.Context, issueURI string) ([]string, error) {
	records, err := c.pdsListRecords(ctx, CollectionLabelOp, 100)
	if err != nil {
		return nil, err
	}

	labelSet := make(map[string]bool)
	for _, rec := range records {
		subject := jsonStr(rec.Value, "subject")
		if subject != issueURI {
			continue
		}
		if addItems, ok := rec.Value["add"].([]any); ok {
			for _, item := range addItems {
				if sm, ok := item.(map[string]any); ok {
					if key, ok := sm["key"].(string); ok {
						labelSet[key] = true
					}
				}
			}
		}
		if deleteItems, ok := rec.Value["delete"].([]any); ok {
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
		parts := strings.Split(k, "/")
		labels = append(labels, parts[len(parts)-1])
	}
	return labels, nil
}

func (c *Client) getLabelsForIssues(ctx context.Context, issueURIs []string) (map[string][]string, error) {
	records, err := c.pdsListRecords(ctx, CollectionLabelOp, 100)
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

	for _, rec := range records {
		subject := jsonStr(rec.Value, "subject")
		if !uriSet[subject] {
			continue
		}
		if addItems, ok := rec.Value["add"].([]any); ok {
			for _, item := range addItems {
				if sm, ok := item.(map[string]any); ok {
					if key, ok := sm["key"].(string); ok {
						result[subject][key] = true
					}
				}
			}
		}
		if deleteItems, ok := rec.Value["delete"].([]any); ok {
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
