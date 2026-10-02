package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/lukaszraczylo/ghtracker/internal/collector"
	"github.com/lukaszraczylo/ghtracker/internal/config"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

var now = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

type fakeSource struct {
	st collector.Stats
	v  collector.View
}

func (f fakeSource) View() collector.View   { return f.v }
func (f fakeSource) Stats() collector.Stats { return f.st }

func source() fakeSource {
	failed := &model.Run{Status: "completed", Conclusion: "failure"}
	repo := model.Repo{
		FullName: "o/r", Up: true, Health: model.Crit, RefreshedAt: now.Add(-time.Hour),
		Issues: []model.Issue{{UpdatedAt: now.Add(-40 * 24 * time.Hour)}, {UpdatedAt: now}},
		PRs: []model.PullRequest{
			{Number: 7, CreatedAt: now.Add(-50 * time.Hour), Checks: model.CheckSummary{State: model.ChecksFail}},
			{Number: 8, Draft: true, CreatedAt: now.Add(-900 * time.Hour)},
		},
		Workflows: []model.Workflow{
			{Name: "CI", Latest: *failed, LastDone: failed},
			{Name: "CI", Latest: *failed, LastDone: failed}, // duplicate name must not break the scrape
			{Name: "Nightly", Background: true, Latest: model.Run{Status: "in_progress", CreatedAt: now}},
		},
		Release: &model.Release{Tag: "v1.0.0", URL: "https://x/rel", PublishedAt: now.Add(-24 * time.Hour)},
	}
	return fakeSource{
		v: collector.View{Now: now, Repos: []model.Repo{repo}, Crit: 1, Warn: 2,
			Thresholds: config.Thresholds{IssueStaleAfter: 30 * 24 * time.Hour}},
		st: collector.Stats{LastRefresh: now.Add(-time.Hour), LastDuration: 3 * time.Second, RefreshTotal: 4, RepoErrorsTotal: 1, Loaded: true},
	}
}

func TestScrapeValuesAndLint(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(New(source(), func() (int, bool) { return 4321, true }))
	want := `
# HELP ghtracker_repo_up 1 if the last refresh of the repo succeeded.
# TYPE ghtracker_repo_up gauge
ghtracker_repo_up{repo="o/r"} 1
# HELP ghtracker_repo_open_prs_failing_checks Open non-draft pull requests whose checks fail.
# TYPE ghtracker_repo_open_prs_failing_checks gauge
ghtracker_repo_open_prs_failing_checks{repo="o/r"} 1
# HELP ghtracker_repo_oldest_pr_age_seconds Age of the oldest open non-draft pull request; 0 if none.
# TYPE ghtracker_repo_oldest_pr_age_seconds gauge
ghtracker_repo_oldest_pr_age_seconds{repo="o/r"} 180000
# HELP ghtracker_repo_stale_issues Open issues without activity for longer than thresholds.issue_stale_after.
# TYPE ghtracker_repo_stale_issues gauge
ghtracker_repo_stale_issues{repo="o/r"} 1
# HELP ghtracker_github_rate_limit_remaining Remaining GitHub API requests in the current window.
# TYPE ghtracker_github_rate_limit_remaining gauge
ghtracker_github_rate_limit_remaining 4321
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"ghtracker_repo_up", "ghtracker_repo_open_prs_failing_checks", "ghtracker_repo_oldest_pr_age_seconds",
		"ghtracker_repo_stale_issues", "ghtracker_github_rate_limit_remaining"); err != nil {
		t.Fatal(err)
	}
	if probs, err := testutil.GatherAndLint(reg); err != nil || len(probs) != 0 {
		t.Fatalf("lint: %v %v", probs, err)
	}
}

func TestDraftPRsExcludedFromAgeSeries(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(New(source(), nil))
	n, err := testutil.GatherAndCount(reg, "ghtracker_pr_age_seconds")
	if err != nil || n != 1 {
		t.Fatalf("pr_age series = %d err=%v, want 1 (draft excluded)", n, err)
	}
}

func TestWorkflowSeriesDedupedAndLabelled(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(New(source(), nil))
	want := `
# HELP ghtracker_workflow_failing 1 if the newest completed default-branch run of the workflow failed.
# TYPE ghtracker_workflow_failing gauge
ghtracker_workflow_failing{background="false",repo="o/r",workflow="CI"} 1
ghtracker_workflow_failing{background="true",repo="o/r",workflow="Nightly"} 0
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "ghtracker_workflow_failing"); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseAndCounters(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(New(source(), nil))
	want := `
# HELP ghtracker_release_info Latest release or tag; value is always 1.
# TYPE ghtracker_release_info gauge
ghtracker_release_info{repo="o/r",url="https://x/rel",version="v1.0.0"} 1
# HELP ghtracker_refreshes_total Completed refresh cycles.
# TYPE ghtracker_refreshes_total counter
ghtracker_refreshes_total 4
# HELP ghtracker_repo_refresh_errors_total Repo refresh failures.
# TYPE ghtracker_repo_refresh_errors_total counter
ghtracker_repo_refresh_errors_total 1
# HELP ghtracker_alerts Active alerts by severity.
# TYPE ghtracker_alerts gauge
ghtracker_alerts{severity="crit"} 1
ghtracker_alerts{severity="warn"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"ghtracker_release_info", "ghtracker_refreshes_total", "ghtracker_repo_refresh_errors_total", "ghtracker_alerts"); err != nil {
		t.Fatal(err)
	}
}

func TestRateLimitOmittedWhenUnknown(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(New(source(), func() (int, bool) { return 0, false }))
	n, err := testutil.GatherAndCount(reg, "ghtracker_github_rate_limit_remaining")
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
