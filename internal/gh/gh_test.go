package gh

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/model"
)

func testKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	return k, string(p)
}

// fakeGitHub serves canned JSON by path and counts hits per path.
type fakeGitHub struct {
	srv       *httptest.Server
	routes    map[string]func(w http.ResponseWriter, r *http.Request)
	hits      map[string]*atomic.Int64
	appKeyPub *rsa.PublicKey
	t         *testing.T
	installs  atomic.Int64
	tokens    atomic.Int64
}

func newFake(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{t: t, routes: map[string]func(http.ResponseWriter, *http.Request){}, hits: map[string]*atomic.Int64{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/installation"):
		f.installs.Add(1)
		f.checkJWT(r)
		_, _ = w.Write([]byte(`{"id":42}`))
		return
	case strings.HasPrefix(r.URL.Path, "/app/installations/"):
		f.tokens.Add(1)
		f.checkJWT(r)
		exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		_, _ = fmt.Fprintf(w, `{"token":"inst-token","expires_at":%q}`, exp)
		return
	}
	if r.Header.Get("Authorization") != "Bearer inst-token" {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	h, ok := f.routes[r.URL.Path]
	if !ok {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	c := f.hits[r.URL.Path]
	if c == nil {
		c = &atomic.Int64{}
		f.hits[r.URL.Path] = c
	}
	c.Add(1)
	h(w, r)
}

func (f *fakeGitHub) checkJWT(r *http.Request) {
	if f.appKeyPub == nil {
		return
	}
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		f.t.Errorf("jwt has %d parts", len(parts))
		return
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(f.appKeyPub, crypto.SHA256, sum[:], sig); err != nil {
		f.t.Errorf("jwt signature invalid: %v", err)
	}
}

func (f *fakeGitHub) json(path, body string) {
	f.routes[path] = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }
}

func (f *fakeGitHub) client(t *testing.T, maxItems int) *Client {
	t.Helper()
	key, pemStr := testKey(t)
	f.appKeyPub = &key.PublicKey
	auth, err := NewAppAuth(7, pemStr, f.srv.URL, 0, f.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(f.srv.URL, auth, f.srv.Client(), maxItems)
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestJWTClaimsAndKeyFormats(t *testing.T) {
	key, pemStr := testKey(t)
	a, err := NewAppAuth(99, pemStr, "http://x", 0, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Unix(1_700_000_000, 0)
	a.now = func() time.Time { return fixed }
	jwt, err := a.appJWT()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	claimsRaw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["iss"] != "99" || int64(claims["iat"].(float64)) != fixed.Unix()-60 || int64(claims["exp"].(float64)) != fixed.Unix()+540 {
		t.Fatalf("claims = %v", claims)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatal(err)
	}

	der, _ := x509.MarshalPKCS8PrivateKey(key)
	if _, err := ParsePrivateKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); err != nil {
		t.Fatalf("pkcs8: %v", err)
	}
	if _, err := ParsePrivateKey([]byte("garbage")); err == nil {
		t.Fatal("garbage key must fail")
	}
}

func TestTokenCachedAndInstallationDiscoveredOnce(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	f.json("/repos/o/r", `{"html_url":"https://github.com/o/r","default_branch":"main"}`)
	for range 3 {
		if err := c.getJSON(context.Background(), "o/r", "/repos/o/r", nil, &struct{}{}); err != nil {
			t.Fatal(err)
		}
	}
	if f.installs.Load() != 1 || f.tokens.Load() != 1 {
		t.Fatalf("installs=%d tokens=%d, want 1 each", f.installs.Load(), f.tokens.Load())
	}
}

func TestPaginationStopsAtLimit(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 3)
	f.routes["/repos/o/r/issues"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"number":3,"title":"c"},{"number":4,"title":"d"}]`))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/issues?page=2>; rel="next", <x>; rel="last"`, f.srv.URL))
		_, _ = w.Write([]byte(`[{"number":1,"title":"a"},{"number":2,"title":"b"}]`))
	}
	got, err := c.openIssues(context.Background(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d issues, want 3 (limit)", len(got))
	}
}

