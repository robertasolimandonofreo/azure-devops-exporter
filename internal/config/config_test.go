package config

import "testing"

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AZURE_DEVOPS_ORGANIZATION", "AZURE_DEVOPS_PROJECTS", "AZURE_DEVOPS_TOKEN",
		"AZURE_DEVOPS_API_URL", "EXPORTER_PORT", "SCRAPE_INTERVAL_SECONDS", "LOG_LEVEL",
	} {
		t.Setenv(k, "")
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	clearEnv(t)
	if _, err := Load(); err == nil {
		t.Fatal("expected error when required env vars are missing")
	}
}

func TestLoad_MultiProjectAndDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("AZURE_DEVOPS_ORGANIZATION", "my-org")
	t.Setenv("AZURE_DEVOPS_PROJECTS", " proj-a, proj-b ,,proj-c")
	t.Setenv("AZURE_DEVOPS_TOKEN", "secret-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantProjects := []string{"proj-a", "proj-b", "proj-c"}
	if len(cfg.Projects) != len(wantProjects) {
		t.Fatalf("got %v projects, want %v", cfg.Projects, wantProjects)
	}
	for i, p := range wantProjects {
		if cfg.Projects[i] != p {
			t.Errorf("project[%d] = %q, want %q", i, cfg.Projects[i], p)
		}
	}

	if cfg.APIURL != defaultAPIURL {
		t.Errorf("APIURL = %q, want default %q", cfg.APIURL, defaultAPIURL)
	}
	if cfg.Port != defaultPort {
		t.Errorf("Port = %d, want default %d", cfg.Port, defaultPort)
	}
	if cfg.ScrapeInterval != defaultScrapeInterval {
		t.Errorf("ScrapeInterval = %v, want default %v", cfg.ScrapeInterval, defaultScrapeInterval)
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("AZURE_DEVOPS_ORGANIZATION", "my-org")
	t.Setenv("AZURE_DEVOPS_PROJECTS", "proj-a")
	t.Setenv("AZURE_DEVOPS_TOKEN", "secret-token")
	t.Setenv("EXPORTER_PORT", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid EXPORTER_PORT")
	}
}
