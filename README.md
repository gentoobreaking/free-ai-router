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
| `NEW_API_API_KEY` | - | New-API gateway API key (Chinese providers) |
| `SILICONFLOW_API_KEY` | - | SiliconFlow API key (Chinese models) |
| `QIANFAN_API_KEY` | - | Baidu QianFan (Ernie) API key |
| `DASHSCOPE_API_KEY` | - | Alibaba DashScope (BaiLian) API key |
| `TENCENT_CLOUD_API_KEY` | - | Tencent Cloud API key |
| `KUAIPAO_API_KEY` | - | Kuaipao / 筷跑 API key |

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

## Model Aggregation Flow

模型發現與聚合發生在 `LoadSources()` 中，由 `buildRegistry()` 在啟動時調用，單次呼叫即觸發三條平行路徑。所有結果在記憶體中合併，不產生中間檔案。

```
                          main.go: buildRegistry()
                                  │
                                  ▼
                   providers.LoadSources("data/sources.json")
                                  │
              ┌───────────────────┼───────────────────────┐
              ▼                   ▼                       ▼
  ┌─────────────────────┐ ┌─────────────────┐ ┌─────────────────────────┐
  │ ① Static Models     │ │ ② ClawLabs AI   │ │ ③ Dynamic Discovery     │
  │ (data/sources.json) │ │ Aggregation     │ │                         │
  └─────────┬───────────┘ └───────┬─────────┘ └───────────┬─────────────┘
            │                     │                       │
  ┌─────────┴───────────┐  ┌──────┴──────────┐  ┌────────┴───────────────┐
  │ Parse JSON           │  │ HTTP GET         │  │ ③a. Community Relay    │
  │ 讀取 sources.json    │  │ openrouter.ai    │  │ Scanner               │
  │                      │  │ /api/v1/models   │  │ ScannedRelaySites()   │
  │ 每個 provider 展開   │  │ (公開端點, no auth)│  │                        │
  │ src.Models[][]       │  │                  │  │ ├─ scanV2EXRelaySites()│
  │                      │  │ 過濾:            │  │ │  爬 go/ai 節點       │
  │ OpenRouter 額外過濾: │  │ pricing.prompt   │  │ │  關鍵字: 公益api,     │
  │ → 僅保留定價為       │  │   === "0" &&     │  │ │  new-api, 免費轉發   │
  │   "0"/"0" 的模型     │  │ pricing.completion│  │ │                      │
  │                      │  │   === "0"        │  │ ├─ scanLinuxDoRelaySites│
  │ Provider 列表:       │  │                  │  │ │  爬 c/ai/analysis    │
  │ nvidia, groq,        │  │ → map[id]bool    │  │ │  關鍵字: 公益, 免費,  │
  │ cerebras, openrouter,│  │   ~50-60 models  │  │ │  new-api, one-api    │
  │ googleai, opencode,  │  └──────┬───────────┘  │ │                      │
  │ codestral, scaleway, │         │               │ ├─ isRelayCandidate()  │
  │ kilocode, ollama,    │  ┌──────┴───────────┐  │ │  過濾非API URL       │
  │ new-api, siliconflow,│  │ Pollinations AI   │  │ │  (github, v2ex,      │
  │ baidu, alibabacloud, │  │ 18 static models  │  │ │   .js/.css/.png…)    │
  │ tencent, kuaipao     │  │ (hardcoded list)  │  │ │                      │
  │                      │  │                   │  │ ├─ ValidateNewApiRelay()│
  │ → m.providers[key]  │  │ pollinations/openai│  │ │  GET /v1/models      │
  │   per provider       │  │ pollinations/      │  │ │  驗證 JSON {"data":  │
  │                      │  │   deepseek        │  │ │    [{"id":"..."}]}   │
  └─────────┬───────────┘  │ pollinations/gemini │  │ │                      │
            │              │ ... (共18個)        │  │ └─ DiscoverModelsFrom  │
            │              └──────┬──────────────┘  │    Relay()            │
            │                     │                  │    GET /v1/models     │
            │                     │                  │    → ModelEntry[]     │
            │                     ▼                  │                       │
            │              ┌──────────────────┐      │ → m.providers[       │
            │              │ m.providers      │      │    "relay-{host}"]   │
            │              │ ["clawlabs"]     │      └──────────┬────────────┘
            │              │                  │                  │
            │              │ Discoverable:    │      ┌───────────┴───────────┐
            │              │ false            │      │ ③b. AutoDiscover      │
            │              │ (不參與動態發現)  │      │ (LoadSources 回傳後)  │
            │              └──────────────────┘      │                       │
            │                                        │ 對每個 provider       │
            │                                        │ Discoverable=true     │
            │                                        │ 且 BaseURL != ""     │
            │                                        │                       │
            │                                        │ HTTP GET              │
            │                                        │ {BaseURL}/v1/models   │
            │                                        │ (google → /v1beta)    │
            │                                        │                       │
            │                                        │ 合併新模型進           │
            │                                        │ p.Models (去重)       │
            │                                        └───────────┬───────────┘
            │                                                    │
            └──────────────────┬─────────────────────────────────┘
                               ▼
                    ┌──────────────────────┐
                    │  Manager.providers   │
                    │  (全在記憶體, no file)│
                    │                      │
                    │  ~20+ providers      │
                    │  ~130+ models        │
                    └──────────┬───────────┘
                               │
                 ┌─────────────┴──────────────┐
                 ▼                             ▼
        ┌─────────────────┐          ┌──────────────────┐
        │ registry.       │          │ 品質分數合併       │
        │ LoadFromSources │          │ LoadScores()     │
        │ (寫入 models     │          │ scores.json      │
        │  Registry)      │          │ QualityScore     │
        └────────┬────────┘          │ Tier             │
                 │                    └────────┬─────────┘
                 │                             │
                 └──────────┬──────────────────┘
                            ▼
                 ┌──────────────────────┐
                 │  Tag 合併            │
                 │  LoadBuiltIn()       │
                 │  model-tags.json     │
                 │  (coding, reasoning, │
                 │   vision, ...)       │
                 └──────────┬───────────┘
                            │
                            ▼
                 ┌──────────────────────┐
                 │  applyEndpoints()    │
                 │  m.Endpoint          │
                 │  m.UpstreamModelID   │
                 │  m.ProviderHost      │
                 └──────────┬───────────┘
                            │
              ┌─────────────┴──────────────┐
              ▼                             ▼
     ┌────────────────┐           ┌─────────────────┐
     │  Config 合併    │           │  Router Config   │
     │  API keys       │           │  CodingOnly     │
     │  cfg.CodingOnly │           │  BannedModels   │
     │  cfg.BannedModels│           │                 │
     └────────┬───────┘           └────────┬────────┘
              │                             │
              └──────────┬──────────────────┘
                         ▼
         ┌───────────────────────────────┐
         │         FINAL REGISTRY        │
         │  (記憶體中的模型全集)           │
         │                               │
         │  每個 Model 包含:              │
         │  • ID / Label / Provider     │
         │  • Endpoint / UpstreamModelID│
         │  • APIKey / ProviderHost     │
         │  • QualityScore / Tier       │
         │  • Tags (coding, vision, …)  │
         │  • Ping stats (runtime)      │
         └───────────────┬───────────────┘
                         │
          ┌──────────────┼──────────────┐
          ▼              ▼              ▼
   ┌────────────┐ ┌────────────┐ ┌──────────────┐
   │ Ping Engine│ │ TUI Display│ │ Router Server│
   │ (每2秒探測) │ │ (互動面板)  │ │ (127.0.0.1:  │
   │ 延遲/可用性 │ │ 排序/過濾   │ │  7352,       │
   │ QoS 計算   │ │ target設定  │ │  OpenAI compat│
   └────────────┘ └────────────┘ └──────────────┘
```

