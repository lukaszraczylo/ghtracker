package web

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/lukaszraczylo/ghtracker/internal/collector"
	"github.com/lukaszraczylo/ghtracker/internal/config"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

func autoPR(n int, sha, author string, mut func(*model.PullRequest)) model.PullRequest {
	p := model.PullRequest{Number: n, Title: "t", URL: prURL + strings.Repeat("x", n), Author: author, HeadSHA: sha, CreatedAt: now.Add(-time.Hour)}
	if mut != nil {
		mut(&p)
	}
	return p
}

// autoView builds the view through the real alert evaluation so fork and draft rules are exercised.
func autoView(prs ...model.PullRequest) collector.View {
	r := model.Repo{FullName: "o/r", Up: true, RefreshedAt: now, PRs: prs}
	alerts := collector.Evaluate(&r, now, config.Thresholds{PRWarnAfter: -1, PRCritAfter: -1, IssueStaleAfter: -1, WorkflowStuck: -1, DataStaleAfter: -1})
	return collector.View{Now: now, Repos: []model.Repo{r}, Alerts: alerts}
}

type autoStep struct {
	prs       []model.PullRequest
	code      int
	wantPosts int32
}

func TestAutoRunner(t *testing.T) {
	fork := func(p *model.PullRequest) { p.Fork = true }
	draft := func(p *model.PullRequest) { p.Draft = true }
	tests := map[string]struct {
		authors []string
		steps   []autoStep
	}{
		"fork triggers once per sha": {steps: []autoStep{
			{[]model.PullRequest{autoPR(1, "a", "x", fork)}, 200, 1},
			{[]model.PullRequest{autoPR(1, "a", "x", fork)}, 200, 0},
		}},
		"draft skipped": {steps: []autoStep{{[]model.PullRequest{autoPR(1, "a", "x", draft)}, 200, 0}}},
		"author filter": {authors: []string{"Alice"}, steps: []autoStep{
			{[]model.PullRequest{autoPR(1, "a", "bob", nil), autoPR(2, "b", "alice", nil)}, 200, 1},
		}},
		"once per sha": {steps: []autoStep{
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 200, 1},
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 200, 0},
		}},
		"new sha resends": {steps: []autoStep{
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 200, 1},
			{[]model.PullRequest{autoPR(1, "b", "x", nil)}, 201, 1},
		}},
		"reappearing alert resends": {steps: []autoStep{
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 200, 1},
			{nil, 200, 0},
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 200, 1},
		}},
		"failure retries": {steps: []autoStep{
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 500, 1},
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 400, 1},
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 200, 1},
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 200, 0},
		}},
		"409 retries": {steps: []autoStep{
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 409, 1},
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 200, 1},
			{[]model.PullRequest{autoPR(1, "a", "x", nil)}, 200, 0},
		}},
		"cap per refresh": {steps: []autoStep{
			{manyPRs(8), 200, maxAutoSends},
			{manyPRs(8), 200, 3},
			{manyPRs(8), 200, 0},
		}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			hook := newFakeHook(t)
			a := NewActions([]config.Action{{ID: "review", Kinds: []string{collector.KindPROpen}, Auto: true, Authors: tc.authors,
				Token: testToken, Webhook: config.Webhook{URL: hook.srv.URL + "/hook", Timeout: 2 * time.Second}}},
				slog.New(slog.NewTextHandler(io.Discard, nil)), prometheus.NewRegistry())
			runner := NewAutoRunner(a)
			for i, st := range tc.steps {
				hook.postCode = st.code
				before := hook.postCalls.Load()
				runner.Run(t.Context(), autoView(st.prs...))
				if got := hook.postCalls.Load() - before; got != st.wantPosts {
					t.Fatalf("step %d: posts = %d, want %d", i, got, st.wantPosts)
				}
			}
		})
	}
}

func manyPRs(n int) []model.PullRequest {
	out := make([]model.PullRequest, n)
	for i := range out {
		out[i] = autoPR(i+1, "s", "x", nil)
	}
	return out
}

func TestAutoRunnerPayloadAuthAndMetrics(t *testing.T) {
	hook := newFakeHook(t)
	a := NewActions([]config.Action{{ID: "review", Kinds: []string{collector.KindPROpen}, Auto: true,
		Token: testToken, Webhook: config.Webhook{URL: hook.srv.URL + "/hook", Timeout: 2 * time.Second}}},
		slog.New(slog.NewTextHandler(io.Discard, nil)), prometheus.NewRegistry())
	NewAutoRunner(a).Run(t.Context(), autoView(autoPR(1, "abc", "x", nil)))
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if hook.lastAuth != "Bearer "+testToken {
		t.Fatalf("auth = %q", hook.lastAuth)
	}
	for _, want := range []string{`"action":"review"`, `"kind":"pr_open"`, `"ref":"abc"`, `"url":"` + prURL + `x"`} {
		if !strings.Contains(hook.lastBody, want) {
			t.Fatalf("body %s lacks %s", hook.lastBody, want)
		}
	}
	if got := testutil.ToFloat64(a.counter.WithLabelValues("review", resultAutoOK)); got != 1 {
		t.Fatalf("auto_ok = %v, want 1", got)
	}
}

func TestNewAutoRunnerNilWithoutAutoActions(t *testing.T) {
	a := NewActions([]config.Action{{ID: "x", Webhook: config.Webhook{URL: "http://h.test"}}}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if NewAutoRunner(a) != nil {
		t.Fatal("runner must be nil when no action is auto")
	}
}

func TestStateCarriesPROpenRef(t *testing.T) {
	st := buildState(autoView(autoPR(1, "abc", "x", nil)), true)
	if len(st.Alerts) != 2 || st.Alerts[0].Kind != collector.KindPROpen || st.Alerts[0].Ref != "abc" || st.Alerts[0].Severity != "ok" || st.Alerts[1].Kind != collector.KindPRFresh {
		t.Fatalf("alerts = %+v", st.Alerts)
	}
	if st.Repos[0].Health != "warn" {
		t.Fatalf("health = %s, want warn", st.Repos[0].Health)
	}
}
