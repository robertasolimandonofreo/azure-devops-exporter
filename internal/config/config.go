// Package config loads exporter configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"azure-devops-exporter/internal/azuredevops"
)

const (
	defaultAPIURL         = "https://dev.azure.com"
	defaultPort           = 8080
	defaultScrapeInterval = 300 * time.Second
	defaultLogLevel       = "info"
)

// Collector names accepted in a project's optional AZURE_DEVOPS_PROJECTS collector list —
// these must match the component names main.go passes to scrapeComponent.
const (
	ComponentRepos     = "repos"
	ComponentBoards    = "boards"
	ComponentPipelines = "pipelines"
	ComponentReleases  = "releases"
)

var validComponents = map[string]bool{
	ComponentRepos:     true,
	ComponentBoards:    true,
	ComponentPipelines: true,
	ComponentReleases:  true,
}

// ProjectConfig is one project to scrape, and which collectors to run for it.
type ProjectConfig struct {
	Name string
	// Collectors is the set of component names to run for this project. Empty (nil) means "run
	// all four" — the exporter's original, still-default behavior — so a project listed without
	// a collector list is unaffected by this feature.
	Collectors map[string]bool
}

// Enabled reports whether the given component should be scraped for this project.
func (p ProjectConfig) Enabled(component string) bool {
	if len(p.Collectors) == 0 {
		return true
	}
	return p.Collectors[component]
}

// String renders the project for logging, e.g. "proj-a" (all collectors) or
// "proj-b(boards+pipelines)" (restricted).
func (p ProjectConfig) String() string {
	if len(p.Collectors) == 0 {
		return p.Name
	}
	names := make([]string, 0, len(p.Collectors))
	for c := range p.Collectors {
		names = append(names, c)
	}
	sort.Strings(names)
	return fmt.Sprintf("%s(%s)", p.Name, strings.Join(names, "+"))
}

// Config holds the exporter's runtime configuration.
type Config struct {
	Organization   string
	Projects       []ProjectConfig
	Token          string
	APIURL         string
	Port           int
	ScrapeInterval time.Duration
	LogLevel       string
	// BoardsCustomFields is the project-specific work item fields (e.g. a "Platform" picklist)
	// to break azure_devops_boards_work_items_by_custom_field_total down by, in addition to the
	// fixed set of fields every other Boards metric already uses. Applies to every project —
	// there's no per-project custom field list, only the per-project collector selection above.
	BoardsCustomFields []azuredevops.CustomField
	// BoardsExcludeStates is a list of work item state names to exclude from all Boards metrics.
	// Defaults to ["Removed"] (the standard "removed" state name in built-in Azure DevOps process
	// templates). Add custom states such as "Canceled" for projects using a custom process template
	// where the removed-category state has a different name.
	// Set via AZURE_DEVOPS_BOARDS_EXCLUDE_STATES (comma-separated, e.g. "Removed,Canceled").
	BoardsExcludeStates []string
}

// Load reads configuration from environment variables and validates required fields.
func Load() (*Config, error) {
	cfg := &Config{
		Organization: strings.TrimSpace(os.Getenv("AZURE_DEVOPS_ORGANIZATION")),
		Token:        os.Getenv("AZURE_DEVOPS_TOKEN"),
		APIURL:       envOrDefault("AZURE_DEVOPS_API_URL", defaultAPIURL),
		LogLevel:     envOrDefault("LOG_LEVEL", defaultLogLevel),
	}

	projects, err := parseProjects(os.Getenv("AZURE_DEVOPS_PROJECTS"))
	if err != nil {
		return nil, err
	}
	cfg.Projects = projects

	customFields, err := parseCustomFields(os.Getenv("AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS"))
	if err != nil {
		return nil, err
	}
	cfg.BoardsCustomFields = customFields
	cfg.BoardsExcludeStates = parseExcludeStates(os.Getenv("AZURE_DEVOPS_BOARDS_EXCLUDE_STATES"))

	port, err := envIntOrDefault("EXPORTER_PORT", defaultPort)
	if err != nil {
		return nil, err
	}
	cfg.Port = port

	interval, err := envDurationSecondsOrDefault("SCRAPE_INTERVAL_SECONDS", defaultScrapeInterval)
	if err != nil {
		return nil, err
	}
	cfg.ScrapeInterval = interval

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Organization == "" {
		return fmt.Errorf("AZURE_DEVOPS_ORGANIZATION is required")
	}
	if len(c.Projects) == 0 {
		return fmt.Errorf("AZURE_DEVOPS_PROJECTS is required (comma-separated list)")
	}
	if c.Token == "" {
		return fmt.Errorf("AZURE_DEVOPS_TOKEN is required")
	}
	return nil
}

