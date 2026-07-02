# Azure DevOps Prometheus Exporter

Prometheus exporter for Azure DevOps, covering **Repos**, **Boards**,
**Pipelines** and **Releases**.

Supports multiple projects within a single organization from one instance.

## Status

| Collector | Status |
| --- | --- |
| Repos | Implemented |
| Boards | Implemented |
| Pipelines | Implemented |
| Releases | Implemented |

## Configuration

| Variable | Description | Required | Default |
| --- | --- | --- | --- |
| `AZURE_DEVOPS_ORGANIZATION` | Azure DevOps organization name | Yes | - |
| `AZURE_DEVOPS_PROJECTS` | Comma-separated list of project names, each optionally restricted to specific collectors — see below | Yes | - |
| `AZURE_DEVOPS_TOKEN` | Personal Access Token (needs Code: Read, Work Items: Read, Build: Read, Release: Read, Project and Team: Read) | Yes | - |
| `AZURE_DEVOPS_API_URL` | Azure DevOps API base URL | No | `https://dev.azure.com` |
| `EXPORTER_PORT` | HTTP port | No | `8080` |
| `SCRAPE_INTERVAL_SECONDS` | Seconds between scrape cycles | No | `300` |
| `LOG_LEVEL` | `debug`, `info`, `warn`, or `error` | No | `info` |
| `AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS` | Comma-separated list of custom work item fields to break Boards metrics down by — see below | No | - |

The token is never logged; it is only sent in the `Authorization` header.

### Per-project collector selection

By default every project in `AZURE_DEVOPS_PROJECTS` gets all four collectors
(Repos, Boards, Pipelines, Releases) — this is unchanged from before this
option existed. To scrape only some collectors for a given project, append
`:` and a `+`-separated list of collector names (`repos`, `boards`,
`pipelines`, `releases`) to that project's name:

```bash
AZURE_DEVOPS_PROJECTS=proj-a:pipelines+boards,proj-b:repos,proj-c
```

- `proj-a` — only Pipelines and Boards
- `proj-b` — only Repos
- `proj-c` — no `:`, so all four collectors (the default)

This is per-project, not global: mixing restricted and unrestricted projects
in the same `AZURE_DEVOPS_PROJECTS` value, as above, is expected. An unknown
collector name (typo, wrong case) fails startup with a clear error rather
than silently being ignored. This only controls which collectors *run* for a
project — it doesn't change any collector's own behavior or metrics.

### Custom fields (Boards)

Every process template ends up with project-specific work item fields —
e.g. a "Platform" picklist a team added to track iOS/Android/Web work. By
default the Boards collector doesn't know these exist; `AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS`
tells it which ones to fetch and break `azure_devops_boards_work_items_by_custom_field_total`
down by, as a comma-separated list of Azure DevOps field **reference names**
(not their display names — see below for how to find these), each optionally
followed by `:` and a friendlier label to use in the metric instead:

```bash
AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS=Custom.Platform:platform,Custom.Squad
```

- `Custom.Platform` is exposed under `field="platform"` (the override)
- `Custom.Squad` has no override, so it's exposed under `field="Custom.Squad"`
  (the raw reference name)

This applies to every project the exporter scrapes (there's no per-project
custom field list, unlike the per-project collector selection above) — a
project whose process template doesn't have a given field simply reports no
data for it. A work item where the field is unset counts under
`value="unset"`, same convention as `unassigned` for `work_items_by_assignee`.

**A wrong reference name (typo, or a display name used by mistake) doesn't
just omit that field — Azure DevOps rejects the *entire* work item fetch
with an HTTP 400** ("TF51005: The field ... does not exist"), which would
otherwise take down every other Boards metric for that project. The
collector catches this: on a failed fetch with custom fields configured, it
retries once without them and logs a `WARN` (component `boards`) with the
underlying error, so a bad field name degrades to "no data for
`work_items_by_custom_field_total` this scrape," not a broken Boards
collector. Watch the exporter's logs for that warning if the metric never
shows data for a configured field — the message includes Azure DevOps' own
explanation of what was wrong with the reference name.

**Finding a field's reference name.** The Azure DevOps UI shows a field's
*display* name ("Platform"), not the reference name the REST API needs
(typically `Custom.Platform`, but process-template-dependent). The
reliable way to get it: **Project Settings → Process → (work item type) →
(the field) → Reference name**, or query
`GET https://dev.azure.com/{org}/{project}/_apis/wit/fields?api-version=7.1`
and match on the field's display name.

