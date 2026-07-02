package collectors

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"azure-devops-exporter/internal/azuredevops"
	"azure-devops-exporter/internal/metrics"
)

const unassigned = "unassigned"

// staleThreshold is how long a non-closed work item can go without a field change before it
// counts as stale. ponytail: fixed threshold, make configurable if a project needs a different one.
const staleThreshold = 14 * 24 * time.Hour

// CollectBoards fetches work item data for a project and updates the corresponding
// metrics. On error, previously collected metrics for this project are left untouched.
func CollectBoards(client *azuredevops.Client, organization, project string) error {
	ids, err := client.QueryWorkItemIDs(project)
	if err != nil {
		return fmt.Errorf("query work items: %w", err)
	}

	items, err := client.GetWorkItems(project, ids)
	if err != nil {
		return fmt.Errorf("get work items: %w", err)
	}

	created, err := client.CountWorkItemsCreatedSince(project)
	if err != nil {
		return fmt.Errorf("count created work items: %w", err)
	}
	closed, err := client.CountWorkItemsClosedSince(project)
	if err != nil {
		return fmt.Errorf("count closed work items: %w", err)
	}

	type stateKey struct{ workItemType, state, areaPath, iterationPath string }
	type leadTimeKey struct{ workItemType, areaPath, iterationPath string }
	type typeStateKey struct{ workItemType, state string }
	type priorityKey struct{ workItemType, priority string }
	byState := make(map[stateKey]int)
	byAssignee := make(map[string]int)
	staleByType := make(map[stateKey]int)
	leadTimesByKey := make(map[leadTimeKey][]float64)
	byPriority := make(map[priorityKey]int)
	bugsBySeverity := make(map[string]int)
	withoutEstimate := make(map[typeStateKey]int)
	withoutIteration := make(map[string]int)
	withoutAreaPath := make(map[string]int)
	storyPoints := make(map[typeStateKey]float64)
	effort := make(map[typeStateKey]float64)
	now := time.Now()
	for _, item := range items {
		f := item.Fields
		sKey := stateKey{f.WorkItemType, f.State, f.AreaPath, f.IterationPath}
		tsKey := typeStateKey{f.WorkItemType, f.State}

		byState[sKey]++
		byAssignee[assigneeOf(item)]++

		if !isTerminalState(f.State) && now.Sub(f.ChangedDate) > staleThreshold {
			staleByType[stateKey{workItemType: f.WorkItemType, state: f.State}]++
		}

		if isTerminalState(f.State) && !f.ClosedDate.IsZero() && !f.CreatedDate.IsZero() {
			k := leadTimeKey{f.WorkItemType, f.AreaPath, f.IterationPath}
			days := f.ClosedDate.Sub(f.CreatedDate).Hours() / 24
			leadTimesByKey[k] = append(leadTimesByKey[k], days)
		}

		if f.Priority != 0 {
			byPriority[priorityKey{f.WorkItemType, strconv.Itoa(f.Priority)}]++
		}
		if f.WorkItemType == "Bug" && f.Severity != "" {
			bugsBySeverity[f.Severity]++
		}
		if f.StoryPoints == nil && f.Effort == nil {
			withoutEstimate[tsKey]++
		}
		if f.IterationPath == "" {
			withoutIteration[f.WorkItemType]++
		}
		if f.AreaPath == "" {
			withoutAreaPath[f.WorkItemType]++
		}
		if f.StoryPoints != nil {
			storyPoints[tsKey] += *f.StoryPoints
		}
		if f.Effort != nil {
			effort[tsKey] += *f.Effort
		}
	}

	// Clear stale per-state, per-assignee, per-item, lead time and breakdown series before
	// writing fresh values, now that every work item was fetched successfully.
	labelFilter := prometheus.Labels{"organization": organization, "project": project}
	metrics.BoardsWorkItemsByState.DeletePartialMatch(labelFilter)
	metrics.BoardsWorkItemsByAssignee.DeletePartialMatch(labelFilter)
	metrics.BoardsWorkItemAgeDays.DeletePartialMatch(labelFilter)
	metrics.BoardsWorkItemsStaleTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsLeadTimeAvgDays.DeletePartialMatch(labelFilter)
	metrics.BoardsLeadTimeP50Days.DeletePartialMatch(labelFilter)
	metrics.BoardsLeadTimeP90Days.DeletePartialMatch(labelFilter)
	metrics.BoardsLeadTimeMaxDays.DeletePartialMatch(labelFilter)
	metrics.BoardsWorkItemsByPriority.DeletePartialMatch(labelFilter)
	metrics.BoardsBugsBySeverity.DeletePartialMatch(labelFilter)
	metrics.BoardsWorkItemsWithoutEstimateTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsWorkItemsWithoutIterationTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsWorkItemsWithoutAreaPathTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsStoryPointsTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsEffortTotal.DeletePartialMatch(labelFilter)

	metrics.BoardsWorkItemsTotal.WithLabelValues(organization, project).Set(float64(len(items)))
	metrics.BoardsWorkItemsCreatedTotal.WithLabelValues(organization, project).Set(float64(created))
	metrics.BoardsWorkItemsClosedTotal.WithLabelValues(organization, project).Set(float64(closed))
	for k, count := range byState {
		metrics.BoardsWorkItemsByState.WithLabelValues(organization, project, k.workItemType, k.state, k.areaPath, k.iterationPath).Set(float64(count))
	}
	for assignee, count := range byAssignee {
		metrics.BoardsWorkItemsByAssignee.WithLabelValues(organization, project, assignee).Set(float64(count))
	}
	for k, count := range staleByType {
		metrics.BoardsWorkItemsStaleTotal.WithLabelValues(organization, project, k.workItemType, k.state).Set(float64(count))
	}
	for _, item := range items {
		ageDays := now.Sub(item.Fields.CreatedDate).Hours() / 24
		metrics.BoardsWorkItemAgeDays.WithLabelValues(organization, project, item.Fields.WorkItemType, item.Fields.State, assigneeOf(item), strconv.Itoa(item.ID)).Set(ageDays)
	}
	for k, days := range leadTimesByKey {
		sort.Float64s(days)
		labels := []string{organization, project, k.workItemType, k.areaPath, k.iterationPath}
		metrics.BoardsLeadTimeAvgDays.WithLabelValues(labels...).Set(average(days))
		metrics.BoardsLeadTimeP50Days.WithLabelValues(labels...).Set(percentile(days, 0.5))
		metrics.BoardsLeadTimeP90Days.WithLabelValues(labels...).Set(percentile(days, 0.9))
		metrics.BoardsLeadTimeMaxDays.WithLabelValues(labels...).Set(days[len(days)-1])
	}
	for k, count := range byPriority {
		metrics.BoardsWorkItemsByPriority.WithLabelValues(organization, project, k.workItemType, k.priority).Set(float64(count))
	}
	for severity, count := range bugsBySeverity {
		metrics.BoardsBugsBySeverity.WithLabelValues(organization, project, severity).Set(float64(count))
	}
	for k, count := range withoutEstimate {
		metrics.BoardsWorkItemsWithoutEstimateTotal.WithLabelValues(organization, project, k.workItemType, k.state).Set(float64(count))
	}
	for workItemType, count := range withoutIteration {
		metrics.BoardsWorkItemsWithoutIterationTotal.WithLabelValues(organization, project, workItemType).Set(float64(count))
	}
	for workItemType, count := range withoutAreaPath {
		metrics.BoardsWorkItemsWithoutAreaPathTotal.WithLabelValues(organization, project, workItemType).Set(float64(count))
	}
	for k, sum := range storyPoints {
		metrics.BoardsStoryPointsTotal.WithLabelValues(organization, project, k.workItemType, k.state).Set(sum)
	}
	for k, sum := range effort {
		metrics.BoardsEffortTotal.WithLabelValues(organization, project, k.workItemType, k.state).Set(sum)
	}

	collectActiveSprints(client, organization, project, items)
	return nil
}

