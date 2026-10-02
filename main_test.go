package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, ""
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestEndToEnd wires config, app auth, collector, metrics and web against a fake GitHub API.
func TestEndToEnd(t *testing.T) {
	var apiCalls atomic.Int64
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/installation"):
			fmt.Fprint(w, `{"id":1}`)
		case strings.HasPrefix(p, "/app/installations/"):
			fmt.Fprintf(w, `{"token":"t","expires_at":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
		case p == "/repos/o/r":
			fmt.Fprint(w, `{"html_url":"https://github.com/o/r","default_branch":"main"}`)
		case p == "/repos/o/r/issues", p == "/repos/o/r/pulls", p == "/repos/o/r/tags":
			fmt.Fprint(w, `[]`)
		case p == "/repos/o/r/actions/workflows":
			fmt.Fprint(w, `{"workflows":[{"id":1,"name":"CI","html_url":"w","state":"active"}]}`)
		case p == "/repos/o/r/actions/workflows/1/runs":
			fmt.Fprint(w, `{"workflow_runs":[{"id":1,"html_url":"r","event":"push","head_branch":"main","status":"completed","conclusion":"failure","created_at":"2026-10-01T00:00:00Z"}]}`)
		case p == "/repos/o/r/releases/latest":
			fmt.Fprint(w, `{"tag_name":"v9.9.9","html_url":"rel","published_at":"2026-10-01T00:00:00Z"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "k.pem")
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600)
	addr := freeAddr(t)
	cfgPath := filepath.Join(dir, "c.yaml")
	cfg := fmt.Sprintf("listen: %q\ngithub:\n  app_id: 1\n  private_key_file: %s\n  api_url: %s\nrepos: [o/r]\n", addr, keyPath, gh.URL)
	_ = os.WriteFile(cfgPath, []byte(cfg), 0o600)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfgPath, slog.New(slog.NewTextHandler(io.Discard, nil))) }()

	base := "http://" + addr
	deadline := time.Now().Add(10 * time.Second)
	for {
		if code, _ := get(t, base+"/healthz"); code == 200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server never became healthy")
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, metrics := get(t, base+"/metrics")
	for _, want := range []string{
		`ghtracker_repo_up{repo="o/r"} 1`,
		`ghtracker_workflow_failing{background="false",repo="o/r",workflow="CI"} 1`,
		`ghtracker_release_info{repo="o/r",url="rel",version="v9.9.9"} 1`,
		`ghtracker_repo_health{repo="o/r"} 2`,
		`ghtracker_alerts{severity="crit"} 1`,
		"ghtracker_refreshes_total 1",
	} {
		if !strings.Contains(metrics, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
	_, state := get(t, base+"/api/state")
	for _, want := range []string{`"tag":"v9.9.9"`, `"failure on main (push)"`, `"health":"crit"`} {
		if !strings.Contains(state, want) {
			t.Errorf("state missing %q", want)
		}
	}

	if n := apiCalls.Load(); n > 12 {
		t.Errorf("one refresh of one repo used %d API calls, want a handful", n)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no graceful shutdown")
	}
}
