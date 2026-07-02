// Package azuredevops is a minimal client for the Azure DevOps REST API,
// covering only the endpoints needed by the exporter's collectors.
package azuredevops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const apiVersion = "7.1"

// pageSize is the page size used for endpoints paginated via $top/$skip.
const pageSize = 100

// workItemBatchSize is the max number of IDs accepted per workitemsbatch call.
const workItemBatchSize = 200

// Client talks to the Azure DevOps REST API using a Personal Access Token.
type Client struct {
	baseURL        string
	releaseBaseURL string
	token          string
	httpClient     *http.Client
}

// NewClient creates a client for the given Azure DevOps API base URL (e.g. https://dev.azure.com)
// and organization, authenticating with a Personal Access Token.
//
// Release Management (classic Releases) is served from a different host than every other
// API used here — vsrm.dev.azure.com instead of dev.azure.com — so releaseBaseURL is derived
// separately. On Azure DevOps Server (on-prem), where there's no such host split, the
// substitution is a no-op and releaseBaseURL falls back to baseURL.
func NewClient(apiURL, organization, token string) *Client {
	releaseAPIURL := strings.Replace(apiURL, "dev.azure.com", "vsrm.dev.azure.com", 1)
	return &Client{
		baseURL:        fmt.Sprintf("%s/%s", apiURL, organization),
		releaseBaseURL: fmt.Sprintf("%s/%s", releaseAPIURL, organization),
		token:          token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type Repository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"` // bytes
}

// Reviewer is a pull request reviewer's vote. Vote > 0 means approved (10) or approved with
// suggestions (5); 0 means no vote cast; negative means waiting for author (-5) or rejected (-10).
type Reviewer struct {
	Vote int `json:"vote"`
}

type PullRequest struct {
	PullRequestID int        `json:"pullRequestId"`
	Status        string     `json:"status"` // active, completed, abandoned
	IsDraft       bool       `json:"isDraft"`
	MergeStatus   string     `json:"mergeStatus"` // succeeded, conflicts, failure, ...
	CreationDate  time.Time  `json:"creationDate"`
	ClosedDate    time.Time  `json:"closedDate"` // zero unless status is completed or abandoned
	Reviewers     []Reviewer `json:"reviewers"`
}

type ref struct {
	Name string `json:"name"`
}

// ListRepositories returns all Git repositories in a project.
func (c *Client) ListRepositories(project string) ([]Repository, error) {
	path := fmt.Sprintf("%s/%s/_apis/git/repositories", c.baseURL, url.PathEscape(project))
	var result struct {
		Value []Repository `json:"value"`
	}
	if err := c.get(path, url.Values{"api-version": {apiVersion}}, &result); err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	return result.Value, nil
}

// ListPullRequests returns all pull requests (any status) for a repository,
// paginating through results via $top/$skip.
func (c *Client) ListPullRequests(project, repositoryID string) ([]PullRequest, error) {
	path := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/pullrequests", c.baseURL, url.PathEscape(project), url.PathEscape(repositoryID))

	var all []PullRequest
	skip := 0
	for {
		var page struct {
			Value []PullRequest `json:"value"`
		}
		query := url.Values{
			"api-version":           {apiVersion},
			"searchCriteria.status": {"all"},
			"$top":                  {fmt.Sprint(pageSize)},
			"$skip":                 {fmt.Sprint(skip)},
		}
		if err := c.get(path, query, &page); err != nil {
			return nil, fmt.Errorf("list pull requests: %w", err)
		}
		all = append(all, page.Value...)
		if len(page.Value) < pageSize {
			break
		}
		skip += pageSize
	}
	return all, nil
}

// CountBranches returns the number of Git branches (heads) in a repository,
// paginating through results via the continuation token header.
func (c *Client) CountBranches(project, repositoryID string) (int, error) {
	path := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/refs", c.baseURL, url.PathEscape(project), url.PathEscape(repositoryID))

	count := 0
	continuationToken := ""
	for {
		var page struct {
			Value []ref `json:"value"`
		}
		query := url.Values{
			"api-version": {apiVersion},
			"filter":      {"heads/"},
		}
		if continuationToken != "" {
			query.Set("continuationToken", continuationToken)
		}
		header, err := c.getWithHeader(path, query, &page)
		if err != nil {
			return 0, fmt.Errorf("list branches: %w", err)
		}
		count += len(page.Value)
		continuationToken = header.Get("x-ms-continuationtoken")
		if continuationToken == "" {
			break
		}
	}
	return count, nil
}

// Commit is a Git commit's author, for grouping recent activity by author.
type Commit struct {
	Author struct {
		Name string `json:"name"`
	} `json:"author"`
}

// ListCommitsSince returns commits pushed to a repository (any branch) since the given time,
// paginating through results via $top/$skip. Bounding the query with fromDate keeps this cheap
// even on repositories with long history — unlike a full commit list, which would require
// walking the entire history with no server-side total.
func (c *Client) ListCommitsSince(project, repositoryID string, since time.Time) ([]Commit, error) {
	path := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/commits", c.baseURL, url.PathEscape(project), url.PathEscape(repositoryID))

	var all []Commit
	skip := 0
	for {
		var page struct {
			Value []Commit `json:"value"`
		}
		query := url.Values{
			"api-version":             {apiVersion},
			"searchCriteria.fromDate": {since.UTC().Format(time.RFC3339)},
			"$top":                    {fmt.Sprint(pageSize)},
			"$skip":                   {fmt.Sprint(skip)},
		}
		if err := c.get(path, query, &page); err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}
		all = append(all, page.Value...)
		if len(page.Value) < pageSize {
			break
		}
		skip += pageSize
	}
	return all, nil
}

