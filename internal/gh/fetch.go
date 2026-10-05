package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/model"
)

const (
	maxFailingNames = 5
	runsPerWorkflow = 5
	// freshRunsWindowDays bounds the created filter of the confirming run query.
	freshRunsWindowDays = 30
	maxWorkflows        = 100
	eventPullRequest    = "pull_request"
	eventPullReqTgt     = "pull_request_target"
	statusStateFail     = "failure"
	statusStateError    = "error"
	statusStatePend     = "pending"
)

// backgroundEvents are triggers that run without a person pushing code.
var backgroundEvents = []string{"schedule", "dynamic", "workflow_dispatch", "workflow_run", "repository_dispatch"}

type userRef struct {
	Login string `json:"login"`
}

type labelRef struct {
	Name string `json:"name"`
}

func labelNames(ls []labelRef) []string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.Name)
	}
	return out
}

// FetchRepo gathers the dashboard data for one "owner/name" repository.
// A failure of the repository lookup fails the call; other sections degrade into Repo.Warnings.
func (c *Client) FetchRepo(ctx context.Context, full string) (model.Repo, error) {
	var info struct {
		HTMLURL       string `json:"html_url"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Archived      bool   `json:"archived"`
	}
	if err := c.getJSON(ctx, full, "/repos/"+full, nil, &info); err != nil {
		return model.Repo{}, fmt.Errorf("repo %s: %w", full, err)
	}
	r := model.Repo{
		FullName:      full,
		URL:           info.HTMLURL,
		ReleasesURL:   info.HTMLURL + "/releases",
		Description:   info.Description,
		DefaultBranch: info.DefaultBranch,
		Archived:      info.Archived,
		Up:            true,
	}
	var limited error
	warn := func(section string, err error) {
		if errors.Is(err, model.ErrRateLimited) {
			limited = err
		}
		r.Warnings = append(r.Warnings, fmt.Sprintf("%s: %v", section, err))
	}

	var err error
	if r.Issues, err = c.openIssues(ctx, full); err != nil {
		warn("issues", err)
	}
	if r.PRs, err = c.openPRs(ctx, full); err != nil {
		warn("pull requests", err)
	}
	if cerr := c.attachChecks(ctx, full, r.PRs); cerr != nil {
		warn("pr checks", cerr)
	}
	if r.Workflows, err = c.workflows(ctx, full, r.DefaultBranch); err != nil {
		warn("workflows", err)
	}
	if r.Release, err = c.latestRelease(ctx, full, r.URL); err != nil {
		warn("releases", err)
	}
	if limited != nil {
		return model.Repo{}, limited
	}
	return r, nil
}

func (c *Client) openIssues(ctx context.Context, full string) ([]model.Issue, error) {
	type raw struct {
		CreatedAt   time.Time       `json:"created_at"`
		UpdatedAt   time.Time       `json:"updated_at"`
		Title       string          `json:"title"`
		HTMLURL     string          `json:"html_url"`
		User        userRef         `json:"user"`
		Labels      []labelRef      `json:"labels"`
		PullRequest json.RawMessage `json:"pull_request"`
		Number      int             `json:"number"`
		Comments    int             `json:"comments"`
	}
	// The issues endpoint also returns pull requests; they are dropped here and fetched separately.
	q := url.Values{"state": {"open"}, "sort": {"created"}, "direction": {"asc"}}
	items, err := listAll(ctx, c, full, "/repos/"+full+"/issues", q, c.maxItems, unwrapArray[raw])
	out := make([]model.Issue, 0, len(items))
	for _, i := range items {
		if len(i.PullRequest) > 0 {
			continue
		}
		out = append(out, model.Issue{
			Number: i.Number, Title: i.Title, URL: i.HTMLURL, Author: i.User.Login,
			Labels: labelNames(i.Labels), Comments: i.Comments, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
		})
	}
	return out, err
}

func (c *Client) openPRs(ctx context.Context, full string) ([]model.PullRequest, error) {
	type raw struct {
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Title     string    `json:"title"`
		HTMLURL   string    `json:"html_url"`
		User      userRef   `json:"user"`
		Head      struct {
			Repo *repoRef `json:"repo"`
			SHA  string   `json:"sha"`
		} `json:"head"`
		Base struct {
			Repo *repoRef `json:"repo"`
		} `json:"base"`
		Labels []labelRef `json:"labels"`
		Number int        `json:"number"`
		Draft  bool       `json:"draft"`
	}
	q := url.Values{"state": {"open"}, "sort": {"created"}, "direction": {"asc"}}
	items, err := listAll(ctx, c, full, "/repos/"+full+"/pulls", q, c.maxItems, unwrapArray[raw])
	out := make([]model.PullRequest, 0, len(items))
	for _, p := range items {
		out = append(out, model.PullRequest{
			Number: p.Number, Title: p.Title, URL: p.HTMLURL, Author: p.User.Login, Draft: p.Draft,
			Labels: labelNames(p.Labels), HeadSHA: p.Head.SHA, Fork: isFork(p.Head.Repo, p.Base.Repo), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
			Checks: model.CheckSummary{State: model.ChecksUnknown},
		})
	}
	return out, err
}

type repoRef struct {
	FullName string `json:"full_name"`
}

// isFork reports a head repo that differs from the base repo; a deleted head repo counts as a fork.
func isFork(head, base *repoRef) bool {
	return head == nil || base == nil || !strings.EqualFold(head.FullName, base.FullName)
}

// attachChecks fills each PR's check summary; it reports the first failure but still tries every PR.
func (c *Client) attachChecks(ctx context.Context, full string, prs []model.PullRequest) error {
	var first error
	for i := range prs {
		s, err := c.checkSummary(ctx, full, prs[i].HeadSHA)
		if err != nil {
			if first == nil {
				first = fmt.Errorf("#%d: %w", prs[i].Number, err)
			}
			continue
		}
		prs[i].Checks = s
	}
	return first
}

func (c *Client) checkSummary(ctx context.Context, full, sha string) (model.CheckSummary, error) {
	var runs struct {
		CheckRuns []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	base := "/repos/" + full + "/commits/" + sha
	if err := c.getJSON(ctx, full, base+"/check-runs", url.Values{"per_page": {"100"}}, &runs); err != nil {
		return model.CheckSummary{}, err
	}
	var status struct {
		Statuses []struct {
			Context string `json:"context"`
			State   string `json:"state"`
		} `json:"statuses"`
		TotalCount int `json:"total_count"`
	}
	if err := c.getJSON(ctx, full, base+"/status", nil, &status); err != nil {
		return model.CheckSummary{}, err
	}

	var s model.CheckSummary
	for _, cr := range runs.CheckRuns {
		s.Total++
		switch {
		case cr.Status != "completed":
			s.Pending++
		case (model.Run{Status: cr.Status, Conclusion: cr.Conclusion}).Failed():
			s.Failed++
			s.Failing = append(s.Failing, cr.Name)
		}
	}
	for _, st := range status.Statuses {
		s.Total++
		switch st.State {
		case statusStateFail, statusStateError:
			s.Failed++
			s.Failing = append(s.Failing, st.Context)
		case statusStatePend:
			s.Pending++
		}
	}
	if len(s.Failing) > maxFailingNames {
		s.Failing = s.Failing[:maxFailingNames]
	}
	switch {
	case s.Total == 0:
		s.State = model.ChecksNone
	case s.Failed > 0:
		s.State = model.ChecksFail
	case s.Pending > 0:
		s.State = model.ChecksPending
	default:
		s.State = model.ChecksPass
	}
	return s, nil
}

type rawRun struct {
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	HTMLURL    string    `json:"html_url"`
	Event      string    `json:"event"`
	HeadBranch string    `json:"head_branch"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	ID         int64     `json:"id"`
}

func (r rawRun) model() model.Run {
	return model.Run{
		ID: r.ID, URL: r.HTMLURL, Event: r.Event, Branch: r.HeadBranch, Status: r.Status,
		Conclusion: r.Conclusion, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// workflows returns, per active workflow, its newest default-branch run and newest completed one.
// Pull request runs are excluded because PR checks cover them.
func (c *Client) workflows(ctx context.Context, full, branch string) ([]model.Workflow, error) {
	var list struct {
		Workflows []struct {
			Name    string `json:"name"`
			HTMLURL string `json:"html_url"`
			State   string `json:"state"`
			ID      int64  `json:"id"`
		} `json:"workflows"`
	}
	if err := c.getJSON(ctx, full, "/repos/"+full+"/actions/workflows", url.Values{"per_page": {"100"}}, &list); err != nil {
		return nil, err
	}
	var out []model.Workflow
	var first error
	for _, w := range list.Workflows {
		if w.State != "active" || len(out) >= maxWorkflows {
			continue
		}
		runs, err := c.workflowRuns(ctx, full, w.ID, branch, false)
		if err != nil {
			if first == nil {
				first = fmt.Errorf("%s: %w", w.Name, err)
			}
			continue
		}
		wf, ok := buildWorkflow(w.Name, w.HTMLURL, runs)
		if !ok {
			continue
		}
		// GitHub sometimes answers a run listing from a lagging replica, and a reused connection
		// keeps hitting it. A failure is what raises an alert, so confirm it on a new connection
		// and keep the union of both answers.
		if wf.LastDone != nil && wf.LastDone.Failed() {
			if again, err := c.workflowRuns(ctx, full, w.ID, branch, true); err == nil {
				if merged, ok := buildWorkflow(w.Name, w.HTMLURL, mergeRuns(runs, again)); ok {
					wf = merged
				}
			}
		}
		out = append(out, wf)
	}
	return out, first
}

func buildWorkflow(name, htmlURL string, runs []rawRun) (model.Workflow, bool) {
	var kept []model.Run
	for _, r := range runs {
		if r.Event == eventPullRequest || r.Event == eventPullReqTgt {
			continue
		}
		kept = append(kept, r.model())
	}
	if len(kept) == 0 {
		return model.Workflow{}, false
	}
	slices.SortStableFunc(kept, func(a, b model.Run) int { return b.CreatedAt.Compare(a.CreatedAt) })
	wf := model.Workflow{
		Name:       name,
		URL:        htmlURL,
		Latest:     kept[0],
		Background: slices.Contains(backgroundEvents, kept[0].Event),
	}
	for i := range kept {
		if !kept[i].Running() {
			done := kept[i]
			wf.LastDone = &done
			break
		}
	}
	return wf, true
}

func (c *Client) latestRelease(ctx context.Context, full, repoURL string) (*model.Release, error) {
	var rel struct {
		PublishedAt time.Time `json:"published_at"`
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		HTMLURL     string    `json:"html_url"`
		Prerelease  bool      `json:"prerelease"`
	}
	err := c.getJSON(ctx, full, "/repos/"+full+"/releases/latest", nil, &rel)
	if err == nil {
		return &model.Release{Tag: rel.TagName, Name: rel.Name, URL: rel.HTMLURL, PublishedAt: rel.PublishedAt, Prerelease: rel.Prerelease}, nil
	}
	if !isNotFound(err) {
		return nil, err
	}
	// No published release: fall back to the newest tag so unreleased-but-tagged projects show a version.
	var tags []struct {
		Name string `json:"name"`
	}
	if err := c.getJSON(ctx, full, "/repos/"+full+"/tags", url.Values{"per_page": {"1"}}, &tags); err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return nil, nil
	}
	return &model.Release{
		Tag:     tags[0].Name,
		URL:     repoURL + "/releases/tag/" + url.PathEscape(tags[0].Name),
		FromTag: true,
	}, nil
}

func (c *Client) workflowRuns(ctx context.Context, full string, id int64, branch string, fresh bool) ([]rawRun, error) {
	var out struct {
		WorkflowRuns []rawRun `json:"workflow_runs"`
	}
	q := url.Values{
		"branch":                {branch},
		"exclude_pull_requests": {"true"},
		"per_page":              {fmt.Sprint(runsPerWorkflow)},
	}
	path := fmt.Sprintf("/repos/%s/actions/workflows/%d/runs", full, id)
	get := c.getJSON
	if fresh {
		// GitHub can keep answering a branch query from an old cached result, for weeks, whatever
		// the connection, and page=1 alone does not skip it. A created filter changes the query
		// every day and reads current runs.
		q.Set("page", "1")
		q.Set("created", ">="+c.now().UTC().AddDate(0, 0, -freshRunsWindowDays).Format(time.DateOnly))
		get = c.getJSONFresh
	}
	if err := get(ctx, full, path, q, &out); err != nil {
		return nil, err
	}
	return out.WorkflowRuns, nil
}

// mergeRuns returns the runs of a and b once each, keyed by run ID.
func mergeRuns(a, b []rawRun) []rawRun {
	seen := make(map[int64]bool, len(a)+len(b))
	out := make([]rawRun, 0, len(a)+len(b))
	for _, r := range slices.Concat(a, b) {
		if !seen[r.ID] {
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	return out
}
