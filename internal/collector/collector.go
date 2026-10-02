// Package collector refreshes repository data on a schedule and evaluates health from it.
package collector

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/config"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

const repoFetchTimeout = 5 * time.Minute

var (
	// ErrUnknownRepo means the name is not in the configured repository list.
	ErrUnknownRepo = errors.New("repository is not configured")
	// ErrRefreshBusy means a full refresh or a refresh of the same repository is running.
	ErrRefreshBusy = errors.New("a refresh is already running")
)

// Fetcher loads the data for one "owner/name" repository.
type Fetcher interface {
	FetchRepo(ctx context.Context, full string) (model.Repo, error)
}

// View is a consistent, health-evaluated read of the collector state.
type View struct {
	Now         time.Time
	LastRefresh time.Time
	Repos       []model.Repo
	Alerts      []model.Alert
	Thresholds  config.Thresholds
	Interval    time.Duration
	Crit        int
	Warn        int
	Loaded      bool
	Refreshing  bool
	// ScanDone and ScanTotal count the repos of the running (or last) scan.
	ScanDone  int
	ScanTotal int
}

// Stats are monotonic counters and timings of the refresh loop.
type Stats struct {
	LastRefresh     time.Time
	LastDuration    time.Duration
	RefreshTotal    uint64
	RepoErrorsTotal uint64
	Loaded          bool
	RefreshInFlight bool
}

type Collector struct {
	lastRefresh time.Time
	fetch       Fetcher
	repos       map[string]model.Repo
	inflight    map[string]struct{}
	now         func() time.Time
	log         *slog.Logger
	webBase     string
	kick        chan struct{}
	names       []string
	th          config.Thresholds
	interval    time.Duration
	refreshes   atomic.Uint64
	scanDone    atomic.Int64
	scanTotal   atomic.Int64
	repoErrs    atomic.Uint64
	lastDur     time.Duration
	concurrency int
	mu          sync.RWMutex
	limited     atomic.Bool
	refreshing  atomic.Bool
	loaded      bool
}

func New(f Fetcher, cfg *config.Config, log *slog.Logger) *Collector {
	return &Collector{
		fetch:       f,
		names:       cfg.Repos,
		th:          cfg.Thresholds,
		interval:    cfg.RefreshInterval,
		concurrency: cfg.Concurrency,
		now:         time.Now,
		webBase:     config.WebBase(cfg.GitHub.APIURL),
		log:         log,
		kick:        make(chan struct{}, 1),
		repos:       make(map[string]model.Repo, len(cfg.Repos)),
		inflight:    make(map[string]struct{}),
	}
}

// Run refreshes immediately, then on every interval or kick, until ctx ends.
func (c *Collector) Run(ctx context.Context) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	c.Refresh(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.Refresh(ctx)
		case <-c.kick:
			c.Refresh(ctx)
		}
	}
}

// RequestRefresh asks Run for an extra refresh; it reports false when one is already running or queued.
func (c *Collector) RequestRefresh() bool {
	if c.refreshing.Load() {
		return false
	}
	select {
	case c.kick <- struct{}{}:
		return true
	default:
		return false
	}
}

// Refresh fetches every repo once; a failing repo keeps its last good data and is marked down.
func (c *Collector) Refresh(ctx context.Context) {
	if !c.refreshing.CompareAndSwap(false, true) {
		return
	}
	defer c.refreshing.Store(false)

	start := c.now()
	c.limited.Store(false)
	c.scanDone.Store(0)
	c.scanTotal.Store(int64(len(c.names)))
	results := make([]model.Repo, len(c.names))
	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup
	for i, name := range c.names {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = c.refreshOne(ctx, name, start, true)
			c.scanDone.Add(1)
		}()
	}
	wg.Wait()

	c.mu.Lock()
	for _, r := range results {
		c.repos[r.FullName] = r
	}
	c.lastRefresh = start
	c.lastDur = c.now().Sub(start)
	c.loaded = true
	c.mu.Unlock()
	c.refreshes.Add(1)
	c.log.Info("refresh complete", "repos", len(c.names), "duration", c.lastDur.Round(time.Millisecond))
}