### 三條 Discovery 路徑詳解

#### ① Static Models — 本地定義

來源：`data/sources.json`。為每個 provider 展開其 `models` 陣列（三欄：`[model_id, display_name, context_window]`）。

對 `openrouter` provider 的特殊處理：比對 `fetchFreeOpenRouterModels()` 回傳的免費模型 map，只保留 `pricing.prompt == "0" && pricing.completion == "0"` 的靜態條目。

> **為何 OpenRouter 需要雙重過濾？** `sources.json` 是靜態 snapshot，可能包含已變為付費的模型。即時查詢 OpenRouter API 確保只保留當前定價為 $0 的模型。

涵蓋 19 個 provider：`nvidia`, `groq`, `cerebras`, `openrouter`, `googleai`, `opencode`, `codestral`, `scaleway`, `kilocode`, `ollama`, `new-api`, `siliconflow`, `baidu`, `alibabacloud`, `tencent`, `kuaipao`, `openai-compatible` 等。

#### ② ClawLabs AI Aggregation — 即時聚合

採用 [ClawLabsAI/free-ai-models](https://github.com/ClawLabsAI/free-ai-models) 的方法論（非讀取其靜態 JSON，而是每次啟動即時拉取）：

1. **HTTP GET** `https://openrouter.ai/api/v1/models`（公開端點，無需 API key）→ 篩選 `pricing.prompt == "0" && pricing.completion == "0"` → 約 50-60 個模型
2. **Pollinations AI 靜態清單**：18 個硬編碼模型（`pollinations/openai`, `pollinations/deepseek`, `pollinations/gemini`, `pollinations/claude`…），皆無需 API key

合併後掛在獨立 provider `"clawlabs"` 下，`Discoverable: false`（不參與③b 動態發現）。

#### ③ Dynamic Discovery — 社群與 API

分兩階段執行：

**③a. 社群中轉站掃描（`LoadSources()` 內）**

採用 [QuantumNous/new-api](https://github.com/QuantumNous/new-api) 相容的中轉站掃描策略（new-api 是部署在社群伺服器上的 OpenAI 相容閘道）：

1. **V2EX 爬蟲**：爬 `go/ai` 節點，搜尋關鍵字 `公益api`, `免費轉發`, `new-api`, `one-api`
2. **linux.do 爬蟲**：爬 `c/ai/analysis` 看板，搜尋關鍵字 `公益`, `免費`, `new-api`, `one-api`, `轉發`
3. **URL 過濾** (`isRelayCandidate`)：排除 GitHub/V2EX/linux.do/搜尋引擎、排除 `.js/.css/.png` 等靜態資源，保留含 `/v1/`、`new-api`、`one-api`、`.ai/`、`api.` 的 URL
4. **健檢** (`ValidateNewApiRelay`)：GET `{baseURL}/v1/models`，驗證回應為 `{"data":[{"id":"..."}]}` 格式
5. **模型發現** (`DiscoverModelsFromRelay`)：從健康中轉站拉取模型清單，掛到 `m.providers["relay-{host}"]`

所有中轉站以 `BaseURL` 去重，按 round-robin 方式提供 HA 故障轉移。

⚠️ 爬蟲結果取決於當前網路環境；受限環境下可能無可用中轉站。

**③b. Discoverable Provider 自動發現（`LoadSources()` 回傳後）**

`AutoDiscoverModels()` 對每個 `Discoverable: true` 的 provider（如 nvidia、groq、cerebras、googleai）：

- HTTP GET `{BaseURL}/v1/models`（Google AI 使用 `/v1beta/models`）
- 將回傳的模型列表合併進該 provider 的 `Models`（以 `provider/id` 格式命名，根據 model ID 去重）

### 過濾與 QoS 管線（LoadSources 之後）

在 `buildRegistry()` 中，模型合併完成後會經過一系列後處理：

| 步驟 | 函數 | 資料來源 |
|------|------|----------|
| 寫入 Registry | `registry.LoadFromSources(provMgr)` | `Manager.providers` → `Registry.models` |
| 品質分數 | `models.LoadScores()` | `data/scores.json`（離線 fallback），依 canonical ID 比對，未命中 → 0.45 |
| 層級計算 | `models.ComputeTier()` | QualityScore → S+/S/A+/A/A-/B+/B/C |
| 標籤合併 | `tagMgr.LoadBuiltIn()` | `data/model-tags.json`（coding、reasoning、vision 等） |
| Endpoint 設定 | `applyEndpoints()` | `m.Endpoint = provider.URL`、`m.UpstreamModelID`（去掉 provider prefix） |
| API Key 注入 | `config.ResolveAPIKey()` | `~/.freemodel-router.json` 或環境變數 |
| Coding filter | `registry.FlagCodingOnly()` | Config `codingOnly`（預設 true） |
| 模型封鎖 | `registry.BanModel()` | Config `bannedModels` + CLI `--ban` |

### 零依賴原則

**免去多方註冊 (No multi-party registration)**：
- **Pollinations AI**：唯一真正零 API key 的免費模型 — 無需註冊，無限制使用
- **OpenRouter catalog**：模型發現無需 API key（定價過濾）
- **ClawLabs path**：即時聚合 OpenRouter + Pollinations，無需單獨設定
- **中轉站路徑**：社群部署的 new-api 中轉站，自動發現無需手動配置
- **中國 providers**：配置一個 `NEW_API_API_KEY` 即可串接矽基流動/百度/阿里/騰訊等多家

## License

MIT
