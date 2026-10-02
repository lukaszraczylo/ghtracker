package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func noFile(string) ([]byte, error)               { return nil, errors.New("no file") }
func keyFile(string) ([]byte, error)              { return []byte("PEM"), nil }
func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func valid() *Config {
	return &Config{GitHub: GitHub{AppID: 1, PrivateKeyFile: "k.pem"}, Repos: []string{"a/b"}}
}

func TestFinalizeDefaults(t *testing.T) {
	c := valid()
	if err := c.finalize(env(nil), keyFile); err != nil {
		t.Fatal(err)
	}
	if c.RefreshInterval != 6*time.Hour || c.Listen != ":8080" || c.Concurrency != 2 || c.MaxItems != 100 {
		t.Fatalf("defaults wrong: %+v", c)
	}
	if c.ManualRefresh == nil || !*c.ManualRefresh {
		t.Fatal("manual refresh must default to on")
	}
	if c.GitHub.APIURL != DefaultAPIURL || c.GitHub.PrivateKeyPEM != "PEM" {
		t.Fatalf("github defaults wrong: %+v", c.GitHub)
	}
	if want := 12*time.Hour + 30*time.Minute; c.Thresholds.DataStaleAfter != want {
		t.Fatalf("data stale = %s, want %s", c.Thresholds.DataStaleAfter, want)
	}
}

func TestFinalizeEnvKeyWins(t *testing.T) {
	c := valid()
	c.GitHub.PrivateKeyFile = ""
	if err := c.finalize(env(map[string]string{EnvPrivateKey: "ENVPEM"}), noFile); err != nil {
		t.Fatal(err)
	}
	if c.GitHub.PrivateKeyPEM != "ENVPEM" {
		t.Fatalf("got %q", c.GitHub.PrivateKeyPEM)
	}
}

func TestFinalizeErrors(t *testing.T) {
	tests := map[string]func(*Config){
		"no app id":         func(c *Config) { c.GitHub.AppID = 0 },
		"no key":            func(c *Config) { c.GitHub.PrivateKeyFile = "" },
		"unreadable key":    func(c *Config) { c.GitHub.PrivateKeyFile = "x" },
		"no repos":          func(c *Config) { c.Repos = nil },
		"bad repo":          func(c *Config) { c.Repos = []string{"justname"} },
		"bad repo slashes":  func(c *Config) { c.Repos = []string{"a/b/c"} },
		"refresh too small": func(c *Config) { c.RefreshInterval = time.Second },
		"crit below warn": func(c *Config) {
			c.Thresholds.PRWarnAfter, c.Thresholds.PRCritAfter = 10*time.Hour, time.Hour
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c := valid()
			mutate(c)
			if err := c.finalize(env(nil), noFile); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNormalizeReposDedupes(t *testing.T) {
	got, err := normalizeRepos([]string{"A/b", " a/B ", "c/d"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "A/b" || got[1] != "c/d" {
		t.Fatalf("got %v", got)
	}
}

func TestNegativeThresholdDisables(t *testing.T) {
	c := valid()
	c.Thresholds.PRCritAfter = -1
	if err := c.finalize(env(nil), keyFile); err != nil {
		t.Fatal(err)
	}
	if c.Thresholds.PRCritAfter != -1 {
		t.Fatalf("negative must be kept, got %s", c.Thresholds.PRCritAfter)
	}
}

func TestLoadRejectsUnknownKeysAndReadsFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	key := write("k.pem", "PEM")
	good := write("ok.yaml", "refresh_interval: 2h\ngithub:\n  app_id: 5\n  private_key_file: "+key+"\nrepos: [a/b]\n")
	c, err := Load(good)
	if err != nil || c.RefreshInterval != 2*time.Hour || c.GitHub.PrivateKeyPEM != "PEM" {
		t.Fatalf("c=%+v err=%v", c, err)
	}
	typo := write("typo.yaml", "refresh_intervall: 2h\ngithub:\n  app_id: 5\n  private_key_file: "+key+"\nrepos: [a/b]\n")
	if _, err := Load(typo); err == nil {
		t.Fatal("unknown key must be rejected")
	}
	if _, err := Load(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("missing file must error")
	}
}

func TestManualRefreshCanBeDisabled(t *testing.T) {
	off := false
	c := valid()
	c.ManualRefresh = &off
	if err := c.finalize(env(nil), keyFile); err != nil {
		t.Fatal(err)
	}
	if *c.ManualRefresh {
		t.Fatal("explicit false must stay off")
	}
}

func TestWebBase(t *testing.T) {
	tests := map[string]string{
		"":                                      "https://github.com",
		"https://api.github.com":                "https://github.com",
		"https://api.github.com/":               "https://github.com",
		"https://ghe.example.com/api/v3":        "https://ghe.example.com",
		"https://ghe.example.com/api/v3/":       "https://ghe.example.com",
		"http://ghe.internal:8080/api/v3":       "http://ghe.internal:8080",
		"https://api.acme.ghe.com":              "https://acme.ghe.com",
		"https://github.example.com/sub/api/v3": "https://github.example.com/sub",
		"not a url":                             "https://github.com",
	}
	for in, want := range tests {
		if got := WebBase(in); got != want {
			t.Errorf("WebBase(%q) = %q, want %q", in, got, want)
		}
	}
}