// refreshOne fetches one repo. With respectLimit set, it skips the fetch once the scan hit a rate limit.
func (c *Collector) refreshOne(ctx context.Context, name string, at time.Time, respectLimit bool) model.Repo {
	var r model.Repo
	err := model.ErrRateLimited
	// Once any repo hits the limit, remaining repos are skipped so no further requests go out.
	if !respectLimit || !c.limited.Load() {
		r, err = c.fetchRepo(ctx, name)
	}
	if respectLimit && errors.Is(err, model.ErrRateLimited) {
		c.limited.Store(true)
	}
	return c.settle(name, at, r, err)
}

func (c *Collector) fetchRepo(ctx context.Context, name string) (model.Repo, error) {
	ctx, cancel := context.WithTimeout(ctx, repoFetchTimeout)
	defer cancel()
	return c.fetch.FetchRepo(ctx, name)
}

// settle turns a fetch result into the repo to store; a failure keeps the last good data.
func (c *Collector) settle(name string, at time.Time, r model.Repo, err error) model.Repo {
	if err == nil {
		r.FullName, r.Up, r.RefreshedAt = name, true, at
		return r
	}
	c.repoErrs.Add(1)
	c.log.Error("repo refresh failed", "repo", name, "err", err)
	c.mu.RLock()
	prev, ok := c.repos[name]
	c.mu.RUnlock()
	if !ok {
		prev = model.Repo{FullName: name, URL: c.webBase + "/" + name}
	}
	prev.Up, prev.Err = false, err.Error()
	return prev
}

// RefreshRepo fetches one configured repo now and replaces only its state, leaving the schedule,
// last scan time and other repos untouched. It fails with ErrRefreshBusy while a full refresh or a
// refresh of the same repo runs. The GitHub client still enforces the rate-limit reserve.
func (c *Collector) RefreshRepo(ctx context.Context, name string) (View, error) {
	canon, ok := c.configured(name)
	if !ok {
		return View{}, ErrUnknownRepo
	}
	if c.refreshing.Load() || !c.claim(canon) {
		return View{}, ErrRefreshBusy
	}
	defer c.release(canon)

	at := c.now()
	r, err := c.fetchRepo(ctx, canon)
	if err != nil && ctx.Err() != nil {
		return View{}, ctx.Err()
	}
	r = c.settle(canon, at, r, err)
	c.mu.Lock()
	c.repos[canon] = r
	c.mu.Unlock()
	return c.View(), nil
}

func (c *Collector) configured(name string) (string, bool) {
	for _, n := range c.names {
		if strings.EqualFold(n, name) {
			return n, true
		}
	}
	return "", false
}

func (c *Collector) claim(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, busy := c.inflight[name]; busy {
		return false
	}
	c.inflight[name] = struct{}{}
	return true
}

func (c *Collector) release(name string) {
	c.mu.Lock()
	delete(c.inflight, name)
	c.mu.Unlock()
}

// Stats returns refresh-loop counters.
func (c *Collector) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Stats{
		LastRefresh:     c.lastRefresh,
		LastDuration:    c.lastDur,
		RefreshTotal:    c.refreshes.Load(),
		RepoErrorsTotal: c.repoErrs.Load(),
		Loaded:          c.loaded,
		RefreshInFlight: c.refreshing.Load(),
	}
}

// View evaluates all repos at now. Repos not yet fetched appear with no data and are not alerted on.
func (c *Collector) View() View {
	now := c.now()
	c.mu.RLock()
	v := View{
		Now: now, LastRefresh: c.lastRefresh, Interval: c.interval, Thresholds: c.th,
		Loaded: c.loaded, Refreshing: c.refreshing.Load(),
		ScanDone: int(c.scanDone.Load()), ScanTotal: int(c.scanTotal.Load()),
	}
	v.Repos = make([]model.Repo, 0, len(c.names))
	for _, n := range c.names {
		if r, ok := c.repos[n]; ok {
			v.Repos = append(v.Repos, r)
		}
	}
	c.mu.RUnlock()

	for i := range v.Repos {
		v.Alerts = append(v.Alerts, Evaluate(&v.Repos[i], now, c.th)...)
	}
	sortAlerts(v.Alerts)
	for _, a := range v.Alerts {
		switch a.Severity {
		case model.Crit:
			v.Crit++
		case model.Warn:
			v.Warn++
		}
	}
	// Worst repos first so problems surface at the top of the dashboard.
	slices.SortStableFunc(v.Repos, func(a, b model.Repo) int { return int(b.Health) - int(a.Health) })
	return v
}
