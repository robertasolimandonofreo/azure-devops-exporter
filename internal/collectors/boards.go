package collectors

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"azure-devops-exporter/internal/azuredevops"
	"azure-devops-exporter/internal/metrics"
)

const unassigned = "unassigned"

// unsetCustomFieldValue is the "value" label used for azure_devops_boards_work_items_by_custom_field_total
// when a configured custom field is unset on a given work item.
const unsetCustomFieldValue = "unset"

// staleThreshold is how long a non-closed work item can go without a field change before it
// counts as stale. ponytail: fixed threshold, make configurable if a project needs a different one.
const staleThreshold = 14 * 24 * time.Hour

// CollectBoards fetches work item data for a project and updates the corresponding metrics.
// customFields is the project's configured set of extra fields to break work items down by
// (see azuredevops.CustomField and the README) — nil/empty if none are configured, in which
// case azure_devops_boards_work_items_by_custom_field_total is simply never populated. On
// error, previously collected metrics for this project are left untouched.
func CollectBoards(client *azuredevops.Client, organization, project string, customFields []azuredevops.CustomField) error {
	ids, err := client.QueryWorkItemIDs(project)
	if err != nil {
		return fmt.Errorf("query work items: %w", err)
	}

	items, err := client.GetWorkItems(project, ids, customFields)
	if err != nil && len(customFields) > 0 {
		// Some invalid custom field reference names (a typo, or a field that doesn't exist on
		// this project's process template) make Azure DevOps reject the *entire* workitemsbatch
		// request with an HTTP 400, not just omit that one field from the response. Retry once
		// without any custom fields so a bad AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS value degrades to
		// "work_items_by_custom_field_total has no data this scrape" instead of taking down every
		// other Boards metric for the project.
		slog.Warn("boards: workitemsbatch failed with custom fields, retrying without them",
			"project", project, "custom_fields", customFields, "error", err)
		customFields = nil
		items, err = client.GetWorkItems(project, ids, customFields)
	}
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
	type typeStateKey struct{ workItemType, state, areaPath, iterationPath string }
	type priorityKey struct{ workItemType, priority, areaPath, iterationPath string }
	type assigneeKey struct{ assignee, areaPath, iterationPath string }
	type severityKey struct{ severity, areaPath, iterationPath string }
	type customFieldKey struct{ workItemType, state, field, value, areaPath, iterationPath string }
	byState := make(map[stateKey]int)
	byAssignee := make(map[assigneeKey]int)
	staleByType := make(map[stateKey]int)
	leadTimesByKey := make(map[leadTimeKey][]float64)
	cycleTimesByKey := make(map[leadTimeKey][]float64)
	byPriority := make(map[priorityKey]int)
	bugsBySeverity := make(map[severityKey]int)
	withoutEstimate := make(map[typeStateKey]int)
	withoutIteration := make(map[string]int)
	withoutAreaPath := make(map[string]int)
	storyPoints := make(map[typeStateKey]float64)
	effort := make(map[typeStateKey]float64)
	byCustomField := make(map[customFieldKey]int)
	blockedByState := make(map[stateKey]int)
	now := time.Now()
	for _, item := range items {
		f := item.Fields
		sKey := stateKey{f.WorkItemType, f.State, f.AreaPath, f.IterationPath}
		tsKey := typeStateKey{f.WorkItemType, f.State, f.AreaPath, f.IterationPath}

		byState[sKey]++
		byAssignee[assigneeKey{assigneeOf(item), f.AreaPath, f.IterationPath}]++

		if !isTerminalState(f.State) && now.Sub(f.ChangedDate) > staleThreshold {
			staleByType[sKey]++
		}

		if hasTag(f.Tags, "blocked") {
			blockedByState[sKey]++
		}

		if isTerminalState(f.State) && !f.ClosedDate.IsZero() && !f.CreatedDate.IsZero() {
			k := leadTimeKey{f.WorkItemType, f.AreaPath, f.IterationPath}
			leadTimesByKey[k] = append(leadTimesByKey[k], f.ClosedDate.Sub(f.CreatedDate).Hours()/24)
			// Cycle time: time from first active state (ActivatedDate) to closed.
			// Only recorded when Azure DevOps set ActivatedDate (requires the item to have
			// passed through an InProgress-category state at least once).
			if !f.ActivatedDate.IsZero() {
				cycleTimesByKey[k] = append(cycleTimesByKey[k], f.ClosedDate.Sub(f.ActivatedDate).Hours()/24)
			}
		}

		if f.Priority != 0 {
			byPriority[priorityKey{f.WorkItemType, strconv.Itoa(f.Priority), f.AreaPath, f.IterationPath}]++
		}
		if f.WorkItemType == "Bug" && f.Severity != "" {
			bugsBySeverity[severityKey{f.Severity, f.AreaPath, f.IterationPath}]++
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
		for _, cf := range customFields {
			values := splitCustomFieldValues(item.CustomFields[cf.Label])
			if len(values) == 0 {
				byCustomField[customFieldKey{f.WorkItemType, f.State, cf.Label, unsetCustomFieldValue, f.AreaPath, f.IterationPath}]++
				continue
			}
			// A multi-select field contributes to every value it has, not just one — an item
			// tagged "cxm;nps" must count under both value="cxm" and value="nps", so a query
			// filtered to one value isn't blind to items that also carry other values.
			for _, v := range values {
				byCustomField[customFieldKey{f.WorkItemType, f.State, cf.Label, v, f.AreaPath, f.IterationPath}]++
			}
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
	metrics.BoardsWorkItemsByCustomFieldTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsWorkItemStoryPoints.DeletePartialMatch(labelFilter)
	metrics.BoardsWorkItemsBlockedTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsCycleTimeAvgDays.DeletePartialMatch(labelFilter)
	metrics.BoardsCycleTimeP50Days.DeletePartialMatch(labelFilter)
	metrics.BoardsCycleTimeP90Days.DeletePartialMatch(labelFilter)

	metrics.BoardsWorkItemsTotal.WithLabelValues(organization, project).Set(float64(len(items)))
	metrics.BoardsWorkItemsCreatedTotal.WithLabelValues(organization, project).Set(float64(created))
	metrics.BoardsWorkItemsClosedTotal.WithLabelValues(organization, project).Set(float64(closed))
	for k, count := range byState {
		metrics.BoardsWorkItemsByState.WithLabelValues(organization, project, k.workItemType, k.state, k.areaPath, k.iterationPath).Set(float64(count))
	}
	for k, count := range byAssignee {
		metrics.BoardsWorkItemsByAssignee.WithLabelValues(organization, project, k.assignee, k.areaPath, k.iterationPath).Set(float64(count))
	}
	for k, count := range staleByType {
		metrics.BoardsWorkItemsStaleTotal.WithLabelValues(organization, project, k.workItemType, k.state, k.areaPath, k.iterationPath).Set(float64(count))
	}
	for _, item := range items {
		ageDays := now.Sub(item.Fields.CreatedDate).Hours() / 24
		metrics.BoardsWorkItemAgeDays.WithLabelValues(organization, project, item.Fields.WorkItemType, item.Fields.State, assigneeOf(item), strconv.Itoa(item.ID), item.Fields.AreaPath, item.Fields.IterationPath).Set(ageDays)
		if item.Fields.StoryPoints != nil {
			metrics.BoardsWorkItemStoryPoints.WithLabelValues(organization, project, item.Fields.WorkItemType, item.Fields.State, strconv.Itoa(item.ID), item.Fields.AreaPath, item.Fields.IterationPath).Set(*item.Fields.StoryPoints)
		}
	}
	for k, days := range leadTimesByKey {
		sort.Float64s(days)
		labels := []string{organization, project, k.workItemType, k.areaPath, k.iterationPath}
		metrics.BoardsLeadTimeAvgDays.WithLabelValues(labels...).Set(average(days))
		metrics.BoardsLeadTimeP50Days.WithLabelValues(labels...).Set(percentile(days, 0.5))
		metrics.BoardsLeadTimeP90Days.WithLabelValues(labels...).Set(percentile(days, 0.9))
		metrics.BoardsLeadTimeMaxDays.WithLabelValues(labels...).Set(days[len(days)-1])
	}
	for k, days := range cycleTimesByKey {
		sort.Float64s(days)
		labels := []string{organization, project, k.workItemType, k.areaPath, k.iterationPath}
		metrics.BoardsCycleTimeAvgDays.WithLabelValues(labels...).Set(average(days))
		metrics.BoardsCycleTimeP50Days.WithLabelValues(labels...).Set(percentile(days, 0.5))
		metrics.BoardsCycleTimeP90Days.WithLabelValues(labels...).Set(percentile(days, 0.9))
	}
	for k, count := range byPriority {
		metrics.BoardsWorkItemsByPriority.WithLabelValues(organization, project, k.workItemType, k.priority, k.areaPath, k.iterationPath).Set(float64(count))
	}
	for k, count := range bugsBySeverity {
		metrics.BoardsBugsBySeverity.WithLabelValues(organization, project, k.severity, k.areaPath, k.iterationPath).Set(float64(count))
	}
	for k, count := range withoutEstimate {
		metrics.BoardsWorkItemsWithoutEstimateTotal.WithLabelValues(organization, project, k.workItemType, k.state, k.areaPath, k.iterationPath).Set(float64(count))
	}
	for workItemType, count := range withoutIteration {
		metrics.BoardsWorkItemsWithoutIterationTotal.WithLabelValues(organization, project, workItemType).Set(float64(count))
	}
	for workItemType, count := range withoutAreaPath {
		metrics.BoardsWorkItemsWithoutAreaPathTotal.WithLabelValues(organization, project, workItemType).Set(float64(count))
	}
	for k, sum := range storyPoints {
		metrics.BoardsStoryPointsTotal.WithLabelValues(organization, project, k.workItemType, k.state, k.areaPath, k.iterationPath).Set(sum)
	}
	for k, sum := range effort {
		metrics.BoardsEffortTotal.WithLabelValues(organization, project, k.workItemType, k.state, k.areaPath, k.iterationPath).Set(sum)
	}
	for k, count := range byCustomField {
		metrics.BoardsWorkItemsByCustomFieldTotal.WithLabelValues(organization, project, k.workItemType, k.state, k.field, k.value, k.areaPath, k.iterationPath).Set(float64(count))
	}
	for k, count := range blockedByState {
		metrics.BoardsWorkItemsBlockedTotal.WithLabelValues(organization, project, k.workItemType, k.state, k.areaPath, k.iterationPath).Set(float64(count))
	}

	collectTeamMetrics(client, organization, project, items)
	return nil
}

// velocitySprintCount is how many of a team's most recent past sprints collectTeamMetrics
// computes velocity for.
const velocitySprintCount = 5

type teamStateKey struct{ team, workItemType, state string }

// collectTeamMetrics updates the per-team metrics (active sprint composition, active sprint
// story points, team capacity and sprint velocity) for every team in the project. It's
// best-effort per team: a team with no current or past iterations (most teams that don't run
// sprints), or a transient API error on any of the calls below, just contributes no series for
// that piece of data, rather than failing the whole CollectBoards call over data that's already
// layered on top of the core work item counts above.
func collectTeamMetrics(client *azuredevops.Client, organization, project string, items []azuredevops.WorkItem) {
	labelFilter := prometheus.Labels{"organization": organization, "project": project}
	metrics.BoardsActiveSprintWorkItemsTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsActiveSprintStoryPointsTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsTeamCapacityHoursPerDay.DeletePartialMatch(labelFilter)
	metrics.BoardsSprintVelocityStoryPoints.DeletePartialMatch(labelFilter)
	metrics.BoardsSprintDeliveryTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsSprintThroughputTotal.DeletePartialMatch(labelFilter)
	metrics.BoardsSprintScopeAddedTotal.DeletePartialMatch(labelFilter)

	teams, err := client.ListTeams(project)
	if err != nil {
		return
	}

	itemByID := make(map[int]azuredevops.WorkItem, len(items))
	for _, item := range items {
		itemByID[item.ID] = item
	}

	byTeamState := make(map[teamStateKey]int)
	for _, team := range teams {
		collectActiveSprint(client, organization, project, team, itemByID, byTeamState)
		collectSprintHistory(client, organization, project, team, itemByID)
	}

	for k, count := range byTeamState {
		metrics.BoardsActiveSprintWorkItemsTotal.WithLabelValues(organization, project, k.team, k.workItemType, k.state).Set(float64(count))
	}
}

// collectActiveSprint fills byTeamState with the team's current sprint composition and sets
// its active-sprint story points and capacity metrics directly (both are per-team scalars, so
// unlike work item counts they don't need a shared aggregation map).
func collectActiveSprint(client *azuredevops.Client, organization, project string, team azuredevops.Team, itemByID map[int]azuredevops.WorkItem, byTeamState map[teamStateKey]int) {
	iteration, err := client.GetCurrentIteration(project, team.Name)
	if err != nil || iteration == nil {
		return
	}
	ids, err := client.ListIterationWorkItemIDs(project, team.Name, iteration.ID)
	if err != nil {
		return
	}

	var points float64
	for _, id := range ids {
		item, ok := itemByID[id]
		if !ok {
			continue
		}
		byTeamState[teamStateKey{team.Name, item.Fields.WorkItemType, item.Fields.State}]++
		points += pointsOf(item)
	}
	metrics.BoardsActiveSprintStoryPointsTotal.WithLabelValues(organization, project, team.Name).Set(points)

	if capacity, err := client.GetTeamIterationCapacity(project, team.Name, iteration.ID); err == nil {
		metrics.BoardsTeamCapacityHoursPerDay.WithLabelValues(organization, project, team.Name).Set(totalCapacityPerDay(capacity))
	}
}

// collectSprintHistory sets the sprint velocity and delivery metrics for a team's last
// velocitySprintCount sprints (past + current, fewer if the team doesn't have that many), one
// series per iteration, from a single fetch of each iteration's work items — delivery status is a
// by-product of the same data velocity already needs, so this costs no extra API calls beyond
// what velocity alone would.
//
// The current (active) sprint is included so teams can track in-progress velocity without waiting
// for the sprint to end. Delivery status (on_time / late / not_delivered) is only emitted for
// iterations whose FinishDate is in the past — classifying an in-progress sprint as "not
// delivered" would be misleading, so delivery metrics are skipped for it.
//
// Velocity only counts work items whose current state is terminal — an item still open past its
// sprint's end isn't "completed work" for that sprint, even though it's still associated with it.
// Delivery status classifies every item in a finished sprint's backlog three ways: "on_time"
// (closed on or before the sprint's own end date), "late" (closed after), or "not_delivered"
// (never reached a terminal state). A terminal item with no ClosedDate recorded still counts
// toward velocity but can't be classified as on_time or late. If the iteration has no FinishDate
// on record, delivery status is skipped entirely — velocity is unaffected either way.
func collectSprintHistory(client *azuredevops.Client, organization, project string, team azuredevops.Team, itemByID map[int]azuredevops.WorkItem) {
	// Fetch past + current iterations together so the current sprint appears in velocity charts.
	// ListTeamIterations with an empty timeframe returns everything; we exclude future sprints.
	allIterations, err := client.ListTeamIterations(project, team.Name, "")
	if err != nil {
		return
	}
	var iterations []azuredevops.Iteration
	for _, it := range allIterations {
		if it.Attributes.TimeFrame == "past" || it.Attributes.TimeFrame == "current" {
			iterations = append(iterations, it)
		}
	}
	if len(iterations) == 0 {
		return
	}
	sort.Slice(iterations, func(i, j int) bool {
		return iterations[i].Attributes.StartDate.Before(iterations[j].Attributes.StartDate)
	})
	if len(iterations) > velocitySprintCount {
		iterations = iterations[len(iterations)-velocitySprintCount:]
	}

	now := time.Now()
	for _, iteration := range iterations {
		ids, err := client.ListIterationWorkItemIDs(project, team.Name, iteration.ID)
		if err != nil {
			continue
		}

		// sprintEnded is true only when the sprint's finish date has actually passed.
		// The current sprint has a FinishDate but it's in the future, so delivery metrics
		// would be meaningless (all items would show as "not_delivered" mid-sprint).
		sprintEnded := !iteration.Attributes.FinishDate.IsZero() && !iteration.Attributes.FinishDate.After(now)

		var points float64
		var throughput, scopeAdded int
		var onTime, late, notDelivered int
		for _, id := range ids {
			item, ok := itemByID[id]
			if !ok {
				continue
			}
			// Scope creep: item was created after the sprint started.
			if !iteration.Attributes.StartDate.IsZero() && item.Fields.CreatedDate.After(iteration.Attributes.StartDate) {
				scopeAdded++
			}
			if !isTerminalState(item.Fields.State) {
				if sprintEnded {
					notDelivered++
				}
				continue
			}
			throughput++
			points += pointsOf(item)
			if item.Fields.ClosedDate.IsZero() || !sprintEnded {
				continue
			}
			if !item.Fields.ClosedDate.After(iteration.Attributes.FinishDate) {
				onTime++
			} else {
				late++
			}
		}
		metrics.BoardsSprintVelocityStoryPoints.WithLabelValues(organization, project, team.Name, iteration.Name).Set(points)
		metrics.BoardsSprintThroughputTotal.WithLabelValues(organization, project, team.Name, iteration.Name).Set(float64(throughput))
		metrics.BoardsSprintScopeAddedTotal.WithLabelValues(organization, project, team.Name, iteration.Name).Set(float64(scopeAdded))

		if sprintEnded {
			metrics.BoardsSprintDeliveryTotal.WithLabelValues(organization, project, team.Name, iteration.Name, "on_time").Set(float64(onTime))
			metrics.BoardsSprintDeliveryTotal.WithLabelValues(organization, project, team.Name, iteration.Name, "late").Set(float64(late))
			metrics.BoardsSprintDeliveryTotal.WithLabelValues(organization, project, team.Name, iteration.Name, "not_delivered").Set(float64(notDelivered))
		}
	}
}

// pointsOf returns a work item's Story Points, falling back to Effort when Story Points isn't
// set — the same fallback convention used by BoardsWorkItemsWithoutEstimateTotal.
func pointsOf(item azuredevops.WorkItem) float64 {
	if item.Fields.StoryPoints != nil {
		return *item.Fields.StoryPoints
	}
	if item.Fields.Effort != nil {
		return *item.Fields.Effort
	}
	return 0
}

// totalCapacityPerDay sums every team member's capacityPerDay across all their activities.
func totalCapacityPerDay(capacity *azuredevops.Capacity) float64 {
	var total float64
	for _, member := range capacity.TeamMembers {
		for _, activity := range member.Activities {
			total += activity.CapacityPerDay
		}
	}
	return total
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

// splitCustomFieldValues splits a custom field's raw string on ";" — how Azure DevOps
// serializes a multi-select picklist field's selected values (e.g. "cxm;nps;opencx") — and
// trims whitespace around each one. A single-value field (no ";") comes back as one element,
// same as before this split existed. An empty/unset field returns nil.
func splitCustomFieldValues(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	values := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			values = append(values, p)
		}
	}
	return values
}

// hasTag reports whether tags (Azure DevOps' own ";"-separated System.Tags value, e.g.
// "Blocked; UrgentFix") contains want, matched case-insensitively and trimmed the same way
// splitCustomFieldValues handles multi-value custom fields.
func hasTag(tags, want string) bool {
	for _, t := range splitCustomFieldValues(tags) {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}
