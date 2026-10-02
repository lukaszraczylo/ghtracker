package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/config"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

var t0 = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

func th() config.Thresholds {
	return config.Thresholds{
		PRWarnAfter: 48 * time.Hour, PRCritAfter: 7 * 24 * time.Hour,
		IssueStaleAfter: 30 * 24 * time.Hour, WorkflowStuck: 6 * time.Hour, WorkflowDormant: 30 * 24 * time.Hour, DataStaleAfter: 13 * time.Hour,
	}
}

func kinds(a []model.Alert) map[string]model.Severity {
	m := map[string]model.Severity{}
	for _, x := range a {
		m[x.Kind] = max(m[x.Kind], x.Severity)
	}
	return m
}

func TestEvaluate(t *testing.T) {
	fresh := func() model.Repo {
		return model.Repo{FullName: "o/r", URL: "https://github.com/o/r", Up: true, RefreshedAt: t0}
	}
	failedRun := &model.Run{Status: "completed", Conclusion: "failure", URL: "run"}
	tests := map[string]struct {
		mutate func(*model.Repo)
		want   map[string]model.Severity
		health model.Severity
	}{
		"healthy":        {func(*model.Repo) {}, map[string]model.Severity{}, model.OK},
		"refresh failed": {func(r *model.Repo) { r.Err = "boom"; r.Up = false }, map[string]model.Severity{KindRefreshFailed: model.Crit}, model.Crit},
		"data stale":     {func(r *model.Repo) { r.RefreshedAt = t0.Add(-14 * time.Hour) }, map[string]model.Severity{KindDataStale: model.Warn}, model.Warn},
		"partial data":   {func(r *model.Repo) { r.Warnings = []string{"workflows: 403"} }, map[string]model.Severity{KindPartialData: model.Warn}, model.Warn},
		"workflow failed": {func(r *model.Repo) {
			r.Workflows = []model.Workflow{{Name: "CI", Latest: *failedRun, LastDone: failedRun}}
		}, map[string]model.Severity{KindWorkflowFail: model.Crit}, model.Crit},
		"workflow rerun in progress after failure still critical": {func(r *model.Repo) {
			r.Workflows = []model.Workflow{{Name: "CI", Latest: model.Run{Status: "in_progress", CreatedAt: t0}, LastDone: failedRun}}
		}, map[string]model.Severity{KindWorkflowFail: model.Crit}, model.Crit},
		"dormant workflow failure muted": {func(r *model.Repo) {
			old := &model.Run{Status: "completed", Conclusion: "failure", CreatedAt: t0.Add(-40 * 24 * time.Hour)}
			r.Workflows = []model.Workflow{{Name: "Old", Latest: *old, LastDone: old}}
		}, map[string]model.Severity{}, model.OK},
		"workflow success": {func(r *model.Repo) {
			ok := &model.Run{Status: "completed", Conclusion: "success"}
			r.Workflows = []model.Workflow{{Name: "CI", Latest: *ok, LastDone: ok}}
		}, map[string]model.Severity{}, model.OK},
		"cancelled is not failure": {func(r *model.Repo) {
			c := &model.Run{Status: "completed", Conclusion: "cancelled"}
			r.Workflows = []model.Workflow{{Name: "CI", Latest: *c, LastDone: c}}
		}, map[string]model.Severity{}, model.OK},
		"workflow stuck": {func(r *model.Repo) {
			r.Workflows = []model.Workflow{{Name: "CI", Latest: model.Run{Status: "queued", CreatedAt: t0.Add(-7 * time.Hour)}}}
		}, map[string]model.Severity{KindWorkflowStuck: model.Warn}, model.Warn},
		"pr fresh": {func(r *model.Repo) {
			r.PRs = []model.PullRequest{{Number: 1, CreatedAt: t0.Add(-47 * time.Hour)}}
		}, map[string]model.Severity{}, model.OK},
		"pr waiting warn": {func(r *model.Repo) {
			r.PRs = []model.PullRequest{{Number: 1, CreatedAt: t0.Add(-49 * time.Hour)}}
		}, map[string]model.Severity{KindPRWaiting: model.Warn}, model.Warn},
		"pr waiting crit": {func(r *model.Repo) {
			r.PRs = []model.PullRequest{{Number: 1, CreatedAt: t0.Add(-8 * 24 * time.Hour)}}
		}, map[string]model.Severity{KindPRWaiting: model.Crit}, model.Crit},
		"draft pr ignored": {func(r *model.Repo) {
			r.PRs = []model.PullRequest{{Number: 1, Draft: true, CreatedAt: t0.Add(-30 * 24 * time.Hour), Checks: model.CheckSummary{State: model.ChecksFail}}}
		}, map[string]model.Severity{}, model.OK},
		"pr checks failing": {func(r *model.Repo) {
			r.PRs = []model.PullRequest{{Number: 1, CreatedAt: t0, Checks: model.CheckSummary{State: model.ChecksFail, Failing: []string{"build"}}}}
		}, map[string]model.Severity{KindPRChecks: model.Warn}, model.Warn},
		"issues stale aggregated": {func(r *model.Repo) {
			old := t0.Add(-40 * 24 * time.Hour)
			r.Issues = []model.Issue{{Number: 1, UpdatedAt: old}, {Number: 2, UpdatedAt: old}, {Number: 3, UpdatedAt: t0}}
		}, map[string]model.Severity{KindIssuesStale: model.Warn}, model.Warn},
		"archived repo skips work alerts": {func(r *model.Repo) {
			r.Archived = true
			r.PRs = []model.PullRequest{{Number: 1, CreatedAt: t0.Add(-30 * 24 * time.Hour)}}
		}, map[string]model.Severity{}, model.OK},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := fresh()
			tt.mutate(&r)
			alerts := Evaluate(&r, t0, th())
			got := kinds(alerts)
			if len(got) != len(tt.want) {
				t.Fatalf("alerts = %+v, want kinds %v", alerts, tt.want)
			}
			for k, sev := range tt.want {
				if got[k] != sev {
					t.Fatalf("kind %s severity = %v, want %v", k, got[k], sev)
				}
			}
			if r.Health != tt.health {
				t.Fatalf("health = %v, want %v", r.Health, tt.health)
			}
		})
	}
}

