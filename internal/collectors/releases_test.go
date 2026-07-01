package collectors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"azure-devops-exporter/internal/azuredevops"
	"azure-devops-exporter/internal/metrics"
)

// releaseFakeServerCalls counts requests to the release-detail and build-changes endpoints,
// so tests can verify the lead-time-for-changes cache actually avoids repeat calls.
type releaseFakeServerCalls struct {
	releases int
	changes  int
}

// releasesFakeServer serves 1 release definition ("web") with, in the last 30 days, 2
// succeeded deployments (releases 501 and 502, tracing to builds 601 and 602, with commit
// lead times of 3d+5d and 1d respectively), 1 failed and 1 notDeployed deployment, all to the
// "Production" environment.
func releasesFakeServer(t *testing.T, calls *releaseFakeServerCalls) *httptest.Server {
	t.Helper()
	now := time.Now()
	dep1Completed := now.Add(-10*24*time.Hour + 30*time.Minute)
	dep2Completed := now.Add(-2*24*time.Hour + 15*time.Minute)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_apis/release/definitions"):
			json.NewEncoder(w).Encode(map[string]any{
				"value": []azuredevops.ReleaseDefinition{{ID: 1, Name: "web"}},
			})
		case strings.HasSuffix(r.URL.Path, "/_apis/release/deployments"):
			deployments := []azuredevops.Deployment{
				{DeploymentStatus: "succeeded", StartedOn: now.Add(-10 * 24 * time.Hour), CompletedOn: dep1Completed},
				{DeploymentStatus: "succeeded", StartedOn: now.Add(-2 * 24 * time.Hour), CompletedOn: dep2Completed},
				{DeploymentStatus: "failed", StartedOn: now.Add(-5 * 24 * time.Hour), CompletedOn: now.Add(-5*24*time.Hour + 5*time.Minute)},
				{DeploymentStatus: "notDeployed"},
			}
			deployments[0].Release.ID = 501
			deployments[1].Release.ID = 502
			for i := range deployments {
				deployments[i].ReleaseDefinition.ID = 1
				deployments[i].ReleaseDefinition.Name = "web"
				deployments[i].ReleaseEnvironment.ID = 10
				deployments[i].ReleaseEnvironment.Name = "Production"
			}
			json.NewEncoder(w).Encode(map[string]any{"value": deployments})
		case strings.HasSuffix(r.URL.Path, "/release/releases/501"):
			calls.releases++
			writeReleaseWithBuildArtifact(w, 601)
		case strings.HasSuffix(r.URL.Path, "/release/releases/502"):
			calls.releases++
			writeReleaseWithBuildArtifact(w, 602)
		case strings.HasSuffix(r.URL.Path, "/build/builds/601/changes"):
			calls.changes++
			json.NewEncoder(w).Encode(map[string]any{
				"value": []azuredevops.Change{{Timestamp: dep1Completed.Add(-3 * 24 * time.Hour)}, {Timestamp: dep1Completed.Add(-5 * 24 * time.Hour)}},
			})
		case strings.HasSuffix(r.URL.Path, "/build/builds/602/changes"):
			calls.changes++
			json.NewEncoder(w).Encode(map[string]any{
				"value": []azuredevops.Change{{Timestamp: dep2Completed.Add(-1 * 24 * time.Hour)}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func writeReleaseWithBuildArtifact(w http.ResponseWriter, buildID int) {
	var release azuredevops.Release
	release.Artifacts = []struct {
		Type                string `json:"type"`
		DefinitionReference struct {
			Version struct {
				ID string `json:"id"`
			} `json:"version"`
		} `json:"definitionReference"`
	}{{Type: "Build"}}
	release.Artifacts[0].DefinitionReference.Version.ID = strconv.Itoa(buildID)
	json.NewEncoder(w).Encode(release)
}

func TestCollectReleases(t *testing.T) {
	resetReleaseChangesCache()
	server := releasesFakeServer(t, &releaseFakeServerCalls{})
	defer server.Close()

	client := azuredevops.NewClient(server.URL, "org", "token")
	if err := CollectReleases(client, "org", "proj"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := gaugeValue(t, metrics.ReleasesTotal, "org", "proj"); got != 1 {
		t.Errorf("ReleasesTotal = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.ReleaseDeploymentsSucceeded, "org", "proj", "web", "Production"); got != 2 {
		t.Errorf("ReleaseDeploymentsSucceeded = %v, want 2", got)
	}
	if got := gaugeValue(t, metrics.ReleaseDeploymentsFailed, "org", "proj", "web", "Production"); got != 1 {
		t.Errorf("ReleaseDeploymentsFailed = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.ReleaseDeploymentsNotDeployed, "org", "proj", "web", "Production"); got != 1 {
		t.Errorf("ReleaseDeploymentsNotDeployed = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.ReleaseDeploymentDurationSeconds, "org", "proj", "web", "Production"); got <= 0 {
		t.Errorf("ReleaseDeploymentDurationSeconds = %v, want > 0", got)
	}
	// The most recent deployment (succeeded, 2 days ago) should win over the older ones.
	// Compared with a few seconds of tolerance since the fixture and assertion each call
	// time.Now() independently.
	wantTimestamp := float64(time.Now().Add(-2*24*time.Hour + 15*time.Minute).Unix())
	if got := gaugeValue(t, metrics.ReleaseLastDeploymentTimestamp, "org", "proj", "web", "Production"); got < wantTimestamp-5 || got > wantTimestamp+5 {
		t.Errorf("ReleaseLastDeploymentTimestamp = %v, want ~%v", got, wantTimestamp)
	}
	// Release 501 (build 601) contributes lead times of 3d and 5d; release 502 (build 602)
	// contributes 1d. Sorted: [1, 3, 5].
	if got := gaugeValue(t, metrics.ReleaseLeadTimeForChangesAvgDays, "org", "proj", "web", "Production"); got < 2.9 || got > 3.1 {
		t.Errorf("ReleaseLeadTimeForChangesAvgDays = %v, want ~3", got)
	}
	if got := gaugeValue(t, metrics.ReleaseLeadTimeForChangesMaxDays, "org", "proj", "web", "Production"); got < 4.9 || got > 5.1 {
		t.Errorf("ReleaseLeadTimeForChangesMaxDays = %v, want ~5", got)
	}
}

func TestCollectReleases_ChangesAreCachedAcrossScrapes(t *testing.T) {
	resetReleaseChangesCache()
	calls := &releaseFakeServerCalls{}
	server := releasesFakeServer(t, calls)
	defer server.Close()

	client := azuredevops.NewClient(server.URL, "org", "token")
	if err := CollectReleases(client, "org", "proj-cache"); err != nil {
		t.Fatalf("unexpected error on first collect: %v", err)
	}
	if calls.releases != 2 || calls.changes != 2 {
		t.Fatalf("after first collect: releases=%d changes=%d, want 2 and 2", calls.releases, calls.changes)
	}

	// A second scrape of the same (unchanged) deployments must not re-resolve releases 501/502
	// — that's the whole point of the cache described in releases.go.
	if err := CollectReleases(client, "org", "proj-cache"); err != nil {
		t.Fatalf("unexpected error on second collect: %v", err)
	}
	if calls.releases != 2 || calls.changes != 2 {
		t.Fatalf("after second collect: releases=%d changes=%d, want unchanged 2 and 2", calls.releases, calls.changes)
	}
}

func resetReleaseChangesCache() {
	releaseChangesCache.mu.Lock()
	releaseChangesCache.entries = make(map[int]releaseChangesCacheEntry)
	releaseChangesCache.mu.Unlock()
}

func TestCollectReleases_KeepsPreviousMetricsOnError(t *testing.T) {
	resetReleaseChangesCache()
	server := releasesFakeServer(t, &releaseFakeServerCalls{})
	client := azuredevops.NewClient(server.URL, "org", "token")
	if err := CollectReleases(client, "org", "proj-keep"); err != nil {
		t.Fatalf("unexpected error on first collect: %v", err)
	}
	server.Close()

	if err := CollectReleases(client, "org", "proj-keep"); err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if got := gaugeValue(t, metrics.ReleasesTotal, "org", "proj-keep"); got != 1 {
		t.Errorf("ReleasesTotal after failed scrape = %v, want unchanged 1", got)
	}
}
