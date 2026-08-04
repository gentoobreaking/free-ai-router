package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/freemodel/router/internal/cli"
	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
	"github.com/freemodel/router/internal/providers"
	"github.com/freemodel/router/internal/router"
	"github.com/freemodel/router/internal/tui"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("freemodel: %v", err)
	}
}

func run() error {
	opts, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		return err
	}

	switch opts.Command {
	case "help":
		cli.PrintHelp()
		return nil
	case "version":
		cli.PrintVersion()
		return nil
	case "onboard":
		return cli.RunOnboard()
	case "status":
		return cli.RunStatus()
	case "update":
		return cli.RunUpdate()
	case "refresh-scores":
		return nil
	case "config", "config-set-keys", "config-add-key", "config-remove-key", "config-set-maxturns":
		return cli.RunConfigCommand(opts)
	case "autoupdate":
		return cli.RunAutoUpdate(opts.AutoUpdate)
	case "autostart":
		return cli.RunAutostart(opts.AutoStart)
	case "start":
		return runServer(opts)
	}

	if opts.Onboard {
		return cli.RunOnboard()
	}
	if opts.ShowVersion {
		cli.PrintVersion()
		return nil
	}
	if opts.Help {
		cli.PrintHelp()
		return nil
	}
	if opts.Best {
		return runBest(opts)
	}

	return runTUI(opts)
}

func buildRegistry() (*models.Registry, *models.TagManager, error) {
	provMgr := providers.NewManager()
	if err := provMgr.LoadSources(providers.DataDir() + "/data/sources.json"); err != nil {
		return nil, nil, fmt.Errorf("failed to load sources: %w", err)
	}

	registry := models.NewRegistry()
	registry.LoadFromSources(provMgr)

	aliases, err := models.LoadAliases(models.DataPath("model-aliases.json"))
	if err == nil {
		_ = aliases
	}

	offlineScores, err := models.LoadScores(models.DataPath("scores.json"))
	if err == nil {
		for _, m := range registry.GetAll() {
			canonicalID := m.ID
			if strings.Contains(canonicalID, "/") {
				parts := strings.SplitN(canonicalID, "/", 2)
				canonicalID = parts[1]
			}
			score, ok := offlineScores[canonicalID]
			if !ok {
				score, ok = offlineScores[m.ID]
			}
			if !ok {
				parts := strings.SplitN(m.ID, "/", 3)
				if len(parts) == 3 {
					score, ok = offlineScores[parts[1]+"/"+parts[2]]
				}
			}
			if ok {
				m.QualityScore = score
			} else {
				m.QualityScore = 0.45
			}
			m.Tier = models.ComputeTier(m.QualityScore)
		}
	}

	tagMgr := models.NewTagManager()
	if err := tagMgr.LoadBuiltIn(models.DataPath("model-tags.json")); err == nil {
		for _, m := range registry.GetAll() {
			m.Tags = tagMgr.GetModelTags(m.ID)
		}
	}

	applyEndpoints(registry, provMgr)

	return registry, tagMgr, nil
}

func applyEndpoints(registry *models.Registry, provMgr *providers.Manager) {
	for _, m := range registry.GetAll() {
		provider := provMgr.GetProvider(m.Provider)
		if provider == nil {
			continue
		}
		if provider.URL != "" {
			m.Endpoint = provider.URL
		}
		parts := strings.SplitN(m.ID, "/", 2)
		upstreamID := m.ID
		if len(parts) == 2 {
			upstreamID = parts[1]
		}
		m.UpstreamModelID = upstreamID
		m.ProviderHost = provider.BaseURL
	}
}

func runTUI(opts *cli.Options) error {
	registry, _, err := buildRegistry()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	for _, m := range registry.GetAll() {
		m.APIKey = config.ResolveAPIKey(m.Provider, cfg)
	}

	if opts.AllModels {
		cfg.CodingOnly = false
	}

	t := tui.New(&tui.Config{
		ScrollSortPauseMs: cfg.UI.ScrollSortPauseMs,
		ForceClear:        os.Getenv("FREMODEL_TUI_FORCE_CLEAR") == "1",
		ConfigPath:        os.Getenv("FREMODEL_CONFIG_PATH"),
	})
	t.SetRegistry(registry)
	return t.Run()
}

func runServer(opts *cli.Options) error {
	registry, _, err := buildRegistry()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	for _, m := range registry.GetAll() {
		m.APIKey = config.ResolveAPIKey(m.Provider, cfg)
	}

	engine := ping.NewEngine(nil)
	engine.SetModels(registry.GetAll())
	engine.Start()
	defer engine.Stop()

	port := opts.Port
	if env := os.Getenv("FREMODEL_PORT"); env != "" {
		var p int
		_, err := fmt.Sscanf(env, "%d", &p)
		if err == nil && p > 0 {
			port = p
		}
	}

	logEnabled := opts.Log || os.Getenv("FREMODEL_LOG") == "1"
	srv := router.NewServer(registry, cfg, port, cli.Version, logEnabled)

	log.Printf("freemodel router listening on 127.0.0.1:%d", port)
	return srv.Start()
}

func runBest(opts *cli.Options) error {
	registry, _, err := buildRegistry()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	resolveKey := func(provider string) string {
		return config.ResolveAPIKey(provider, cfg)
	}

	_, err = cli.RunBest(registry, resolveKey)
	return err
}
