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

func buildRegistry() (*models.Registry, *models.TagManager, *providers.Manager, error) {
	provMgr := providers.NewManager()
	if err := provMgr.LoadSources(providers.DataDir() + "/data/sources.json"); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load sources: %w", err)
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

	return registry, tagMgr, provMgr, nil
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
	registry, _, _, err := buildRegistry()
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

	applyRouterConfig(registry, cfg, opts)

	t := tui.New(&tui.Config{
		ScrollSortPauseMs: cfg.UI.ScrollSortPauseMs,
	})
	t.SetRegistry(registry)
	t.SetConfig(cfg)
	return t.Run()
}

func runServer(opts *cli.Options) error {
	registry, _, provMgr, err := buildRegistry()
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

	applyRouterConfig(registry, cfg, opts)

	pool := ping.NewTransportPool()

	engine := ping.NewEngine(nil)
	engine.SetRegistry(registry)
	engine.SetPool(pool)
	engine.SetModels(registry.GetAll())
	if cfg.AutoPingEnabled {
		engine.Start()
	}
	defer engine.Stop()

	port := config.GetPort(opts.Port)

	logEnabled := opts.Log || os.Getenv("FREMODEL_LOG") == "1"
	srv := router.NewServer(registry, cfg, port, cli.Version, logEnabled)
	srv.SetPool(pool)
	srv.SetProviders(provMgr)
	srv.SetEngine(engine)
	srv.SetUpdateChecker(func() (string, error) { return cli.CheckForUpdate(false) })

	log.Printf("freemodel router listening on 127.0.0.1:%d", port)
	return srv.Start()
}

// applyRouterConfig propagates config-level selection rules into the registry
// so router eligibility honors them (§3.2, §3.3).
func applyRouterConfig(registry *models.Registry, cfg *config.Config, opts *cli.Options) {
	registry.FlagCodingOnly(cfg.CodingOnly)

	for _, banned := range cfg.BannedModels {
		registry.BanModel(banned)
	}

	if opts.Ban != "" {
		for _, banned := range strings.Split(opts.Ban, ",") {
			banned = strings.TrimSpace(banned)
			if banned != "" {
				registry.BanModel(banned)
			}
		}
	}
}

func runBest(opts *cli.Options) error {
	registry, _, _, err := buildRegistry()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	applyRouterConfig(registry, cfg, opts)

	resolveKey := func(provider string) string {
		return config.ResolveAPIKey(provider, cfg)
	}

	_, err = cli.RunBest(registry, resolveKey)
	return err
}
