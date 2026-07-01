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
| `AZURE_DEVOPS_PROJECTS` | Comma-separated list of project names | Yes | - |
| `AZURE_DEVOPS_TOKEN` | Personal Access Token (needs Code: Read, Work Items: Read, Build: Read, Release: Read) | Yes | - |
| `AZURE_DEVOPS_API_URL` | Azure DevOps API base URL | No | `https://dev.azure.com` |
| `EXPORTER_PORT` | HTTP port | No | `8080` |
| `SCRAPE_INTERVAL_SECONDS` | Seconds between scrape cycles | No | `300` |
| `LOG_LEVEL` | `debug`, `info`, `warn`, or `error` | No | `info` |

The token is never logged; it is only sent in the `Authorization` header.

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
```

`azure_devops_boards_work_items_by_type` is intentionally not exposed —
`work_item_type` is already a label on `work_items_by_state`, so sum over
`state`/`area_path`/`iteration_path` in a Prometheus query instead of
duplicating the data.

`area_path` and `iteration_path` add real cardinality (they grow with the
number of teams and sprints in the project) — if that becomes a problem,
drop them from `work_items_by_state` first.

`azure_devops_boards_active_sprint_work_items_total` (per team/sprint) is
deferred: it requires the Teams and Iterations APIs and a new
`AZURE_DEVOPS_TEAMS`-style config surface, which is a bigger scope decision
than the rest of this collector.

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
Operator — `kubectl apply -f alerts/prometheus-rules.yaml`). Only
`AzureDevOpsExporterScrapeFailing` uses `increase()`, because
`azure_devops_exporter_scrape_errors_total` is the one metric here that's a
real Prometheus counter; every other alert compares a gauge directly
(`> 0`), since the rest of this exporter's metrics are recomputed snapshots
per scrape, not monotonic counters — see the "Metrics" sections above for
which window each one uses.

## Grafana dashboard

`dashboards/grafana-dashboard.json` covers all four collectors plus a DORA
overview row (Deployment Frequency and Change Failure Rate for both
Pipelines and Releases, Lead Time for Changes from PR and work item lead
time). Time to Restore Service is intentionally left unimplemented — Azure
DevOps pipelines/releases have no first-class "incident" concept to compute
it from, and approximating it from failed→succeeded run gaps was judged too
unreliable to ship.

Import it via Grafana's dashboard import (JSON upload or paste); it prompts
for a Prometheus datasource on import (`DS_PROMETHEUS` input) and exposes
`organization`/`project` template variables for filtering. Panels reading
`_last_run_timestamp` / `_last_deployment_timestamp` will simply omit a
series when a pipeline or environment has had no activity within that
metric's window (24h / 30d) — that's the same "absence means no recent
data" behavior documented for those metrics above, not a dashboard bug.
