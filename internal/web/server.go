// Package web serves the embedded UI, the JSON state, the health probe and metrics.
package web

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/collector"
)

const (
	minManualRefreshGap = 5 * time.Minute
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
}

type Server struct {
	lastManual    time.Time
	now           func() time.Time
	src           Source
	ui            fs.FS
	metrics       http.Handler
	log           *slog.Logger
	mu            sync.Mutex
	manualRefresh bool
}

// New builds the server; ui is the built frontend (index.html at its root).
func New(src Source, metrics http.Handler, ui fs.FS, manualRefresh bool, log *slog.Logger) *Server {
	return &Server{src: src, ui: ui, metrics: metrics, manualRefresh: manualRefresh, log: log, now: time.Now}
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

func (s *Server) state(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(buildState(s.src.View(), s.manualRefresh)); err != nil {
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
	// Browsers label cross-site requests; refuse them so another page cannot spend the API budget.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		http.Error(w, "cross-site refresh refused", http.StatusForbidden)
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