// parseProjects parses AZURE_DEVOPS_PROJECTS: a comma-separated list of project names, each
// optionally followed by ":" and a "+"-separated list of collectors to restrict that project
// to (e.g. "proj-a:boards+pipelines,proj-b:repos,proj-c" — proj-c has no ":", so it gets every
// collector, same as every project did before this option existed).
func parseProjects(raw string) ([]ProjectConfig, error) {
	parts := strings.Split(raw, ",")
	projects := make([]ProjectConfig, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		name, collectorList, hasCollectors := strings.Cut(p, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("AZURE_DEVOPS_PROJECTS: empty project name in %q", p)
		}

		proj := ProjectConfig{Name: name}
		if hasCollectors {
			collectors, err := parseCollectors(name, collectorList)
			if err != nil {
				return nil, err
			}
			proj.Collectors = collectors
		}
		projects = append(projects, proj)
	}
	return projects, nil
}

func parseCollectors(projectName, raw string) (map[string]bool, error) {
	names := strings.Split(raw, "+")
	collectors := make(map[string]bool, len(names))
	for _, c := range names {
		c = strings.TrimSpace(c)
		if !validComponents[c] {
			return nil, fmt.Errorf(
				"AZURE_DEVOPS_PROJECTS: unknown collector %q for project %q (must be one of repos, boards, pipelines, releases)",
				c, projectName,
			)
		}
		collectors[c] = true
	}
	return collectors, nil
}

// parseCustomFields parses AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS: a comma-separated list of Azure
// DevOps field reference names, each optionally followed by ":" and a friendlier label to use
// in the metric's "field" label instead of the raw reference name (e.g.
// "Custom.Platform:platform,Custom.Squad" — the second field has no ":", so its label defaults
// to its reference name, "Custom.Squad").
func parseCustomFields(raw string) ([]azuredevops.CustomField, error) {
	parts := strings.Split(raw, ",")
	fields := make([]azuredevops.CustomField, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		refName, label, hasLabel := strings.Cut(p, ":")
		refName = strings.TrimSpace(refName)
		if refName == "" {
			return nil, fmt.Errorf("AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS: empty field reference name in %q", p)
		}
		if seen[refName] {
			return nil, fmt.Errorf("AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS: field %q listed more than once", refName)
		}
		seen[refName] = true

		if hasLabel {
			label = strings.TrimSpace(label)
		} else {
			label = refName
		}
		if label == "" {
			return nil, fmt.Errorf("AZURE_DEVOPS_BOARDS_CUSTOM_FIELDS: empty label for field %q", refName)
		}
		fields = append(fields, azuredevops.CustomField{RefName: refName, Label: label})
	}
	return fields, nil
}

// parseExcludeStates parses AZURE_DEVOPS_BOARDS_EXCLUDE_STATES: a comma-separated list of work
// item state names to exclude. Defaults to ["Removed"] when the env var is unset so that the
// standard Azure DevOps "Removed" state is excluded out of the box.
func parseExcludeStates(raw string) []string {
	if raw == "" {
		return []string{"Removed"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func envDurationSecondsOrDefault(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer number of seconds: %w", key, err)
	}
	return time.Duration(n) * time.Second, nil
}
