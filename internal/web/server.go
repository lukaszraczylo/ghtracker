// Package web serves the embedded UI, the JSON state, the health probe and metrics.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/collector"
)

const (
	minManualRefreshGap = 5 * time.Minute
	// repoRefreshCooldown limits each repository separately from the full-refresh gap.
	repoRefreshCooldown = 60 * time.Second
	indexFile           = "index.html"
	assetsPrefix        = "/assets/"
	immutableCache      = "public, max-age=31536000, immutable"
	contentSecurity     = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"connect-src 'self'; img-src 'self' data:; font-src 'self'; form-action 'self'; base-uri 'none'"
)

// Source supplies dashboard state and accepts refresh requests.
type Source interface {
	View() collector.View
	Stats() collector.Stats
	RequestRefresh() bool
	RefreshRepo(ctx context.Context, name string) (collector.View, error)
}

type Server struct {
	lastManual    time.Time
	lastRepo      map[string]time.Time
	now           func() time.Time
	src           Source
	actions       *Actions
	ui            fs.FS
	metrics       http.Handler
	log           *slog.Logger
	mu            sync.Mutex
	manualRefresh bool
}

// New builds the server; ui is the built frontend (index.html at its root).
func New(src Source, metrics http.Handler, ui fs.FS, manualRefresh bool, log *slog.Logger) *Server {
	return &Server{src: src, ui: ui, metrics: metrics, manualRefresh: manualRefresh, log: log, now: time.Now,
		lastRepo: make(map[string]time.Time)}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /assets/", s.asset)
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("GET /metrics", s.metrics)
	if s.manualRefresh {
		mux.HandleFunc("POST /refresh", s.refresh)
	}
	if s.actions != nil {
		mux.HandleFunc("POST /api/actions/{id}", s.runAction)
	}
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurity)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	b, err := fs.ReadFile(s.ui, indexFile)
	if err != nil {
		http.Error(w, "UI is not built: run `pnpm build` before `go build`", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// asset serves hashed build output under /assets/ from the embedded filesystem.
func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if !strings.HasPrefix("/"+name, assetsPrefix) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", immutableCache)
	http.ServeFileFS(w, r, s.ui, name)
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	st := buildState(s.src.View(), s.manualRefresh)
	s.decorate(r.Context(), &st)
	if err := json.NewEncoder(w).Encode(st); err != nil {
		s.log.Error("encode state", "err", err)
	}
}

// health reports 503 until the first refresh finishes, for probes.
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	if !s.src.Stats().Loaded {
		http.Error(w, "loading", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	if crossSite(r) {
		http.Error(w, "cross-site refresh refused", http.StatusForbidden)
		return
	}
	if repo := r.URL.Query().Get("repo"); repo != "" {
		s.refreshRepo(w, r, repo)
		return
	}
	s.mu.Lock()
	tooSoon := s.now().Sub(s.lastManual) < minManualRefreshGap
	if !tooSoon {
		s.lastManual = s.now()
	}
	s.mu.Unlock()
	if tooSoon {
		http.Error(w, "refresh allowed once per "+minManualRefreshGap.String(), http.StatusTooManyRequests)
		return
	}
	s.src.RequestRefresh()
	w.WriteHeader(http.StatusAccepted)
}

// crossSite reports whether the browser labelled the request as cross-site, so another page
// cannot spend the API budget or fire a webhook.
func crossSite(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	return site != "" && site != "same-origin" && site != "none"
}

// repoRefreshResponse is the full state after the refresh, so the UI can swap it in, plus the repo name.
type repoRefreshResponse struct {
	Repo string `json:"repo"`
	apiState
}

// refreshRepo refreshes one repository and answers when it finishes. It has its own cooldown and
// never touches the full-refresh gap. A running full refresh or same-repo refresh gives 409.
func (s *Server) refreshRepo(w http.ResponseWriter, r *http.Request, repo string) {
	key := strings.ToLower(repo)
	s.mu.Lock()
	prev, had := s.lastRepo[key]
	wait := repoRefreshCooldown - s.now().Sub(prev)
	if had && wait > 0 {
		s.mu.Unlock()
		w.Header().Set("Retry-After", strconv.Itoa(int((wait+time.Second-1)/time.Second)))
		writeJSONError(w, http.StatusTooManyRequests, "repository refresh allowed once per "+repoRefreshCooldown.String())
		return
	}
	s.lastRepo[key] = s.now()
	s.mu.Unlock()

	v, err := s.src.RefreshRepo(r.Context(), repo)
	if err != nil {
		// A refused or cancelled attempt spent no GitHub budget, so it must not start a cooldown.
		s.mu.Lock()
		if had {
			s.lastRepo[key] = prev
		} else {
			delete(s.lastRepo, key)
		}
		s.mu.Unlock()
		switch {
		case errors.Is(err, collector.ErrUnknownRepo):
			writeJSONError(w, http.StatusNotFound, "unknown repository")
		case errors.Is(err, collector.ErrRefreshBusy):
			writeJSONError(w, http.StatusConflict, "a refresh is already running; try again shortly")
		default:
			s.log.Error("refresh repo", "repo", repo, "err", err)
			writeJSONError(w, http.StatusInternalServerError, "refresh failed")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	st := buildState(v, s.manualRefresh)
	s.decorate(r.Context(), &st)
	if err := json.NewEncoder(w).Encode(repoRefreshResponse{Repo: repo, apiState: st}); err != nil {
		s.log.Error("encode repo refresh", "err", err)
	}
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