Non-string fields are stringified best-effort: identity-picker fields use
the assigned person's display name, numbers/booleans use their default
string form, and anything that doesn't decode as a plain string, an
identity, or one of those falls back to an empty (effectively `unset`)
value — this covers the common text/picklist case a "Platform" field is,
not every custom field type Azure DevOps supports.

**Multi-select picklist fields are split.** Azure DevOps serializes a
multi-select field's selected values as one `;`-separated string (e.g.
`"cxm;nps;opencx"`). Rather than exposing that whole string as a single
`value`, the collector splits it and counts the work item under *every*
value it has — so `work_items_by_custom_field_total{field="platform",
value="cxm"}` includes an item whose full selection is `"cxm;nps"`, not
just items where `cxm` is the only value selected. This means, for a
multi-select field, `sum(...) by (value)` no longer adds up to
`work_items_total` — a single item can contribute to more than one `value`
series, same as tags conceptually work anywhere else.

## Endpoints

- `GET /metrics` — Prometheus metrics
- `GET /health` — always 200 once the process is up
- `GET /ready` — 200 once the first scrape cycle has run, 503 before that

## Metrics (Repos)

```text
azure_devops_repos_total{organization,project}
azure_devops_repo_pull_requests_active{organization,project,repository}
azure_devops_repo_pull_requests_completed{organization,project,repository}
azure_devops_repo_pull_requests_abandoned{organization,project,repository}
azure_devops_repo_branches_total{organization,project,repository}
azure_devops_repo_commits_total{organization,project,repository}
azure_devops_repo_pull_request_age_days{organization,project,repository,pull_request_id}
azure_devops_repo_stale_pull_requests_total{organization,project,repository}
azure_devops_repo_pr_lead_time_avg_days{organization,project,repository}
azure_devops_repo_pr_lead_time_p50_days{organization,project,repository}
azure_devops_repo_pr_lead_time_p90_days{organization,project,repository}
azure_devops_repo_pr_lead_time_max_days{organization,project,repository}
azure_devops_repo_draft_pull_requests_total{organization,project,repository}
azure_devops_repo_pull_requests_with_conflicts_total{organization,project,repository}
azure_devops_repo_pull_requests_without_reviewer_total{organization,project,repository}
azure_devops_repo_pull_requests_pending_approval_total{organization,project,repository}
azure_devops_repo_size_bytes{organization,project,repository}
azure_devops_repo_commits_by_author_total{organization,project,repository,author}

azure_devops_exporter_scrape_duration_seconds{component,organization,project}
azure_devops_exporter_scrape_errors_total{component,organization,project}
azure_devops_exporter_last_successful_scrape_timestamp{component,organization,project}
```

`azure_devops_repo_pull_requests_total` is intentionally not exposed — sum the
three status series in a Prometheus query instead of duplicating the data.

`azure_devops_repo_commits_total` was originally deferred because a true
historical total requires a paginated walk over full commit history with no
server-side count. It's now implemented, but with different semantics than
the name suggests: it's a gauge recomputed every scrape as "commits pushed in
the last 24 hours" (via the Git API's `searchCriteria.fromDate` filter),
which bounds the query to a small, cheap page range regardless of the
repository's total history size — not an ever-growing counter, and not a
full historical count.

`pull_request_age_days` exposes one series per active pull request (age
since `creationDate`), keyed by `pull_request_id` for the same cardinality
reason as `azure_devops_boards_work_item_age_days`. `stale_pull_requests_total`
counts active pull requests older than the same 14-day `staleThreshold` used
by the Boards collector — for PRs this is based on creation date rather than
last-activity date, because the pull request list endpoint doesn't return a
cheap "last updated" timestamp (getting one would mean an extra API call per
PR, per scrape).

PR lead time (`closedDate` minus `creationDate`) only counts **completed**
(merged) pull requests — not abandoned ones — matching DORA's "Lead Time for
Changes". Like the Boards lead time metrics, it's exposed pre-aggregated
(avg/p50/p90/max per repository) rather than per-PR, using the same
nearest-rank percentile method.