func TestEvaluateDisabledThresholds(t *testing.T) {
	r := model.Repo{FullName: "o/r", RefreshedAt: t0, PRs: []model.PullRequest{{Number: 1, CreatedAt: t0.Add(-100 * 24 * time.Hour)}},
		Issues: []model.Issue{{UpdatedAt: t0.Add(-100 * 24 * time.Hour)}}}
	off := config.Thresholds{PRWarnAfter: -1, PRCritAfter: -1, IssueStaleAfter: -1, WorkflowStuck: -1, DataStaleAfter: -1}
	if a := Evaluate(&r, t0, off); len(a) != 0 {
		t.Fatalf("disabled checks produced %+v", a)
	}
}

func TestAgeAdvancesWithoutRefresh(t *testing.T) {
	r := model.Repo{FullName: "o/r", RefreshedAt: t0, PRs: []model.PullRequest{{Number: 1, CreatedAt: t0.Add(-40 * time.Hour)}}}
	cfg := th()
	cfg.DataStaleAfter = -1
	if a := Evaluate(&r, t0, cfg); len(a) != 0 {
		t.Fatalf("unexpected %+v", a)
	}
	if a := Evaluate(&r, t0.Add(10*time.Hour), cfg); len(a) != 1 || a[0].Severity != model.Warn {
		t.Fatalf("PR must cross the warn threshold as time passes: %+v", a)
	}
}

func TestAge(t *testing.T) {
	cases := map[time.Duration]string{-time.Minute: "0m", 5 * time.Minute: "5m", 90 * time.Minute: "1h", 47 * time.Hour: "47h", 49 * time.Hour: "2d", 10 * 24 * time.Hour: "10d"}
	for d, want := range cases {
		if got := Age(d); got != want {
			t.Errorf("Age(%s) = %s, want %s", d, got, want)
		}
	}
}

type fakeFetcher struct {
	fn    func(name string) (model.Repo, error)
	calls []string
	mu    sync.Mutex
}

func (f *fakeFetcher) FetchRepo(_ context.Context, name string) (model.Repo, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.mu.Unlock()
	return f.fn(name)
}

