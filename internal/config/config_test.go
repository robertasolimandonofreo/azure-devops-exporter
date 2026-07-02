package config

import "testing"

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AZURE_DEVOPS_ORGANIZATION", "AZURE_DEVOPS_PROJECTS", "AZURE_DEVOPS_TOKEN",
		"AZURE_DEVOPS_API_URL", "EXPORTER_PORT", "SCRAPE_INTERVAL_SECONDS", "LOG_LEVEL",
		"AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS",
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
		if cfg.Projects[i].Name != p {
			t.Errorf("project[%d].Name = %q, want %q", i, cfg.Projects[i].Name, p)
		}
		if !cfg.Projects[i].Enabled(ComponentRepos) || !cfg.Projects[i].Enabled(ComponentBoards) ||
			!cfg.Projects[i].Enabled(ComponentPipelines) || !cfg.Projects[i].Enabled(ComponentReleases) {
			t.Errorf("project[%d] = %+v, want every collector enabled (no collector list given)", i, cfg.Projects[i])
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

func TestLoad_PerProjectCollectors(t *testing.T) {
	clearEnv(t)
	t.Setenv("AZURE_DEVOPS_ORGANIZATION", "my-org")
	t.Setenv("AZURE_DEVOPS_PROJECTS", "proj-a:pipelines+boards, proj-b:repos ,proj-c")
	t.Setenv("AZURE_DEVOPS_TOKEN", "secret-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Projects) != 3 {
		t.Fatalf("got %d projects, want 3", len(cfg.Projects))
	}

	a := cfg.Projects[0]
	if a.Name != "proj-a" {
		t.Errorf("project[0].Name = %q, want proj-a", a.Name)
	}
	if !a.Enabled(ComponentPipelines) || !a.Enabled(ComponentBoards) {
		t.Errorf("proj-a = %+v, want pipelines and boards enabled", a)
	}
	if a.Enabled(ComponentRepos) || a.Enabled(ComponentReleases) {
		t.Errorf("proj-a = %+v, want repos and releases disabled", a)
	}

	b := cfg.Projects[1]
	if b.Name != "proj-b" {
		t.Errorf("project[1].Name = %q, want proj-b", b.Name)
	}
	if !b.Enabled(ComponentRepos) {
		t.Errorf("proj-b = %+v, want repos enabled", b)
	}
	if b.Enabled(ComponentBoards) || b.Enabled(ComponentPipelines) || b.Enabled(ComponentReleases) {
		t.Errorf("proj-b = %+v, want only repos enabled", b)
	}

	// No ":" at all — every collector runs, same as the exporter's original behavior.
	c := cfg.Projects[2]
	if c.Name != "proj-c" {
		t.Errorf("project[2].Name = %q, want proj-c", c.Name)
	}
	if !c.Enabled(ComponentRepos) || !c.Enabled(ComponentBoards) || !c.Enabled(ComponentPipelines) || !c.Enabled(ComponentReleases) {
		t.Errorf("proj-c = %+v, want every collector enabled", c)
	}
}

func TestLoad_UnknownCollector(t *testing.T) {
	clearEnv(t)
	t.Setenv("AZURE_DEVOPS_ORGANIZATION", "my-org")
	t.Setenv("AZURE_DEVOPS_PROJECTS", "proj-a:pipelines+bogus")
	t.Setenv("AZURE_DEVOPS_TOKEN", "secret-token")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for unknown collector name")
	}
}

func TestLoad_NoCustomFields(t *testing.T) {
	clearEnv(t)
	t.Setenv("AZURE_DEVOPS_ORGANIZATION", "my-org")
	t.Setenv("AZURE_DEVOPS_PROJECTS", "proj-a")
	t.Setenv("AZURE_DEVOPS_TOKEN", "secret-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.BoardsCustomFields) != 0 {
		t.Errorf("BoardsCustomFields = %+v, want empty", cfg.BoardsCustomFields)
	}
}

func TestLoad_CustomFields(t *testing.T) {
	clearEnv(t)
	t.Setenv("AZURE_DEVOPS_ORGANIZATION", "my-org")
	t.Setenv("AZURE_DEVOPS_PROJECTS", "proj-a")
	t.Setenv("AZURE_DEVOPS_TOKEN", "secret-token")
	t.Setenv("AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS", "Custom.Platform:platform, Custom.Squad")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.BoardsCustomFields) != 2 {
		t.Fatalf("got %d custom fields, want 2", len(cfg.BoardsCustomFields))
	}
	if cfg.BoardsCustomFields[0].RefName != "Custom.Platform" || cfg.BoardsCustomFields[0].Label != "platform" {
		t.Errorf("custom field[0] = %+v, want RefName=Custom.Platform Label=platform", cfg.BoardsCustomFields[0])
	}
	// No ":label" — the label defaults to the reference name itself.
	if cfg.BoardsCustomFields[1].RefName != "Custom.Squad" || cfg.BoardsCustomFields[1].Label != "Custom.Squad" {
		t.Errorf("custom field[1] = %+v, want RefName=Label=Custom.Squad", cfg.BoardsCustomFields[1])
	}
}

func TestLoad_DuplicateCustomField(t *testing.T) {
	clearEnv(t)
	t.Setenv("AZURE_DEVOPS_ORGANIZATION", "my-org")
	t.Setenv("AZURE_DEVOPS_PROJECTS", "proj-a")
	t.Setenv("AZURE_DEVOPS_TOKEN", "secret-token")
	t.Setenv("AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS", "Custom.Platform:platform,Custom.Platform:platform2")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for duplicate custom field reference name")
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