`draft_pull_requests_total`, `pull_requests_with_conflicts_total`,
`pull_requests_without_reviewer_total` and `pull_requests_pending_approval_total`
only count **active** pull requests, using fields (`isDraft`, `mergeStatus`,
`reviewers`) already present in the same pull request list response — no
extra API calls. "Pending approval" means the PR has at least one reviewer
but none of them has cast an approval vote (`vote > 0`); a PR with zero
reviewers counts toward `without_reviewer_total` instead, not both.

`azure_devops_repo_size_bytes` is the `size` field Azure DevOps already
returns in the repository list response.

`commits_by_author_total` groups the same 24-hour commit window used by
`commits_total` by author name — useful for a rough "who's actually pushing"
view, but it's git author identity (whatever name/email is in the commit),
which can differ from the Azure DevOps user who opened the PR.

On a failed scrape, the previous values for that project are kept as-is (not
reset to zero) so dashboards don't show a false drop to zero during a
transient API error.

## Metrics (Boards)

```text
azure_devops_boards_work_items_total{organization,project}
azure_devops_boards_work_items_by_state{organization,project,work_item_type,state,area_path,iteration_path}
azure_devops_boards_work_items_by_assignee{organization,project,assigned_to}
azure_devops_boards_work_items_created_total{organization,project}
azure_devops_boards_work_items_closed_total{organization,project}
azure_devops_boards_work_item_age_days{organization,project,work_item_type,state,assigned_to,work_item_id}
azure_devops_boards_work_items_stale_total{organization,project,work_item_type,state}
azure_devops_boards_lead_time_avg_days{organization,project,work_item_type,area_path,iteration_path}
azure_devops_boards_lead_time_p50_days{organization,project,work_item_type,area_path,iteration_path}
azure_devops_boards_lead_time_p90_days{organization,project,work_item_type,area_path,iteration_path}
azure_devops_boards_lead_time_max_days{organization,project,work_item_type,area_path,iteration_path}
azure_devops_boards_work_items_by_priority{organization,project,work_item_type,priority}
azure_devops_boards_bugs_by_severity{organization,project,severity}
azure_devops_boards_work_items_without_estimate_total{organization,project,work_item_type,state}
azure_devops_boards_work_items_without_iteration_total{organization,project,work_item_type}
azure_devops_boards_work_items_without_area_path_total{organization,project,work_item_type}
azure_devops_boards_story_points_total{organization,project,work_item_type,state}
azure_devops_boards_effort_total{organization,project,work_item_type,state}
azure_devops_boards_active_sprint_work_items_total{organization,project,team,work_item_type,state}
azure_devops_boards_active_sprint_story_points_total{organization,project,team}
azure_devops_boards_team_capacity_hours_per_day{organization,project,team}
azure_devops_boards_sprint_velocity_story_points{organization,project,team,iteration}
azure_devops_boards_work_items_by_custom_field_total{organization,project,work_item_type,state,field,value}
```

`azure_devops_boards_work_items_by_type` is intentionally not exposed —
`work_item_type` is already a label on `work_items_by_state`, so sum over
`state`/`area_path`/`iteration_path` in a Prometheus query instead of
duplicating the data.

`area_path` and `iteration_path` add real cardinality (they grow with the
number of teams and sprints in the project) — if that becomes a problem,
drop them from `work_items_by_state` first.

`active_sprint_work_items_total` counts, per team, the work items in that
team's current sprint. Teams are auto-discovered via the Teams API — no
`AZURE_DEVOPS_TEAMS` config needed — and for each team the collector looks up
its current-timeframe iteration (Iterations API) and the work items assigned
to it (the iteration's dedicated work items endpoint, which reflects the
team's actual sprint backlog rather than a best-effort match against
`System.IterationPath`, since that path alone doesn't indicate which
iteration is *current* for a given team). Type/state for each item come from
the same work item fetch already done for the rest of this collector — no
extra per-item API calls. A team with no iteration marked current — the
common case for teams that don't run sprints — or a per-team API error just
contributes no series for that team, rather than failing the whole
`CollectBoards` call.

`active_sprint_story_points_total` sums the same current-sprint work items'
Story Points (falling back to Effort, same convention as `story_points_total`
above) into one number per team — meant to be read next to
`team_capacity_hours_per_day` as a rough load-vs-capacity signal, e.g.
`active_sprint_story_points_total / team_capacity_hours_per_day` in a
Grafana query, though the two aren't the same unit (points vs hours/day) so
that ratio is a relative signal, not a literal "hours needed."

