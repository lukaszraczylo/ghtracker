// Package metrics exposes dashboard state as Prometheus metrics, computed at scrape time
// so age-based series keep advancing between refreshes.
package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/lukaszraczylo/ghtracker/internal/collector"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

const ns = "ghtracker"

// Source supplies the state to export.
type Source interface {
	View() collector.View
	Stats() collector.Stats
}

// RateLimit reports the remaining GitHub request budget; ok is false before the first response.
type RateLimit func() (remaining int, ok bool)

type Collector struct {
	src  Source
	rate RateLimit
	d    map[string]*prometheus.Desc
}

var (
	repoLabels     = []string{"repo"}
	prLabels       = []string{"repo", "number"}
	workflowLabels = []string{"repo", "workflow", "background"}
)

var descs = []struct {
	key, help string
	labels    []string
}{
	{"repo_up", "1 if the last refresh of the repo succeeded.", repoLabels},
	{"repo_health", "Worst alert severity for the repo: 0 ok, 1 warn, 2 crit.", repoLabels},
	{"repo_last_refresh_timestamp_seconds", "Unix time of the last successful refresh of the repo.", repoLabels},
	{"repo_open_issues", "Open issues, excluding pull requests.", repoLabels},
	{"repo_open_prs", "Open pull requests, including drafts.", repoLabels},
	{"repo_open_prs_failing_checks", "Open non-draft pull requests whose checks fail.", repoLabels},
	{"repo_oldest_pr_age_seconds", "Age of the oldest open non-draft pull request; 0 if none.", repoLabels},
	{"repo_stale_issues", "Open issues without activity for longer than thresholds.issue_stale_after.", repoLabels},
	{"pr_age_seconds", "Age of an open non-draft pull request.", prLabels},
	{"pr_checks_failing", "1 if the pull request has failing checks.", prLabels},
	{"workflow_failing", "1 if the newest completed default-branch run of the workflow failed.", workflowLabels},
	{"workflow_running", "1 if the workflow has a run in progress or queued.", workflowLabels},
	{"workflow_last_run_timestamp_seconds", "Unix time the newest default-branch run was created.", workflowLabels},
	{"release_info", "Latest release or tag; value is always 1.", []string{"repo", "version", "url"}},
	{"release_published_timestamp_seconds", "Unix time the latest release was published.", repoLabels},
	{"alerts", "Active alerts by severity.", []string{"severity"}},
	{"last_refresh_timestamp_seconds", "Unix time the last refresh started.", nil},
	{"last_refresh_duration_seconds", "Duration of the last refresh.", nil},
	{"refreshes_total", "Completed refresh cycles.", nil},
	{"repo_refresh_errors_total", "Repo refresh failures.", nil},
	{"github_rate_limit_remaining", "Remaining GitHub API requests in the current window.", nil},
}

func New(src Source, rate RateLimit) *Collector {
	c := &Collector{src: src, rate: rate, d: make(map[string]*prometheus.Desc, len(descs))}
	for _, d := range descs {
		c.d[d.key] = prometheus.NewDesc(prometheus.BuildFQName(ns, "", d.key), d.help, d.labels, nil)
	}
	return c
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.d {
		ch <- d
	}
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	g := func(key string, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(c.d[key], prometheus.GaugeValue, v, labels...)
	}
	ctr := func(key string, v float64) {
		ch <- prometheus.MustNewConstMetric(c.d[key], prometheus.CounterValue, v)
	}

	v, st := c.src.View(), c.src.Stats()
	for _, r := range v.Repos {
		c.collectRepo(g, r, v)
	}
	for _, sev := range []model.Severity{model.Crit, model.Warn} {
		n := v.Crit
		if sev == model.Warn {
			n = v.Warn
		}
		g("alerts", float64(n), sev.String())
	}

	g("last_refresh_timestamp_seconds", unix(st.LastRefresh))
	g("last_refresh_duration_seconds", st.LastDuration.Seconds())
	ctr("refreshes_total", float64(st.RefreshTotal))
	ctr("repo_refresh_errors_total", float64(st.RepoErrorsTotal))
	if c.rate != nil {
		if rem, ok := c.rate(); ok {
			g("github_rate_limit_remaining", float64(rem))
		}
	}
}

func (c *Collector) collectRepo(g func(string, float64, ...string), r model.Repo, v collector.View) {
	g("repo_up", b2f(r.Up), r.FullName)
	g("repo_health", float64(r.Health), r.FullName)
	g("repo_last_refresh_timestamp_seconds", unix(r.RefreshedAt), r.FullName)
	g("repo_open_issues", float64(len(r.Issues)), r.FullName)
	g("repo_open_prs", float64(len(r.PRs)), r.FullName)

	var failing int
	var oldest time.Duration
	for _, pr := range r.PRs {
		if pr.Draft {
			continue
		}
		age := v.Now.Sub(pr.CreatedAt)
		oldest = max(oldest, age)
		num := strconv.Itoa(pr.Number)
		g("pr_age_seconds", age.Seconds(), r.FullName, num)
		g("pr_checks_failing", b2f(pr.Checks.State == model.ChecksFail), r.FullName, num)
		if pr.Checks.State == model.ChecksFail {
			failing++
		}
	}
	g("repo_open_prs_failing_checks", float64(failing), r.FullName)
	g("repo_oldest_pr_age_seconds", oldest.Seconds(), r.FullName)

	var stale int
	if v.Thresholds.IssueStaleAfter > 0 {
		for _, is := range r.Issues {
			if v.Now.Sub(is.UpdatedAt) >= v.Thresholds.IssueStaleAfter {
				stale++
			}
		}
	}
	g("repo_stale_issues", float64(stale), r.FullName)

	seen := map[string]bool{}
	for _, wf := range r.Workflows {
		// Two workflows may share a name; duplicate label sets would fail the whole scrape.
		if seen[wf.Name] {
			continue
		}
		seen[wf.Name] = true
		bg := strconv.FormatBool(wf.Background)
		g("workflow_failing", b2f(wf.LastDone != nil && wf.LastDone.Failed() && !collector.Dormant(wf, v.Now, v.Thresholds)), r.FullName, wf.Name, bg)
		g("workflow_running", b2f(wf.Latest.Running()), r.FullName, wf.Name, bg)
		g("workflow_last_run_timestamp_seconds", unix(wf.Latest.CreatedAt), r.FullName, wf.Name, bg)
	}

	if rel := r.Release; rel != nil {
		g("release_info", 1, r.FullName, rel.Tag, rel.URL)
		if !rel.PublishedAt.IsZero() {
			g("release_published_timestamp_seconds", unix(rel.PublishedAt), r.FullName)
		}
	}
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func unix(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return float64(t.Unix())
}
