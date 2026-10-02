package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/collector"
	"github.com/lukaszraczylo/ghtracker/internal/config"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

var now = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

type fakeSource struct {
	st        collector.Stats
	v         collector.View
	refreshed int
}

func (f *fakeSource) View() collector.View   { return f.v }
func (f *fakeSource) Stats() collector.Stats { return f.st }
func (f *fakeSource) RequestRefresh() bool   { f.refreshed++; return true }

func testView() collector.View {
	repo := model.Repo{
		FullName: "o/r", URL: "https://github.com/o/r", ReleasesURL: "https://github.com/o/r/releases", Up: true,
		Health: model.Crit, RefreshedAt: now.Add(-time.Hour), Description: "desc",
		Release: &model.Release{Tag: "v1.2.3", URL: "https://github.com/o/r/releases/tag/v1.2.3", PublishedAt: now.Add(-48 * time.Hour)},
		PRs: []model.PullRequest{
			{Number: 5, CreatedAt: now.Add(-72 * time.Hour), Checks: model.CheckSummary{State: model.ChecksFail}},
			{Number: 6, Draft: true, CreatedAt: now.Add(-900 * time.Hour)},
		},
		Issues: []model.Issue{{Number: 9, UpdatedAt: now.Add(-40 * 24 * time.Hour)}, {Number: 10, UpdatedAt: now}},
		Workflows: []model.Workflow{
			{Name: "CI", Latest: model.Run{Status: "completed", Conclusion: "failure", CreatedAt: now.Add(-time.Hour)}, LastDone: &model.Run{Status: "completed", Conclusion: "failure", CreatedAt: now.Add(-time.Hour)}},
			{Name: "Nightly", Latest: model.Run{Status: "in_progress", CreatedAt: now}},
		},
	}
	bare := model.Repo{FullName: "o/bare", URL: "https://github.com/o/bare", ReleasesURL: "https://github.com/o/bare/releases", Err: "boom"}
	return collector.View{
		Now: now, LastRefresh: now.Add(-time.Hour), Interval: 6 * time.Hour, Loaded: true, Crit: 1, Warn: 1, ScanDone: 5, ScanTotal: 9,
		Thresholds: config.Thresholds{PRWarnAfter: 48 * time.Hour, PRCritAfter: 168 * time.Hour, IssueStaleAfter: 720 * time.Hour},
		Repos:      []model.Repo{repo, bare},
		Alerts: []model.Alert{
			{Severity: model.Crit, Repo: "o/r", Kind: "workflow_failed", Subject: "CI", Detail: "failure on main (push)", URL: "https://github.com/o/r/actions/runs/1", Since: now.Add(-2 * time.Hour)},
			{Severity: model.Warn, Repo: "o/r", Kind: "pr_waiting", Subject: "#5 fix", Detail: "open 3d", URL: "https://github.com/o/r/pull/5", Since: now.Add(-72 * time.Hour)},
		},
	}
}

func testUI() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>ui</title>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
		"secret.txt":    {Data: []byte("nope")},
	}
}

func newServer(src *fakeSource, ui fstest.MapFS, manual bool) *Server {
	return New(src, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("metrics")) }),
		ui, manual, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestIndexServesBuiltUIWithSecurityHeaders(t *testing.T) {
	rec := do(newServer(&fakeSource{}, testUI(), false).Handler(), "GET", "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>ui</title>") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-cache" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("headers: %v", rec.Header())
	}
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "script-src 'self'", "connect-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q missing %q", csp, want)
		}
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("nosniff missing")
	}
}

