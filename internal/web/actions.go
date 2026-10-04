package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/lukaszraczylo/ghtracker/internal/config"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

const (
	statusCacheTTL    = 10 * time.Second
	maxActionBody     = 4 << 10
	maxUpstreamBody   = 1 << 20
	maxErrorChars     = 300
	actionsUnavail    = "action status unavailable"
	resultOK          = "ok"
	resultRejected    = "rejected"
	resultFailed      = "error"
	actionMetricName  = "action_requests_total"
	contentTypeHeader = "Content-Type"
)

// apiAction is the public description of an action; it never carries URLs or tokens.
type apiAction struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Confirm string   `json:"confirm,omitempty"`
	Kinds   []string `json:"kinds"`
}

// apiActionStatus is what the UI shows next to an alert after an action ran.
type apiActionStatus struct {
	State string `json:"state"`
	Label string `json:"label,omitempty"`
	Link  string `json:"link,omitempty"`
}

type statusRow struct {
	Repo string `json:"repo"`
	URL  string `json:"url"`
	apiActionStatus
}

type alertRef struct{ repo, url string }

// actionRuntime holds one action and the cache of its status rows.
type actionRuntime struct {
	fetched time.Time
	rows    map[alertRef]apiActionStatus
	err     error
	cfg     config.Action
	mu      sync.Mutex
}

// Actions runs the configured webhook actions and merges their status into the state.
type Actions struct {
	now     func() time.Time
	hc      *http.Client
	log     *slog.Logger
	counter *prometheus.CounterVec
	byID    map[string]*actionRuntime
	list    []*actionRuntime
}

// NewActions builds the runner and registers ghtracker_action_requests_total on reg when reg is not nil.
func NewActions(cfgs []config.Action, log *slog.Logger, reg prometheus.Registerer) *Actions {
	a := &Actions{
		now: time.Now, log: log, byID: make(map[string]*actionRuntime, len(cfgs)),
		// Redirects would carry the bearer token to another host, so none are followed.
		hc: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		counter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ghtracker", Name: actionMetricName,
			Help: "Action requests sent to webhooks, by action and result.",
		}, []string{"action", "result"}),
	}
	for _, c := range cfgs {
		rt := &actionRuntime{cfg: c}
		a.list = append(a.list, rt)
		a.byID[c.ID] = rt
		for _, r := range []string{resultOK, resultRejected, resultFailed} {
			a.counter.WithLabelValues(c.ID, r)
		}
	}
	if reg != nil {
		reg.MustRegister(a.counter)
	}
	return a
}

// WithActions enables POST /api/actions/{id} and the action data in the state document.
func (s *Server) WithActions(a *Actions) *Server {
	if a != nil && len(a.list) > 0 {
		s.actions = a
	}
	return s
}

// decorate adds the action list and per-alert status to a state document.
func (s *Server) decorate(ctx context.Context, st *apiState) {
	if s.actions == nil {
		return
	}
	for _, rt := range s.actions.list {
		kinds := rt.cfg.Kinds
		if kinds == nil {
			kinds = []string{}
		}
		st.Actions = append(st.Actions, apiAction{ID: rt.cfg.ID, Label: rt.cfg.Label, Confirm: rt.cfg.Confirm, Kinds: kinds})
		if rt.cfg.StatusURL == "" {
			continue
		}
		rows, err := s.actions.statusRows(ctx, rt)
		if err != nil {
			st.ActionsError = actionsUnavail
			continue
		}
		for i := range st.Alerts {
			al := &st.Alerts[i]
			if !rt.cfg.AppliesTo(al.Kind) {
				continue
			}
			if row, ok := rows[alertRef{strings.ToLower(al.Repo), al.URL}]; ok {
				if al.ActionStatus == nil {
					al.ActionStatus = map[string]apiActionStatus{}
				}
				al.ActionStatus[rt.cfg.ID] = row
			}
		}
	}
}

// statusRows returns the cached rows of an action's status_url; failures are cached for the TTL too.
func (a *Actions) statusRows(ctx context.Context, rt *actionRuntime) (map[alertRef]apiActionStatus, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.fetched.IsZero() && a.now().Sub(rt.fetched) < statusCacheTTL {
		return rt.rows, rt.err
	}
	rows, err := a.fetchStatus(ctx, rt.cfg)
	if err != nil {
		a.log.Warn("action status unavailable", "action", rt.cfg.ID, "err", err)
	}
	rt.rows, rt.err, rt.fetched = rows, err, a.now()
	return rows, err
}

func (rt *actionRuntime) invalidate() {
	rt.mu.Lock()
	rt.fetched = time.Time{}
	rt.mu.Unlock()
}

