package collectors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"azure-devops-exporter/internal/azuredevops"
	"azure-devops-exporter/internal/metrics"
)

// fakeServer serves one repository (repo-1/repo-a, 4096 bytes) with 2 active PRs (PR1 fresh
// draft with no reviewers, PR2 stale with a merge conflict and an unapproved reviewer), 1
// completed PR (lead time ~5 days), 1 abandoned PR, 3 branches and 7 recent commits (4 by
// Alice, 3 by Bob). Routes match by path suffix so the same fake server works regardless of
// organization/project names.
func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	now := time.Now()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_apis/git/repositories"):
			json.NewEncoder(w).Encode(map[string]any{
				"value": []azuredevops.Repository{{ID: "repo-1", Name: "repo-a", Size: 4096}},
			})
		case strings.HasSuffix(r.URL.Path, "/repo-1/pullrequests"):
			json.NewEncoder(w).Encode(map[string]any{
				"value": []azuredevops.PullRequest{
					{PullRequestID: 1, Status: "active", CreationDate: now.Add(-2 * 24 * time.Hour), IsDraft: true},
					{PullRequestID: 2, Status: "active", CreationDate: now.Add(-20 * 24 * time.Hour), MergeStatus: "conflicts", Reviewers: []azuredevops.Reviewer{{Vote: 0}}},
					{PullRequestID: 3, Status: "completed", CreationDate: now.Add(-10 * 24 * time.Hour), ClosedDate: now.Add(-5 * 24 * time.Hour)},
					{PullRequestID: 4, Status: "abandoned"},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/repo-1/refs"):
			json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]string{{"name": "refs/heads/a"}, {"name": "refs/heads/b"}, {"name": "refs/heads/c"}},
			})
		case strings.HasSuffix(r.URL.Path, "/repo-1/commits"):
			commits := make([]azuredevops.Commit, 7)
			for i := range commits {
				if i < 4 {
					commits[i].Author.Name = "Alice"
				} else {
					commits[i].Author.Name = "Bob"
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"value": commits})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func gaugeValue(t *testing.T, vec *prometheus.GaugeVec, labels ...string) float64 {
	t.Helper()
	var m dto.Metric
	if err := vec.WithLabelValues(labels...).Write(&m); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	return m.GetGauge().GetValue()
}

func TestCollectRepos(t *testing.T) {
	server := fakeServer(t)
	defer server.Close()

	client := azuredevops.NewClient(server.URL, "org", "token")
	if err := CollectRepos(client, "org", "proj"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := gaugeValue(t, metrics.ReposTotal, "org", "proj"); got != 1 {
		t.Errorf("ReposTotal = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.PullRequestsActive, "org", "proj", "repo-a"); got != 2 {
		t.Errorf("PullRequestsActive = %v, want 2", got)
	}
	if got := gaugeValue(t, metrics.PullRequestsCompleted, "org", "proj", "repo-a"); got != 1 {
		t.Errorf("PullRequestsCompleted = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.PullRequestsAbandoned, "org", "proj", "repo-a"); got != 1 {
		t.Errorf("PullRequestsAbandoned = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.BranchesTotal, "org", "proj", "repo-a"); got != 3 {
		t.Errorf("BranchesTotal = %v, want 3", got)
	}
	if got := gaugeValue(t, metrics.CommitsTotal, "org", "proj", "repo-a"); got != 7 {
		t.Errorf("CommitsTotal = %v, want 7", got)
	}
	if got := gaugeValue(t, metrics.PullRequestAgeDays, "org", "proj", "repo-a", "1"); got < 1.9 || got > 2.1 {
		t.Errorf("PullRequestAgeDays[1] = %v, want ~2", got)
	}
	if got := gaugeValue(t, metrics.PullRequestAgeDays, "org", "proj", "repo-a", "2"); got < 19.9 || got > 20.1 {
		t.Errorf("PullRequestAgeDays[2] = %v, want ~20", got)
	}
	// PR 1 is 2 days old (fresh) and PR 2 is 20 days old (stale, past the 14-day threshold).
	if got := gaugeValue(t, metrics.StalePullRequestsTotal, "org", "proj", "repo-a"); got != 1 {
		t.Errorf("StalePullRequestsTotal = %v, want 1", got)
	}
	// PR 3 is the only completed PR, with a lead time of ~5 days.
	if got := gaugeValue(t, metrics.PRLeadTimeAvgDays, "org", "proj", "repo-a"); got < 4.9 || got > 5.1 {
		t.Errorf("PRLeadTimeAvgDays = %v, want ~5", got)
	}
	if got := gaugeValue(t, metrics.PRLeadTimeMaxDays, "org", "proj", "repo-a"); got < 4.9 || got > 5.1 {
		t.Errorf("PRLeadTimeMaxDays = %v, want ~5", got)
	}
	if got := gaugeValue(t, metrics.RepoSizeBytes, "org", "proj", "repo-a"); got != 4096 {
		t.Errorf("RepoSizeBytes = %v, want 4096", got)
	}
	// PR1 is a draft with no reviewers; PR2 has a merge conflict and a reviewer who hasn't
	// approved yet.
	if got := gaugeValue(t, metrics.DraftPullRequestsTotal, "org", "proj", "repo-a"); got != 1 {
		t.Errorf("DraftPullRequestsTotal = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.PullRequestsWithConflictsTotal, "org", "proj", "repo-a"); got != 1 {
		t.Errorf("PullRequestsWithConflictsTotal = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.PullRequestsWithoutReviewerTotal, "org", "proj", "repo-a"); got != 1 {
		t.Errorf("PullRequestsWithoutReviewerTotal = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.PullRequestsPendingApprovalTotal, "org", "proj", "repo-a"); got != 1 {
		t.Errorf("PullRequestsPendingApprovalTotal = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.CommitsByAuthorTotal, "org", "proj", "repo-a", "Alice"); got != 4 {
		t.Errorf("CommitsByAuthorTotal[Alice] = %v, want 4", got)
	}
	if got := gaugeValue(t, metrics.CommitsByAuthorTotal, "org", "proj", "repo-a", "Bob"); got != 3 {
		t.Errorf("CommitsByAuthorTotal[Bob] = %v, want 3", got)
	}
}

func TestCollectRepos_KeepsPreviousMetricsOnError(t *testing.T) {
	server := fakeServer(t)
	client := azuredevops.NewClient(server.URL, "org", "token")
	if err := CollectRepos(client, "org", "proj-keep"); err != nil {
		t.Fatalf("unexpected error on first collect: %v", err)
	}
	server.Close()

	// Second collect fails (server is closed); previous values must remain.
	if err := CollectRepos(client, "org", "proj-keep"); err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if got := gaugeValue(t, metrics.ReposTotal, "org", "proj-keep"); got != 1 {
		t.Errorf("ReposTotal after failed scrape = %v, want unchanged 1", got)
	}
}
