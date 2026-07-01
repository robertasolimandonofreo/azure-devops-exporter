// Package collectors fetches data from Azure DevOps and updates Prometheus metrics.
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

type repoStats struct {
	name               string
	size               int64
	active             int
	completed          int
	abandoned          int
	branches           int
	commitsRecent      int
	stalePRs           int
	draftPRs           int
	conflictPRs        int
	noReviewerPRs      int
	pendingApprovalPRs int
	leadTimeDays       []float64 // merged (completed) PRs only
	activePRAges       []prAge
	commitsByAuthor    map[string]int
}

type prAge struct {
	id  int
	age float64
}

// CollectRepos fetches repository, pull request and branch data for a project
// and updates the corresponding metrics. On error, previously collected metrics
// for this project are left untouched.
func CollectRepos(client *azuredevops.Client, organization, project string) error {
	repos, err := client.ListRepositories(project)
	if err != nil {
		return fmt.Errorf("list repositories: %w", err)
	}
	metrics.ReposTotal.WithLabelValues(organization, project).Set(float64(len(repos)))

	stats := make([]repoStats, 0, len(repos))
	for _, repo := range repos {
		s, err := collectRepoStats(client, project, repo)
		if err != nil {
			return fmt.Errorf("repository %s: %w", repo.Name, err)
		}
		stats = append(stats, s)
	}

	// Clear stale per-repository series (e.g. deleted repositories) before writing
	// fresh values, now that every repository was fetched successfully.
	labelFilter := prometheus.Labels{"organization": organization, "project": project}
	metrics.PullRequestsActive.DeletePartialMatch(labelFilter)
	metrics.PullRequestsCompleted.DeletePartialMatch(labelFilter)
	metrics.PullRequestsAbandoned.DeletePartialMatch(labelFilter)
	metrics.BranchesTotal.DeletePartialMatch(labelFilter)
	metrics.CommitsTotal.DeletePartialMatch(labelFilter)
	metrics.PullRequestAgeDays.DeletePartialMatch(labelFilter)
	metrics.StalePullRequestsTotal.DeletePartialMatch(labelFilter)
	metrics.PRLeadTimeAvgDays.DeletePartialMatch(labelFilter)
	metrics.PRLeadTimeP50Days.DeletePartialMatch(labelFilter)
	metrics.PRLeadTimeP90Days.DeletePartialMatch(labelFilter)
	metrics.PRLeadTimeMaxDays.DeletePartialMatch(labelFilter)
	metrics.DraftPullRequestsTotal.DeletePartialMatch(labelFilter)
	metrics.PullRequestsWithConflictsTotal.DeletePartialMatch(labelFilter)
	metrics.PullRequestsWithoutReviewerTotal.DeletePartialMatch(labelFilter)
	metrics.PullRequestsPendingApprovalTotal.DeletePartialMatch(labelFilter)
	metrics.RepoSizeBytes.DeletePartialMatch(labelFilter)
	metrics.CommitsByAuthorTotal.DeletePartialMatch(labelFilter)

	for _, s := range stats {
		metrics.PullRequestsActive.WithLabelValues(organization, project, s.name).Set(float64(s.active))
		metrics.PullRequestsCompleted.WithLabelValues(organization, project, s.name).Set(float64(s.completed))
		metrics.PullRequestsAbandoned.WithLabelValues(organization, project, s.name).Set(float64(s.abandoned))
		metrics.BranchesTotal.WithLabelValues(organization, project, s.name).Set(float64(s.branches))
		metrics.CommitsTotal.WithLabelValues(organization, project, s.name).Set(float64(s.commitsRecent))
		metrics.StalePullRequestsTotal.WithLabelValues(organization, project, s.name).Set(float64(s.stalePRs))
		metrics.DraftPullRequestsTotal.WithLabelValues(organization, project, s.name).Set(float64(s.draftPRs))
		metrics.PullRequestsWithConflictsTotal.WithLabelValues(organization, project, s.name).Set(float64(s.conflictPRs))
		metrics.PullRequestsWithoutReviewerTotal.WithLabelValues(organization, project, s.name).Set(float64(s.noReviewerPRs))
		metrics.PullRequestsPendingApprovalTotal.WithLabelValues(organization, project, s.name).Set(float64(s.pendingApprovalPRs))
		metrics.RepoSizeBytes.WithLabelValues(organization, project, s.name).Set(float64(s.size))

		for _, a := range s.activePRAges {
			metrics.PullRequestAgeDays.WithLabelValues(organization, project, s.name, strconv.Itoa(a.id)).Set(a.age)
		}

		for author, count := range s.commitsByAuthor {
			metrics.CommitsByAuthorTotal.WithLabelValues(organization, project, s.name, author).Set(float64(count))
		}

		if len(s.leadTimeDays) > 0 {
			sort.Float64s(s.leadTimeDays)
			metrics.PRLeadTimeAvgDays.WithLabelValues(organization, project, s.name).Set(average(s.leadTimeDays))
			metrics.PRLeadTimeP50Days.WithLabelValues(organization, project, s.name).Set(percentile(s.leadTimeDays, 0.5))
			metrics.PRLeadTimeP90Days.WithLabelValues(organization, project, s.name).Set(percentile(s.leadTimeDays, 0.9))
			metrics.PRLeadTimeMaxDays.WithLabelValues(organization, project, s.name).Set(s.leadTimeDays[len(s.leadTimeDays)-1])
		}
	}
	return nil
}

func collectRepoStats(client *azuredevops.Client, project string, repo azuredevops.Repository) (repoStats, error) {
	s := repoStats{name: repo.Name, size: repo.Size}

	prs, err := client.ListPullRequests(project, repo.ID)
	if err != nil {
		return s, fmt.Errorf("pull requests: %w", err)
	}
	now := time.Now()
	for _, pr := range prs {
		switch pr.Status {
		case "active":
			s.active++
			age := now.Sub(pr.CreationDate).Hours() / 24
			s.activePRAges = append(s.activePRAges, prAge{id: pr.PullRequestID, age: age})
			if now.Sub(pr.CreationDate) > staleThreshold {
				s.stalePRs++
			}
			if pr.IsDraft {
				s.draftPRs++
			}
			if pr.MergeStatus == "conflicts" {
				s.conflictPRs++
			}
			if len(pr.Reviewers) == 0 {
				s.noReviewerPRs++
			} else if !anyApproved(pr.Reviewers) {
				s.pendingApprovalPRs++
			}
		case "completed":
			s.completed++
			if !pr.ClosedDate.IsZero() && !pr.CreationDate.IsZero() {
				s.leadTimeDays = append(s.leadTimeDays, pr.ClosedDate.Sub(pr.CreationDate).Hours()/24)
			}
		case "abandoned":
			s.abandoned++
		}
	}

	branches, err := client.CountBranches(project, repo.ID)
	if err != nil {
		return s, fmt.Errorf("branches: %w", err)
	}
	s.branches = branches

	commits, err := client.ListCommitsSince(project, repo.ID, now.Add(-24*time.Hour))
	if err != nil {
		return s, fmt.Errorf("commits: %w", err)
	}
	s.commitsRecent = len(commits)
	s.commitsByAuthor = make(map[string]int)
	for _, c := range commits {
		author := c.Author.Name
		if author == "" {
			author = "unknown"
		}
		s.commitsByAuthor[author]++
	}

	return s, nil
}

func anyApproved(reviewers []azuredevops.Reviewer) bool {
	for _, r := range reviewers {
		if r.Vote > 0 {
			return true
		}
	}
	return false
}
