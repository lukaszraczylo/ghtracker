// Package model holds the data shapes shared by the collector, metrics and web layers.
package model

import (
	"errors"
	"time"
)

// ErrRateLimited means GitHub limits were hit or are about to be; callers must stop issuing requests.
var ErrRateLimited = errors.New("github rate limit reached")

type Severity int

const (
	OK Severity = iota
	Warn
	Crit
)

func (s Severity) String() string {
	switch s {
	case Warn:
		return "warn"
	case Crit:
		return "crit"
	default:
		return "ok"
	}
}

type CheckState string

const (
	ChecksNone    CheckState = "none"
	ChecksPending CheckState = "pending"
	ChecksPass    CheckState = "success"
	ChecksFail    CheckState = "failure"
	ChecksUnknown CheckState = "unknown"
)

type Repo struct {
	RefreshedAt   time.Time
	Release       *Release
	Description   string
	FullName      string
	DefaultBranch string
	ReleasesURL   string
	// Err is set when the last refresh failed entirely; the other fields hold the last good data.
	Err       string
	URL       string
	Issues    []Issue
	PRs       []PullRequest
	Workflows []Workflow
	// Warnings lists sections that failed to load while the rest succeeded.
	Warnings []string
	Health   Severity
	Archived bool
	Up       bool
}

type Release struct {
	PublishedAt time.Time
	Tag         string
	Name        string
	URL         string
	Prerelease  bool
	// FromTag marks a version taken from tags because the repo has no releases.
	FromTag bool
}

type Issue struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	Title     string
	URL       string
	Author    string
	Labels    []string
	Number    int
	Comments  int
}

type PullRequest struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	Title     string
	URL       string
	Author    string
	HeadSHA   string
	Labels    []string
	Checks    CheckSummary
	Number    int
	Draft     bool
}

type CheckSummary struct {
	State   CheckState
	Failing []string
	Total   int
	Failed  int
	Pending int
}

type Workflow struct {
	// LastDone is the newest completed run; nil when no run has completed yet.
	LastDone   *Run
	Name       string
	URL        string
	Latest     Run
	Background bool
}

type Run struct {
	CreatedAt  time.Time
	UpdatedAt  time.Time
	URL        string
	Event      string
	Branch     string
	Status     string
	Conclusion string
	ID         int64
}

// Running reports whether the run has not completed.
func (r Run) Running() bool { return r.Status != "completed" }

// Failed reports whether a completed run ended in a failing conclusion.
func (r Run) Failed() bool {
	switch r.Conclusion {
	case "failure", "timed_out", "startup_failure", "action_required":
		return true
	}
	return false
}

type Alert struct {
	Since    time.Time
	Repo     string
	Kind     string
	Subject  string
	Detail   string
	URL      string
	Severity Severity
}

// Message renders the alert as one line for logs and the JSON API.
func (a Alert) Message() string { return a.Subject + ": " + a.Detail }
