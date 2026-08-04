# FreeModel Router

A high-performance, free-tier-only AI model router that discovers, pings, and routes requests to the best available free LLM, with an interactive TUI for live model health monitoring.

## Features

- **100% free**: Only free-tier models, no paid APIs required
- **Live TUI**: Parallel pings every 2s with real-time latency, uptime, verdict
- **Smart routing**: QoS-based model selection with automatic failover
- **Multi-agent support**: OpenCode, OpenClaw, Hermes, and Pi agent
- **Coding filter**: Only models flagged with coding capability are eligible by default

## Requirements

- Go 1.23+
- macOS, Linux, or Windows

## Install & Build

```bash
git clone https://github.com/freemodel/router
cd router
make build          # builds dist/freemodel
make build-all      # cross-compile for all 5 platforms
make test           # run all tests
make lint           # go vet
```

## Quick Start

```bash
# 1. Onboard API keys (interactive wizard)
freemodel onboard

# 2. Launch the interactive TUI
freemodel

# 3. (Optional) Start the router server in background
freemodel start --port 7352

# 4. Point your agent at the router
#    baseURL: http://127.0.0.1:7352/v1
#    model:   auto-fastest
```

## CLI Commands

```bash
freemodel                           # Interactive TUI (default)
freemodel start [--port 7352]       # Start router server
freemodel onboard                   # Interactive key setup wizard
freemodel --best                    # Print best model ID (scripting)
freemodel status                    # Show provider/account status
freemodel update                    # Manual update check & apply
freemodel refresh-scores            # Re-fetch model quality scores
freemodel config export             # Print config as transfer token
freemodel config import <token>     # Import config from token
freemodel config set-keys <provider> <key1,key2,...>
freemodel config add-key <provider> <key>
freemodel config remove-key <provider> <key|index>
freemodel config set-maxturns <provider> <number>
freemodel autoupdate [enable|disable|status]
freemodel autostart [install|start|uninstall|status]
```

### Flags

| Flag | Description |
|---|---|
| `--port <n>` | Router HTTP port (default: 7352) |
| `--log` | Enable request payload logging |
| `--no-log` | Disable request logging (default) |
| `--ban <ids>` | Comma-separated model IDs to ban |
| `--all-models` | Disable coding-only filter |
| `--onboard` | Same as `onboard` subcommand |
| `--help` / `-h` | Show help |
| `--version` / `-v` | Show version |

### `--best` Mode

Non-interactive mode that pings all models for 4 rounds and prints the best model ID to stdout. Designed for scripting:

```bash
MODEL=$(freemodel --best)
```

Selection: status=up → lowest avg latency → highest uptime.

## TUI Keyboard Shortcuts

| Key | Action |
|---|---|
| ↑↓ / j k | Navigate models |
| PgUp / PgDn | Page up/down |
| g / G | Jump to top / bottom |
| `/` | Toggle search |
| Enter | Configure model for a target agent |
| `A` | Quick API key add/change |
| `P` | Settings screen |
| `T` | Cycle tier filter |
| `C` | Toggle coding-only filter |
| `W` / `X` | Decrease / increase ping interval |
| `N` | Cycle provider filter |
| `0-9` | Sort by column (press again to reverse) |
| `?` | Help overlay |
| `q` / Ctrl+C | Quit |

## Config

Config file: `~/.freemodel-router.json` (mode 0600; legacy `~/.free-router.json` auto-migrated on first load)

```json
{
  "apiKeys": {
    "nvidia": "nvapi-xxx",
    "openrouter": ["sk-or-xxx", "sk-or-yyy"]
  },
  "providers": {
    "nvidia": { "enabled": true },
    "openai-compatible:my-vllm": {
      "enabled": true,
      "name": "Local vLLM",
      "baseUrl": "http://localhost:8000/v1",
      "modelId": "qwen-coder",
      "discoverModels": true,
      "maxTurns": 20
    }
  },
  "bannedModels": [],
  "minSweScore": null,
  "excludedProviders": [],
  "pinningMode": "canonical",
  "autoPingEnabled": true,
  "codingOnly": true,
  "ui": { "scrollSortPauseMs": 1500 }
}
```

Config export/import uses `mrconf:v1:<base64url>` token format, compatible with modelrelay.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `FREMODEL_PORT` | 7352 | Router listen port |
| `FREMODEL_LOG` | false | Enable request payload logging |
| `FREMODEL_CONFIG_PATH` | `~/.freemodel-router.json` | Override config file path |
| `NVIDIA_API_KEY` | - | NIM API key |
| `GROQ_API_KEY` | - | Groq API key |
| `CEREBRAS_API_KEY` | - | Cerebras API key |
| `OPENROUTER_API_KEY` | - | OpenRouter API key |
| `OPENCODE_API_KEY` | - | OpenCode Zen API key |
| `GOOGLE_API_KEY` | - | Google AI API key |

## Docker

```bash
make docker
docker build -t freemodel-router .
docker compose up -d
```

## Target Agent Integration

Press Enter on a model in the TUI to configure a target agent:

| Agent | Config File |
|---|---|
| OpenCode | `~/.config/opencode/opencode.json` |
| OpenClaw | `~/.openclaw/openclaw.json` |
| Hermes Agent | `~/.hermes/config.yaml` |
| Pi Agent | `~/.pi/pi.json` |

## Testing

```bash
go test ./... -v
```

Unit tests: config, utils, tags, models, CLI parsing, ping logic.
Integration tests: TUI rendering, proxy failover, provider discovery, target handoff.

## Architecture

```
freemodel-router (Go binary)
├── cmd/freemodel/          # CLI entry point
├── internal/
│   ├── config/             # Config I/O (~/.freemodel-router.json)
│   ├── providers/          # Provider definitions, discovery, auth
│   ├── models/             # Model catalog, aliasing, quality scoring, tags
│   ├── ping/               # Parallel ping engine with keep-alive pool
│   ├── router/             # OpenAI-compatible HTTP reverse proxy
│   ├── tui/                # Interactive terminal UI (ANSI raw mode)
│   ├── targets/            # Config writers for agent targets
│   └── cli/                # CLI argument parsing, commands
├── data/
│   ├── sources.json        # Provider definitions
│   ├── scores.json         # Offline model quality fallbacks
│   ├── model-tags.json     # Built-in capability tags
│   └── model-aliases.json  # Alias to canonical ID mapping
└── go.mod
```

## License

MIT
