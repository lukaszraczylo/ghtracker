// Command ghtracker serves a dashboard and Prometheus metrics for a list of GitHub repositories.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/lukaszraczylo/ghtracker/internal/collector"
	"github.com/lukaszraczylo/ghtracker/internal/config"
	"github.com/lukaszraczylo/ghtracker/internal/gh"
	"github.com/lukaszraczylo/ghtracker/internal/metrics"
	"github.com/lukaszraczylo/ghtracker/internal/web"
)

// distFS holds the built UI; dist/.gitkeep keeps the pattern valid before the first UI build.
//
//go:embed all:dist
var distFS embed.FS

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

const (
	httpTimeout     = 30 * time.Second
	shutdownTimeout = 10 * time.Second
	headerTimeout   = 10 * time.Second
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to the YAML config file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *cfgPath, log); err != nil {
		stop()
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfgPath string, log *slog.Logger) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	hc := &http.Client{Timeout: httpTimeout}
	auth, err := gh.NewAppAuth(cfg.GitHub.AppID, cfg.GitHub.PrivateKeyPEM, cfg.GitHub.APIURL, cfg.GitHub.InstallationID, hc)
	if err != nil {
		return err
	}
	client := gh.NewClient(cfg.GitHub.APIURL, auth, hc, cfg.MaxItems)
	coll := collector.New(client, cfg, log)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		metrics.New(coll, client.RateLimitRemaining))
	ui, err := fs.Sub(distFS, "dist")
	if err != nil {
		return err
	}
	srv := web.New(coll, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), ui, *cfg.ManualRefresh, log)

	go coll.Run(ctx)

	hs := &http.Server{Addr: cfg.Listen, Handler: srv.Handler(), ReadHeaderTimeout: headerTimeout}
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("listening", "version", version, "addr", cfg.Listen, "repos", len(cfg.Repos), "refresh", cfg.RefreshInterval)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := hs.Shutdown(sctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