func TestIndexWithoutBuiltUIIs503(t *testing.T) {
	rec := do(newServer(&fakeSource{}, fstest.MapFS{}, false).Handler(), "GET", "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "pnpm build") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestAssetsCachedAndScopedToAssetsDir(t *testing.T) {
	h := newServer(&fakeSource{}, testUI(), false).Handler()
	rec := do(h, "GET", "/assets/app.js")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != immutableCache || rec.Body.String() != "console.log(1)" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	for _, p := range []string{"/assets/missing.js", "/secret.txt", "/assets/../secret.txt", "/nope"} {
		if rec := do(h, "GET", p); rec.Code != http.StatusNotFound && rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusTemporaryRedirect {
			t.Errorf("%s -> %d, want 404", p, rec.Code)
		} else if rec.Body.Len() > 0 && strings.Contains(rec.Body.String(), "nope") {
			t.Errorf("%s leaked a file outside assets", p)
		}
	}
}

func TestStateJSONContract(t *testing.T) {
	src := &fakeSource{v: testView()}
	rec := do(newServer(src, testUI(), true).Handler(), "GET", "/api/state")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	var got struct {
		Repos           []map[string]any
		Alerts          []map[string]any
		Counts          struct{ Crit, Warn int }
		Scan            struct{ Done, Total int }
		Thresholds      struct{ PRWarnSeconds, PRCritSeconds int64 }
		IntervalSeconds int64
		Loaded          bool
		ManualRefresh   bool
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Scan.Done != 5 || got.Scan.Total != 9 {
		t.Fatalf("scan: %+v", got.Scan)
	}
	if got.IntervalSeconds != 21600 || !got.Loaded || !got.ManualRefresh || got.Counts.Crit != 1 || got.Counts.Warn != 1 {
		t.Fatalf("top level: %+v", got)
	}
	if got.Thresholds.PRWarnSeconds != 172800 || got.Thresholds.PRCritSeconds != 604800 {
		t.Fatalf("thresholds: %+v", got.Thresholds)
	}
	r := got.Repos[0]
	if r["fullName"] != "o/r" || r["health"] != "crit" {
		t.Fatalf("repo: %v", r)
	}
	prs, issues, wf := r["prs"].(map[string]any), r["issues"].(map[string]any), r["workflows"].(map[string]any)
	if prs["open"] != float64(2) || prs["waiting"] != float64(1) || prs["failingChecks"] != float64(1) || prs["oldestSeconds"] != float64(72*3600) {
		t.Fatalf("prs: %v", prs)
	}
	if issues["open"] != float64(2) || issues["stale"] != float64(1) {
		t.Fatalf("issues: %v", issues)
	}
	if wf["total"] != float64(2) || wf["failing"] != float64(1) || wf["running"] != float64(1) {
		t.Fatalf("workflows: %v", wf)
	}
	if rel := r["release"].(map[string]any); rel["tag"] != "v1.2.3" || rel["url"] == "" {
		t.Fatalf("release: %v", rel)
	}
	if got.Repos[1]["release"] != nil || got.Repos[1]["error"] != "boom" || got.Repos[1]["health"] != "ok" {
		t.Fatalf("bare repo: %v", got.Repos[1])
	}
	if len(got.Alerts) != 2 || got.Alerts[0]["severity"] != "crit" || got.Alerts[0]["subject"] != "CI" {
		t.Fatalf("alerts: %v", got.Alerts)
	}
}

func TestStateJSONEmptyListsAreArrays(t *testing.T) {
	rec := do(newServer(&fakeSource{}, testUI(), false).Handler(), "GET", "/api/state")
	body := rec.Body.String()
	if !strings.Contains(body, `"repos":[]`) || !strings.Contains(body, `"alerts":[]`) {
		t.Fatalf("lists must encode as [], got %s", body)
	}
}

func TestHealthzAndMetrics(t *testing.T) {
	src := &fakeSource{}
	h := newServer(src, testUI(), false).Handler()
	if rec := do(h, "GET", "/healthz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("before first refresh: %d", rec.Code)
	}
	src.st.Loaded = true
	if rec := do(h, "GET", "/healthz"); rec.Code != 200 {
		t.Fatalf("after refresh: %d", rec.Code)
	}
	if rec := do(h, "GET", "/metrics"); rec.Body.String() != "metrics" {
		t.Fatal("metrics handler not mounted")
	}
}

func TestManualRefreshDisabledByDefault(t *testing.T) {
	src := &fakeSource{}
	rec := do(newServer(src, testUI(), false).Handler(), "POST", "/refresh")
	if rec.Code == http.StatusAccepted || src.refreshed != 0 {
		t.Fatalf("refresh must not be routed when disabled: %d", rec.Code)
	}
}

func TestManualRefreshRateLimited(t *testing.T) {
	src := &fakeSource{}
	s := newServer(src, testUI(), true)
	clock := now
	s.now = func() time.Time { return clock }
	h := s.Handler()
	if rec := do(h, "POST", "/refresh"); rec.Code != http.StatusAccepted {
		t.Fatalf("first: %d", rec.Code)
	}
	clock = clock.Add(time.Minute)
	if rec := do(h, "POST", "/refresh"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second within gap: %d", rec.Code)
	}
	clock = clock.Add(5 * time.Minute)
	if rec := do(h, "POST", "/refresh"); rec.Code != http.StatusAccepted || src.refreshed != 2 {
		t.Fatalf("after gap: %d refreshed=%d", rec.Code, src.refreshed)
	}
	if rec := do(h, "GET", "/refresh"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /refresh: %d", rec.Code)
	}
}

func TestManualRefreshRefusesCrossSiteRequests(t *testing.T) {
	src := &fakeSource{}
	h := newServer(src, testUI(), true).Handler()
	for _, site := range []string{"cross-site", "same-site"} {
		req := httptest.NewRequest("POST", "/refresh", nil)
		req.Header.Set("Sec-Fetch-Site", site)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || src.refreshed != 0 {
			t.Fatalf("%s: %d refreshed=%d", site, rec.Code, src.refreshed)
		}
	}
	req := httptest.NewRequest("POST", "/refresh", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("same-origin: %d", rec.Code)
	}
}
