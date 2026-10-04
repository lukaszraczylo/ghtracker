// Package config loads and validates the YAML configuration.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	EnvPrivateKey = "GHTRACKER_PRIVATE_KEY"

	DefaultListen          = ":8080"
	DefaultRefresh         = 6 * time.Hour
	DefaultAPIURL          = "https://api.github.com"
	DefaultPRWarnAfter     = 48 * time.Hour
	DefaultPRCritAfter     = 7 * 24 * time.Hour
	DefaultIssueStale      = 30 * 24 * time.Hour
	DefaultWorkflowStuck   = 6 * time.Hour
	DefaultWorkflowDormant = 30 * 24 * time.Hour
	DefaultMaxItems        = 100
	DefaultConcurrency     = 2
	MinRefresh             = time.Minute
	dataStaleGrace         = 30 * time.Minute
	defaultDataStaleFactor = 2
	DefaultActionTimeout   = 10 * time.Second
)

var actionIDPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// Action is an operator-defined button on alerts. Pressing it POSTs the alert to a webhook.
type Action struct {
	ID        string `yaml:"id"`
	Label     string `yaml:"label"`
	Confirm   string `yaml:"confirm"`
	StatusURL string `yaml:"status_url"`
	// Token comes from the environment variable named by Webhook.TokenEnv, never from the file.
	Token   string  `yaml:"-"`
	Webhook Webhook `yaml:"webhook"`
	// Kinds limits the action to these alert kinds; empty means every kind.
	Kinds []string `yaml:"kinds"`
}

type Webhook struct {
	URL      string        `yaml:"url"`
	TokenEnv string        `yaml:"token_env"`
	Timeout  time.Duration `yaml:"timeout"`
}

