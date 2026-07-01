package collectors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"azure-devops-exporter/internal/azuredevops"
	"azure-devops-exporter/internal/metrics"
)

// pipelinesFakeServer serves 2 pipeline definitions ("ci" idle for 20 days, "cd" active) and,
// within the last 24h, 3 completed runs of "cd" (2 succeeded on main, 1 failed on feature-x,
// with queue times of 2/1/3 minutes) plus 1 run of "cd" still in progress.
func pipelinesFakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	now := time.Now()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_apis/build/definitions"):
			json.NewEncoder(w).Encode(map[string]any{
				"value": []azuredevops.BuildDefinition{{ID: 1, Name: "ci"}, {ID: 2, Name: "cd"}},
			})
		case strings.HasSuffix(r.URL.Path, "/_apis/build/builds"):
			if r.URL.Query().Get("$top") == "1" {
				// GetLatestBuild lookup, keyed by definition ID.
				defID := r.URL.Query().Get("definitions")
				var build azuredevops.Build
				switch defID {
				case "1":
					build = azuredevops.Build{ID: 100, Status: "completed", Result: "succeeded", FinishTime: now.Add(-20 * 24 * time.Hour)}
				case "2":
					build = azuredevops.Build{ID: 101, Status: "completed", Result: "succeeded", FinishTime: now.Add(-1 * time.Hour)}
				}
				json.NewEncoder(w).Encode(map[string]any{"value": []azuredevops.Build{build}})
				return
			}
			// ListBuildsSince: only "cd" ran in the last 24h.
			builds := []azuredevops.Build{
				{
					ID: 201, Status: "completed", Result: "succeeded",
					QueueTime: now.Add(-2*time.Hour - 2*time.Minute), StartTime: now.Add(-2 * time.Hour), FinishTime: now.Add(-2*time.Hour + 10*time.Minute),
					SourceBranch: "refs/heads/main",
				},
				{
					ID: 202, Status: "completed", Result: "succeeded",
					QueueTime: now.Add(-3*time.Hour - 1*time.Minute), StartTime: now.Add(-3 * time.Hour), FinishTime: now.Add(-3*time.Hour + 20*time.Minute),
					SourceBranch: "refs/heads/main",
				},
				{
					ID: 203, Status: "completed", Result: "failed",
					QueueTime: now.Add(-4*time.Hour - 3*time.Minute), StartTime: now.Add(-4 * time.Hour), FinishTime: now.Add(-4*time.Hour + 5*time.Minute),
					SourceBranch: "refs/heads/feature-x",
				},
				{ID: 204, Status: "inProgress"},
			}
			for i := range builds {
				builds[i].Definition.ID = 2
				builds[i].Definition.Name = "cd"
			}
			json.NewEncoder(w).Encode(map[string]any{"value": builds})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCollectPipelines(t *testing.T) {
	server := pipelinesFakeServer(t)
	defer server.Close()

	client := azuredevops.NewClient(server.URL, "org", "token")
	if err := CollectPipelines(client, "org", "proj"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := gaugeValue(t, metrics.PipelinesTotal, "org", "proj"); got != 2 {
		t.Errorf("PipelinesTotal = %v, want 2", got)
	}
	if got := gaugeValue(t, metrics.PipelineRunsSucceeded, "org", "proj", "cd", "2"); got != 2 {
		t.Errorf("PipelineRunsSucceeded[cd] = %v, want 2", got)
	}
	if got := gaugeValue(t, metrics.PipelineRunsFailed, "org", "proj", "cd", "2"); got != 1 {
		t.Errorf("PipelineRunsFailed[cd] = %v, want 1", got)
	}
	// "ci" is a known pipeline that just didn't run in the 24h window, so it should report
	// an explicit 0 rather than being absent from the metric.
	if got := gaugeValue(t, metrics.PipelineRunsSucceeded, "org", "proj", "ci", "1"); got != 0 {
		t.Errorf("PipelineRunsSucceeded[ci] = %v, want 0", got)
	}
	// "ci" last ran 20 days ago, well outside the run-count window, but its last-run
	// timestamp must still be reported via the dedicated per-pipeline lookup.
	if got := gaugeValue(t, metrics.PipelineLastRunTimestamp, "org", "proj", "ci", "1"); got == 0 {
		t.Errorf("PipelineLastRunTimestamp[ci] = %v, want nonzero", got)
	}
	if got := gaugeValue(t, metrics.PipelineLastRunTimestamp, "org", "proj", "cd", "2"); got == 0 {
		t.Errorf("PipelineLastRunTimestamp[cd] = %v, want nonzero", got)
	}
	if got := gaugeValue(t, metrics.PipelineRunDurationSeconds, "org", "proj", "cd", "2"); got <= 0 {
		t.Errorf("PipelineRunDurationSeconds[cd] = %v, want > 0", got)
	}
	// Durations are 600s/1200s/300s, sorted [300, 600, 1200].
	if got := gaugeValue(t, metrics.PipelineRunDurationP50Seconds, "org", "proj", "cd", "2"); got != 600 {
		t.Errorf("PipelineRunDurationP50Seconds[cd] = %v, want 600", got)
	}
	if got := gaugeValue(t, metrics.PipelineRunDurationMaxSeconds, "org", "proj", "cd", "2"); got != 1200 {
		t.Errorf("PipelineRunDurationMaxSeconds[cd] = %v, want 1200", got)
	}
	// Queue times are 120s/60s/180s, avg 120s.
	if got := gaugeValue(t, metrics.PipelineQueueTimeSeconds, "org", "proj", "cd", "2"); got != 120 {
		t.Errorf("PipelineQueueTimeSeconds[cd] = %v, want 120", got)
	}
	// Build 204 is still inProgress.
	if got := gaugeValue(t, metrics.PipelineRunsInProgress, "org", "proj", "cd", "2"); got != 1 {
		t.Errorf("PipelineRunsInProgress[cd] = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.PipelineRunsByBranchTotal, "org", "proj", "cd", "2", "main", "succeeded"); got != 2 {
		t.Errorf("PipelineRunsByBranchTotal[cd,main,succeeded] = %v, want 2", got)
	}
	if got := gaugeValue(t, metrics.PipelineRunsByBranchTotal, "org", "proj", "cd", "2", "feature-x", "failed"); got != 1 {
		t.Errorf("PipelineRunsByBranchTotal[cd,feature-x,failed] = %v, want 1", got)
	}
}

func TestCollectPipelines_KeepsPreviousMetricsOnError(t *testing.T) {
	server := pipelinesFakeServer(t)
	client := azuredevops.NewClient(server.URL, "org", "token")
	if err := CollectPipelines(client, "org", "proj-keep"); err != nil {
		t.Fatalf("unexpected error on first collect: %v", err)
	}
	server.Close()

	if err := CollectPipelines(client, "org", "proj-keep"); err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if got := gaugeValue(t, metrics.PipelinesTotal, "org", "proj-keep"); got != 2 {
		t.Errorf("PipelinesTotal after failed scrape = %v, want unchanged 2", got)
	}
}