func newCollector(f Fetcher, repos ...string) *Collector {
	cfg := &config.Config{Repos: repos, RefreshInterval: time.Hour, Concurrency: 1, Thresholds: th()}
	c := New(f, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.now = func() time.Time { return t0 }
	return c
}

func TestRefreshKeepsLastGoodDataOnFailure(t *testing.T) {
	fail := false
	f := &fakeFetcher{fn: func(name string) (model.Repo, error) {
		if fail {
			return model.Repo{}, errors.New("boom")
		}
		return model.Repo{FullName: name, URL: "u", Issues: []model.Issue{{Number: 1, UpdatedAt: t0}}}, nil
	}}
	c := newCollector(f, "o/a")
	c.Refresh(context.Background())
	if v := c.View(); !v.Loaded || len(v.Repos) != 1 || !v.Repos[0].Up || len(v.Repos[0].Issues) != 1 {
		t.Fatalf("first view: %+v", v)
	}
	fail = true
	c.Refresh(context.Background())
	v := c.View()
	r := v.Repos[0]
	if r.Up || r.Err != "boom" || len(r.Issues) != 1 {
		t.Fatalf("must keep old data and flag error: %+v", r)
	}
	if v.Crit != 1 || r.Health != model.Crit {
		t.Fatalf("crit=%d health=%v", v.Crit, r.Health)
	}
	if s := c.Stats(); s.RepoErrorsTotal != 1 || s.RefreshTotal != 2 {
		t.Fatalf("stats %+v", s)
	}
}

func TestFirstRefreshFailureStillListsRepo(t *testing.T) {
	c := newCollector(&fakeFetcher{fn: func(string) (model.Repo, error) { return model.Repo{}, errors.New("nope") }}, "o/a")
	c.Refresh(context.Background())
	v := c.View()
	if len(v.Repos) != 1 || v.Repos[0].Up || v.Repos[0].URL != "https://github.com/o/a" {
		t.Fatalf("%+v", v.Repos)
	}
}

func TestRateLimitStopsRemainingRepos(t *testing.T) {
	f := &fakeFetcher{fn: func(name string) (model.Repo, error) {
		if name == "o/b" {
			return model.Repo{}, model.ErrRateLimited
		}
		return model.Repo{FullName: name}, nil
	}}
	c := newCollector(f, "o/a", "o/b", "o/c", "o/d")
	c.Refresh(context.Background())
	if len(f.calls) != 2 {
		t.Fatalf("calls after limit = %v, want only a and b", f.calls)
	}
	v := c.View()
	if len(v.Repos) != 4 {
		t.Fatalf("all repos must stay listed: %d", len(v.Repos))
	}
	// Next cycle starts with a clean limit flag.
	f.fn = func(name string) (model.Repo, error) { return model.Repo{FullName: name}, nil }
	f.calls = nil
	c.Refresh(context.Background())
	if len(f.calls) != 4 {
		t.Fatalf("limit flag must reset, calls=%v", f.calls)
	}
}

func TestViewSortsWorstFirstAndAlertsBySeverity(t *testing.T) {
	f := &fakeFetcher{fn: func(name string) (model.Repo, error) {
		r := model.Repo{FullName: name}
		switch name {
		case "o/crit":
			c := &model.Run{Status: "completed", Conclusion: "failure"}
			r.Workflows = []model.Workflow{{Name: "CI", Latest: *c, LastDone: c}}
		case "o/warn":
			r.PRs = []model.PullRequest{{Number: 1, CreatedAt: t0.Add(-72 * time.Hour)}}
		}
		return r, nil
	}}
	c := newCollector(f, "o/ok", "o/warn", "o/crit")
	c.Refresh(context.Background())
	v := c.View()
	if v.Repos[0].FullName != "o/crit" || v.Repos[1].FullName != "o/warn" || v.Repos[2].FullName != "o/ok" {
		t.Fatalf("order: %s %s %s", v.Repos[0].FullName, v.Repos[1].FullName, v.Repos[2].FullName)
	}
	if len(v.Alerts) != 2 || v.Alerts[0].Severity != model.Crit || v.Crit != 1 || v.Warn != 1 {
		t.Fatalf("alerts: %+v", v.Alerts)
	}
}

func TestRequestRefreshCoalesces(t *testing.T) {
	c := newCollector(&fakeFetcher{fn: func(n string) (model.Repo, error) { return model.Repo{FullName: n}, nil }}, "o/a")
	if !c.RequestRefresh() {
		t.Fatal("first request must queue")
	}
	if c.RequestRefresh() {
		t.Fatal("second request must coalesce while one is queued")
	}
}

func TestRunRefreshesOnStartAndStopsOnCancel(t *testing.T) {
	c := newCollector(&fakeFetcher{fn: func(n string) (model.Repo, error) { return model.Repo{FullName: n}, nil }}, "o/a")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	deadline := time.After(2 * time.Second)
	for !c.Stats().Loaded {
		select {
		case <-deadline:
			t.Fatal("no initial refresh")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestSummarizeChecks(t *testing.T) {
	tests := map[string]struct {
		want string
		in   model.CheckSummary
	}{
		"few distinct": {in: model.CheckSummary{Failed: 2, Failing: []string{"build", "lint"}}, want: "2 checks failing: build, lint"},
		"duplicates collapse and extras count": {
			in:   model.CheckSummary{Failed: 6, Failing: []string{"pr-checks / Tests", "pr-checks / Tests", "pr-checks / Lint", "pr-checks / Scan", "pr-checks / Gosec"}},
			want: "6 checks failing: Gosec, Lint +2 more",
		},
		"mixed prefixes stay intact": {in: model.CheckSummary{Failed: 2, Failing: []string{"a / x", "b / y"}}, want: "2 checks failing: a / x, b / y"},
		"no names":                   {in: model.CheckSummary{Failed: 1}, want: "1 checks failing"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := summarizeChecks(tt.in); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeWarningsMergesSharedReason(t *testing.T) {
	got := summarizeWarnings([]string{
		"issues: github: HTTP 403: Resource not accessible by integration",
		"workflows: github: HTTP 403: Resource not accessible by integration",
		"releases: github: HTTP 500: boom",
	})
	want := []string{
		"issues, workflows: github: HTTP 403: Resource not accessible by integration",
		"releases: github: HTTP 500: boom",
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %q", got)
	}
}
