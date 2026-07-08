// Package metrics defines the Prometheus metrics exposed by the exporter.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Repos domain.
	ReposTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repos_total",
		Help: "Number of Git repositories in the project.",
	}, []string{"organization", "project"})

	PullRequestsActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pull_requests_active",
		Help: "Number of active pull requests in the repository.",
	}, []string{"organization", "project", "repository"})

	PullRequestsCompleted = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pull_requests_completed",
		Help: "Number of completed pull requests in the repository.",
	}, []string{"organization", "project", "repository"})

	PullRequestsAbandoned = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pull_requests_abandoned",
		Help: "Number of abandoned pull requests in the repository.",
	}, []string{"organization", "project", "repository"})

	BranchesTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_branches_total",
		Help: "Number of branches in the repository.",
	}, []string{"organization", "project", "repository"})

	CommitsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_commits_total",
		Help: "Number of commits pushed in the last 24 hours, recomputed on every scrape.",
	}, []string{"organization", "project", "repository"})

	PullRequestAgeDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pull_request_age_days",
		Help: "Age in days since creation of each active pull request.",
	}, []string{"organization", "project", "repository", "pull_request_id"})

	StalePullRequestsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_stale_pull_requests_total",
		Help: "Number of active pull requests open for longer than the staleness threshold — see README.",
	}, []string{"organization", "project", "repository"})

	prLeadTimeLabels = []string{"organization", "project", "repository"}

	PRLeadTimeAvgDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pr_lead_time_avg_days",
		Help: "Average lead time (closed date minus creation date) in days for merged pull requests.",
	}, prLeadTimeLabels)

	PRLeadTimeP50Days = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pr_lead_time_p50_days",
		Help: "Median lead time (closed date minus creation date) in days for merged pull requests.",
	}, prLeadTimeLabels)

	PRLeadTimeP90Days = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pr_lead_time_p90_days",
		Help: "90th percentile lead time (closed date minus creation date) in days for merged pull requests.",
	}, prLeadTimeLabels)

	PRLeadTimeMaxDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pr_lead_time_max_days",
		Help: "Maximum lead time (closed date minus creation date) in days for merged pull requests.",
	}, prLeadTimeLabels)

	DraftPullRequestsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_draft_pull_requests_total",
		Help: "Number of active pull requests marked as draft.",
	}, []string{"organization", "project", "repository"})

	PullRequestsWithConflictsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pull_requests_with_conflicts_total",
		Help: "Number of active pull requests with merge conflicts.",
	}, []string{"organization", "project", "repository"})

	PullRequestsWithoutReviewerTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pull_requests_without_reviewer_total",
		Help: "Number of active pull requests with no reviewers assigned.",
	}, []string{"organization", "project", "repository"})

	PullRequestsPendingApprovalTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_pull_requests_pending_approval_total",
		Help: "Number of active pull requests with reviewers assigned but no approval vote yet.",
	}, []string{"organization", "project", "repository"})

	RepoSizeBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_size_bytes",
		Help: "Size of the repository in bytes.",
	}, []string{"organization", "project", "repository"})

	CommitsByAuthorTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_repo_commits_by_author_total",
		Help: "Number of commits pushed in the last 24 hours, by author.",
	}, []string{"organization", "project", "repository", "author"})

	// Boards domain.
	BoardsWorkItemsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_total",
		Help: "Number of non-removed work items in the project.",
	}, []string{"organization", "project"})

	BoardsWorkItemsByState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_by_state",
		Help: "Number of work items by type, state, area path and iteration path.",
	}, []string{"organization", "project", "work_item_type", "state", "area_path", "iteration_path"})

	BoardsWorkItemsByAssignee = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_by_assignee",
		Help: "Number of work items by assignee, area path and iteration path.",
	}, []string{"organization", "project", "assigned_to", "area_path", "iteration_path"})

	BoardsWorkItemsCreatedTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_created_total",
		Help: "Number of work items created since midnight yesterday, recomputed on every scrape.",
	}, []string{"organization", "project"})

	BoardsWorkItemsClosedTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_closed_total",
		Help: "Number of work items closed (approximate, see README) since midnight yesterday, recomputed on every scrape.",
	}, []string{"organization", "project"})

	BoardsWorkItemAgeDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_item_age_days",
		Help: "Age in days since creation of each work item.",
	}, []string{"organization", "project", "work_item_type", "state", "assigned_to", "work_item_id", "area_path", "iteration_path"})

	BoardsWorkItemsStaleTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_stale_total",
		Help: "Number of non-closed work items with no field changes recently, by type, state, area path and iteration path — see README for the staleness threshold.",
	}, []string{"organization", "project", "work_item_type", "state", "area_path", "iteration_path"})

	leadTimeLabels = []string{"organization", "project", "work_item_type", "area_path", "iteration_path"}

	BoardsLeadTimeAvgDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_lead_time_avg_days",
		Help: "Average lead time (closed date minus created date) in days for closed work items.",
	}, leadTimeLabels)

	BoardsLeadTimeP50Days = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_lead_time_p50_days",
		Help: "Median lead time (closed date minus created date) in days for closed work items.",
	}, leadTimeLabels)

	BoardsLeadTimeP90Days = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_lead_time_p90_days",
		Help: "90th percentile lead time (closed date minus created date) in days for closed work items.",
	}, leadTimeLabels)

	BoardsLeadTimeMaxDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_lead_time_max_days",
		Help: "Maximum lead time (closed date minus created date) in days for closed work items.",
	}, leadTimeLabels)

	BoardsWorkItemsByPriority = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_by_priority",
		Help: "Number of work items by type, priority, area path and iteration path. Items with no priority set are excluded.",
	}, []string{"organization", "project", "work_item_type", "priority", "area_path", "iteration_path"})

	BoardsBugsBySeverity = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_bugs_by_severity",
		Help: "Number of Bug work items by severity, area path and iteration path. Bugs with no severity set are excluded.",
	}, []string{"organization", "project", "severity", "area_path", "iteration_path"})

	BoardsWorkItemsWithoutEstimateTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_without_estimate_total",
		Help: "Number of work items with neither Story Points nor Effort set, by type, state, area path and iteration path.",
	}, []string{"organization", "project", "work_item_type", "state", "area_path", "iteration_path"})

	BoardsWorkItemsWithoutIterationTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_without_iteration_total",
		Help: "Number of work items with an empty iteration path, by type. Azure DevOps defaults this field to the project's root iteration, so this is usually 0 — see README.",
	}, []string{"organization", "project", "work_item_type"})

	BoardsWorkItemsWithoutAreaPathTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_without_area_path_total",
		Help: "Number of work items with an empty area path, by type. Azure DevOps defaults this field to the project's root area, so this is usually 0 — see README.",
	}, []string{"organization", "project", "work_item_type"})

	BoardsStoryPointsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_story_points_total",
		Help: "Sum of Microsoft.VSTS.Scheduling.StoryPoints by type, state, area path and iteration path. Work items without this field set (process-template-dependent) don't contribute.",
	}, []string{"organization", "project", "work_item_type", "state", "area_path", "iteration_path"})

	BoardsEffortTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_effort_total",
		Help: "Sum of Microsoft.VSTS.Scheduling.Effort by type, state, area path and iteration path. Work items without this field set (process-template-dependent) don't contribute.",
	}, []string{"organization", "project", "work_item_type", "state", "area_path", "iteration_path"})

	BoardsActiveSprintWorkItemsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_active_sprint_work_items_total",
		Help: "Number of work items in each team's current sprint, by team, type and state. Teams with no current iteration contribute no series.",
	}, []string{"organization", "project", "team", "work_item_type", "state"})

	BoardsActiveSprintStoryPointsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_active_sprint_story_points_total",
		Help: "Sum of Story Points (falling back to Effort when Story Points is unset) for work items in each team's current sprint. Compare with team_capacity_hours_per_day to gauge over/under-allocation — see README.",
	}, []string{"organization", "project", "team"})

	BoardsTeamCapacityHoursPerDay = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_team_capacity_hours_per_day",
		Help: "Sum of each team member's configured capacityPerDay (all activities) for the team's current sprint. Not adjusted for days off or sprint length — see README. Teams with no current iteration contribute no series.",
	}, []string{"organization", "project", "team"})

	BoardsSprintVelocityStoryPoints = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_sprint_velocity_story_points",
		Help: "Sum of Story Points (falling back to Effort) completed in each of a team's last few past sprints, by iteration name — see README for how many sprints and what counts as completed.",
	}, []string{"organization", "project", "team", "iteration"})

	BoardsWorkItemsByCustomFieldTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_by_custom_field_total",
		Help: "Number of work items by type, state, area path, iteration path and configured custom field value. Only populated for fields listed in AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS — see README. Items where the field is unset are counted under value=\"unset\"; a multi-select field's \";\"-separated values are split, so one item can count under more than one value.",
	}, []string{"organization", "project", "work_item_type", "state", "field", "value", "area_path", "iteration_path"})

	BoardsSprintDeliveryTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_sprint_delivery_total",
		Help: "Number of work items in each of a team's last few past sprints, by delivery status: on_time (closed on or before the sprint's end date), late (closed after it), or not_delivered (never closed). See README for exact semantics.",
	}, []string{"organization", "project", "team", "iteration", "status"})

	BoardsWorkItemStoryPoints = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_item_story_points",
		Help: "Story Points of each non-removed work item that has them set (Microsoft.VSTS.Scheduling.StoryPoints — no Effort fallback, unlike story_points_total). One series per item, meant for a per-item threshold alert (e.g. \"still open above N points, should have been split\") that the aggregated story_points_total can't express. Items without Story Points set contribute no series.",
	}, []string{"organization", "project", "work_item_type", "state", "work_item_id", "area_path", "iteration_path"})

	BoardsWorkItemsBlockedTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_work_items_blocked_total",
		Help: "Number of non-removed work items with a \"blocked\" tag (case-insensitive match against System.Tags), by type, state, area path and iteration path.",
	}, []string{"organization", "project", "work_item_type", "state", "area_path", "iteration_path"})

	cycleTimeLabels = []string{"organization", "project", "work_item_type", "area_path", "iteration_path"}

	BoardsCycleTimeAvgDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_cycle_time_avg_days",
		Help: "Average cycle time (ActivatedDate → ClosedDate) in days for closed work items. Only items where Microsoft.VSTS.Common.ActivatedDate is set contribute.",
	}, cycleTimeLabels)

	BoardsCycleTimeP50Days = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_cycle_time_p50_days",
		Help: "Median cycle time (ActivatedDate → ClosedDate) in days for closed work items.",
	}, cycleTimeLabels)

	BoardsCycleTimeP90Days = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_cycle_time_p90_days",
		Help: "90th-percentile cycle time (ActivatedDate → ClosedDate) in days for closed work items.",
	}, cycleTimeLabels)

	BoardsSprintThroughputTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_sprint_throughput_items_total",
		Help: "Number of work items (count, not story points) that reached a terminal state during each sprint.",
	}, []string{"organization", "project", "team", "iteration"})

	BoardsSprintScopeAddedTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_boards_sprint_scope_added_total",
		Help: "Number of work items added to the sprint backlog after the sprint started (CreatedDate > sprint StartDate). A proxy for mid-sprint scope creep.",
	}, []string{"organization", "project", "team", "iteration"})

	// Pipelines domain.
	PipelinesTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipelines_total",
		Help: "Number of pipeline (build) definitions in the project.",
	}, []string{"organization", "project"})

	pipelineLabels = []string{"organization", "project", "pipeline", "pipeline_id"}

	PipelineRunsSucceeded = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_runs_succeeded",
		Help: "Number of pipeline runs that succeeded in the last 24 hours, recomputed on every scrape.",
	}, pipelineLabels)

	PipelineRunsFailed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_runs_failed",
		Help: "Number of pipeline runs that failed in the last 24 hours, recomputed on every scrape.",
	}, pipelineLabels)

	PipelineRunsCanceled = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_runs_canceled",
		Help: "Number of pipeline runs that were canceled in the last 24 hours, recomputed on every scrape.",
	}, pipelineLabels)

	PipelineRunDurationSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_run_duration_seconds",
		Help: "Average duration in seconds of completed pipeline runs in the last 24 hours.",
	}, pipelineLabels)

	PipelineLastRunTimestamp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_last_run_timestamp",
		Help: "Unix timestamp of the most recent run of the pipeline, regardless of the 24-hour window above.",
	}, pipelineLabels)

	PipelineRunsInProgress = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_runs_in_progress",
		Help: "Number of runs currently in progress, among those queued in the last 24 hours. A run queued earlier and still running would not be counted — see README.",
	}, pipelineLabels)

	PipelineQueueTimeSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_queue_time_seconds",
		Help: "Average time in seconds completed runs spent waiting in the queue before starting, in the last 24 hours.",
	}, pipelineLabels)

	PipelineRunDurationP50Seconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_run_duration_p50_seconds",
		Help: "Median duration in seconds of completed pipeline runs in the last 24 hours.",
	}, pipelineLabels)

	PipelineRunDurationP90Seconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_run_duration_p90_seconds",
		Help: "90th percentile duration in seconds of completed pipeline runs in the last 24 hours.",
	}, pipelineLabels)

	PipelineRunDurationMaxSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_run_duration_max_seconds",
		Help: "Maximum duration in seconds of completed pipeline runs in the last 24 hours.",
	}, pipelineLabels)

	PipelineRunsByBranchTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_pipeline_runs_by_branch_total",
		Help: "Number of completed pipeline runs in the last 24 hours, by source branch and result. High-cardinality if CI runs on every feature branch — see README.",
	}, []string{"organization", "project", "pipeline", "pipeline_id", "branch", "result"})

	// Releases domain (classic Release Management).
	ReleasesTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_releases_total",
		Help: "Number of classic release definitions in the project.",
	}, []string{"organization", "project"})

	releaseLabels = []string{"organization", "project", "release_definition", "environment"}

	ReleaseDeploymentsSucceeded = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_release_deployments_succeeded",
		Help: "Number of release deployments that succeeded in the last 30 days, recomputed on every scrape.",
	}, releaseLabels)

	ReleaseDeploymentsFailed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_release_deployments_failed",
		Help: "Number of release deployments that failed in the last 30 days, recomputed on every scrape.",
	}, releaseLabels)

	ReleaseDeploymentDurationSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_release_deployment_duration_seconds",
		Help: "Average duration in seconds of completed release deployments in the last 30 days.",
	}, releaseLabels)

	ReleaseLastDeploymentTimestamp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_release_last_deployment_timestamp",
		Help: "Unix timestamp of the most recent deployment to the environment, within the last 30 days.",
	}, releaseLabels)

	ReleaseDeploymentsNotDeployed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_release_deployments_not_deployed",
		Help: "Number of deployments with deploymentStatus=notDeployed in the last 30 days — skipped or not triggered, not the same as \"pending manual approval\". See README.",
	}, releaseLabels)

	ReleaseLeadTimeForChangesAvgDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_release_lead_time_for_changes_avg_days",
		Help: "Average lead time in days from commit to production deployment (DORA Lead Time for Changes), for successful deployments in the last 30 days. See README for how commits are traced and what's excluded.",
	}, releaseLabels)

	ReleaseLeadTimeForChangesP50Days = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_release_lead_time_for_changes_p50_days",
		Help: "Median lead time in days from commit to production deployment (DORA Lead Time for Changes), for successful deployments in the last 30 days.",
	}, releaseLabels)

	ReleaseLeadTimeForChangesP90Days = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_release_lead_time_for_changes_p90_days",
		Help: "90th percentile lead time in days from commit to production deployment (DORA Lead Time for Changes), for successful deployments in the last 30 days.",
	}, releaseLabels)

	ReleaseLeadTimeForChangesMaxDays = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_release_lead_time_for_changes_max_days",
		Help: "Maximum lead time in days from commit to production deployment (DORA Lead Time for Changes), for successful deployments in the last 30 days.",
	}, releaseLabels)

	// Exporter self-observability, shared by all collectors.
	ScrapeDurationSeconds = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_exporter_scrape_duration_seconds",
		Help: "Duration of the last scrape per component.",
	}, []string{"component", "organization", "project"})

	ScrapeErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "azure_devops_exporter_scrape_errors_total",
		Help: "Total number of failed scrapes per component.",
	}, []string{"component", "organization", "project"})

	LastSuccessfulScrapeTimestamp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "azure_devops_exporter_last_successful_scrape_timestamp",
		Help: "Unix timestamp of the last successful scrape per component.",
	}, []string{"component", "organization", "project"})
)
