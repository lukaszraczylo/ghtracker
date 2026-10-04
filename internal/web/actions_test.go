package web

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/lukaszraczylo/ghtracker/internal/config"
)

const (
	testToken = "tok-SECRET-123"
	alertURL  = "https://github.com/o/r/actions/runs/1"
	prURL     = "https://github.com/o/r/pull/5"
)

type fakeHook struct {
	srv        *httptest.Server
	postBody   string
	statBody   string
	lastAuth   string
	lastBody   string
	lastMethod string
	postCode   int
	statCode   int
	mu         sync.Mutex
	postCalls  atomic.Int32
	statCalls  atomic.Int32
}

func newFakeHook(t *testing.T) *fakeHook {
	t.Helper()
	f := &fakeHook{postCode: 200, postBody: `{"state":"queued","label":"Queued","link":"https://ci.example.test/run/9"}`, statCode: 200, statBody: `[]`}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.lastAuth, f.lastBody, f.lastMethod = r.Header.Get("Authorization"), string(b), r.Method
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/status" {
			f.statCalls.Add(1)
			w.WriteHeader(f.statCode)
			_, _ = w.Write([]byte(f.statBody))
			return
		}
		f.postCalls.Add(1)
		w.WriteHeader(f.postCode)
		_, _ = w.Write([]byte(f.postBody))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

type actionHarness struct {
	srv     *Server
	actions *Actions
	logs    *bytes.Buffer
	clock   time.Time
}

func newActionServer(t *testing.T, mutate func(*config.Action), base string) *actionHarness {
	t.Helper()
	c := config.Action{ID: "rerun", Label: "Re-run", Kinds: []string{"workflow_failed"}, Confirm: "Run it?",
		StatusURL: base + "/status", Token: testToken,
		Webhook: config.Webhook{URL: base + "/hook", TokenEnv: "T", Timeout: 2 * time.Second}}
	if mutate != nil {
		mutate(&c)
	}
	h := &actionHarness{logs: &bytes.Buffer{}, clock: now}
	log := slog.New(slog.NewTextHandler(h.logs, nil))
	h.actions = NewActions([]config.Action{c}, log, prometheus.NewRegistry())
	h.actions.now = func() time.Time { return h.clock }
	h.srv = New(&fakeSource{v: testView()}, http.NotFoundHandler(), testUI(), true, log).WithActions(h.actions)
	h.srv.now = func() time.Time { return now }
	return h
}

