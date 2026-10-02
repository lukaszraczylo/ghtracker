// Package gh is a small read-only GitHub REST client authenticated as a GitHub App.
package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/model"
)

const (
	acceptHeader = "application/vnd.github+json"
	apiVersion   = "2022-11-28"
	userAgent    = "ghtracker"
	perPage      = 100
	maxBody      = 16 << 20
	maxRetries   = 2
	// rateReserve is the request budget left untouched so the app never exhausts its hourly quota.
	rateReserve = 100
	// secondaryBackoff applies when GitHub signals abuse limits without a Retry-After.
	secondaryBackoff = time.Minute
)

// APIError is a non-2xx GitHub response.
type APIError struct {
	Message string
	Status  int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github: HTTP %d: %s", e.Status, e.Message)
}

func apiMessage(body []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &m) == nil && m.Message != "" {
		return m.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func isNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

// TokenSource yields a bearer token for requests against repo.
type TokenSource interface {
	Token(ctx context.Context, repo string) (string, error)
}

type Client struct {
	rateReset    time.Time
	blockedUntil time.Time
	auth         TokenSource
	hc           *http.Client
	// sleep is replaced in tests.
	sleep         func(context.Context, time.Duration) error
	now           func() time.Time
	cache         map[string]cachedResponse
	base          string
	rateRemaining atomic.Int64
	maxItems      int
	mu            sync.Mutex
}

// cachedResponse backs conditional requests: a 304 reply does not count against the primary rate limit.
type cachedResponse struct {
	etag string
	link string
	body []byte
}

func NewClient(baseURL string, auth TokenSource, hc *http.Client, maxItems int) *Client {
	c := &Client{base: strings.TrimRight(baseURL, "/"), hc: hc, auth: auth, maxItems: maxItems, sleep: sleepCtx, now: time.Now, cache: map[string]cachedResponse{}}
	c.rateRemaining.Store(-1)
	return c
}

// RateLimitRemaining returns the last reported remaining request budget.
func (c *Client) RateLimitRemaining() (int, bool) {
	v := c.rateRemaining.Load()
	return int(v), v >= 0
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// do performs one authenticated GET, retrying transient failures, and returns the body and Link header.
func (c *Client) do(ctx context.Context, repo, target string) ([]byte, string, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, time.Duration(attempt)*time.Second); err != nil {
				return nil, "", err
			}
		}
		body, link, retry, err := c.once(ctx, repo, target)
		if err == nil {
			return body, link, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return nil, "", lastErr
}

func (c *Client) once(ctx context.Context, repo, target string) (body []byte, link string, retry bool, err error) {
	if gerr := c.guard(); gerr != nil {
		return nil, "", false, gerr
	}
	tok, err := c.auth.Token(ctx, repo)
	if err != nil {
		return nil, "", false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)
	c.mu.Lock()
	cached, haveCache := c.cache[target]
	c.mu.Unlock()
	if haveCache {
		req.Header.Set("If-None-Match", cached.etag)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, "", ctx.Err() == nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	c.noteRate(resp.Header)
	if resp.StatusCode == http.StatusNotModified && haveCache {
		return cached.body, cached.link, false, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, "", true, err
	}
	if resp.StatusCode/100 != 2 {
		ae := &APIError{Status: resp.StatusCode, Message: apiMessage(data)}
		if c.noteLimited(resp, ae) {
			return nil, "", false, fmt.Errorf("%w: %v", model.ErrRateLimited, ae)
		}
		return nil, "", resp.StatusCode >= 500, ae
	}
	link = resp.Header.Get("Link")
	if etag := resp.Header.Get("ETag"); etag != "" {
		c.mu.Lock()
		c.cache[target] = cachedResponse{etag: etag, body: data, link: link}
		c.mu.Unlock()
	}
	return data, link, false, nil
}

// guard refuses requests while a limit block is active or the remaining budget is at the reserve.
func (c *Client) guard() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if now.Before(c.blockedUntil) {
		return fmt.Errorf("%w: blocked until %s", model.ErrRateLimited, c.blockedUntil.Format(time.RFC3339))
	}
	if rem := c.rateRemaining.Load(); rem >= 0 && rem < rateReserve && now.Before(c.rateReset) {
		return fmt.Errorf("%w: %d requests left, resets %s", model.ErrRateLimited, rem, c.rateReset.Format(time.RFC3339))
	}
	return nil
}

func (c *Client) noteRate(h http.Header) {
	if v, err := strconv.ParseInt(h.Get("X-RateLimit-Remaining"), 10, 64); err == nil {
		c.rateRemaining.Store(v)
	}
	if v, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		c.mu.Lock()
		c.rateReset = time.Unix(v, 0)
		c.mu.Unlock()
	}
}

// noteLimited records a block when the response is a primary or secondary rate-limit rejection.
func (c *Client) noteLimited(resp *http.Response, ae *APIError) bool {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return false
	}
	now := c.now()
	var until time.Time
	switch secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); {
	case err == nil:
		until = now.Add(time.Duration(secs) * time.Second)
	case resp.Header.Get("X-RateLimit-Remaining") == "0":
		c.mu.Lock()
		until = c.rateReset
		c.mu.Unlock()
	case strings.Contains(strings.ToLower(ae.Message), "rate limit"):
		until = now.Add(secondaryBackoff)
	default:
		return false
	}
	c.mu.Lock()
	c.blockedUntil = until
	c.mu.Unlock()
	return true
}

func (c *Client) url(path string, q url.Values) string {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (c *Client) getJSON(ctx context.Context, repo, path string, q url.Values, out any) error {
	body, _, err := c.do(ctx, repo, c.url(path, q))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// listAll follows Link pagination, collecting at most limit items.
func listAll[T any](ctx context.Context, c *Client, repo, path string, q url.Values, limit int, unwrap func([]byte) ([]T, error)) ([]T, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("per_page", strconv.Itoa(perPage))
	next := c.url(path, q)
	var all []T
	for next != "" && len(all) < limit {
		body, link, err := c.do(ctx, repo, next)
		if err != nil {
			return all, err
		}
		page, err := unwrap(body)
		if err != nil {
			return all, err
		}
		all = append(all, page...)
		next = nextLink(link)
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func unwrapArray[T any](b []byte) ([]T, error) {
	var out []T
	return out, json.Unmarshal(b, &out)
}

// nextLink extracts rel="next" from a Link header.
func nextLink(h string) string {
	for part := range strings.SplitSeq(h, ",") {
		target, params, found := strings.Cut(part, ";")
		if found && strings.TrimSpace(params) == `rel="next"` {
			return strings.Trim(strings.TrimSpace(target), "<>")
		}
	}
	return ""
}
