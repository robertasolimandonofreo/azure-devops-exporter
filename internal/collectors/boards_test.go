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

// boardsFakeServer serves 5 work items: two active tasks (one assigned, one not, the
// unassigned one stale), two closed bugs in the same area/iteration (lead times of 50 and 10
// days, to exercise the lead time aggregation), and a third active task with no area/iteration
// path set (to exercise the without_iteration/without_area_path metrics). It also serves a
// single team ("Team A") with one current-timeframe iteration ("Sprint 1", backlog: items 1
// and 2, exercising the active-sprint and capacity metrics) and two past iterations ("Sprint
// P1" backlog: item 3; "Sprint P2" backlog: items 2 and 4 — item 2 is still Active, so it must
// be excluded from velocity, while item 4 has no Story Points and exercises the Effort
// fallback), exercising the velocity metric and its sort-by-start-date behavior (the server
// deliberately returns them out of chronological order).
func boardsFakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	now := time.Now()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_apis/wit/wiql"):
			var body struct {
				Query string `json:"query"`
			}
			json.NewDecoder(r.Body).Decode(&body)

			var ids []map[string]int
			switch {
			case strings.Contains(body.Query, "CreatedDate"):
				ids = []map[string]int{{"id": 1}}
			case strings.Contains(body.Query, "ChangedDate"):
				ids = []map[string]int{{"id": 3}}
			default:
				ids = []map[string]int{{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}, {"id": 5}}
			}
			json.NewEncoder(w).Encode(map[string]any{"workItems": ids})
		case strings.HasSuffix(r.URL.Path, "/_apis/wit/workitemsbatch"):
			items := []map[string]any{
				{"id": 1, "fields": map[string]any{
					"System.WorkItemType":                   "Task",
					"System.State":                          "Active",
					"System.AreaPath":                       "proj\\TeamA",
					"System.IterationPath":                  "proj\\Sprint 1",
					"System.CreatedDate":                    now.Add(-5 * 24 * time.Hour).Format(time.RFC3339),
					"System.ChangedDate":                    now.Add(-1 * 24 * time.Hour).Format(time.RFC3339),
					"Microsoft.VSTS.Common.Priority":        2,
					"Microsoft.VSTS.Scheduling.StoryPoints": 3.0,
					"System.AssignedTo":                     map[string]string{"displayName": "Alice"},
				}},
				{"id": 2, "fields": map[string]any{
					"System.WorkItemType":  "Task",
					"System.State":         "Active",
					"System.AreaPath":      "proj\\TeamA",
					"System.IterationPath": "proj\\Sprint 1",
					"System.CreatedDate":   now.Add(-20 * 24 * time.Hour).Format(time.RFC3339),
					"System.ChangedDate":   now.Add(-20 * 24 * time.Hour).Format(time.RFC3339),
				}},
				{"id": 3, "fields": map[string]any{
					"System.WorkItemType":                   "Bug",
					"System.State":                          "Closed",
					"System.AreaPath":                       "proj\\TeamB",
					"System.IterationPath":                  "proj\\Sprint 2",
					"System.CreatedDate":                    now.Add(-100 * 24 * time.Hour).Format(time.RFC3339),
					"System.ChangedDate":                    now.Add(-50 * 24 * time.Hour).Format(time.RFC3339),
					"Microsoft.VSTS.Common.ClosedDate":      now.Add(-50 * 24 * time.Hour).Format(time.RFC3339),
					"Microsoft.VSTS.Common.Priority":        1,
					"Microsoft.VSTS.Common.Severity":        "2 - High",
					"Microsoft.VSTS.Scheduling.StoryPoints": 5.0,
					"Microsoft.VSTS.Scheduling.Effort":      8.0,
					"System.AssignedTo":                     map[string]string{"displayName": "Alice"},
				}},
				{"id": 4, "fields": map[string]any{
					"System.WorkItemType":              "Bug",
					"System.State":                     "Closed",
					"System.AreaPath":                  "proj\\TeamB",
					"System.IterationPath":             "proj\\Sprint 2",
					"System.CreatedDate":               now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
					"System.ChangedDate":               now.Add(-20 * 24 * time.Hour).Format(time.RFC3339),
					"Microsoft.VSTS.Common.ClosedDate": now.Add(-20 * 24 * time.Hour).Format(time.RFC3339),
					"Microsoft.VSTS.Common.Priority":   1,
					"Microsoft.VSTS.Common.Severity":   "3 - Medium",
					"Microsoft.VSTS.Scheduling.Effort": 2.0,
				}},
				{"id": 5, "fields": map[string]any{
					"System.WorkItemType":                   "Task",
					"System.State":                          "Active",
					"System.AreaPath":                       "",
					"System.IterationPath":                  "",
					"System.CreatedDate":                    now.Add(-3 * 24 * time.Hour).Format(time.RFC3339),
					"System.ChangedDate":                    now.Add(-1 * 24 * time.Hour).Format(time.RFC3339),
					"Microsoft.VSTS.Scheduling.StoryPoints": 2.0,
				}},
			}
			json.NewEncoder(w).Encode(map[string]any{"value": items})
		case strings.Contains(r.URL.Path, "/_apis/projects/") && strings.HasSuffix(r.URL.Path, "/teams"):
			json.NewEncoder(w).Encode(map[string]any{"value": []map[string]string{{"id": "team-a-id", "name": "Team A"}}})
		case strings.HasSuffix(r.URL.Path, "/_apis/work/teamsettings/iterations"):
			switch r.URL.Query().Get("$timeframe") {
			case "current":
				json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"id": "iter-1", "name": "Sprint 1"}}})
			case "past":
				// Deliberately out of chronological order, to exercise the collector's own sort.
				json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{
					{"id": "iter-p2", "name": "Sprint P2", "attributes": map[string]any{"startDate": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}},
					{"id": "iter-p1", "name": "Sprint P1", "attributes": map[string]any{"startDate": now.Add(-24 * 24 * time.Hour).Format(time.RFC3339)}},
				}})
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		case strings.HasSuffix(r.URL.Path, "/_apis/work/teamsettings/iterations/iter-1/workitems"):
			json.NewEncoder(w).Encode(map[string]any{"workItemRelations": []map[string]any{
				{"target": map[string]any{"id": 1}},
				{"target": map[string]any{"id": 2}},
			}})
		case strings.HasSuffix(r.URL.Path, "/_apis/work/teamsettings/iterations/iter-1/capacities"):
			json.NewEncoder(w).Encode(map[string]any{"teamMembers": []map[string]any{
				{"activities": []map[string]any{{"capacityPerDay": 4.0}, {"capacityPerDay": 2.0}}},
				{"activities": []map[string]any{{"capacityPerDay": 3.0}}},
			}})
		case strings.HasSuffix(r.URL.Path, "/_apis/work/teamsettings/iterations/iter-p1/workitems"):
			json.NewEncoder(w).Encode(map[string]any{"workItemRelations": []map[string]any{
				{"target": map[string]any{"id": 3}},
			}})
		case strings.HasSuffix(r.URL.Path, "/_apis/work/teamsettings/iterations/iter-p2/workitems"):
			json.NewEncoder(w).Encode(map[string]any{"workItemRelations": []map[string]any{
				{"target": map[string]any{"id": 2}},
				{"target": map[string]any{"id": 4}},
			}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCollectBoards(t *testing.T) {
	server := boardsFakeServer(t)
	defer server.Close()

	client := azuredevops.NewClient(server.URL, "org", "token")
	if err := CollectBoards(client, "org", "proj"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := gaugeValue(t, metrics.BoardsWorkItemsTotal, "org", "proj"); got != 5 {
		t.Errorf("BoardsWorkItemsTotal = %v, want 5", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsByState, "org", "proj", "Task", "Active", "proj\\TeamA", "proj\\Sprint 1"); got != 2 {
		t.Errorf("BoardsWorkItemsByState[Task,Active] = %v, want 2", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsByState, "org", "proj", "Bug", "Closed", "proj\\TeamB", "proj\\Sprint 2"); got != 2 {
		t.Errorf("BoardsWorkItemsByState[Bug,Closed] = %v, want 2", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsByAssignee, "org", "proj", "Alice"); got != 2 {
		t.Errorf("BoardsWorkItemsByAssignee[Alice] = %v, want 2", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsByAssignee, "org", "proj", unassigned); got != 3 {
		t.Errorf("BoardsWorkItemsByAssignee[unassigned] = %v, want 3", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsCreatedTotal, "org", "proj"); got != 1 {
		t.Errorf("BoardsWorkItemsCreatedTotal = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsClosedTotal, "org", "proj"); got != 1 {
		t.Errorf("BoardsWorkItemsClosedTotal = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemAgeDays, "org", "proj", "Task", "Active", "Alice", "1"); got < 4.9 || got > 5.1 {
		t.Errorf("BoardsWorkItemAgeDays[1] = %v, want ~5", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemAgeDays, "org", "proj", "Bug", "Closed", "Alice", "3"); got < 99.9 || got > 100.1 {
		t.Errorf("BoardsWorkItemAgeDays[3] = %v, want ~100", got)
	}
	// Item 1 changed 1 day ago (fresh) and item 2 changed 20 days ago (stale); both are
	// Task/Active, so the stale count for that state should only include item 2.
	if got := gaugeValue(t, metrics.BoardsWorkItemsStaleTotal, "org", "proj", "Task", "Active"); got != 1 {
		t.Errorf("BoardsWorkItemsStaleTotal[Task,Active] = %v, want 1", got)
	}
	// Item 3 is old and unchanged for 50 days, but Closed is a terminal state, so it must
	// not count as stale.
	if got := gaugeValue(t, metrics.BoardsWorkItemsStaleTotal, "org", "proj", "Bug", "Closed"); got != 0 {
		t.Errorf("BoardsWorkItemsStaleTotal[Bug,Closed] = %v, want 0", got)
	}
	// Item 3 has a lead time of ~50 days (closed 50 days after creation) and item 4 of
	// ~10 days; both are Bug/TeamB/Sprint 2, so sorted lead times are [10, 50].
	if got := gaugeValue(t, metrics.BoardsLeadTimeAvgDays, "org", "proj", "Bug", "proj\\TeamB", "proj\\Sprint 2"); got < 29.9 || got > 30.1 {
		t.Errorf("BoardsLeadTimeAvgDays = %v, want ~30", got)
	}
	if got := gaugeValue(t, metrics.BoardsLeadTimeP50Days, "org", "proj", "Bug", "proj\\TeamB", "proj\\Sprint 2"); got < 9.9 || got > 10.1 {
		t.Errorf("BoardsLeadTimeP50Days = %v, want ~10", got)
	}
	if got := gaugeValue(t, metrics.BoardsLeadTimeP90Days, "org", "proj", "Bug", "proj\\TeamB", "proj\\Sprint 2"); got < 9.9 || got > 10.1 {
		t.Errorf("BoardsLeadTimeP90Days = %v, want ~10", got)
	}
	if got := gaugeValue(t, metrics.BoardsLeadTimeMaxDays, "org", "proj", "Bug", "proj\\TeamB", "proj\\Sprint 2"); got < 49.9 || got > 50.1 {
		t.Errorf("BoardsLeadTimeMaxDays = %v, want ~50", got)
	}

	// Priority: item1 (Task, priority 2), items 3+4 (Bug, priority 1). Items 2 and 5 have no
	// priority set and must be excluded.
	if got := gaugeValue(t, metrics.BoardsWorkItemsByPriority, "org", "proj", "Task", "2"); got != 1 {
		t.Errorf("BoardsWorkItemsByPriority[Task,2] = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsByPriority, "org", "proj", "Bug", "1"); got != 2 {
		t.Errorf("BoardsWorkItemsByPriority[Bug,1] = %v, want 2", got)
	}

	// Severity is only tallied for Bug work items.
	if got := gaugeValue(t, metrics.BoardsBugsBySeverity, "org", "proj", "2 - High"); got != 1 {
		t.Errorf("BoardsBugsBySeverity[2 - High] = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.BoardsBugsBySeverity, "org", "proj", "3 - Medium"); got != 1 {
		t.Errorf("BoardsBugsBySeverity[3 - Medium] = %v, want 1", got)
	}

	// Only item 2 (Task/Active) has neither Story Points nor Effort set; items 1 and 5 have
	// Story Points, so Task/Active without_estimate is 1. Bug/Closed items both have at least
	// one of the two fields set, so it's 0.
	if got := gaugeValue(t, metrics.BoardsWorkItemsWithoutEstimateTotal, "org", "proj", "Task", "Active"); got != 1 {
		t.Errorf("BoardsWorkItemsWithoutEstimateTotal[Task,Active] = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsWithoutEstimateTotal, "org", "proj", "Bug", "Closed"); got != 0 {
		t.Errorf("BoardsWorkItemsWithoutEstimateTotal[Bug,Closed] = %v, want 0", got)
	}

	// Only item 5 has an empty area path / iteration path.
	if got := gaugeValue(t, metrics.BoardsWorkItemsWithoutIterationTotal, "org", "proj", "Task"); got != 1 {
		t.Errorf("BoardsWorkItemsWithoutIterationTotal[Task] = %v, want 1", got)
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsWithoutAreaPathTotal, "org", "proj", "Task"); got != 1 {
		t.Errorf("BoardsWorkItemsWithoutAreaPathTotal[Task] = %v, want 1", got)
	}

	// Story Points: Task/Active = item1(3) + item5(2) = 5; Bug/Closed = item3(5) only.
	if got := gaugeValue(t, metrics.BoardsStoryPointsTotal, "org", "proj", "Task", "Active"); got != 5 {
		t.Errorf("BoardsStoryPointsTotal[Task,Active] = %v, want 5", got)
	}
	if got := gaugeValue(t, metrics.BoardsStoryPointsTotal, "org", "proj", "Bug", "Closed"); got != 5 {
		t.Errorf("BoardsStoryPointsTotal[Bug,Closed] = %v, want 5", got)
	}

	// Effort: Bug/Closed = item3(8) + item4(2) = 10.
	if got := gaugeValue(t, metrics.BoardsEffortTotal, "org", "proj", "Bug", "Closed"); got != 10 {
		t.Errorf("BoardsEffortTotal[Bug,Closed] = %v, want 10", got)
	}

	// Team A's current sprint (iter-1) contains items 1 and 2, both Task/Active.
	if got := gaugeValue(t, metrics.BoardsActiveSprintWorkItemsTotal, "org", "proj", "Team A", "Task", "Active"); got != 2 {
		t.Errorf("BoardsActiveSprintWorkItemsTotal[Team A,Task,Active] = %v, want 2", got)
	}

	// Active sprint story points: item1 has 3, item2 has neither Story Points nor Effort.
	if got := gaugeValue(t, metrics.BoardsActiveSprintStoryPointsTotal, "org", "proj", "Team A"); got != 3 {
		t.Errorf("BoardsActiveSprintStoryPointsTotal[Team A] = %v, want 3", got)
	}

	// Team capacity: two members with capacityPerDay [4, 2] and [3] = 9 total.
	if got := gaugeValue(t, metrics.BoardsTeamCapacityHoursPerDay, "org", "proj", "Team A"); got != 9 {
		t.Errorf("BoardsTeamCapacityHoursPerDay[Team A] = %v, want 9", got)
	}

	// Sprint P1's backlog is item 3 (Bug/Closed, Story Points 5) — fully completed.
	if got := gaugeValue(t, metrics.BoardsSprintVelocityStoryPoints, "org", "proj", "Team A", "Sprint P1"); got != 5 {
		t.Errorf("BoardsSprintVelocityStoryPoints[Team A,Sprint P1] = %v, want 5", got)
	}
	// Sprint P2's backlog is items 2 and 4: item 2 is Task/Active (not completed, excluded);
	// item 4 is Bug/Closed with no Story Points, so its Effort (2) is used instead.
	if got := gaugeValue(t, metrics.BoardsSprintVelocityStoryPoints, "org", "proj", "Team A", "Sprint P2"); got != 2 {
		t.Errorf("BoardsSprintVelocityStoryPoints[Team A,Sprint P2] = %v, want 2", got)
	}
}

func TestCollectBoards_KeepsPreviousMetricsOnError(t *testing.T) {
	server := boardsFakeServer(t)
	client := azuredevops.NewClient(server.URL, "org", "token")
	if err := CollectBoards(client, "org", "proj-keep"); err != nil {
		t.Fatalf("unexpected error on first collect: %v", err)
	}
	server.Close()

	if err := CollectBoards(client, "org", "proj-keep"); err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if got := gaugeValue(t, metrics.BoardsWorkItemsTotal, "org", "proj-keep"); got != 5 {
		t.Errorf("BoardsWorkItemsTotal after failed scrape = %v, want unchanged 5", got)
	}
}
