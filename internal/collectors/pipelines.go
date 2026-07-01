package collectors

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"azure-devops-exporter/internal/azuredevops"
	"azure-devops-exporter/internal/metrics"
)

// pipelineWindow is how far back CollectPipelines looks for run counts and duration —
// pipelines run frequently, so a short window keeps the numbers meaningful as "recent activity".
const pipelineWindow = 24 * time.Hour

type branchResult struct{ branch, result string }

type pipelineStats struct {
	id          int
	name        string
	succeeded   int
	failed      int
	canceled    int
	inProgress  int
	durations   []float64 // seconds, completed runs only
	queueTimes  []float64 // seconds, completed runs only
	byBranch    map[branchResult]int
	lastRunUnix float64
	hasLastRun  bool
}

// CollectPipelines fetches pipeline definition and run data for a project and updates the
// corresponding metrics. On error, previously collected metrics for this project are left
// untouched.
func CollectPipelines(client *azuredevops.Client, organization, project string) error {
	defs, err := client.ListBuildDefinitions(project)
	if err != nil {
		return fmt.Errorf("list build definitions: %w", err)
	}
	metrics.PipelinesTotal.WithLabelValues(organization, project).Set(float64(len(defs)))

	builds, err := client.ListBuildsSince(project, time.Now().Add(-pipelineWindow))
	if err != nil {
		return fmt.Errorf("list builds: %w", err)
	}

	// Seed every known definition so run counts are explicit zeros for idle pipelines,
	// not absent series — same convention as the Repos collector's per-repository PR counts.
	statsByDef := make(map[int]*pipelineStats)
	for _, d := range defs {
		statFor(statsByDef, d.ID, d.Name)
	}

	for _, b := range builds {
		if b.Status == "inProgress" {
			statFor(statsByDef, b.Definition.ID, b.Definition.Name).inProgress++
			continue
		}
		if b.Status != "completed" {
			continue
		}

		s := statFor(statsByDef, b.Definition.ID, b.Definition.Name)
		switch b.Result {
		case "succeeded":
			s.succeeded++
		case "failed":
			s.failed++
		case "canceled":
			s.canceled++
		}
		if !b.StartTime.IsZero() && !b.FinishTime.IsZero() {
			s.durations = append(s.durations, b.FinishTime.Sub(b.StartTime).Seconds())
		}
		if !b.QueueTime.IsZero() && !b.StartTime.IsZero() {
			s.queueTimes = append(s.queueTimes, b.StartTime.Sub(b.QueueTime).Seconds())
		}
		if branch := strings.TrimPrefix(b.SourceBranch, "refs/heads/"); branch != "" && b.Result != "" {
			s.byBranch[branchResult{branch: branch, result: b.Result}]++
		}
	}

	// The true last-run timestamp needs a dedicated lookup: a pipeline idle for longer than
	// pipelineWindow would otherwise silently drop out of this metric.
	for _, d := range defs {
		latest, err := client.GetLatestBuild(project, d.ID)
		if err != nil {
			return fmt.Errorf("get latest build for pipeline %s: %w", d.Name, err)
		}
		if latest == nil || latest.Status != "completed" || latest.FinishTime.IsZero() {
			continue
		}
		s := statFor(statsByDef, d.ID, d.Name)
		s.lastRunUnix = float64(latest.FinishTime.Unix())
		s.hasLastRun = true
	}

	labelFilter := prometheus.Labels{"organization": organization, "project": project}
	metrics.PipelineRunsSucceeded.DeletePartialMatch(labelFilter)
	metrics.PipelineRunsFailed.DeletePartialMatch(labelFilter)
	metrics.PipelineRunsCanceled.DeletePartialMatch(labelFilter)
	metrics.PipelineRunDurationSeconds.DeletePartialMatch(labelFilter)
	metrics.PipelineLastRunTimestamp.DeletePartialMatch(labelFilter)
	metrics.PipelineRunsInProgress.DeletePartialMatch(labelFilter)
	metrics.PipelineQueueTimeSeconds.DeletePartialMatch(labelFilter)
	metrics.PipelineRunDurationP50Seconds.DeletePartialMatch(labelFilter)
	metrics.PipelineRunDurationP90Seconds.DeletePartialMatch(labelFilter)
	metrics.PipelineRunDurationMaxSeconds.DeletePartialMatch(labelFilter)
	metrics.PipelineRunsByBranchTotal.DeletePartialMatch(labelFilter)

	for _, s := range statsByDef {
		labels := []string{organization, project, s.name, strconv.Itoa(s.id)}
		metrics.PipelineRunsSucceeded.WithLabelValues(labels...).Set(float64(s.succeeded))
		metrics.PipelineRunsFailed.WithLabelValues(labels...).Set(float64(s.failed))
		metrics.PipelineRunsCanceled.WithLabelValues(labels...).Set(float64(s.canceled))
		metrics.PipelineRunsInProgress.WithLabelValues(labels...).Set(float64(s.inProgress))
		if len(s.durations) > 0 {
			sort.Float64s(s.durations)
			metrics.PipelineRunDurationSeconds.WithLabelValues(labels...).Set(average(s.durations))
			metrics.PipelineRunDurationP50Seconds.WithLabelValues(labels...).Set(percentile(s.durations, 0.5))
			metrics.PipelineRunDurationP90Seconds.WithLabelValues(labels...).Set(percentile(s.durations, 0.9))
			metrics.PipelineRunDurationMaxSeconds.WithLabelValues(labels...).Set(s.durations[len(s.durations)-1])
		}
		if len(s.queueTimes) > 0 {
			metrics.PipelineQueueTimeSeconds.WithLabelValues(labels...).Set(average(s.queueTimes))
		}
		if s.hasLastRun {
			metrics.PipelineLastRunTimestamp.WithLabelValues(labels...).Set(s.lastRunUnix)
		}
		for k, count := range s.byBranch {
			branchLabels := append(append([]string{}, labels...), k.branch, k.result)
			metrics.PipelineRunsByBranchTotal.WithLabelValues(branchLabels...).Set(float64(count))
		}
	}
	return nil
}

func statFor(byDef map[int]*pipelineStats, id int, name string) *pipelineStats {
	s, ok := byDef[id]
	if !ok {
		s = &pipelineStats{id: id, name: name, byBranch: make(map[branchResult]int)}
		byDef[id] = s
	}
	return s
}
