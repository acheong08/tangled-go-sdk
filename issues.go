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
	repoRef := repoInfo.RepoDID
	if repoRef == "" {
		repoRef = repoInfo.DID
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

	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if !repoRefMatches(repo, repoInfo) {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id != issueID {
			continue
		}

		issue := rawRecordToIssue(rec)
		issue.Labels, _ = c.getIssueLabels(ctx, issue.URI)
		// Resolve issue state
		state, _ := c.getIssueState(ctx, issue.URI)
		if state != "" {
			issue.State = state
		}
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

	var issues []*Issue
	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if !repoRefMatches(repo, repoInfo) {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id == 0 {
			continue
		}
		issues = append(issues, rawRecordToIssue(rec))
	}

	// Resolve issue states (closed issues have sh.tangled.repo.issue.state records)
	if len(issues) > 0 {
		stateMap, _ := c.getIssueStates(ctx, issues)
		for _, issue := range issues {
			if state, ok := stateMap[issue.URI]; ok {
				issue.State = state
			}
		}
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

	var issues []*Issue
	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if !repoRefMatches(repo, repoInfo) {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id == 0 {
			continue
		}
		issues = append(issues, rawRecordToIssue(rec))
	}

	// Resolve issue states from the owner's PDS
	if len(issues) > 0 {
		accessToken := ""
		if c.IsAuthenticated() {
			accessToken = c.accessJWT
		}
		stateRecords, err := pdsListRecords(ctx, pdsURL, repoInfo.DID, CollectionIssueState, 100, accessToken)
		if err == nil {
			// Build URI -> latest state map (replay in order, last write wins)
			uriToState := make(map[string]string)
			issueURISet := make(map[string]bool)
			for _, issue := range issues {
				issueURISet[issue.URI] = true
			}
			for _, rec := range stateRecords {
				issueAtURI := jsonStr(rec.Value, "issue")
				if !issueURISet[issueAtURI] {
					continue
			}
				state := jsonStr(rec.Value, "state")
				// listRecords returns in reverse chronological order (newest first),
				// so the first match is the latest state
				if _, exists := uriToState[issueAtURI]; !exists {
					uriToState[issueAtURI] = state
				}
			}
			for _, issue := range issues {
				if state, ok := uriToState[issue.URI]; ok {
					issue.State = state
				}
			}
		}
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

	var (
		existingMap  map[string]any
		existingCID  string
		existingRkey string
	)
	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if !repoRefMatches(repo, repoInfo) {
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

	// Handle state change
	if params.State != "" {
		if err := c.setIssueState(ctx, uri, params.State); err != nil {
			return nil, fmt.Errorf("failed to set issue state: %w", err)
		}
		result.State = params.State
	} else {
		// Preserve existing state
		state, _ := c.getIssueState(ctx, uri)
		if state != "" {
			result.State = state
		} else {
			result.State = IssueStateOpen
		}
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

	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if !repoRefMatches(repo, repoInfo) {
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
// Resolves label definition records to get the human-readable name,
// falling back to the rkey for newer labels that use human-readable rkeys.
func (c *Client) ListLabels(ctx context.Context, ownerSlashRepo string) ([]string, error) {
	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return nil, err
	}

	if len(repoInfo.Labels) == 0 {
		return nil, nil
	}

	// Resolve the label owner's PDS from the first label URI
	// Label URIs look like: at://did:plc:.../sh.tangled.label.definition/rkey
	labelOwnerDID, _ := extractDIDFromATURI(repoInfo.Labels[0])
	if labelOwnerDID == "" {
		// Fallback: just use rkeys
		return labelRkeys(repoInfo.Labels), nil
	}

	pdsURL, err := resolvePDS(ctx, labelOwnerDID)
	if err != nil {
		return labelRkeys(repoInfo.Labels), nil
	}

	// Fetch label definitions to resolve TID rkeys to names
	accessToken := ""
	if c.IsAuthenticated() {
		accessToken = c.accessJWT
	}
	records, err := pdsListRecords(ctx, pdsURL, labelOwnerDID, "sh.tangled.label.definition", 100, accessToken)
	if err != nil {
		return labelRkeys(repoInfo.Labels), nil
	}

	// Build rkey -> name map
	rkeyToName := make(map[string]string)
	for _, rec := range records {
		rkey, _ := extractRkey(rec.URI)
		name, _ := rec.Value["name"].(string)
		if name == "" {
			name = rkey
		}
		rkeyToName[rkey] = name
	}

	labels := make([]string, 0, len(repoInfo.Labels))
	for _, uri := range repoInfo.Labels {
		rkey, _ := extractRkey(uri)
		if name, ok := rkeyToName[rkey]; ok {
			labels = append(labels, name)
		} else {
			labels = append(labels, rkey)
		}
	}
	return labels, nil
}

// labelRkeys extracts rkeys from label URIs as a fallback.
func labelRkeys(uris []string) []string {
	labels := make([]string, 0, len(uris))
	for _, uri := range uris {
		rkey, _ := extractRkey(uri)
		labels = append(labels, rkey)
	}
	return labels
}

// extractDIDFromATURI extracts the DID from an AT-URI (at://did:plc:.../collection/rkey).
func extractDIDFromATURI(uri string) (string, error) {
	if !strings.HasPrefix(uri, "at://") {
		return "", fmt.Errorf("not an AT-URI: %q", uri)
	}
	// at://did:plc:xxx/collection/rkey → ["at:", "", "did:plc:xxx", "collection", "rkey"]
	parts := strings.Split(uri, "/")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid AT-URI: %q", uri)
	}
	return parts[2], nil
}

// rawRecordToIssue converts a raw pdsRecord to an Issue.
func rawRecordToIssue(rec pdsRecord) *Issue {
	return &Issue{
		URI:       rec.URI,
		CID:       rec.CID,
		ID:        int(jsonFloat(rec.Value, "issueId")),
		Title:     jsonStr(rec.Value, "title"),
		Body:      jsonStr(rec.Value, "body"),
		State:     IssueStateOpen, // default; will be overridden if a state record exists
		Owner:     jsonStr(rec.Value, "owner"),
		CreatedAt: jsonStr(rec.Value, "createdAt"),
	}
}

// nextIssueID determines the next sequential issue ID for a repo.
// It queries both the authenticated user's PDS and the repo owner's PDS
// to find the highest existing issue ID.
// Note: Issues from other contributors on different PDS instances may not be visible,
// so the ID could conflict. The Tangled appview tracks the definitive ID sequence.
func (c *Client) nextIssueID(ctx context.Context, repoInfo *RepoInfo) (int, error) {
	maxID := 0

	// Check authenticated user's PDS
	authRecords, err := c.pdsListRecords(ctx, CollectionIssue, 100)
	if err == nil {
		for _, rec := range authRecords {
			repo, _ := rec.Value["repo"].(string)
			if !repoRefMatches(repo, repoInfo) {
				continue
			}
			id := int(jsonFloat(rec.Value, "issueId"))
			if id > maxID {
				maxID = id
			}
		}
	}

	// Check repo owner's PDS (may have issues from other collaborators on the same PDS)
	ownerPDS, err := resolvePDS(ctx, repoInfo.DID)
	if err == nil {
		accessToken := ""
		if c.IsAuthenticated() {
			accessToken = c.accessJWT
		}
		ownerRecords, err := pdsListRecords(ctx, ownerPDS, repoInfo.DID, CollectionIssue, 100, accessToken)
		if err == nil {
			for _, rec := range ownerRecords {
				repo, _ := rec.Value["repo"].(string)
				if !repoRefMatches(repo, repoInfo) {
					continue
				}
				id := int(jsonFloat(rec.Value, "issueId"))
				if id > maxID {
					maxID = id
				}
			}
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

// CloseIssue closes an issue by creating a sh.tangled.repo.issue.state record
// with state "sh.tangled.repo.issue.state.closed".
// This follows the ATProto event-sourcing pattern where state is stored separately
// from the issue record itself.
func (c *Client) CloseIssue(ctx context.Context, ownerSlashRepo string, issueID int) error {
	return c.setIssueStateByID(ctx, ownerSlashRepo, issueID, IssueStateClosed)
}

// ReopenIssue reopens a closed issue by creating a sh.tangled.repo.issue.state record
// with state "sh.tangled.repo.issue.state.open".
func (c *Client) ReopenIssue(ctx context.Context, ownerSlashRepo string, issueID int) error {
	return c.setIssueStateByID(ctx, ownerSlashRepo, issueID, IssueStateOpen)
}

// setIssueStateByID sets the state of an issue by its numeric ID.
func (c *Client) setIssueStateByID(ctx context.Context, ownerSlashRepo string, issueID int, state string) error {
	if err := c.requireAuth(); err != nil {
		return err
	}

	repoInfo, err := c.ResolveRepo(ctx, ownerSlashRepo)
	if err != nil {
		return err
	}

	// Find the issue record to get its AT-URI
	records, err := c.pdsListRecords(ctx, CollectionIssue, 100)
	if err != nil {
		return fmt.Errorf("failed to list issues: %w", err)
	}

	var issueURI string
	for _, rec := range records {
		repo, _ := rec.Value["repo"].(string)
		if !repoRefMatches(repo, repoInfo) {
			continue
		}
		id := int(jsonFloat(rec.Value, "issueId"))
		if id != issueID {
			continue
		}
		issueURI = rec.URI
		break
	}

	if issueURI == "" {
		return fmt.Errorf("issue #%d not found in repo %s", issueID, ownerSlashRepo)
	}

	return c.setIssueState(ctx, issueURI, state)
}

// setIssueState creates or updates a sh.tangled.repo.issue.state record for the given issue AT-URI.
func (c *Client) setIssueState(ctx context.Context, issueURI, state string) error {
	rkey := generateTID()

	record := map[string]any{
		"$type":     CollectionIssueState,
		"issue":     issueURI,
		"state":     state,
		"createdAt": nowISO(),
	}

	_, _, err := c.pdsPutRecord(ctx, CollectionIssueState, rkey, record, nil)
	if err != nil {
		return fmt.Errorf("failed to create issue state record: %w", err)
	}
	return nil
}

// getIssueState returns the current state of an issue by replaying state records.
// Returns empty string if no state record exists (which means open).
func (c *Client) getIssueState(ctx context.Context, issueURI string) (string, error) {
	records, err := c.pdsListRecords(ctx, CollectionIssueState, 100)
	if err != nil {
		return "", err
	}

	// Find the latest state for this issue
	var latestState string
	var latestCreatedAt string
	for _, rec := range records {
		issueAtURI := jsonStr(rec.Value, "issue")
		if issueAtURI != issueURI {
			continue
		}
		state := jsonStr(rec.Value, "state")
		createdAt := jsonStr(rec.Value, "createdAt")
		if createdAt >= latestCreatedAt { // >= picks the last one if timestamps match
			latestState = state
			latestCreatedAt = createdAt
		}
	}

	return latestState, nil
}

// getIssueStates returns a map of issue AT-URI → current state for the given issues.
func (c *Client) getIssueStates(ctx context.Context, issues []*Issue) (map[string]string, error) {
	records, err := c.pdsListRecords(ctx, CollectionIssueState, 100)
	if err != nil {
		return nil, err
	}

	// Build a set of issue URIs we care about
	uriSet := make(map[string]bool)
	for _, issue := range issues {
		uriSet[issue.URI] = true
	}

	// listRecords returns newest first, so first write wins
	result := make(map[string]string)
	for _, rec := range records {
		issueAtURI := jsonStr(rec.Value, "issue")
		if !uriSet[issueAtURI] {
			continue
		}
		state := jsonStr(rec.Value, "state")
		if _, exists := result[issueAtURI]; !exists {
			result[issueAtURI] = state
		}
	}

	return result, nil
}
