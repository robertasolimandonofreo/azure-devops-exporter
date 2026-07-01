package collectors

import (
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"azure-devops-exporter/internal/azuredevops"
	"azure-devops-exporter/internal/metrics"
)

// releaseWindow is how far back CollectReleases looks for deployment counts, duration and the
// last-deployment timestamp. Classic releases deploy far less often than pipelines run, so this
// window is wider than pipelineWindow — a 24h window would show near-zero activity for most teams.
const releaseWindow = 30 * 24 * time.Hour

type releaseKey struct{ definition, environment string }

type releaseStats struct {
	definition     string
	environment    string
	succeeded      int
	failed         int
	notDeployed    int
	durations      []float64 // seconds, completed deployments only
	lastDeployUnix float64
	hasLastDeploy  bool
	leadTimeDays   []float64 // commit -> deployment, successful deployments only
}

// releaseChangesCache caches, per release ID, the commit timestamps traced back through that
// release's Build artifact. This is what makes Lead Time for Changes affordable: resolving it
// requires two extra API calls per deployment (GetRelease, then GetBuildChanges), and a given
// release's artifacts never change once created, so paying that cost more than once per release
// — e.g. once per scrape, forever, for a deployment from weeks ago — would be pure waste. Entries
// are swept once they're older than the release window, since a deployment that old has already
// dropped out of ListDeploymentsSince and won't be looked up again.
var releaseChangesCache = struct {
	mu      sync.Mutex
	entries map[int]releaseChangesCacheEntry
}{entries: make(map[int]releaseChangesCacheEntry)}

type releaseChangesCacheEntry struct {
	timestamps []time.Time
	cachedAt   time.Time
}

// CollectReleases fetches classic release definition and deployment data for a project and
// updates the corresponding metrics. On error, previously collected metrics for this project
// are left untouched.
func CollectReleases(client *azuredevops.Client, organization, project string) error {
	evictStaleReleaseChanges()

	defs, err := client.ListReleaseDefinitions(project)
	if err != nil {
		return fmt.Errorf("list release definitions: %w", err)
	}
	metrics.ReleasesTotal.WithLabelValues(organization, project).Set(float64(len(defs)))

	deployments, err := client.ListDeploymentsSince(project, time.Now().Add(-releaseWindow))
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	statsByKey := make(map[releaseKey]*releaseStats)
	for _, d := range deployments {
		if d.DeploymentStatus != "succeeded" && d.DeploymentStatus != "failed" && d.DeploymentStatus != "notDeployed" {
			continue
		}
		k := releaseKey{d.ReleaseDefinition.Name, d.ReleaseEnvironment.Name}
		s, ok := statsByKey[k]
		if !ok {
			s = &releaseStats{definition: k.definition, environment: k.environment}
			statsByKey[k] = s
		}
		switch d.DeploymentStatus {
		case "succeeded":
			s.succeeded++
		case "failed":
			s.failed++
		case "notDeployed":
			s.notDeployed++
		}
		if !d.StartedOn.IsZero() && !d.CompletedOn.IsZero() {
			s.durations = append(s.durations, d.CompletedOn.Sub(d.StartedOn).Seconds())
		}
		if !d.CompletedOn.IsZero() && (!s.hasLastDeploy || d.CompletedOn.Unix() > int64(s.lastDeployUnix)) {
			s.lastDeployUnix = float64(d.CompletedOn.Unix())
			s.hasLastDeploy = true
		}

		if d.DeploymentStatus == "succeeded" && !d.CompletedOn.IsZero() && d.Release.ID != 0 {
			// Best-effort: a release with no Build artifact (container image, manual release)
			// or a transient API failure just means this one deployment doesn't contribute a
			// lead time sample. It must not abort the whole scrape over a metric that's already
			// layered on top of the core, always-available deployment counts above.
			if timestamps, err := resolveReleaseChangeTimestamps(client, project, d.Release.ID); err == nil {
				for _, t := range timestamps {
					s.leadTimeDays = append(s.leadTimeDays, d.CompletedOn.Sub(t).Hours()/24)
				}
			}
		}
	}

	labelFilter := prometheus.Labels{"organization": organization, "project": project}
	metrics.ReleaseDeploymentsSucceeded.DeletePartialMatch(labelFilter)
	metrics.ReleaseDeploymentsFailed.DeletePartialMatch(labelFilter)
	metrics.ReleaseDeploymentDurationSeconds.DeletePartialMatch(labelFilter)
	metrics.ReleaseLastDeploymentTimestamp.DeletePartialMatch(labelFilter)
	metrics.ReleaseDeploymentsNotDeployed.DeletePartialMatch(labelFilter)
	metrics.ReleaseLeadTimeForChangesAvgDays.DeletePartialMatch(labelFilter)
	metrics.ReleaseLeadTimeForChangesP50Days.DeletePartialMatch(labelFilter)
	metrics.ReleaseLeadTimeForChangesP90Days.DeletePartialMatch(labelFilter)
	metrics.ReleaseLeadTimeForChangesMaxDays.DeletePartialMatch(labelFilter)

	for _, s := range statsByKey {
		labels := []string{organization, project, s.definition, s.environment}
		metrics.ReleaseDeploymentsSucceeded.WithLabelValues(labels...).Set(float64(s.succeeded))
		metrics.ReleaseDeploymentsFailed.WithLabelValues(labels...).Set(float64(s.failed))
		metrics.ReleaseDeploymentsNotDeployed.WithLabelValues(labels...).Set(float64(s.notDeployed))
		if len(s.durations) > 0 {
			metrics.ReleaseDeploymentDurationSeconds.WithLabelValues(labels...).Set(average(s.durations))
		}
		if s.hasLastDeploy {
			metrics.ReleaseLastDeploymentTimestamp.WithLabelValues(labels...).Set(s.lastDeployUnix)
		}
		if len(s.leadTimeDays) > 0 {
			sort.Float64s(s.leadTimeDays)
			metrics.ReleaseLeadTimeForChangesAvgDays.WithLabelValues(labels...).Set(average(s.leadTimeDays))
			metrics.ReleaseLeadTimeForChangesP50Days.WithLabelValues(labels...).Set(percentile(s.leadTimeDays, 0.5))
			metrics.ReleaseLeadTimeForChangesP90Days.WithLabelValues(labels...).Set(percentile(s.leadTimeDays, 0.9))
			metrics.ReleaseLeadTimeForChangesMaxDays.WithLabelValues(labels...).Set(s.leadTimeDays[len(s.leadTimeDays)-1])
		}
	}
	return nil
}