// BuildDefinition is a pipeline definition's ID and name.
type BuildDefinition struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Build is a pipeline run's status, result and timing.
type Build struct {
	ID           int       `json:"id"`
	Status       string    `json:"status"` // completed, inProgress, cancelling, postponed, notStarted
	Result       string    `json:"result"` // succeeded, partiallySucceeded, failed, canceled (only set once completed)
	QueueTime    time.Time `json:"queueTime"`
	StartTime    time.Time `json:"startTime"`
	FinishTime   time.Time `json:"finishTime"`
	SourceBranch string    `json:"sourceBranch"` // e.g. refs/heads/main
	Definition   struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"definition"`
}

// ListBuildDefinitions returns all pipeline (build) definitions in a project.
func (c *Client) ListBuildDefinitions(project string) ([]BuildDefinition, error) {
	path := fmt.Sprintf("%s/%s/_apis/build/definitions", c.baseURL, url.PathEscape(project))
	var result struct {
		Value []BuildDefinition `json:"value"`
	}
	if err := c.get(path, url.Values{"api-version": {apiVersion}}, &result); err != nil {
		return nil, fmt.Errorf("list build definitions: %w", err)
	}
	return result.Value, nil
}

// ListBuildsSince returns all pipeline runs (any definition) queued in a project since the
// given time, paginating through results via the continuation token header.
func (c *Client) ListBuildsSince(project string, since time.Time) ([]Build, error) {
	path := fmt.Sprintf("%s/%s/_apis/build/builds", c.baseURL, url.PathEscape(project))

	var all []Build
	continuationToken := ""
	for {
		var page struct {
			Value []Build `json:"value"`
		}
		query := url.Values{
			"api-version": {apiVersion},
			"minTime":     {since.UTC().Format(time.RFC3339)},
		}
		if continuationToken != "" {
			query.Set("continuationToken", continuationToken)
		}
		header, err := c.getWithHeader(path, query, &page)
		if err != nil {
			return nil, fmt.Errorf("list builds: %w", err)
		}
		all = append(all, page.Value...)
		continuationToken = header.Get("x-ms-continuationtoken")
		if continuationToken == "" {
			break
		}
	}
	return all, nil
}

// GetLatestBuild returns the most recently queued run of a pipeline definition, or nil if it
// has never run.
func (c *Client) GetLatestBuild(project string, definitionID int) (*Build, error) {
	path := fmt.Sprintf("%s/%s/_apis/build/builds", c.baseURL, url.PathEscape(project))
	query := url.Values{
		"api-version": {apiVersion},
		"definitions": {fmt.Sprint(definitionID)},
		"queryOrder":  {"queueTimeDescending"},
		"$top":        {"1"},
	}
	var page struct {
		Value []Build `json:"value"`
	}
	if err := c.get(path, query, &page); err != nil {
		return nil, fmt.Errorf("get latest build: %w", err)
	}
	if len(page.Value) == 0 {
		return nil, nil
	}
	return &page.Value[0], nil
}

// ReleaseDefinition is a classic release definition's ID and name.
type ReleaseDefinition struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Deployment is a classic release deployment's status and timing for one environment.
type Deployment struct {
	DeploymentStatus  string    `json:"deploymentStatus"` // succeeded, failed, partiallySucceeded, inProgress, notDeployed
	StartedOn         time.Time `json:"startedOn"`
	CompletedOn       time.Time `json:"completedOn"`
	ReleaseDefinition struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"releaseDefinition"`
	ReleaseEnvironment struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"releaseEnvironment"`
	Release struct {
		ID int `json:"id"`
	} `json:"release"`
}

// Release is a classic release's artifacts, used to trace a deployment back to the build (and
// therefore commits) that produced it.
type Release struct {
	Artifacts []struct {
		Type                string `json:"type"` // "Build" for a pipeline artifact; container images and other source types don't trace to commits
		DefinitionReference struct {
			Version struct {
				ID string `json:"id"` // build ID, as a string
			} `json:"version"`
		} `json:"definitionReference"`
	} `json:"artifacts"`
}

// GetRelease returns a single release's detail, including its artifacts.
func (c *Client) GetRelease(project string, releaseID int) (*Release, error) {
	path := fmt.Sprintf("%s/%s/_apis/release/releases/%d", c.releaseBaseURL, url.PathEscape(project), releaseID)
	var release Release
	if err := c.get(path, url.Values{"api-version": {apiVersion}}, &release); err != nil {
		return nil, fmt.Errorf("get release: %w", err)
	}
	return &release, nil
}

// Change is a single commit included in a build, relative to the previous build on the same
// branch.
type Change struct {
	Timestamp time.Time `json:"timestamp"`
}

// GetBuildChanges returns the commits included in a build.
func (c *Client) GetBuildChanges(project string, buildID int) ([]Change, error) {
	path := fmt.Sprintf("%s/%s/_apis/build/builds/%d/changes", c.baseURL, url.PathEscape(project), buildID)
	var result struct {
		Value []Change `json:"value"`
	}
	if err := c.get(path, url.Values{"api-version": {apiVersion}}, &result); err != nil {
		return nil, fmt.Errorf("get build changes: %w", err)
	}
	return result.Value, nil
}

// ListReleaseDefinitions returns all classic release definitions in a project.
func (c *Client) ListReleaseDefinitions(project string) ([]ReleaseDefinition, error) {
	path := fmt.Sprintf("%s/%s/_apis/release/definitions", c.releaseBaseURL, url.PathEscape(project))
	var result struct {
		Value []ReleaseDefinition `json:"value"`
	}
	if err := c.get(path, url.Values{"api-version": {apiVersion}}, &result); err != nil {
		return nil, fmt.Errorf("list release definitions: %w", err)
	}
	return result.Value, nil
}

// ListDeploymentsSince returns all classic release deployments (any definition or environment)
// started in a project since the given time, paginating through results via the continuation
// token header.
func (c *Client) ListDeploymentsSince(project string, since time.Time) ([]Deployment, error) {
	path := fmt.Sprintf("%s/%s/_apis/release/deployments", c.releaseBaseURL, url.PathEscape(project))

	var all []Deployment
	continuationToken := ""
	for {
		var page struct {
			Value []Deployment `json:"value"`
		}
		query := url.Values{
			"api-version":    {apiVersion},
			"minStartedTime": {since.UTC().Format(time.RFC3339)},
		}
		if continuationToken != "" {
			query.Set("continuationToken", continuationToken)
		}
		header, err := c.getWithHeader(path, query, &page)
		if err != nil {
			return nil, fmt.Errorf("list deployments: %w", err)
		}
		all = append(all, page.Value...)
		continuationToken = header.Get("x-ms-continuationtoken")
		if continuationToken == "" {
			break
		}
	}
	return all, nil
}

// Team is a project team's ID and name.
type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListTeams returns all teams in a project.
func (c *Client) ListTeams(project string) ([]Team, error) {
	path := fmt.Sprintf("%s/_apis/projects/%s/teams", c.baseURL, url.PathEscape(project))
	var result struct {
		Value []Team `json:"value"`
	}
	if err := c.get(path, url.Values{"api-version": {apiVersion}}, &result); err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	return result.Value, nil
}

// Iteration is a team's sprint/iteration.
type Iteration struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Attributes struct {
		StartDate time.Time `json:"startDate"`
	} `json:"attributes"`
}

// ListTeamIterations returns a team's iterations for the given timeframe ("past", "current",
// "future"), or every iteration if timeframe is empty.
func (c *Client) ListTeamIterations(project, team, timeframe string) ([]Iteration, error) {
	path := fmt.Sprintf("%s/%s/%s/_apis/work/teamsettings/iterations", c.baseURL, url.PathEscape(project), url.PathEscape(team))
	query := url.Values{"api-version": {apiVersion}}
	if timeframe != "" {
		query.Set("$timeframe", timeframe)
	}
	var result struct {
		Value []Iteration `json:"value"`
	}
	if err := c.get(path, query, &result); err != nil {
		return nil, fmt.Errorf("list team iterations: %w", err)
	}
	return result.Value, nil
}

// GetCurrentIteration returns the team's current-timeframe iteration (its active sprint), or
// nil if the team has no iteration classified as current — which is the normal case for teams
// that don't use sprints, not an error.
func (c *Client) GetCurrentIteration(project, team string) (*Iteration, error) {
	iterations, err := c.ListTeamIterations(project, team, "current")
	if err != nil {
		return nil, err
	}
	if len(iterations) == 0 {
		return nil, nil
	}
	return &iterations[0], nil
}

// Capacity is a team's per-member capacity for one iteration.
type Capacity struct {
	TeamMembers []struct {
		Activities []struct {
			CapacityPerDay float64 `json:"capacityPerDay"`
		} `json:"activities"`
	} `json:"teamMembers"`
}

// GetTeamIterationCapacity returns a team's configured capacity for one of its iterations.
func (c *Client) GetTeamIterationCapacity(project, team, iterationID string) (*Capacity, error) {
	path := fmt.Sprintf("%s/%s/%s/_apis/work/teamsettings/iterations/%s/capacities", c.baseURL, url.PathEscape(project), url.PathEscape(team), url.PathEscape(iterationID))
	var capacity Capacity
	if err := c.get(path, url.Values{"api-version": {apiVersion}}, &capacity); err != nil {
		return nil, fmt.Errorf("get team iteration capacity: %w", err)
	}
	return &capacity, nil
}

// ListIterationWorkItemIDs returns the IDs of work items assigned to a team's iteration (its
// sprint backlog), via the dedicated iteration work items endpoint — this reflects the team's
// actual sprint assignment, not a best-effort match against System.IterationPath.
func (c *Client) ListIterationWorkItemIDs(project, team, iterationID string) ([]int, error) {
	path := fmt.Sprintf("%s/%s/%s/_apis/work/teamsettings/iterations/%s/workitems", c.baseURL, url.PathEscape(project), url.PathEscape(team), url.PathEscape(iterationID))
	var result struct {
		WorkItemRelations []struct {
			Target struct {
				ID int `json:"id"`
			} `json:"target"`
		} `json:"workItemRelations"`
	}
	if err := c.get(path, url.Values{"api-version": {apiVersion}}, &result); err != nil {
		return nil, fmt.Errorf("list iteration work items: %w", err)
	}
	ids := make([]int, 0, len(result.WorkItemRelations))
	for _, r := range result.WorkItemRelations {
		if r.Target.ID != 0 {
			ids = append(ids, r.Target.ID)
		}
	}
	return ids, nil
}

// WorkItem is a work item's ID and the subset of fields the boards collector needs.
type WorkItem struct {
	ID     int `json:"id"`
	Fields struct {
		WorkItemType  string    `json:"System.WorkItemType"`
		State         string    `json:"System.State"`
		AreaPath      string    `json:"System.AreaPath"`
		IterationPath string    `json:"System.IterationPath"`
		CreatedDate   time.Time `json:"System.CreatedDate"`
		ChangedDate   time.Time `json:"System.ChangedDate"`
		ClosedDate    time.Time `json:"Microsoft.VSTS.Common.ClosedDate"`
		// Priority is 0 when unset (valid Azure DevOps priorities start at 1).
		Priority int `json:"Microsoft.VSTS.Common.Priority"`
		// Severity is only populated on Bug work items in the built-in process templates.
		Severity string `json:"Microsoft.VSTS.Common.Severity"`
		// StoryPoints/Effort are pointers so an absent field (not applicable to this work
		// item type/process template) is distinguishable from an explicit 0.
		StoryPoints *float64 `json:"Microsoft.VSTS.Scheduling.StoryPoints"`
		Effort      *float64 `json:"Microsoft.VSTS.Scheduling.Effort"`
		AssignedTo  *struct {
			DisplayName string `json:"displayName"`
		} `json:"System.AssignedTo"`
	} `json:"fields"`
}

// TerminalStateNames are the terminal state names used by Azure DevOps' built-in process
// templates (Agile, Scrum, CMMI, Basic). Custom process templates with different terminal
// state names won't be recognized as closed — see the README for this trade-off.
var TerminalStateNames = []string{"Closed", "Done", "Resolved", "Completed"}

// closedStatesClause matches TerminalStateNames in WIQL.
var closedStatesClause = func() string {
	quoted := make([]string, len(TerminalStateNames))
	for i, s := range TerminalStateNames {
		quoted[i] = "'" + s + "'"
	}
	return fmt.Sprintf("[System.State] IN (%s)", strings.Join(quoted, ","))
}()

// sinceYesterday is a WIQL date macro clause: field values are compared server-side against
// midnight yesterday (project time zone), which avoids passing a literal date string — WIQL's
// date literal parsing is locale-sensitive and rejects standard Go/ISO-8601 formats with a
// generic HTTP 400.
const sinceYesterday = "@Today - 1"

// QueryWorkItemIDs returns the IDs of all non-removed work items in a project.
func (c *Client) QueryWorkItemIDs(project string) ([]int, error) {
	wiql := "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.State] <> 'Removed'"
	ids, err := c.queryWorkItemIDs(project, wiql)
	if err != nil {
		return nil, fmt.Errorf("query work items: %w", err)
	}
	return ids, nil
}

// CountWorkItemsCreatedSince returns the number of work items created in a project on or after
// midnight yesterday.
func (c *Client) CountWorkItemsCreatedSince(project string) (int, error) {
	wiql := fmt.Sprintf(
		"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND [System.CreatedDate] >= %s",
		sinceYesterday,
	)
	ids, err := c.queryWorkItemIDs(project, wiql)
	if err != nil {
		return 0, fmt.Errorf("count work items created: %w", err)
	}
	return len(ids), nil
}

// CountWorkItemsClosedSince returns the number of work items currently in a closed-like state
// (see closedStatesClause) that were last changed in a project on or after midnight yesterday.
//
// This is an approximation, not an exact "closed in this window" count: it can also match
// items that were reopened and closed again within the window, and it depends on the
// project's process template using one of the standard terminal state names.
func (c *Client) CountWorkItemsClosedSince(project string) (int, error) {
	wiql := fmt.Sprintf(
		"SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project AND %s AND [System.ChangedDate] >= %s",
		closedStatesClause, sinceYesterday,
	)
	ids, err := c.queryWorkItemIDs(project, wiql)
	if err != nil {
		return 0, fmt.Errorf("count work items closed: %w", err)
	}
	return len(ids), nil
}

func (c *Client) queryWorkItemIDs(project, wiql string) ([]int, error) {
	path := fmt.Sprintf("%s/%s/_apis/wit/wiql", c.baseURL, url.PathEscape(project))
	body := map[string]string{"query": wiql}
	var result struct {
		WorkItems []struct {
			ID int `json:"id"`
		} `json:"workItems"`
	}
	if err := c.post(path, url.Values{"api-version": {apiVersion}}, body, &result); err != nil {
		return nil, err
	}

	ids := make([]int, len(result.WorkItems))
	for i, w := range result.WorkItems {
		ids[i] = w.ID
	}
	return ids, nil
}

// GetWorkItems fetches type, state and assignee for a set of work item IDs,
// batching requests in groups of workItemBatchSize.
func (c *Client) GetWorkItems(project string, ids []int) ([]WorkItem, error) {
	path := fmt.Sprintf("%s/%s/_apis/wit/workitemsbatch", c.baseURL, url.PathEscape(project))

	var all []WorkItem
	for start := 0; start < len(ids); start += workItemBatchSize {
		end := start + workItemBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		body := map[string]any{
			"ids": ids[start:end],
			"fields": []string{
				"System.WorkItemType", "System.State", "System.AreaPath", "System.IterationPath",
				"System.CreatedDate", "System.ChangedDate", "Microsoft.VSTS.Common.ClosedDate",
				"Microsoft.VSTS.Common.Priority", "Microsoft.VSTS.Common.Severity",
				"Microsoft.VSTS.Scheduling.StoryPoints", "Microsoft.VSTS.Scheduling.Effort",
				"System.AssignedTo",
			},
		}
		var page struct {
			Value []WorkItem `json:"value"`
		}
		if err := c.post(path, url.Values{"api-version": {apiVersion}}, body, &page); err != nil {
			return nil, fmt.Errorf("get work items batch: %w", err)
		}
		all = append(all, page.Value...)
	}
	return all, nil
}

func (c *Client) get(path string, query url.Values, out interface{}) error {
	_, err := c.getWithHeader(path, query, out)
	return err
}

func (c *Client) post(path string, query url.Values, body, out interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, path+"?"+query.Encode(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.SetBasicAuth("", c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("rate limited (HTTP 429)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) getWithHeader(path string, query url.Values, out interface{}) (http.Header, error) {
	req, err := http.NewRequest(http.MethodGet, path+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited (HTTP 429)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return resp.Header, nil
}