`team_capacity_hours_per_day` sums every team member's configured
`capacityPerDay` (across all their activities) for the team's *current*
sprint, via the Capacity API. It's named `_hours_per_day` because that's what
most teams configure capacity in, but Azure DevOps doesn't enforce a unit —
if a team configures capacity in story points or another unit, the metric
just reflects whatever number they entered. It is **not** adjusted for team
members' days off or for how many working days are actually in the sprint —
both of those need the same capacities response's `daysOff` field plus the
iteration's date range, which this collector doesn't compute; treat this as
"nominal daily capacity," not a precise "total hours available this sprint."

`sprint_velocity_story_points` sums Story Points (same Effort fallback) for
work items in each of a team's last 5 past sprints (`velocitySprintCount` in
`internal/collectors/boards.go`), one series per iteration name — only items
whose *current* state is terminal count as "completed," so an item still
open past its sprint's end is excluded even though it's still associated
with that sprint. Past iterations come back from the Iterations API in
unspecified order, so the collector sorts them by `startDate` before taking
the most recent 5; a team with fewer than 5 past sprints just gets fewer
series, not zero-padded ones.

**API call cost.** Per scrape, on top of the rest of the Boards collector,
these four metrics together cost `1` (`ListTeams`, once) plus, per team:
`GetCurrentIteration` (1) + `ListTeamIterations` for past sprints (1),
plus **2 more** (`ListIterationWorkItemIDs` + `GetTeamIterationCapacity`) if
the team has a current sprint, plus **up to 5 more**
(`ListIterationWorkItemIDs`, one per past sprint used for velocity) if the
team has that much sprint history. Worst case that's `1 + 9` calls per team.
For a project with 10 teams that all run sprints and have a full 5-sprint
history, that's `1 + 9×10 = 91` extra calls every scrape — at the default
5-minute `SCRAPE_INTERVAL_SECONDS`, ~26,200 calls/day. A more typical mix
(8 of 10 teams have a current sprint, averaging 3 past sprints each) comes
out closer to `1 + 10×2 + 8×2 + 8×3 = 61` calls/scrape (~17,600/day). This is
added to, not multiplied with, the rest of the collector's cost: 3 WIQL
queries (all work items, created-since, closed-since) plus one
`workitemsbatch` call per 200 work items, regardless of team count. Unlike
the Releases collector's lead-time lookups, there's no cache here — sprint
membership and capacity can change between scrapes, so everything is
re-fetched every time. If a project has many teams and this becomes a
noticeable share of API budget, the cheapest mitigations are raising
`SCRAPE_INTERVAL_SECONDS` or lowering `velocitySprintCount`.

