package azuredevops

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListRepositories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"value": []Repository{{ID: "1", Name: "repo-a"}, {ID: "2", Name: "repo-b"}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	repos, err := c.ListRepositories("proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(repos))
	}
}

func TestListPullRequests_Paginates(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		skip := r.URL.Query().Get("$skip")
		var value []PullRequest
		if skip == "0" {
			value = make([]PullRequest, pageSize)
			for i := range value {
				value[i] = PullRequest{PullRequestID: i, Status: "active"}
			}
		} else {
			value = []PullRequest{{PullRequestID: 999, Status: "completed"}}
		}
		json.NewEncoder(w).Encode(map[string]any{"value": value})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	prs, err := c.ListPullRequests("proj", "repo-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != pageSize+1 {
		t.Fatalf("got %d pull requests, want %d", len(prs), pageSize+1)
	}
	if calls != 2 {
		t.Fatalf("got %d requests, want 2 pages", calls)
	}
}

func TestCountBranches_FollowsContinuationToken(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("continuationToken") == "" {
			w.Header().Set("x-ms-continuationtoken", "next-page")
			json.NewEncoder(w).Encode(map[string]any{"value": []map[string]string{{"name": "refs/heads/main"}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"value": []map[string]string{{"name": "refs/heads/dev"}}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	count, err := c.CountBranches("proj", "repo-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("got %d branches, want 2", count)
	}
	if calls != 2 {
		t.Fatalf("got %d requests, want 2 pages", calls)
	}
}

func TestListCommitsSince_Paginates(t *testing.T) {
	calls := 0
	var gotFromDate string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotFromDate = r.URL.Query().Get("searchCriteria.fromDate")
		skip := r.URL.Query().Get("$skip")
		var value []Commit
		if skip == "0" {
			value = make([]Commit, pageSize)
			for i := range value {
				value[i].Author.Name = "Alice"
			}
		} else {
			value = []Commit{{}}
			value[0].Author.Name = "Bob"
		}
		json.NewEncoder(w).Encode(map[string]any{"value": value})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	since := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	commits, err := c.ListCommitsSince("proj", "repo-1", since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != pageSize+1 {
		t.Fatalf("got %d commits, want %d", len(commits), pageSize+1)
	}
	if calls != 2 {
		t.Fatalf("got %d requests, want 2 pages", calls)
	}
	if !strings.Contains(gotFromDate, "2026-06-30") {
		t.Fatalf("fromDate %q missing expected date", gotFromDate)
	}
}

func TestQueryWorkItemIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("got method %s, want POST", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"workItems": []map[string]int{{"id": 1}, {"id": 2}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	ids, err := c.QueryWorkItemIDs("proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("got %v, want [1 2]", ids)
	}
}

func TestCountWorkItemsCreatedSince(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		gotQuery = body.Query
		json.NewEncoder(w).Encode(map[string]any{
			"workItems": []map[string]int{{"id": 1}, {"id": 2}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	count, err := c.CountWorkItemsCreatedSince("proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("got %d, want 2", count)
	}
	if !strings.Contains(gotQuery, "System.CreatedDate") || !strings.Contains(gotQuery, "@Today - 1") {
		t.Fatalf("query %q missing expected CreatedDate filter", gotQuery)
	}
}

func TestCountWorkItemsClosedSince(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		gotQuery = body.Query
		json.NewEncoder(w).Encode(map[string]any{
			"workItems": []map[string]int{{"id": 3}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	count, err := c.CountWorkItemsClosedSince("proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("got %d, want 1", count)
	}
	if !strings.Contains(gotQuery, "System.ChangedDate") || !strings.Contains(gotQuery, "'Closed'") || !strings.Contains(gotQuery, "@Today - 1") {
		t.Fatalf("query %q missing expected closed-state/ChangedDate filter", gotQuery)
	}
}

func TestGetWorkItems_Batches(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			IDs []int `json:"ids"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		value := make([]WorkItem, len(body.IDs))
		for i, id := range body.IDs {
			value[i] = WorkItem{ID: id}
			value[i].Fields.WorkItemType = "Task"
			value[i].Fields.State = "Active"
		}
		json.NewEncoder(w).Encode(map[string]any{"value": value})
	}))
	defer server.Close()

	ids := make([]int, workItemBatchSize+1)
	for i := range ids {
		ids[i] = i
	}

	c := NewClient(server.URL, "org", "token")
	items, err := c.GetWorkItems("proj", ids)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != len(ids) {
		t.Fatalf("got %d work items, want %d", len(items), len(ids))
	}
	if calls != 2 {
		t.Fatalf("got %d requests, want 2 batches", calls)
	}
}

func TestNewClient_ReleaseBaseURLUsesVSRMHost(t *testing.T) {
	c := NewClient("https://dev.azure.com", "my-org", "token")
	want := "https://vsrm.dev.azure.com/my-org"
	if c.releaseBaseURL != want {
		t.Fatalf("releaseBaseURL = %q, want %q", c.releaseBaseURL, want)
	}
}

func TestNewClient_ReleaseBaseURLFallsBackOnPrem(t *testing.T) {
	c := NewClient("https://tfs.internal/collection", "my-org", "token")
	want := "https://tfs.internal/collection/my-org"
	if c.releaseBaseURL != want {
		t.Fatalf("releaseBaseURL = %q, want %q", c.releaseBaseURL, want)
	}
}

func TestListBuildDefinitions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"value": []BuildDefinition{{ID: 1, Name: "ci"}, {ID: 2, Name: "cd"}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	defs, err := c.ListBuildDefinitions("proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("got %d definitions, want 2", len(defs))
	}
}

func TestListBuildsSince_FollowsContinuationToken(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("continuationToken") == "" {
			w.Header().Set("x-ms-continuationtoken", "next-page")
			json.NewEncoder(w).Encode(map[string]any{"value": []Build{{ID: 1, Status: "completed", Result: "succeeded"}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"value": []Build{{ID: 2, Status: "completed", Result: "failed"}}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	builds, err := c.ListBuildsSince("proj", time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(builds) != 2 {
		t.Fatalf("got %d builds, want 2", len(builds))
	}
	if calls != 2 {
		t.Fatalf("got %d requests, want 2 pages", calls)
	}
}

func TestGetLatestBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("$top"); got != "1" {
			t.Errorf("$top = %q, want 1", got)
		}
		json.NewEncoder(w).Encode(map[string]any{"value": []Build{{ID: 42, Status: "completed", Result: "succeeded"}}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	build, err := c.GetLatestBuild("proj", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if build == nil || build.ID != 42 {
		t.Fatalf("got %+v, want build with ID 42", build)
	}
}

func TestGetLatestBuild_NoRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"value": []Build{}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	build, err := c.GetLatestBuild("proj", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if build != nil {
		t.Fatalf("got %+v, want nil", build)
	}
}

func TestListReleaseDefinitions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"value": []ReleaseDefinition{{ID: 1, Name: "release-web"}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	defs, err := c.ListReleaseDefinitions("proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 1 {
		t.Fatalf("got %d definitions, want 1", len(defs))
	}
}

func TestListDeploymentsSince_FollowsContinuationToken(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("continuationToken") == "" {
			w.Header().Set("x-ms-continuationtoken", "next-page")
			json.NewEncoder(w).Encode(map[string]any{"value": []Deployment{{DeploymentStatus: "succeeded"}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"value": []Deployment{{DeploymentStatus: "failed"}}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	deployments, err := c.ListDeploymentsSince("proj", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deployments) != 2 {
		t.Fatalf("got %d deployments, want 2", len(deployments))
	}
	if calls != 2 {
		t.Fatalf("got %d requests, want 2 pages", calls)
	}
}

func TestGetRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/release/releases/42") {
			t.Errorf("path = %q, want suffix /release/releases/42", r.URL.Path)
		}
		var release Release
		release.Artifacts = []struct {
			Type                string `json:"type"`
			DefinitionReference struct {
				Version struct {
					ID string `json:"id"`
				} `json:"version"`
			} `json:"definitionReference"`
		}{{Type: "Build"}}
		release.Artifacts[0].DefinitionReference.Version.ID = "123"
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	release, err := c.GetRelease("proj", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(release.Artifacts) != 1 || release.Artifacts[0].DefinitionReference.Version.ID != "123" {
		t.Fatalf("got %+v, want one Build artifact with version 123", release.Artifacts)
	}
}

func TestGetBuildChanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/build/builds/123/changes") {
			t.Errorf("path = %q, want suffix /build/builds/123/changes", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"value": []Change{{Timestamp: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	changes, err := c.GetBuildChanges("proj", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("got %d changes, want 1", len(changes))
	}
}

func TestListTeams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/_apis/projects/proj/teams") {
			t.Errorf("path = %q, want suffix /_apis/projects/proj/teams", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"value": []Team{{ID: "1", Name: "Team A"}, {ID: "2", Name: "Team B"}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	teams, err := c.ListTeams("proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("got %d teams, want 2", len(teams))
	}
}

func TestGetCurrentIteration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/proj/Team A/_apis/work/teamsettings/iterations") {
			t.Errorf("path = %q, want suffix /proj/Team A/_apis/work/teamsettings/iterations", r.URL.Path)
		}
		if got := r.URL.Query().Get("$timeframe"); got != "current" {
			t.Errorf("$timeframe = %q, want current", got)
		}
		json.NewEncoder(w).Encode(map[string]any{"value": []Iteration{{ID: "iter-1"}}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	iteration, err := c.GetCurrentIteration("proj", "Team A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iteration == nil || iteration.ID != "iter-1" {
		t.Fatalf("got %+v, want iteration iter-1", iteration)
	}
}

func TestGetCurrentIteration_NoneConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"value": []Iteration{}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	iteration, err := c.GetCurrentIteration("proj", "Team A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iteration != nil {
		t.Fatalf("got %+v, want nil", iteration)
	}
}

func TestListTeamIterations_Past(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("$timeframe"); got != "past" {
			t.Errorf("$timeframe = %q, want past", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"value": []Iteration{{ID: "iter-p1", Name: "Sprint P1"}, {ID: "iter-p2", Name: "Sprint P2"}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	iterations, err := c.ListTeamIterations("proj", "Team A", "past")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(iterations) != 2 {
		t.Fatalf("got %d iterations, want 2", len(iterations))
	}
}

func TestGetTeamIterationCapacity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/teamsettings/iterations/iter-1/capacities") {
			t.Errorf("path = %q, want suffix /teamsettings/iterations/iter-1/capacities", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"teamMembers": []map[string]any{
				{"activities": []map[string]any{{"capacityPerDay": 4.0}}},
				{"activities": []map[string]any{{"capacityPerDay": 3.0}, {"capacityPerDay": 1.0}}},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	capacity, err := c.GetTeamIterationCapacity("proj", "Team A", "iter-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var total float64
	for _, m := range capacity.TeamMembers {
		for _, a := range m.Activities {
			total += a.CapacityPerDay
		}
	}
	if total != 8 {
		t.Fatalf("got total capacityPerDay %v, want 8", total)
	}
}

func TestListIterationWorkItemIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/teamsettings/iterations/iter-1/workitems") {
			t.Errorf("path = %q, want suffix /teamsettings/iterations/iter-1/workitems", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"workItemRelations": []map[string]any{
				{"target": map[string]any{"id": 1}},
				{"target": map[string]any{"id": 2}},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	ids, err := c.ListIterationWorkItemIDs("proj", "Team A", "iter-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("got %v, want [1 2]", ids)
	}
}

func TestGet_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	c := NewClient(server.URL, "org", "token")
	if _, err := c.ListRepositories("proj"); err == nil {
		t.Fatal("expected error on HTTP 429")
	}
}