func (h *actionHarness) post(path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.srv.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *actionHarness) state(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(do(h.srv.Handler(), "GET", "/api/state").Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func reqBody(repo, url string) string {
	b, _ := json.Marshal(map[string]string{"repo": repo, "url": url})
	return string(b)
}

func (h *actionHarness) counter(result string) float64 {
	return testutil.ToFloat64(h.actions.counter.WithLabelValues("rerun", result))
}

func TestActionRequest(t *testing.T) {
	tests := []struct {
		hdr        map[string]string
		mutate     func(*config.Action)
		name       string
		path       string
		body       string
		postBody   string
		wantResult string
		wantBody   string
		postCode   int
		wantCode   int
		wantPosts  int32
	}{
		{name: "ok passes status through", wantCode: 200, wantPosts: 1, wantResult: "ok",
			wantBody: `"state":"queued"`},
		{name: "ok with empty body", postBody: "not json", wantCode: 200, wantPosts: 1, wantResult: "ok", wantBody: `"state":""`},
		{name: "repo case-insensitive", body: reqBody("O/R", alertURL), wantCode: 200, wantPosts: 1, wantResult: "ok"},
		{name: "javascript link dropped", postBody: `{"state":"x","link":"javascript:alert(1)"}`,
			wantCode: 200, wantPosts: 1, wantResult: "ok", wantBody: `{"state":"x"}`},
		{name: "no token configured", mutate: func(a *config.Action) { a.Token = "" }, wantCode: 200, wantPosts: 1, wantResult: "ok"},
		{name: "upstream 4xx", postCode: 429, postBody: `{"error":"busy"}`, wantCode: 429, wantPosts: 1, wantResult: "rejected", wantBody: "HTTP 429"},
		{name: "upstream 5xx", postCode: 500, postBody: "boom", wantCode: 500, wantPosts: 1, wantResult: "error", wantBody: "boom"},
		{name: "upstream redirect not followed", postCode: 302, postBody: "moved", wantCode: 502, wantPosts: 1, wantResult: "error"},
		{name: "upstream body truncated", postCode: 500, postBody: strings.Repeat("x", 1000), wantCode: 500, wantPosts: 1, wantResult: "error",
			wantBody: strings.Repeat("x", 300) + "..."},
		{name: "cross-site refused", hdr: map[string]string{"Sec-Fetch-Site": "cross-site"}, wantCode: 403},
		{name: "same-site refused", hdr: map[string]string{"Sec-Fetch-Site": "same-site"}, wantCode: 403},
		{name: "same-origin allowed", hdr: map[string]string{"Sec-Fetch-Site": "same-origin"}, wantCode: 200, wantPosts: 1, wantResult: "ok"},
		{name: "unknown action", path: "/api/actions/nope", wantCode: 404, wantBody: "unknown action"},
		{name: "unknown repo", body: reqBody("x/y", alertURL), wantCode: 404, wantBody: "unknown repository"},
		{name: "unknown alert", body: reqBody("o/r", alertURL+"9"), wantCode: 404, wantBody: "no such alert"},
		{name: "kind not in kinds", body: reqBody("o/r", prURL), wantCode: 404, wantBody: "no such alert"},
		{name: "empty kinds matches every kind", body: reqBody("o/r", prURL), mutate: func(a *config.Action) { a.Kinds = nil },
			wantCode: 200, wantPosts: 1, wantResult: "ok"},
		{name: "malformed body", body: `{`, wantCode: 400},
		{name: "missing url", body: `{"repo":"o/r"}`, wantCode: 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fh := newFakeHook(t)
			if tc.postCode != 0 {
				fh.postCode = tc.postCode
			}
			if tc.postBody != "" {
				fh.postBody = tc.postBody
			}
			h := newActionServer(t, tc.mutate, fh.srv.URL)
			path, body := tc.path, tc.body
			if path == "" {
				path = "/api/actions/rerun"
			}
			if body == "" {
				body = reqBody("o/r", alertURL)
			}
			rec := h.post(path, body, tc.hdr)
			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d: %s", rec.Code, tc.wantCode, rec.Body)
			}
			if fh.postCalls.Load() != tc.wantPosts {
				t.Fatalf("webhook posts = %d, want %d", fh.postCalls.Load(), tc.wantPosts)
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body %q lacks %q", rec.Body, tc.wantBody)
			}
			if tc.wantResult != "" && h.counter(tc.wantResult) != 1 {
				t.Fatalf("counter %s = %v, want 1", tc.wantResult, h.counter(tc.wantResult))
			}
			if tc.wantPosts > 0 {
				wantAuth := "Bearer " + testToken
				if tc.mutate != nil && tc.name == "no token configured" {
					wantAuth = ""
				}
				if fh.lastAuth != wantAuth {
					t.Fatalf("auth = %q, want %q", fh.lastAuth, wantAuth)
				}
			}
			if strings.Contains(h.logs.String(), testToken) || strings.Contains(rec.Body.String(), testToken) {
				t.Fatal("token leaked into logs or response")
			}
		})
	}
}

