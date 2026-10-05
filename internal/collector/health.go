package collector

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/config"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

// Alert kinds; they double as stable identifiers for dashboards.
const (
	KindRefreshFailed = "refresh_failed"
	KindDataStale     = "data_stale"
	KindPartialData   = "partial_data"
	KindWorkflowFail  = "workflow_failed"
	KindWorkflowStuck = "workflow_stuck"
	KindPRWaiting     = "pr_waiting"
	KindPRChecks      = "pr_checks_failing"
	KindIssuesStale   = "issues_stale"
	// KindPROpen is informational plumbing for automatic actions; it has OK severity so it never moves health or counts.
	KindPROpen = "pr_open"
)

// Evaluate derives alerts for one repo at time now and stores the worst severity in r.Health.
// It runs on every read so age-based alerts advance between refreshes.
func Evaluate(r *model.Repo, now time.Time, th config.Thresholds) []model.Alert {
	var alerts []model.Alert
	add := func(sev model.Severity, kind, subject, detail, url string, since time.Time) {
		alerts = append(alerts, model.Alert{Severity: sev, Repo: r.FullName, Kind: kind, Subject: subject, Detail: detail, URL: url, Since: since})
		r.Health = max(r.Health, sev)
	}
	r.Health = model.OK

	if r.Err != "" {
		add(model.Crit, KindRefreshFailed, "Refresh failed", r.Err, r.URL, r.RefreshedAt)
	}
	if th.DataStaleAfter > 0 && !r.RefreshedAt.IsZero() && now.Sub(r.RefreshedAt) > th.DataStaleAfter {
		add(model.Warn, KindDataStale, "Data is stale", "last refreshed "+Age(now.Sub(r.RefreshedAt))+" ago", r.URL, r.RefreshedAt)
	}
	for _, detail := range summarizeWarnings(r.Warnings) {
		add(model.Warn, KindPartialData, "Incomplete data", detail, r.URL, r.RefreshedAt)
	}
	if r.Archived {
		return alerts
	}

	for _, wf := range r.Workflows {
		if wf.LastDone != nil && wf.LastDone.Failed() && !Dormant(wf, now, th) {
			add(model.Crit, KindWorkflowFail, workflowSubject(wf), fmt.Sprintf("%s on %s (%s)", strings.ReplaceAll(wf.LastDone.Conclusion, "_", " "), wf.LastDone.Branch, wf.LastDone.Event), wf.LastDone.URL, wf.LastDone.CreatedAt)
		}
		if wf.Latest.Running() && th.WorkflowStuck > 0 && now.Sub(wf.Latest.CreatedAt) > th.WorkflowStuck {
			add(model.Warn, KindWorkflowStuck, workflowSubject(wf),
				fmt.Sprintf("%s for %s", strings.ReplaceAll(wf.Latest.Status, "_", " "), Age(now.Sub(wf.Latest.CreatedAt))),
				wf.Latest.URL, wf.Latest.CreatedAt)
		}
	}

	for _, pr := range r.PRs {
		if pr.Draft {
			continue
		}
		age := now.Sub(pr.CreatedAt)
		subject := fmt.Sprintf("#%d %s", pr.Number, pr.Title)
		alerts = append(alerts, model.Alert{Severity: model.OK, Repo: r.FullName, Kind: KindPROpen, Subject: subject,
			Detail: "open for " + Age(age), URL: pr.URL, Ref: pr.HeadSHA, Author: pr.Author, Since: pr.CreatedAt})
		switch {
		case th.PRCritAfter > 0 && age >= th.PRCritAfter:
			add(model.Crit, KindPRWaiting, subject, "open "+Age(age), pr.URL, pr.CreatedAt)
		case th.PRWarnAfter > 0 && age >= th.PRWarnAfter:
			add(model.Warn, KindPRWaiting, subject, "open "+Age(age), pr.URL, pr.CreatedAt)
		}
		if pr.Checks.State == model.ChecksFail {
			add(model.Warn, KindPRChecks, subject, summarizeChecks(pr.Checks), pr.URL, pr.UpdatedAt)
		}
	}

	if th.IssueStaleAfter > 0 {
		var stale int
		var oldest time.Time
		for _, is := range r.Issues {
			if now.Sub(is.UpdatedAt) >= th.IssueStaleAfter {
				stale++
				if oldest.IsZero() || is.UpdatedAt.Before(oldest) {
					oldest = is.UpdatedAt
				}
			}
		}
		if stale > 0 {
			add(model.Warn, KindIssuesStale, fmt.Sprintf("%d stale issues", stale),
				"no activity for "+Age(th.IssueStaleAfter)+" or more",
				r.URL+"/issues?q=is%3Aissue+is%3Aopen+sort%3Aupdated-asc", oldest)
		}
	}
	return alerts
}

func workflowSubject(wf model.Workflow) string {
	if wf.Background {
		return wf.Name + " (background)"
	}
	return wf.Name
}

func sortAlerts(a []model.Alert) {
	slices.SortStableFunc(a, func(x, y model.Alert) int {
		if x.Severity != y.Severity {
			return int(y.Severity) - int(x.Severity)
		}
		if c := x.Since.Compare(y.Since); c != 0 {
			return c
		}
		return strings.Compare(x.Repo+x.Subject, y.Repo+y.Subject)
	})
}

// Age renders a duration at a glance, e.g. "3d", "5h", "12m".
func Age(d time.Duration) string {
	switch {
	case d < 0:
		return "0m"
	case d >= 48*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

// Dormant reports whether the workflow has not run for longer than the dormant threshold,
// so an old failure no longer signals a current problem.
func Dormant(wf model.Workflow, now time.Time, th config.Thresholds) bool {
	return th.WorkflowDormant > 0 && !wf.Latest.CreatedAt.IsZero() && now.Sub(wf.Latest.CreatedAt) > th.WorkflowDormant
}

const maxNamedChecks = 2

// summarizeChecks renders failing checks as a count plus the first few distinct names.
func summarizeChecks(c model.CheckSummary) string {
	names := slices.Compact(slices.Sorted(slices.Values(c.Failing)))
	if prefix, ok := sharedPrefix(names); ok {
		for i, n := range names {
			names[i] = strings.TrimPrefix(n, prefix)
		}
	}
	shown := names[:min(len(names), maxNamedChecks)]
	out := fmt.Sprintf("%d checks failing", c.Failed)
	if len(shown) > 0 {
		out += ": " + strings.Join(shown, ", ")
	}
	if extra := len(names) - len(shown); extra > 0 {
		out += fmt.Sprintf(" +%d more", extra)
	}
	return out
}

// sharedPrefix returns the common "group / " prefix of check names such as "pr-checks / Tests".
func sharedPrefix(names []string) (string, bool) {
	if len(names) < 2 {
		return "", false
	}
	group, _, ok := strings.Cut(names[0], " / ")
	if !ok {
		return "", false
	}
	prefix := group + " / "
	for _, n := range names {
		if !strings.HasPrefix(n, prefix) {
			return "", false
		}
	}
	return prefix, true
}

// summarizeWarnings merges "section: reason" warnings that share a reason into one line.
func summarizeWarnings(warnings []string) []string {
	var reasons []string
	sections := map[string][]string{}
	for _, w := range warnings {
		section, reason, ok := strings.Cut(w, ": ")
		if !ok {
			section, reason = "data", w
		}
		if _, seen := sections[reason]; !seen {
			reasons = append(reasons, reason)
		}
		sections[reason] = append(sections[reason], section)
	}
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		out = append(out, strings.Join(sections[reason], ", ")+": "+reason)
	}
	return out
}