// AppliesTo reports whether the action is offered for an alert kind.
func (a Action) AppliesTo(kind string) bool {
	if len(a.Kinds) == 0 {
		return true
	}
	for _, k := range a.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

type Config struct {
	// ManualRefresh enables POST /refresh and the rescan button; it defaults to on.
	ManualRefresh   *bool         `yaml:"manual_refresh"`
	Listen          string        `yaml:"listen"`
	Repos           []string      `yaml:"repos"`
	Actions         []Action      `yaml:"actions"`
	GitHub          GitHub        `yaml:"github"`
	Thresholds      Thresholds    `yaml:"thresholds"`
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	MaxItems        int           `yaml:"max_items"`
	Concurrency     int           `yaml:"concurrency"`
}

type GitHub struct {
	PrivateKeyFile string `yaml:"private_key_file"`
	APIURL         string `yaml:"api_url"`
	// PrivateKeyPEM comes from the environment, never from the file.
	PrivateKeyPEM  string `yaml:"-"`
	AppID          int64  `yaml:"app_id"`
	InstallationID int64  `yaml:"installation_id"`
}

// Thresholds are durations; zero selects the default and a negative value disables the check.
type Thresholds struct {
	PRWarnAfter     time.Duration `yaml:"pr_warn_after"`
	PRCritAfter     time.Duration `yaml:"pr_crit_after"`
	IssueStaleAfter time.Duration `yaml:"issue_stale_after"`
	WorkflowStuck   time.Duration `yaml:"workflow_stuck_after"`
	// WorkflowDormant mutes failures of workflows that have not run for this long.
	WorkflowDormant time.Duration `yaml:"workflow_dormant_after"`
	DataStaleAfter  time.Duration `yaml:"data_stale_after"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-supplied -config flag
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &c, c.finalize(os.Getenv, os.ReadFile)
}

func (c *Config) finalize(getenv func(string) string, readFile func(string) ([]byte, error)) error {
	if c.ManualRefresh == nil {
		on := true
		c.ManualRefresh = &on
	}
	if c.Listen == "" {
		c.Listen = DefaultListen
	}
	if c.RefreshInterval == 0 {
		c.RefreshInterval = DefaultRefresh
	}
	if c.RefreshInterval < MinRefresh {
		return fmt.Errorf("refresh_interval %s is below the %s minimum", c.RefreshInterval, MinRefresh)
	}
	if c.MaxItems <= 0 {
		c.MaxItems = DefaultMaxItems
	}
	if c.Concurrency <= 0 {
		c.Concurrency = DefaultConcurrency
	}
	if c.GitHub.APIURL == "" {
		c.GitHub.APIURL = DefaultAPIURL
	}
	c.GitHub.APIURL = strings.TrimRight(c.GitHub.APIURL, "/")

	t := &c.Thresholds
	setDefault(&t.PRWarnAfter, DefaultPRWarnAfter)
	setDefault(&t.PRCritAfter, DefaultPRCritAfter)
	setDefault(&t.IssueStaleAfter, DefaultIssueStale)
	setDefault(&t.WorkflowStuck, DefaultWorkflowStuck)
	setDefault(&t.WorkflowDormant, DefaultWorkflowDormant)
	setDefault(&t.DataStaleAfter, defaultDataStaleFactor*c.RefreshInterval+dataStaleGrace)
	if t.PRWarnAfter > 0 && t.PRCritAfter > 0 && t.PRCritAfter < t.PRWarnAfter {
		return errors.New("thresholds.pr_crit_after must not be below pr_warn_after")
	}

	if c.GitHub.AppID <= 0 {
		return errors.New("github.app_id is required")
	}
	c.GitHub.PrivateKeyPEM = getenv(EnvPrivateKey)
	if c.GitHub.PrivateKeyPEM == "" {
		if c.GitHub.PrivateKeyFile == "" {
			return fmt.Errorf("github.private_key_file or $%s is required", EnvPrivateKey)
		}
		pem, err := readFile(c.GitHub.PrivateKeyFile)
		if err != nil {
			return fmt.Errorf("read private key: %w", err)
		}
		c.GitHub.PrivateKeyPEM = string(pem)
	}

	if err := finalizeActions(c.Actions, getenv); err != nil {
		return err
	}

	repos, err := normalizeRepos(c.Repos)
	if err != nil {
		return err
	}
	c.Repos = repos
	return nil
}

func finalizeActions(actions []Action, getenv func(string) string) error {
	seen := make(map[string]bool, len(actions))
	for i := range actions {
		a := &actions[i]
		if !actionIDPattern.MatchString(a.ID) {
			return fmt.Errorf("actions[%d].id %q must match [a-z0-9-]+", i, a.ID)
		}
		if seen[a.ID] {
			return fmt.Errorf("actions[%d].id %q is used twice", i, a.ID)
		}
		seen[a.ID] = true
		if a.Label == "" {
			a.Label = a.ID
		}
		if !isHTTPURL(a.Webhook.URL) {
			return fmt.Errorf("actions[%s].webhook.url %q must be an http or https address", a.ID, a.Webhook.URL)
		}
		if a.StatusURL != "" && !isHTTPURL(a.StatusURL) {
			return fmt.Errorf("actions[%s].status_url %q must be an http or https address", a.ID, a.StatusURL)
		}
		if a.Webhook.Timeout < 0 {
			return fmt.Errorf("actions[%s].webhook.timeout must not be negative", a.ID)
		}
		if a.Webhook.Timeout == 0 {
			a.Webhook.Timeout = DefaultActionTimeout
		}
		if env := a.Webhook.TokenEnv; env != "" {
			if a.Token = getenv(env); a.Token == "" {
				return fmt.Errorf("actions[%s].webhook.token_env: $%s is empty", a.ID, env)
			}
		}
	}
	return nil
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func setDefault(d *time.Duration, def time.Duration) {
	if *d == 0 {
		*d = def
	}
}

func normalizeRepos(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, errors.New("repos must list at least one owner/name")
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, r := range in {
		r = strings.TrimSpace(r)
		owner, name, ok := strings.Cut(r, "/")
		if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
			return nil, fmt.Errorf("invalid repo %q: want owner/name", r)
		}
		key := strings.ToLower(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out, nil
}

const (
	defaultWebURL = "https://github.com"
	apiV3Suffix   = "/api/v3"
	apiHostPrefix = "api."
)

// WebBase returns the address of the web interface that belongs to a GitHub API address:
// github.com for the public API, the host without /api/v3 for Enterprise Server, and the host
// without its "api." prefix for Enterprise Cloud with data residency.
func WebBase(apiURL string) string {
	u, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil || u.Host == "" || strings.EqualFold(u.Host, "api.github.com") {
		return defaultWebURL
	}
	u.Path = strings.TrimSuffix(u.Path, apiV3Suffix)
	if u.Path == "" {
		u.Host = strings.TrimPrefix(u.Host, apiHostPrefix)
	}
	return strings.TrimRight(u.String(), "/")
}