func TestActionWebhookPayload(t *testing.T) {
	fh := newFakeHook(t)
	h := newActionServer(t, nil, fh.srv.URL)
	if rec := h.post("/api/actions/rerun", reqBody("o/r", alertURL), nil); rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	var got struct {
		RequestedAt time.Time
		Alert       map[string]string
		Action      string
	}
	if err := json.Unmarshal([]byte(fh.lastBody), &got); err != nil {
		t.Fatal(err)
	}
	if fh.lastMethod != "POST" || got.Action != "rerun" || !got.RequestedAt.Equal(now) {
		t.Fatalf("payload: %+v (%s)", got, fh.lastBody)
	}
	want := map[string]string{"repo": "o/r", "kind": "workflow_failed", "severity": "crit", "subject": "CI", "url": alertURL}
	for k, v := range want {
		if got.Alert[k] != v {
			t.Errorf("alert.%s = %q, want %q", k, got.Alert[k], v)
		}
	}
	if got.Alert["detail"] == "" || got.Alert["since"] == "" {
		t.Errorf("alert lacks detail or since: %v", got.Alert)
	}
}

func TestActionWebhookDown(t *testing.T) {
	fh := newFakeHook(t)
	base := fh.srv.URL
	fh.srv.Close()
	h := newActionServer(t, nil, base)
	rec := h.post("/api/actions/rerun", reqBody("o/r", alertURL), nil)
	if rec.Code != http.StatusBadGateway || h.counter("error") != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if strings.Contains(h.logs.String(), testToken) {
		t.Fatal("token leaked into logs")
	}
}

