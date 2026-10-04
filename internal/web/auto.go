package web

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/lukaszraczylo/ghtracker/internal/collector"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

// maxAutoSends caps webhook calls per refresh so a first start cannot flood the consumer.
const maxAutoSends = 5

type autoKey struct{ action, repo, url, ref string }

// AutoRunner fires auto actions once per (action, repo, url, head commit).
type AutoRunner struct {
	actions *Actions
	sent    map[autoKey]struct{}
	mu      sync.Mutex
}

// NewAutoRunner returns a runner for the auto actions in a, or nil when none is configured.
func NewAutoRunner(a *Actions) *AutoRunner {
	for _, rt := range a.list {
		if rt.cfg.Auto {
			return &AutoRunner{actions: a, sent: map[autoKey]struct{}{}}
		}
	}
	return nil
}

// Run sends the webhooks that are due for the alerts in v and forgets the ones whose alert is gone.
func (r *AutoRunner) Run(ctx context.Context, v collector.View) {
	r.mu.Lock()
	defer r.mu.Unlock()
	live := map[autoKey]struct{}{}
	budget := maxAutoSends
	for _, rt := range r.actions.list {
		if !rt.cfg.Auto {
			continue
		}
		for _, al := range v.Alerts {
			if !rt.cfg.AppliesTo(al.Kind) || len(rt.cfg.Authors) > 0 && !rt.cfg.AuthorAllowed(al.Author) {
				continue
			}
			k := autoKey{rt.cfg.ID, strings.ToLower(al.Repo), al.URL, al.Ref}
			live[k] = struct{}{}
			if _, done := r.sent[k]; done || budget == 0 || ctx.Err() != nil {
				continue
			}
			budget--
			if r.send(ctx, rt, al, v) {
				r.sent[k] = struct{}{}
			}
		}
	}
	for k := range r.sent {
		if _, ok := live[k]; !ok {
			delete(r.sent, k)
		}
	}
}

// send posts one alert and reports whether it counts as delivered; 409 means busy and is retried.
func (r *AutoRunner) send(ctx context.Context, rt *actionRuntime, al model.Alert, v collector.View) bool {
	a := r.actions
	status, _, err := a.post(ctx, rt.cfg, newWebhookPayload(rt.cfg.ID, al, v.Now))
	switch {
	case err != nil:
		a.count(rt.cfg.ID, resultAutoFailed)
		a.log.Warn("auto action webhook failed", "action", rt.cfg.ID, "repo", al.Repo, "err", err)
		return false
	case status == http.StatusConflict:
		a.count(rt.cfg.ID, resultAutoBusy)
		a.log.Info("auto action busy, will retry", "action", rt.cfg.ID, "repo", al.Repo)
		return false
	case status/100 == 2:
		a.count(rt.cfg.ID, resultAutoOK)
		rt.invalidate()
		a.log.Info("auto action sent", "action", rt.cfg.ID, "repo", al.Repo, "ref", al.Ref)
		return true
	}
	result := resultAutoFailed
	if status/100 == 4 {
		result = resultAutoRefused
	}
	a.count(rt.cfg.ID, result)
	a.log.Warn("auto action webhook answered", "action", rt.cfg.ID, "repo", al.Repo, "status", status)
	return false
}