func (a *Actions) fetchStatus(ctx context.Context, c config.Action) (map[alertRef]apiActionStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Webhook.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.StatusURL, nil)
	if err != nil {
		return nil, errors.New("invalid status url")
	}
	req.Header.Set("Accept", "application/json")
	setBearer(req, c.Token)
	resp, err := a.hc.Do(req)
	if err != nil {
		return nil, errors.New("request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var list []statusRow
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxUpstreamBody)).Decode(&list); err != nil {
		return nil, errors.New("invalid reply")
	}
	out := make(map[alertRef]apiActionStatus, len(list))
	for _, r := range list { // the first row per alert wins, so put the newest first
		k := alertRef{strings.ToLower(r.Repo), r.URL}
		if _, seen := out[k]; !seen {
			out[k] = cleanStatus(r.apiActionStatus)
		}
	}
	return out, nil
}

// cleanStatus drops a link that is not http(s), so the UI never renders a script URL as an anchor.
func cleanStatus(st apiActionStatus) apiActionStatus {
	if u, err := url.Parse(st.Link); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		st.Link = ""
	}
	return st
}

func setBearer(req *http.Request, token string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

type actionRequest struct {
	Repo string `json:"repo"`
	URL  string `json:"url"`
}

type webhookAlert struct {
	Since    time.Time `json:"since"`
	Repo     string    `json:"repo"`
	Kind     string    `json:"kind"`
	Severity string    `json:"severity"`
	Subject  string    `json:"subject"`
	Detail   string    `json:"detail"`
	URL      string    `json:"url"`
}

type webhookPayload struct {
	RequestedAt time.Time    `json:"requestedAt"`
	Action      string       `json:"action"`
	Alert       webhookAlert `json:"alert"`
}

// runAction validates the request against the current state and forwards it to the action's webhook.
func (s *Server) runAction(w http.ResponseWriter, r *http.Request) {
	if crossSite(r) {
		http.Error(w, "cross-site action refused", http.StatusForbidden)
		return
	}
	rt, ok := s.actions.byID[r.PathValue("id")]
	if !ok {
		writeJSONError(w, http.StatusNotFound, "unknown action")
		return
	}
	var req actionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxActionBody)).Decode(&req); err != nil || req.Repo == "" || req.URL == "" {
		writeJSONError(w, http.StatusBadRequest, `body must be {"repo":"owner/name","url":"..."}`)
		return
	}
	alert, code, msg := s.findAlert(rt.cfg, req)
	if code != 0 {
		writeJSONError(w, code, msg)
		return
	}
	status, body, err := s.actions.post(r.Context(), rt.cfg, webhookPayload{
		Action: rt.cfg.ID, RequestedAt: s.now().UTC(),
		Alert: webhookAlert{Repo: alert.Repo, Kind: alert.Kind, Severity: alert.Severity.String(), Subject: alert.Subject,
			Detail: alert.Detail, URL: alert.URL, Since: alert.Since},
	})
	if err != nil {
		s.actions.count(rt.cfg.ID, resultFailed)
		s.log.Warn("action webhook failed", "action", rt.cfg.ID, "repo", alert.Repo, "err", err)
		writeJSONError(w, http.StatusBadGateway, "action webhook unreachable")
		return
	}
	s.log.Info("action webhook", "action", rt.cfg.ID, "repo", alert.Repo, "status", status)
	if status/100 != 2 {
		result, out := resultFailed, http.StatusBadGateway
		switch status / 100 {
		case 4:
			result, out = resultRejected, status
		case 5:
			out = status
		}
		s.actions.count(rt.cfg.ID, result)
		writeJSONError(w, out, fmt.Sprintf("action webhook answered HTTP %d: %s", status, truncate(string(body), maxErrorChars)))
		return
	}
	s.actions.count(rt.cfg.ID, resultOK)
	rt.invalidate()
	var st apiActionStatus
	if json.Unmarshal(body, &st) != nil {
		st = apiActionStatus{}
	}
	w.Header().Set(contentTypeHeader, "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(cleanStatus(st))
}

// findAlert returns the current alert a request targets, or a status and message when it has none.
func (s *Server) findAlert(c config.Action, req actionRequest) (model.Alert, int, string) {
	v := s.src.View()
	tracked := false
	for _, repo := range v.Repos {
		if strings.EqualFold(repo.FullName, req.Repo) {
			tracked = true
			break
		}
	}
	if !tracked {
		return model.Alert{}, http.StatusNotFound, "unknown repository"
	}
	for _, a := range v.Alerts {
		if strings.EqualFold(a.Repo, req.Repo) && a.URL == req.URL && c.AppliesTo(a.Kind) {
			return a, 0, ""
		}
	}
	return model.Alert{}, http.StatusNotFound, "no such alert"
}

func (a *Actions) post(ctx context.Context, c config.Action, p webhookPayload) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Webhook.Timeout)
	defer cancel()
	payload, err := json.Marshal(p)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Webhook.URL, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, errors.New("invalid webhook url")
	}
	req.Header.Set(contentTypeHeader, "application/json")
	setBearer(req, c.Token)
	resp, err := a.hc.Do(req)
	if err != nil {
		return 0, nil, errors.New("request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBody))
	if err != nil {
		return 0, nil, errors.New("read reply failed")
	}
	return resp.StatusCode, body, nil
}

func (a *Actions) count(action, result string) { a.counter.WithLabelValues(action, result).Inc() }

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}
