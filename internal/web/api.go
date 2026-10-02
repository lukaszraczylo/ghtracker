package web

import (
	"time"

	"github.com/lukaszraczylo/ghtracker/internal/collector"
	"github.com/lukaszraczylo/ghtracker/internal/model"
)

// apiState is the JSON contract with the UI. Lists of healthy items stay on GitHub;
// the UI gets counts, release info and the alerts that explain every non-green state.
type apiState struct {
	Now             time.Time     `json:"now"`
	LastRefresh     time.Time     `json:"lastRefresh"`
	Repos           []apiRepo     `json:"repos"`
	Alerts          []apiAlert    `json:"alerts"`
	Thresholds      apiThresholds `json:"thresholds"`
	Counts          apiCounts     `json:"counts"`
	IntervalSeconds int64         `json:"intervalSeconds"`
	Loaded          bool          `json:"loaded"`
	Refreshing      bool          `json:"refreshing"`
	ManualRefresh   bool          `json:"manualRefresh"`
}

type apiThresholds struct {
	PRWarnSeconds   int64 `json:"prWarnSeconds"`
	PRCritSeconds   int64 `json:"prCritSeconds"`
	IssueStaleSecs  int64 `json:"issueStaleSeconds"`
	WorkflowStuck   int64 `json:"workflowStuckSeconds"`
	WorkflowDormant int64 `json:"workflowDormantSeconds"`
}

type apiCounts struct {
	Crit int `json:"crit"`
	Warn int `json:"warn"`
}

type apiRepo struct {
	RefreshedAt time.Time    `json:"refreshedAt"`
	Release     *apiRelease  `json:"release"`
	FullName    string       `json:"fullName"`
	URL         string       `json:"url"`
	ReleasesURL string       `json:"releasesUrl"`
	Description string       `json:"description"`
	Health      string       `json:"health"`
	Error       string       `json:"error"`
	Workflows   apiWorkflows `json:"workflows"`
	PRs         apiPRs       `json:"prs"`
	Issues      apiIssues    `json:"issues"`
	Archived    bool         `json:"archived"`
	Up          bool         `json:"up"`
}

type apiWorkflows struct {
	Total   int `json:"total"`
	Failing int `json:"failing"`
	Running int `json:"running"`
}

type apiPRs struct {
	OldestSeconds int64 `json:"oldestSeconds"`
	Open          int   `json:"open"`
	Waiting       int   `json:"waiting"`
	FailingChecks int   `json:"failingChecks"`
}

type apiIssues struct {
	Open  int `json:"open"`
	Stale int `json:"stale"`
}

type apiRelease struct {
	PublishedAt time.Time `json:"publishedAt"`
	Tag         string    `json:"tag"`
	URL         string    `json:"url"`
	Prerelease  bool      `json:"prerelease"`
	FromTag     bool      `json:"fromTag"`
}

type apiAlert struct {
	Since    time.Time `json:"since"`
	Repo     string    `json:"repo"`
	Kind     string    `json:"kind"`
	Severity string    `json:"severity"`
	Subject  string    `json:"subject"`
	Detail   string    `json:"detail"`
	URL      string    `json:"url"`
}

func seconds(d time.Duration) int64 { return int64(d / time.Second) }

func buildState(v collector.View, manualRefresh bool) apiState {
	st := apiState{
		Now:             v.Now,
		LastRefresh:     v.LastRefresh,
		IntervalSeconds: seconds(v.Interval),
		Loaded:          v.Loaded,
		Refreshing:      v.Refreshing,
		ManualRefresh:   manualRefresh,
		Counts:          apiCounts{Crit: v.Crit, Warn: v.Warn},
		Thresholds: apiThresholds{
			PRWarnSeconds:   seconds(v.Thresholds.PRWarnAfter),
			PRCritSeconds:   seconds(v.Thresholds.PRCritAfter),
			IssueStaleSecs:  seconds(v.Thresholds.IssueStaleAfter),
			WorkflowStuck:   seconds(v.Thresholds.WorkflowStuck),
			WorkflowDormant: seconds(v.Thresholds.WorkflowDormant),
		},
		Repos:  make([]apiRepo, 0, len(v.Repos)),
		Alerts: make([]apiAlert, 0, len(v.Alerts)),
	}
	for _, r := range v.Repos {
		ar := apiRepo{
			FullName: r.FullName, URL: r.URL, ReleasesURL: r.ReleasesURL, Description: r.Description,
			Health: r.Health.String(), Error: r.Err, Archived: r.Archived, Up: r.Up, RefreshedAt: r.RefreshedAt,
			Workflows: workflowStats(r, v), PRs: prStats(r, v), Issues: issueStats(r, v),
		}
		if rel := r.Release; rel != nil {
			ar.Release = &apiRelease{Tag: rel.Tag, URL: rel.URL, PublishedAt: rel.PublishedAt, Prerelease: rel.Prerelease, FromTag: rel.FromTag}
		}
		st.Repos = append(st.Repos, ar)
	}
	for _, a := range v.Alerts {
		st.Alerts = append(st.Alerts, apiAlert{
			Severity: a.Severity.String(), Repo: a.Repo, Kind: a.Kind, Subject: a.Subject,
			Detail: a.Detail, URL: a.URL, Since: a.Since,
		})
	}
	return st
}

func workflowStats(r model.Repo, v collector.View) apiWorkflows {
	out := apiWorkflows{Total: len(r.Workflows)}
	for _, wf := range r.Workflows {
		if wf.LastDone != nil && wf.LastDone.Failed() && !collector.Dormant(wf, v.Now, v.Thresholds) {
			out.Failing++
		}
		if wf.Latest.Running() {
			out.Running++
		}
	}
	return out
}

// prStats counts non-draft pull requests; drafts never wait on anyone.
func prStats(r model.Repo, v collector.View) apiPRs {
	out := apiPRs{Open: len(r.PRs)}
	for _, pr := range r.PRs {
		if pr.Draft {
			continue
		}
		age := v.Now.Sub(pr.CreatedAt)
		out.OldestSeconds = max(out.OldestSeconds, seconds(age))
		if v.Thresholds.PRWarnAfter > 0 && age >= v.Thresholds.PRWarnAfter {
			out.Waiting++
		}
		if pr.Checks.State == model.ChecksFail {
			out.FailingChecks++
		}
	}
	return out
}

func issueStats(r model.Repo, v collector.View) apiIssues {
	out := apiIssues{Open: len(r.Issues)}
	if v.Thresholds.IssueStaleAfter <= 0 {
		return out
	}
	for _, is := range r.Issues {
		if v.Now.Sub(is.UpdatedAt) >= v.Thresholds.IssueStaleAfter {
			out.Stale++
		}
	}
	return out
}
