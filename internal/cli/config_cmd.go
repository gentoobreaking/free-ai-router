package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/freemodel/router/internal/config"
)

func RunConfigCommand(opts *Options) error {
	switch {
	case opts.ConfigExport:
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		token, err := config.ExportToken(cfg)
		if err != nil {
			return err
		}
		fmt.Println(token)
		return nil

	case opts.ConfigImport != "":
		cfg, err := config.ImportToken(opts.ConfigImport)
		if err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Println("Config imported.")
		return nil
	}

	parts := strings.Fields(opts.SetKeys)
	if len(parts) >= 2 && strings.Contains(opts.SetKeys, " ") {
		return configSetKeys(parts[0], parts[1:])
	}

	if opts.AddKey != "" {
		parts := strings.Fields(opts.AddKey)
		if len(parts) >= 2 {
			return configAddKey(parts[0], parts[1])
		}
	}

	if opts.RemoveKey != "" {
		parts := strings.Fields(opts.RemoveKey)
		if len(parts) >= 2 {
			return configRemoveKey(parts[0], parts[1])
		}
	}

	if opts.SetMaxTurns != "" {
		parts := strings.Fields(opts.SetMaxTurns)
		if len(parts) >= 2 {
			return configSetMaxTurns(parts[0], parts[1])
		}
	}

	return fmt.Errorf("invalid config command")
}

func configSetKeys(provider string, keys []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cleanKeys := make([]string, 0, len(keys))
	for _, k := range keys {
		for _, single := range strings.Split(k, ",") {
			single = strings.TrimSpace(single)
			if single != "" {
				cleanKeys = append(cleanKeys, single)
			}
		}
	}

	if len(cleanKeys) == 1 {
		cfg.APIKeys[provider] = cleanKeys[0]
	} else {
		arr := make([]interface{}, len(cleanKeys))
		for i, k := range cleanKeys {
			arr[i] = k
		}
		cfg.APIKeys[provider] = arr
	}

	pcfg, ok := cfg.Providers[provider]
	if !ok {
		pcfg = config.ProviderConfig{}
	}
	pcfg.Enabled = true
	cfg.Providers[provider] = pcfg

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Set %d key(s) for %s.\n", len(cleanKeys), provider)
	return nil
}

func configAddKey(provider, key string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	keys := config.ResolveAPIKeys(provider, cfg)
	if len(keys) == 1 && !keyExists(keys, key) {
		keys = append([]string{keys[0]}, key)
	} else if !keyExists(keys, key) {
		keys = append(keys, key)
	}

	if len(keys) == 1 {
		cfg.APIKeys[provider] = keys[0]
	} else {
		arr := make([]interface{}, len(keys))
		for i, k := range keys {
			arr[i] = k
		}
		cfg.APIKeys[provider] = arr
	}

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Added key for %s.\n", provider)
	return nil
}

func configRemoveKey(provider, keyOrIndex string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	keys := config.ResolveAPIKeys(provider, cfg)
	if len(keys) == 0 {
		return fmt.Errorf("no keys configured for %s", provider)
	}

	removeIndex := -1
	if idx, err := strconv.Atoi(keyOrIndex); err == nil {
		if idx >= 0 && idx < len(keys) {
			removeIndex = idx
		}
	} else {
		for i, k := range keys {
			if k == keyOrIndex {
				removeIndex = i
				break
			}
		}
	}

	if removeIndex < 0 {
		return fmt.Errorf("key not found")
	}

	keys = append(keys[:removeIndex], keys[removeIndex+1:]...)
	if len(keys) == 0 {
		delete(cfg.APIKeys, provider)
	} else if len(keys) == 1 {
		cfg.APIKeys[provider] = keys[0]
	} else {
		arr := make([]interface{}, len(keys))
		for i, k := range keys {
			arr[i] = k
		}
		cfg.APIKeys[provider] = arr
	}

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Removed key %d from %s.\n", removeIndex, provider)
	return nil
}

func configSetMaxTurns(provider, turnsStr string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	turns, err := strconv.Atoi(turnsStr)
	if err != nil || turns < 0 {
		return fmt.Errorf("invalid maxTurns: %s", turnsStr)
	}

	pcfg, ok := cfg.Providers[provider]
	if !ok {
		pcfg = config.ProviderConfig{}
	}
	pcfg.MaxTurns = turns
	cfg.Providers[provider] = pcfg

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Set maxTurns=%d for %s.\n", turns, provider)
	return nil
}

func keyExists(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}
