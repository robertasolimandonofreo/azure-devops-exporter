// Command azure-devops-exporter exposes Prometheus metrics collected from Azure DevOps.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"azure-devops-exporter/internal/azuredevops"
	"azure-devops-exporter/internal/collectors"
	"azure-devops-exporter/internal/config"
	"azure-devops-exporter/internal/metrics"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	client := azuredevops.NewClient(cfg.APIURL, cfg.Organization, cfg.Token)

	var ready atomic.Bool
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go runScrapeLoop(ctx, client, cfg, &ready)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if ready.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	slog.Info("starting azure-devops-exporter",
		"organization", cfg.Organization,
		"projects", cfg.Projects,
		"port", cfg.Port,
		"scrape_interval", cfg.ScrapeInterval.String(),
	)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// runScrapeLoop runs one scrape cycle immediately, then repeats every cfg.ScrapeInterval
// until ctx is canceled. ready is set after the first cycle completes, regardless of
// whether individual projects succeeded, so /ready reflects that scraping has started.
func runScrapeLoop(ctx context.Context, client *azuredevops.Client, cfg *config.Config, ready *atomic.Bool) {
	ticker := time.NewTicker(cfg.ScrapeInterval)
	defer ticker.Stop()

	for {
		scrapeAll(client, cfg)
		ready.Store(true)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func scrapeAll(client *azuredevops.Client, cfg *config.Config) {
	for _, project := range cfg.Projects {
		if project.Enabled(config.ComponentRepos) {
			scrapeComponent(config.ComponentRepos, client, cfg, project.Name, collectors.CollectRepos)
		}
		if project.Enabled(config.ComponentBoards) {
			scrapeComponent(config.ComponentBoards, client, cfg, project.Name, collectors.CollectBoards)
		}
		if project.Enabled(config.ComponentPipelines) {
			scrapeComponent(config.ComponentPipelines, client, cfg, project.Name, collectors.CollectPipelines)
		}
		if project.Enabled(config.ComponentReleases) {
			scrapeComponent(config.ComponentReleases, client, cfg, project.Name, collectors.CollectReleases)
		}
	}
}

func scrapeComponent(component string, client *azuredevops.Client, cfg *config.Config, project string, collect func(*azuredevops.Client, string, string) error) {
	start := time.Now()
	err := collect(client, cfg.Organization, project)
	duration := time.Since(start).Seconds()

	metrics.ScrapeDurationSeconds.WithLabelValues(component, cfg.Organization, project).Set(duration)

	if err != nil {
		metrics.ScrapeErrorsTotal.WithLabelValues(component, cfg.Organization, project).Inc()
		slog.Error("scrape failed", "component", component, "project", project, "error", err)
		return
	}

	metrics.LastSuccessfulScrapeTimestamp.WithLabelValues(component, cfg.Organization, project).Set(float64(time.Now().Unix()))
	slog.Info("scrape succeeded", "component", component, "project", project, "duration_seconds", duration)
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