// resolveReleaseChangeTimestamps returns the commit timestamps traced back through a release's
// Build artifact, via the release's cache entry if present.
func resolveReleaseChangeTimestamps(client *azuredevops.Client, project string, releaseID int) ([]time.Time, error) {
	releaseChangesCache.mu.Lock()
	if entry, ok := releaseChangesCache.entries[releaseID]; ok {
		releaseChangesCache.mu.Unlock()
		return entry.timestamps, nil
	}
	releaseChangesCache.mu.Unlock()

	release, err := client.GetRelease(project, releaseID)
	if err != nil {
		return nil, fmt.Errorf("get release %d: %w", releaseID, err)
	}

	buildID, ok := buildArtifactID(release)
	if !ok {
		cacheReleaseChanges(releaseID, nil)
		return nil, nil
	}

	changes, err := client.GetBuildChanges(project, buildID)
	if err != nil {
		return nil, fmt.Errorf("get changes for build %d: %w", buildID, err)
	}

	timestamps := make([]time.Time, 0, len(changes))
	for _, c := range changes {
		if !c.Timestamp.IsZero() {
			timestamps = append(timestamps, c.Timestamp)
		}
	}
	cacheReleaseChanges(releaseID, timestamps)
	return timestamps, nil
}

func cacheReleaseChanges(releaseID int, timestamps []time.Time) {
	releaseChangesCache.mu.Lock()
	releaseChangesCache.entries[releaseID] = releaseChangesCacheEntry{timestamps: timestamps, cachedAt: time.Now()}
	releaseChangesCache.mu.Unlock()
}

func evictStaleReleaseChanges() {
	cutoff := time.Now().Add(-releaseWindow)
	releaseChangesCache.mu.Lock()
	for id, entry := range releaseChangesCache.entries {
		if entry.cachedAt.Before(cutoff) {
			delete(releaseChangesCache.entries, id)
		}
	}
	releaseChangesCache.mu.Unlock()
}

// buildArtifactID returns the build ID of a release's Build-type artifact, if it has one.
// Releases sourced from container images or other non-Build artifact types don't trace to
// commits this way.
func buildArtifactID(release *azuredevops.Release) (int, bool) {
	for _, a := range release.Artifacts {
		if a.Type != "Build" {
			continue
		}
		id, err := strconv.Atoi(a.DefinitionReference.Version.ID)
		if err != nil {
			continue
		}
		return id, true
	}
	return 0, false
}
