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

func buildRegistry(refresh bool, useCache bool, quiet bool) (*models.Registry, *models.TagManager, *providers.Manager, error) {
	provMgr := providers.NewManager()

	if quiet {
		provMgr.SetLogger(providers.NewDefaultLogger(providers.LevelSilent))
	} else {
		provMgr.SetLogger(providers.NewDefaultLogger(providers.LevelInfo))
	}

	if useCache {
		if err := provMgr.LoadSourcesWithCache(providers.DataDir()+"/data/sources.json", providers.DefaultCacheTTL, refresh); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to load sources: %w", err)
		}
	} else {
		if err := provMgr.LoadSources(providers.DataDir() + "/data/sources.json"); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to load sources: %w", err)
		}
	}

	provMgr.AutoDiscoverModels()

	registry := models.NewRegistry()
	registry.LoadFromSources(provMgr)

	// Scores (non-fatal: proceed without scores.json)
	_ = registry.ApplyScores(models.DataPath("scores.json"))

	// Tags (non-fatal: proceed without tags)
	tagMgr := models.NewTagManager()
	_ = registry.ApplyTags(tagMgr, models.DataPath("model-tags.json"))

	registry.ApplyEndpoints(provMgr)

	return registry, tagMgr, provMgr, nil
}

func runTUI(opts *cli.Options) error {
	registry, _, _, err := buildRegistry(opts.Refresh, !opts.NoCache, opts.Quiet)
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

	return tui.Run(registry, cfg)
}

func runServer(opts *cli.Options) error {
	registry, _, provMgr, err := buildRegistry(opts.Refresh, !opts.NoCache, opts.Quiet)
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
	registry, _, _, err := buildRegistry(opts.Refresh, !opts.NoCache, opts.Quiet)
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

	bestID, err := cli.RunBest(registry, resolveKey)
	_ = bestID
	return err
}