`created_total` and `closed_total` are gauges, not Prometheus counters,
despite the `_total` suffix from the original spec: the exporter keeps no
history, so on every scrape they're recomputed from scratch as "created" /
"closed since midnight yesterday", not an ever-growing count. The window is
implemented with WIQL's `@Today - 1` date macro rather than a literal date
string — WIQL's literal date parsing is locale-sensitive and rejected
standard ISO-8601 timestamps with an HTTP 400, so the macro (day-granularity,
evaluated server-side in the project's time zone) is what's actually
reliable. `closed_total` is also an approximation — Azure DevOps doesn't have
a universal "closed" flag, so it matches work items whose current state is
one of `Closed`, `Done`, `Resolved` or `Completed` (the terminal states used
by the built-in process templates) and that changed within the window. That
can miss custom process templates with different terminal state names, and
can double-count an item that was reopened and closed again within the
window.

`work_item_age_days` exposes one series per non-removed work item (age since
`System.CreatedDate`), using `work_item_id` instead of the item's title to
identify it — a title label would be high-cardinality free text, an ID is
bounded by `work_items_total`. `work_items_stale_total` counts, per type and
state, non-closed work items whose `System.ChangedDate` is more than 14 days
old (`staleThreshold` in `internal/collectors/boards.go`); this threshold is
a fixed constant, not configurable yet.

Lead time (`Microsoft.VSTS.Common.ClosedDate` minus `System.CreatedDate`) is
exposed pre-aggregated (avg/p50/p90/max per `work_item_type`/`area_path`/
`iteration_path`) instead of per work item, to avoid doubling the
cardinality `work_item_age_days` already adds. Only work items in a terminal
state (see `TerminalStateNames` above) with a non-empty `ClosedDate` are
included — `ClosedDate` isn't populated on every process template, so some
closed items are silently excluded from the aggregate rather than skewing it
with a zero. Percentiles use the nearest-rank method over all closed items
fetched in the current scrape (no external stats dependency); with very few
closed items in a bucket, p50/p90 can land on the same data point.
Cycle time (time from "work started" to close) isn't implemented — Azure
DevOps doesn't expose a "started" timestamp on the work item itself, only in
its revision history, which would mean one extra API call per work item per
scrape.

`work_items_by_priority` and `bugs_by_severity` use the
`Microsoft.VSTS.Common.Priority`/`Severity` fields already fetched with every
work item — no new API calls. Items with priority `0` (unset) are excluded
from `by_priority`; severity is only tallied for `Bug` work items, and items
without one set are excluded from `bugs_by_severity`.

`work_items_without_estimate_total` counts items with neither
`Microsoft.VSTS.Scheduling.StoryPoints` nor `...Effort` set. `story_points_total`
and `effort_total` sum whichever of those two fields each work item actually
has — which field a process template uses (Story Points, Effort, or neither)
varies, so treat these as informative rather than authoritative capacity
numbers, and expect one or both to be all-zero on a project that doesn't use
either field.

`work_items_without_iteration_total` and `..._without_area_path_total` count
items with a literally empty `System.IterationPath`/`AreaPath`. In practice
Azure DevOps defaults both fields to the project's root iteration/area when
nothing more specific is chosen, so these will usually read `0` — they only
catch work items created via API calls that explicitly left the field blank.

`work_items_by_custom_field_total` is only populated for fields listed in
`AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS` (see "Custom fields (Boards)" above) —
with nothing configured, this metric is simply never written. Unlike the
per-team metrics below, this costs **no extra API calls**: the configured
fields are appended to the same `workitemsbatch` request `GetWorkItems`
already makes for every other Boards metric, so the added cost is a few
extra bytes per request/response, not extra round trips. `field` is the
configured label (or the raw reference name if no label override was
given); `value` is the field's value, stringified as described above, with
unset values bucketed under `value="unset"`. A field with many distinct
values (a free-text field misused as a picklist, for instance) adds real
cardinality here, same caveat as `area_path`/`iteration_path` on
`work_items_by_state` — this is meant for genuinely low-cardinality
classification fields like "Platform," not free text.

## Metrics (Pipelines)

```text
azure_devops_pipelines_total{organization,project}
azure_devops_pipeline_runs_succeeded{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_runs_failed{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_runs_canceled{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_run_duration_seconds{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_last_run_timestamp{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_runs_in_progress{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_queue_time_seconds{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_run_duration_p50_seconds{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_run_duration_p90_seconds{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_run_duration_max_seconds{organization,project,pipeline,pipeline_id}
azure_devops_pipeline_runs_by_branch_total{organization,project,pipeline,pipeline_id,branch,result}
```

`azure_devops_pipeline_runs_total` is intentionally not exposed — sum
`runs_succeeded`/`runs_failed`/`runs_canceled` in a Prometheus query instead
of duplicating the data.

The run-count and duration metrics use a **24-hour rolling window**,
recomputed on every scrape (same "gauge, not counter" caveat as the Boards
`created_total`/`closed_total` metrics), because there's no cheap way to get
a true lifetime total from the Build API. Every known pipeline definition
gets an explicit `0` for these while idle, rather than being absent — the
metric only disappears if the pipeline definition itself is deleted.

`last_run_timestamp` is the one exception: it's fetched with a dedicated
`$top=1` lookup per pipeline definition (one extra API call each, on top of
the single windowed builds list), specifically so a pipeline idle for more
than 24 hours doesn't silently drop out of "when did this last run" — which
is the whole point of a staleness/last-run metric. This roughly doubles the
number of API calls this collector makes per scrape (bounded by the number
of pipelines, not by history size).

`runs_in_progress` only counts runs that were **queued within the same
24-hour window** and are still running — a run queued more than 24h ago and
still stuck in progress wouldn't be counted, since it falls outside the
`ListBuildsSince` fetch entirely. `queue_time_seconds` is the average wait
between `queueTime` and `startTime` for completed runs in the window.
`run_duration_p50/p90/max_seconds` are the same nearest-rank percentiles used
elsewhere in this project, computed from the same duration samples as the
existing (average) `run_duration_seconds`.

`runs_by_branch_total` adds `branch` (stripped of the `refs/heads/` prefix)
and `result` as labels on a separate metric, rather than adding `branch` to
the existing succeeded/failed/canceled gauges — folding branch into those
would have multiplied their cardinality by the branch count too. This metric
is still the highest-cardinality one in the project if CI runs on every
feature branch: cardinality is unbounded by pipeline count and grows with
how many distinct branches got a run in the last 24h. If that's a problem,
don't scrape this specific metric name, or relabel/drop it in Prometheus.

## Metrics (Releases)

```text
azure_devops_releases_total{organization,project}
azure_devops_release_deployments_succeeded{organization,project,release_definition,environment}
azure_devops_release_deployments_failed{organization,project,release_definition,environment}
azure_devops_release_deployment_duration_seconds{organization,project,release_definition,environment}
azure_devops_release_last_deployment_timestamp{organization,project,release_definition,environment}
azure_devops_release_deployments_not_deployed{organization,project,release_definition,environment}
azure_devops_release_lead_time_for_changes_avg_days{organization,project,release_definition,environment}
azure_devops_release_lead_time_for_changes_p50_days{organization,project,release_definition,environment}
azure_devops_release_lead_time_for_changes_p90_days{organization,project,release_definition,environment}
azure_devops_release_lead_time_for_changes_max_days{organization,project,release_definition,environment}
```

This collector targets **classic Release Management** (release definitions
with environments), a separate Azure DevOps feature from Pipelines/YAML
multi-stage — with its own API host, `vsrm.dev.azure.com` instead of
`dev.azure.com` (`NewClient` derives this automatically from
`AZURE_DEVOPS_API_URL`; on Azure DevOps Server / on-prem, where there's no
such host split, it falls back to the same host). If your organization has
fully moved to YAML multi-stage pipelines, this collector will report
`azure_devops_releases_total` as `0` and nothing else — that data lives in
Pipelines/Builds instead, not in the Release API.

Unlike Pipelines' 24-hour window, this collector uses a **30-day window**:
classic releases typically deploy far less often than CI builds run, so a
24-hour window would show near-zero activity for most teams. Both the
deployment counts and `last_deployment_timestamp` come from this same
30-day fetch — there's no dedicated per-environment "true last deployment"
lookup like Pipelines has for `last_run_timestamp`, so an environment that
hasn't deployed in over 30 days will simply disappear from
`last_deployment_timestamp` rather than showing a stale value. Widen
`releaseWindow` in `internal/collectors/releases.go` if that's a problem for
your team's release cadence.

`deployments_not_deployed` counts deployments with `deploymentStatus ==
"notDeployed"` in the same 30-day window. Despite what the name suggests,
this is **not** "pending manual approval" — Azure DevOps uses `notDeployed`
for deployments that were skipped or never triggered (e.g. unmet conditions),
not specifically ones sitting in front of a release gate. A dedicated
"pending approval" signal would need the separate Release Approvals API,
which this collector doesn't call.

### Lead Time for Changes

`lead_time_for_changes_{avg,p50,p90,max}_days` is DORA's actual "commit to
production" Lead Time for Changes — different from (and more expensive than)
the PR-based `azure_devops_repo_pr_lead_time_*` metrics, which only measure
PR creation to merge. For each **successful** deployment in the 30-day
window, the collector:

1. Fetches the parent Release (`GetRelease`) to find its `Build`-type
   artifact and the build ID that produced it.
2. Fetches that build's changes (`GetBuildChanges`) — the commits included in
   it, each with a timestamp, relative to the previous build on the branch.
3. For every one of those commits, records `deployment.completedOn -
   commit.timestamp` as one lead time sample, aggregated the same way as
   every other lead time metric in this project (nearest-rank percentiles,
   avg/p50/p90/max per `release_definition`/`environment`).

**Cost and the reason for the cache.** Steps 1–2 are two extra API calls
*per deployment*, and unlike run/deployment counts, this isn't optional
aggregation — there's no way to bulk-resolve it. Naively doing this on every
scrape would mean re-fetching the same immutable data for deployments from
weeks ago, over and over, every `SCRAPE_INTERVAL_SECONDS`. A project with 10
release definitions × 3 environments deploying weekly (~120 deployments in a
30-day window) would cost ~240 extra calls *per scrape* — at the default
5-minute interval, on the order of tens of thousands of calls a day, almost
all of them redundant. To avoid that, resolved commit timestamps are cached
in memory per release ID (`releaseChangesCache` in
`internal/collectors/releases.go`) — a release's artifacts never change
once created, so each release is resolved at most once, no matter how many
scrapes see its deployment. The cache is swept of entries older than the
30-day release window on every `CollectReleases` call, since a deployment
that's aged out of the window won't be looked up again; it is **not**
persisted across process restarts (in-memory only — a restart re-pays the
resolution cost for whatever's still in the 30-day window at that point).

**What's silently excluded, not errored on:** releases with no `Build`
artifact (container-image artifacts, manually-triggered releases with
nothing attached) contribute no lead time samples. So does a deployment
where `GetRelease` or `GetBuildChanges` fails for any reason (permissions,
a deleted release, a transient API error) — this metric is deliberately
tolerant of per-deployment failures rather than aborting the whole
`CollectReleases` call over what's already an approximation layered on top
of the core (and always-available) deployment counts above.

## Running locally

Requires Go 1.22+.

```bash
export AZURE_DEVOPS_ORGANIZATION=my-org
export AZURE_DEVOPS_PROJECTS=proj-a,proj-b
export AZURE_DEVOPS_TOKEN=xxxxxxxxxxxxxxxx

go mod tidy
go test ./...
go run .
```

## Running with Docker

```bash
docker build -t azure-devops-exporter .
docker run --rm -p 8080:8080 \
  -e AZURE_DEVOPS_ORGANIZATION="my-org" \
  -e AZURE_DEVOPS_PROJECTS="proj-a,proj-b" \
  -e AZURE_DEVOPS_TOKEN="my-token" \
  azure-devops-exporter
```

Or with `docker-compose` (reads the same variables from your shell or a `.env` file):

```bash
docker compose up --build
```

## Container image

`.github/workflows/docker-publish.yml` builds and publishes the image to
GitHub Container Registry on every push to `main` (tag `latest`) and on
`vX.Y.Z` git tags (semver tags: `1.2.3`, `1.2`, `1`) — no separate registry
account or secret needed, it authenticates with the repo's own
`GITHUB_TOKEN`. Both `manifests/deployment.yaml` and the Helm chart's
`values.yaml` default to `ghcr.io/robertasolimandonofreo/azure-devops-exporter`.

The first image a repo publishes to GHCR is **private by default** — go to
the package's settings on GitHub (org → Packages → azure-devops-exporter →
Package settings → Danger Zone) and change visibility to public, or link it
to this repository so it inherits the repo's visibility, otherwise
`kubectl`/Helm will fail to pull it with `ImagePullBackOff` from outside the
org. If you're deploying from a fork or want your own registry instead,
override `image.repository`/`image.tag` (Helm) or edit the `image:` line
directly (raw manifests) and build/push it yourself:

```bash
docker build -t <your-registry>/azure-devops-exporter:latest .
docker push <your-registry>/azure-devops-exporter:latest
```

## Running in Kubernetes

```bash
kubectl apply -f manifests/secret.yaml
kubectl apply -f manifests/configmap.yaml
kubectl apply -f manifests/deployment.yaml
kubectl apply -f manifests/service.yaml
kubectl apply -f manifests/servicemonitor.yaml   # requires Prometheus Operator
```

Edit `manifests/secret.yaml` and `manifests/configmap.yaml` with your
organization, projects and token before applying.

### With Helm

`charts/azure-devops-exporter` is equivalent to the raw manifests above,
parameterized via `values.yaml`:

```bash
helm install azure-devops-exporter charts/azure-devops-exporter \
  --set azureDevOps.organization=my-org \
  --set azureDevOps.projects=proj-a\,proj-b \
  --set azureDevOps.token=my-token
```

For anything beyond a quick smoke test, a values file is easier to manage
than a wall of `--set` flags — `charts/azure-devops-exporter/values-edp.example.yaml`
is a real filled-in example (organization, per-project collectors, custom
fields) to copy from:

```bash
cp charts/azure-devops-exporter/values-edp.example.yaml charts/azure-devops-exporter/values-mine.yaml
# edit values-mine.yaml, then:
helm install azure-devops-exporter charts/azure-devops-exporter -f charts/azure-devops-exporter/values-mine.yaml
```

`values-*.yaml` under the chart directory is gitignored (except
`*.example.yaml` files) so an environment-specific copy doesn't get
committed by accident.

To use a token you already store in a Secret instead of letting the chart
create one, set `azureDevOps.existingSecret=<name>` (that Secret must have an
`AZURE_DEVOPS_TOKEN` key) — `azureDevOps.token` is ignored in that case.

`serviceMonitor.enabled` defaults to `true` and requires the Prometheus
Operator CRDs; set it to `false` (`--set serviceMonitor.enabled=false`) if
you don't have the operator installed and are using the static scrape config
below instead. See `values.yaml` for the rest of the knobs (image,
resources, `nodeSelector`/`tolerations`/`affinity`, etc.).

## Prometheus scrape config (without the Operator)

```yaml
scrape_configs:
  - job_name: azure-devops-exporter
    metrics_path: /metrics
    static_configs:
      - targets:
          - azure-devops-exporter:8080
```

## Alerts

`alerts/prometheus-rules.yaml` is a `PrometheusRule` (requires Prometheus
Operator — `kubectl apply -f alerts/prometheus-rules.yaml`), with 10 rules
covering exporter health, Pipelines, Releases, Repos and Boards. Only
`AzureDevOpsExporterScrapeFailing` uses `increase()`, because
`azure_devops_exporter_scrape_errors_total` is the one metric here that's a
real Prometheus counter; every other alert compares a gauge directly
(`> 0`), since the rest of this exporter's metrics are recomputed snapshots
per scrape, not monotonic counters — see the "Metrics" sections above for
which window each one uses. Two rules are worth knowing the limits of before
you rely on them: `AzureDevOpsPipelineRunStuck` only catches runs queued
within the last 24 hours (the same window `runs_in_progress` itself is
bound by), and `AzureDevOpsCriticalBugsOpen` matches severity by regex
(`severity=~"1.*"`) against whatever string your process template actually
uses — check it matches your org's naming before trusting it. This set
isn't exhaustive — it's the alerts that are unambiguously actionable at a
fixed threshold; plenty of the metrics below are better suited to a
dashboard panel than a default alert (e.g. `deployments_not_deployed` is
often expected behavior, not a problem, so it isn't alerted on).

## Grafana dashboard

`dashboards/grafana-dashboard.json` covers all four collectors (49 panels)
plus a DORA overview row: Deployment Frequency and Change Failure Rate for
both Pipelines and Releases, and Lead Time for Changes computed the DORA
way — commit to production (`azure_devops_release_lead_time_for_changes_avg_days`),
not the cheaper PR-creation-to-merge proxy. That PR-based number is still
available, just relocated to its own panel in the Repos row, since it's a
useful signal on its own (how long PRs take to merge) even though it isn't
DORA's Lead Time for Changes. Time to Restore Service is intentionally left
unimplemented, with a panel explaining why: Azure DevOps pipelines/releases
have no first-class "incident" concept to compute it from, and
approximating it from failed→succeeded run gaps was judged too unreliable
to ship.

Also included: draft/conflicting/reviewer-less PR counts and repo size in
the Repos row, in-progress runs and queue time in Pipelines (with a text
panel explaining why `runs_by_branch_total` — the highest-cardinality
metric in this exporter — is deliberately *not* graphed by default),
not-deployed counts and the full lead-time-for-changes percentile spread in
Releases, and priority/severity/story-points breakdowns plus per-team active
sprint work items, sprint load vs. capacity, and sprint velocity in Boards.

Import it via Grafana's dashboard import (JSON upload or paste); it prompts
for a Prometheus datasource on import (`DS_PROMETHEUS` input) and exposes
`organization`/`project` template variables for filtering. Panels reading
`_last_run_timestamp` / `_last_deployment_timestamp` / lead-time-for-changes
will simply omit a series when there's no data within that metric's window
(24h / 30d) — that's the same "absence means no recent data" behavior
documented for those metrics above, not a dashboard bug.