// collectActiveSprints updates the active-sprint metric for every team in the project. It's
// best-effort per team: a team with no current iteration (most teams that don't run sprints)
// or a transient API error just contributes no series, rather than failing the whole
// CollectBoards call over data that's already layered on top of the core work item counts above.
func collectActiveSprints(client *azuredevops.Client, organization, project string, items []azuredevops.WorkItem) {
	labelFilter := prometheus.Labels{"organization": organization, "project": project}
	metrics.BoardsActiveSprintWorkItemsTotal.DeletePartialMatch(labelFilter)

	teams, err := client.ListTeams(project)
	if err != nil {
		return
	}

	itemByID := make(map[int]azuredevops.WorkItem, len(items))
	for _, item := range items {
		itemByID[item.ID] = item
	}

	type teamStateKey struct{ team, workItemType, state string }
	byTeamState := make(map[teamStateKey]int)
	for _, team := range teams {
		iteration, err := client.GetCurrentIteration(project, team.Name)
		if err != nil || iteration == nil {
			continue
		}
		ids, err := client.ListIterationWorkItemIDs(project, team.Name, iteration.ID)
		if err != nil {
			continue
		}
		for _, id := range ids {
			item, ok := itemByID[id]
			if !ok {
				continue
			}
			byTeamState[teamStateKey{team.Name, item.Fields.WorkItemType, item.Fields.State}]++
		}
	}

	for k, count := range byTeamState {
		metrics.BoardsActiveSprintWorkItemsTotal.WithLabelValues(organization, project, k.team, k.workItemType, k.state).Set(float64(count))
	}
}

func average(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// percentile returns the p-th percentile (0-1) of sorted using the nearest-rank method.
func percentile(sorted []float64, p float64) float64 {
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func assigneeOf(item azuredevops.WorkItem) string {
	if item.Fields.AssignedTo != nil && item.Fields.AssignedTo.DisplayName != "" {
		return item.Fields.AssignedTo.DisplayName
	}
	return unassigned
}

func isTerminalState(state string) bool {
	for _, s := range azuredevops.TerminalStateNames {
		if s == state {
			return true
		}
	}
	return false
}