func TestNextLink(t *testing.T) {
	h := `<https://api/x?page=2>; rel="next", <https://api/x?page=9>; rel="last"`
	if got := nextLink(h); got != "https://api/x?page=2" {
		t.Fatal(got)
	}
	if nextLink(`<https://api/x?page=1>; rel="prev"`) != "" || nextLink("") != "" {
		t.Fatal("no next link expected")
	}
}

func TestETagConditionalRequestServedFromCache(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	var conditional atomic.Int64
	f.routes["/repos/o/r"] = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(`{"default_branch":"main"}`))
	}
	for range 2 {
		var out struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := c.getJSON(context.Background(), "o/r", "/repos/o/r", nil, &out); err != nil || out.DefaultBranch != "main" {
			t.Fatalf("out=%+v err=%v", out, err)
		}
	}
	if conditional.Load() != 1 {
		t.Fatalf("conditional requests = %d, want 1", conditional.Load())
	}
}

func TestRetriesServerErrorsThenSucceeds(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	var n atomic.Int64
	f.routes["/repos/o/r"] = func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1) < 3 {
			http.Error(w, `{"message":"boom"}`, http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}
	if err := c.getJSON(context.Background(), "o/r", "/repos/o/r", nil, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 3 {
		t.Fatalf("attempts = %d", n.Load())
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	err := c.getJSON(context.Background(), "o/r", "/missing", nil, &struct{}{})
	if !isNotFound(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestPrimaryRateLimitBlocksFurtherRequests(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	reset := time.Now().Add(30 * time.Minute)
	f.routes["/repos/o/r"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(reset.Unix()))
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	}
	err := c.getJSON(context.Background(), "o/r", "/repos/o/r", nil, &struct{}{})
	if !errors.Is(err, model.ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
	hits := f.hits["/repos/o/r"].Load()
	err = c.getJSON(context.Background(), "o/r", "/repos/o/r", nil, &struct{}{})
	if !errors.Is(err, model.ErrRateLimited) || f.hits["/repos/o/r"].Load() != hits {
		t.Fatalf("second call must be refused locally; err=%v hits=%d->%d", err, hits, f.hits["/repos/o/r"].Load())
	}
}

func TestSecondaryRateLimitHonoursRetryAfter(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	now := time.Now()
	c.now = func() time.Time { return now }
	f.routes["/repos/o/r"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		http.Error(w, `{"message":"secondary rate limit"}`, http.StatusTooManyRequests)
	}
	if err := c.getJSON(context.Background(), "o/r", "/repos/o/r", nil, &struct{}{}); !errors.Is(err, model.ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
	c.now = func() time.Time { return now.Add(119 * time.Second) }
	if err := c.guard(); !errors.Is(err, model.ErrRateLimited) {
		t.Fatalf("still blocked at 119s, got %v", err)
	}
	c.now = func() time.Time { return now.Add(121 * time.Second) }
	if err := c.guard(); err != nil {
		t.Fatalf("unblocked at 121s, got %v", err)
	}
}

func TestLowBudgetStopsBeforeQuotaExhausted(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	f.routes["/repos/o/r"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "50")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
		_, _ = w.Write([]byte(`{}`))
	}
	if err := c.getJSON(context.Background(), "o/r", "/repos/o/r", nil, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if err := c.getJSON(context.Background(), "o/r", "/repos/o/r", nil, &struct{}{}); !errors.Is(err, model.ErrRateLimited) {
		t.Fatalf("budget below reserve must refuse, got %v", err)
	}
	if rem, ok := c.RateLimitRemaining(); !ok || rem != 50 {
		t.Fatalf("remaining = %d %v", rem, ok)
	}
}

func TestPlain403IsNotTreatedAsRateLimit(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	f.routes["/repos/o/r"] = func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Resource not accessible by integration"}`, http.StatusForbidden)
	}
	err := c.getJSON(context.Background(), "o/r", "/repos/o/r", nil, &struct{}{})
	var ae *APIError
	if errors.Is(err, model.ErrRateLimited) || !errors.As(err, &ae) || ae.Status != 403 {
		t.Fatalf("err = %v", err)
	}
	if err := c.guard(); err != nil {
		t.Fatalf("plain 403 must not block: %v", err)
	}
}

func registerRepo(f *fakeGitHub) {
	f.json("/repos/o/r", `{"html_url":"https://github.com/o/r","description":"d","default_branch":"main"}`)
	f.json("/repos/o/r/issues", `[
	  {"number":1,"title":"real issue","html_url":"u1","user":{"login":"a"},"labels":[{"name":"bug"}],"comments":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"},
	  {"number":2,"title":"a PR","html_url":"u2","pull_request":{"url":"x"}}]`)
	f.json("/repos/o/r/pulls", `[{"number":2,"title":"a PR","html_url":"u2","user":{"login":"b"},"draft":false,"head":{"sha":"abc"},"created_at":"2026-01-03T00:00:00Z","updated_at":"2026-01-03T00:00:00Z"}]`)
	f.json("/repos/o/r/commits/abc/check-runs", `{"check_runs":[{"name":"build","status":"completed","conclusion":"failure"},{"name":"lint","status":"completed","conclusion":"success"},{"name":"slow","status":"in_progress"}]}`)
	f.json("/repos/o/r/commits/abc/status", `{"total_count":1,"statuses":[{"context":"ci/ext","state":"error"}]}`)
	f.json("/repos/o/r/actions/workflows", `{"workflows":[
	  {"id":1,"name":"CI","html_url":"w1","state":"active"},
	  {"id":2,"name":"Nightly","html_url":"w2","state":"active"},
	  {"id":3,"name":"Old","html_url":"w3","state":"disabled_manually"},
	  {"id":4,"name":"Never ran","html_url":"w4","state":"active"}]}`)
	f.json("/repos/o/r/actions/workflows/1/runs", `{"workflow_runs":[
	  {"id":11,"html_url":"r11","event":"push","head_branch":"main","status":"in_progress","created_at":"2026-01-05T00:00:00Z"},
	  {"id":10,"html_url":"r10","event":"push","head_branch":"main","status":"completed","conclusion":"success","created_at":"2026-01-04T00:00:00Z"}]}`)
	f.json("/repos/o/r/actions/workflows/2/runs", `{"workflow_runs":[
	  {"id":20,"html_url":"r20","event":"schedule","head_branch":"main","status":"completed","conclusion":"failure","created_at":"2026-01-05T00:00:00Z"}]}`)
	f.json("/repos/o/r/actions/workflows/4/runs", `{"workflow_runs":[]}`)
	f.json("/repos/o/r/releases/latest", `{"tag_name":"v1.2.3","name":"x","html_url":"rel","published_at":"2026-01-01T00:00:00Z"}`)
}

func TestFetchRepoEndToEnd(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 50)
	registerRepo(f)
	r, err := c.FetchRepo(context.Background(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("warnings: %v", r.Warnings)
	}
	if len(r.Issues) != 1 || r.Issues[0].Number != 1 || r.Issues[0].Labels[0] != "bug" {
		t.Fatalf("issues must exclude PRs: %+v", r.Issues)
	}
	if len(r.PRs) != 1 || r.PRs[0].Checks.State != model.ChecksFail || r.PRs[0].Checks.Failed != 2 || r.PRs[0].Checks.Pending != 1 {
		t.Fatalf("pr checks: %+v", r.PRs)
	}
	if got := r.PRs[0].Checks.Failing; len(got) != 2 || got[0] != "build" || got[1] != "ci/ext" {
		t.Fatalf("failing names: %v", got)
	}
	if len(r.Workflows) != 2 {
		t.Fatalf("want CI and Nightly only, got %+v", r.Workflows)
	}
	ci, nightly := r.Workflows[0], r.Workflows[1]
	if !ci.Latest.Running() || ci.LastDone == nil || ci.LastDone.ID != 10 || ci.Background {
		t.Fatalf("ci: %+v", ci)
	}
	if !nightly.Background || nightly.LastDone == nil || !nightly.LastDone.Failed() {
		t.Fatalf("nightly: %+v", nightly)
	}
	if r.Release == nil || r.Release.Tag != "v1.2.3" || r.Release.URL != "rel" || r.ReleasesURL != "https://github.com/o/r/releases" {
		t.Fatalf("release: %+v %s", r.Release, r.ReleasesURL)
	}
}

func TestReleaseFallsBackToTag(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	f.json("/repos/o/r/tags", `[{"name":"v0.1.0"}]`)
	rel, err := c.latestRelease(context.Background(), "o/r", "https://github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	if !rel.FromTag || rel.Tag != "v0.1.0" || rel.URL != "https://github.com/o/r/releases/tag/v0.1.0" {
		t.Fatalf("%+v", rel)
	}
}

func TestReleaseNoneAtAll(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	f.json("/repos/o/r/tags", `[]`)
	rel, err := c.latestRelease(context.Background(), "o/r", "https://github.com/o/r")
	if err != nil || rel != nil {
		t.Fatalf("rel=%+v err=%v", rel, err)
	}
}

func TestSectionFailureDegradesToWarning(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	registerRepo(f)
	f.routes["/repos/o/r/actions/workflows"] = func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Resource not accessible by integration"}`, http.StatusForbidden)
	}
	r, err := c.FetchRepo(context.Background(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "workflows") || len(r.Issues) != 1 {
		t.Fatalf("warnings=%v issues=%d", r.Warnings, len(r.Issues))
	}
}

func TestRateLimitInSectionFailsWholeRepo(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	registerRepo(f)
	f.routes["/repos/o/r/pulls"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, `{"message":"secondary rate limit"}`, http.StatusForbidden)
	}
	if _, err := c.FetchRepo(context.Background(), "o/r"); !errors.Is(err, model.ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildWorkflowIgnoresPullRequestRuns(t *testing.T) {
	runs := []rawRun{
		{ID: 2, Event: "pull_request", Status: "completed", Conclusion: "failure", CreatedAt: time.Unix(200, 0)},
		{ID: 1, Event: "push", Status: "completed", Conclusion: "success", CreatedAt: time.Unix(100, 0)},
	}
	wf, ok := buildWorkflow("CI", "u", runs)
	if !ok || wf.Latest.ID != 1 || wf.LastDone.ID != 1 {
		t.Fatalf("%+v", wf)
	}
	if _, ok := buildWorkflow("CI", "u", runs[:1]); ok {
		t.Fatal("only PR runs must yield no workflow")
	}
}

func TestAPIErrorMessageParsing(t *testing.T) {
	if got := apiMessage([]byte(`{"message":"nope"}`)); got != "nope" {
		t.Fatal(got)
	}
	if got := apiMessage([]byte(strings.Repeat("x", 500))); len(got) != 200 {
		t.Fatalf("len=%d", len(got))
	}
}

// staleThenFresh models GitHub answering one exact query from an old cached result: the plain
// query returns an old failing run every time, and the same query with page=1 returns current runs.
func staleThenFresh(f *fakeGitHub, path string) *atomic.Int64 {
	var calls atomic.Int64
	f.routes[path] = func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("page") == "" {
			_, _ = w.Write([]byte(`{"workflow_runs":[
			  {"id":1,"html_url":"old","event":"schedule","head_branch":"main","status":"completed","conclusion":"failure","created_at":"2026-09-11T03:02:00Z"}]}`))
			return
		}
		if r.Header.Get("If-None-Match") != "" {
			http.Error(w, "fresh requests must not be conditional", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"workflow_runs":[
		  {"id":3,"html_url":"new","event":"schedule","head_branch":"main","status":"completed","conclusion":"success","created_at":"2026-10-02T03:03:00Z"},
		  {"id":1,"html_url":"old","event":"schedule","head_branch":"main","status":"completed","conclusion":"failure","created_at":"2026-09-11T03:02:00Z"}]}`))
	}
	return &calls
}

func TestStaleFailureIsConfirmedWithADifferentQuery(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	f.json("/repos/o/r/actions/workflows", `{"workflows":[{"id":7,"name":"Autoupdate","html_url":"w","state":"active"}]}`)
	calls := staleThenFresh(f, "/repos/o/r/actions/workflows/7/runs")
	wfs, err := c.workflows(context.Background(), "o/r", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(wfs) != 1 || wfs[0].Latest.ID != 3 || wfs[0].LastDone == nil || wfs[0].LastDone.Failed() {
		t.Fatalf("the newer success must supersede the stale failure: %+v", wfs)
	}
	if calls.Load() != 2 {
		t.Fatalf("runs requested %d times, want 2", calls.Load())
	}
}

func TestHealthyWorkflowIsNotRequestedTwice(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	f.json("/repos/o/r/actions/workflows", `{"workflows":[{"id":7,"name":"CI","html_url":"w","state":"active"}]}`)
	var calls atomic.Int64
	f.routes["/repos/o/r/actions/workflows/7/runs"] = func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"workflow_runs":[{"id":3,"event":"push","head_branch":"main","status":"completed","conclusion":"success","created_at":"2026-10-02T03:03:00Z"}]}`))
	}
	if _, err := c.workflows(context.Background(), "o/r", "main"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("a passing workflow needs one request, got %d", calls.Load())
	}
}

func TestConfirmedFailureStaysFailing(t *testing.T) {
	f := newFake(t)
	c := f.client(t, 10)
	f.json("/repos/o/r/actions/workflows", `{"workflows":[{"id":7,"name":"CI","html_url":"w","state":"active"}]}`)
	f.json("/repos/o/r/actions/workflows/7/runs", `{"workflow_runs":[{"id":3,"event":"push","head_branch":"main","status":"completed","conclusion":"failure","created_at":"2026-10-02T03:03:00Z"}]}`)
	wfs, err := c.workflows(context.Background(), "o/r", "main")
	if err != nil || len(wfs) != 1 || wfs[0].LastDone == nil || !wfs[0].LastDone.Failed() {
		t.Fatalf("a failure that both answers agree on must stay: %+v %v", wfs, err)
	}
}

func TestMergeRunsKeepsEachRunOnce(t *testing.T) {
	got := mergeRuns([]rawRun{{ID: 1}, {ID: 2}}, []rawRun{{ID: 2}, {ID: 3}})
	if len(got) != 3 || got[0].ID != 1 || got[1].ID != 2 || got[2].ID != 3 {
		t.Fatalf("%+v", got)
	}
}

func TestFreshClientDisablesConnectionReuse(t *testing.T) {
	base := &http.Client{Transport: &http.Transport{}, Timeout: 5 * time.Second}
	fc := freshClient(base)
	tr, ok := fc.Transport.(*http.Transport)
	if !ok || !tr.DisableKeepAlives || fc.Timeout != 5*time.Second {
		t.Fatalf("fresh client: %+v", fc)
	}
	if base.Transport.(*http.Transport).DisableKeepAlives {
		t.Fatal("the original client must keep its connections")
	}
	custom := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("x") })}
	if freshClient(custom) != custom {
		t.Fatal("non-standard transports are used as they are")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenPRsHeadSHAAndFork(t *testing.T) {
	tests := map[string]struct {
		pr       string
		wantFork bool
	}{
		"same repo":        {`"head":{"sha":"abc","repo":{"full_name":"o/r"}},"base":{"repo":{"full_name":"o/r"}}`, false},
		"same repo casing": {`"head":{"sha":"abc","repo":{"full_name":"O/R"}},"base":{"repo":{"full_name":"o/r"}}`, false},
		"other repo":       {`"head":{"sha":"abc","repo":{"full_name":"x/r"}},"base":{"repo":{"full_name":"o/r"}}`, true},
		"deleted head":     {`"head":{"sha":"abc","repo":null},"base":{"repo":{"full_name":"o/r"}}`, true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			c := f.client(t, 50)
			f.json("/repos/o/r/pulls", `[{"number":2,"title":"t","html_url":"u2","user":{"login":"b"},`+tc.pr+`}]`)
			prs, err := c.openPRs(context.Background(), "o/r")
			if err != nil || len(prs) != 1 {
				t.Fatalf("prs = %+v, err = %v", prs, err)
			}
			if prs[0].HeadSHA != "abc" || prs[0].Fork != tc.wantFork {
				t.Fatalf("HeadSHA = %q, Fork = %v, want abc, %v", prs[0].HeadSHA, prs[0].Fork, tc.wantFork)
			}
		})
	}
}
