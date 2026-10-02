// Package config loads and validates the YAML configuration.
package config

import (
	"errors"
	"fmt"
	"os"
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
)

type Config struct {
	// ManualRefresh enables POST /refresh and the rescan button; it defaults to on.
	ManualRefresh   *bool         `yaml:"manual_refresh"`
	Listen          string        `yaml:"listen"`
	Repos           []string      `yaml:"repos"`
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

	repos, err := normalizeRepos(c.Repos)
	if err != nil {
		return err
	}
	c.Repos = repos
	return nil
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