func TestActionRouteAbsentWithoutActions(t *testing.T) {
	h := newServer(&fakeSource{v: testView()}, testUI(), true).Handler()
	if rec := do(h, "POST", "/api/actions/rerun"); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestStateUnchangedWithoutActions(t *testing.T) {
	body := do(newServer(&fakeSource{v: testView()}, testUI(), true).Handler(), "GET", "/api/state").Body.String()
	for _, key := range []string{`"actions"`, `"actionStatus"`, `"actionsError"`} {
		if strings.Contains(body, key) {
			t.Fatalf("state gained %s without actions: %s", key, body)
		}
	}
}

func alertField(m map[string]any, kind, field string) any {
	for _, a := range m["alerts"].([]any) {
		if al := a.(map[string]any); al["kind"] == kind {
			return al[field]
		}
	}
	return nil
}

func TestStateMergesActionStatus(t *testing.T) {
	tests := []struct {
		wantState any
		wantLink  any
		mutate    func(*config.Action)
		name      string
		rows      string
	}{
		{name: "match by repo and url", rows: `[{"repo":"O/R","url":"` + alertURL + `","state":"running","label":"Running","link":"https://ci.example.test/1"}]`,
			wantState: "running", wantLink: "https://ci.example.test/1"},
		{name: "first row wins", rows: `[{"repo":"o/r","url":"` + alertURL + `","state":"new"},{"repo":"o/r","url":"` + alertURL + `","state":"old"}]`,
			wantState: "new"},
		{name: "other url ignored", rows: `[{"repo":"o/r","url":"` + alertURL + `x","state":"running"}]`},
		{name: "other repo ignored", rows: `[{"repo":"o/other","url":"` + alertURL + `","state":"running"}]`},
		{name: "unsafe link dropped", rows: `[{"repo":"o/r","url":"` + alertURL + `","state":"done","link":"javascript:x"}]`, wantState: "done"},
		{name: "kind filter respected", rows: `[{"repo":"o/r","url":"` + alertURL + `","state":"running"}]`,
			mutate: func(a *config.Action) { a.Kinds = []string{"pr_waiting"} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fh := newFakeHook(t)
			fh.statBody = tc.rows
			m := newActionServer(t, tc.mutate, fh.srv.URL).state(t)
			if _, ok := m["actionsError"]; ok {
				t.Fatalf("unexpected actionsError: %v", m["actionsError"])
			}
			byAction, _ := alertField(m, "workflow_failed", "actionStatus").(map[string]any)
			st, _ := byAction["rerun"].(map[string]any)
			if tc.wantState == nil {
				if st != nil {
					t.Fatalf("status present: %v", st)
				}
				return
			}
			if st["state"] != tc.wantState || st["link"] != tc.wantLink {
				t.Fatalf("status = %v, want state %v link %v", st, tc.wantState, tc.wantLink)
			}
		})
	}
}

func TestStateListsActionsWithoutSecrets(t *testing.T) {
	fh := newFakeHook(t)
	h := newActionServer(t, nil, fh.srv.URL)
	rec := do(h.srv.Handler(), "GET", "/api/state")
	raw := rec.Body.String()
	for _, secret := range []string{testToken, fh.srv.URL, "/hook", "/status"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("state leaks %q", secret)
		}
	}
	var m struct {
		Actions []map[string]any
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil || len(m.Actions) != 1 {
		t.Fatalf("%v %s", err, raw)
	}
	a := m.Actions[0]
	if a["id"] != "rerun" || a["label"] != "Re-run" || a["confirm"] != "Run it?" || len(a["kinds"].([]any)) != 1 {
		t.Fatalf("action = %v", a)
	}
}

func TestStateStatusDown(t *testing.T) {
	tests := map[string]func(*fakeHook, *string){
		"server error": func(f *fakeHook, _ *string) { f.statCode = 500 },
		"invalid json": func(f *fakeHook, _ *string) { f.statBody = "nope" },
		"unreachable":  func(f *fakeHook, base *string) { f.srv.Close(); *base = f.srv.URL },
	}
	for name, breakIt := range tests {
		t.Run(name, func(t *testing.T) {
			fh := newFakeHook(t)
			base := fh.srv.URL
			breakIt(fh, &base)
			h := newActionServer(t, nil, base)
			m := h.state(t)
			if m["actionsError"] != actionsUnavail {
				t.Fatalf("actionsError = %v", m["actionsError"])
			}
			if alertField(m, "workflow_failed", "actionStatus") != nil {
				t.Fatal("actionStatus present while status is down")
			}
			if len(m["actions"].([]any)) != 1 {
				t.Fatal("actions must still be listed so the buttons work")
			}
			if strings.Contains(h.logs.String(), testToken) {
				t.Fatal("token leaked into logs")
			}
		})
	}
}

func TestStateWithoutStatusURL(t *testing.T) {
	fh := newFakeHook(t)
	h := newActionServer(t, func(a *config.Action) { a.StatusURL = "" }, fh.srv.URL)
	m := h.state(t)
	if _, ok := m["actionsError"]; ok || fh.statCalls.Load() != 0 || len(m["actions"].([]any)) != 1 {
		t.Fatalf("state: %v, status calls %d", m, fh.statCalls.Load())
	}
}

func TestStatusCachedAndInvalidatedByRequest(t *testing.T) {
	fh := newFakeHook(t)
	h := newActionServer(t, nil, fh.srv.URL)
	h.state(t)
	h.state(t)
	if fh.statCalls.Load() != 1 {
		t.Fatalf("status calls = %d, want 1 inside the TTL", fh.statCalls.Load())
	}
	h.clock = h.clock.Add(statusCacheTTL + time.Second)
	h.state(t)
	if fh.statCalls.Load() != 2 {
		t.Fatalf("status calls = %d, want 2 after the TTL", fh.statCalls.Load())
	}
	h.post("/api/actions/rerun", reqBody("o/r", alertURL), nil)
	h.state(t)
	if fh.statCalls.Load() != 3 {
		t.Fatalf("status calls = %d, want 3 after a request", fh.statCalls.Load())
	}
}

func TestRedirectDoesNotForwardToken(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Store(true)
		}
	}))
	t.Cleanup(other.Close)
	fh := newFakeHook(t)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(hook.Close)
	h := newActionServer(t, func(a *config.Action) { a.Webhook.URL = hook.URL }, fh.srv.URL)
	if rec := h.post("/api/actions/rerun", reqBody("o/r", alertURL), nil); rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d", rec.Code)
	}
	if leaked.Load() {
		t.Fatal("bearer token forwarded across a redirect")
	}
}
