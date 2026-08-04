package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultVersion = "v0.1.0"

// Version is the single version source: the VERSION file at the repo/install
// root when present, else the default. Kept in sync so the binary, /api/meta,
// and update checks all report the same value.
var Version = loadVersion()

type Options struct {
	Command      string
	Port         int
	Log          bool
	NoLog        bool
	Ban          string
	AllModels    bool
	Onboard      bool
	Help         bool
	ShowVersion  bool
	Best         bool
	ConfigExport bool
	ConfigImport string
	SetKeys      string
	AddKey       string
	RemoveKey    string
	SetMaxTurns  string
	AutoUpdate   string
	AutoStart    string
}

func ParseArgs(args []string) (*Options, error) {
	opts := &Options{Port: 7352}

	var positional []string

	i := 0
	for i < len(args) {
		arg := args[i]
		i++

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}

		name := strings.TrimLeft(arg, "-")
		value := ""
		hasValue := false
		if idx := strings.Index(name, "="); idx >= 0 {
			value = name[idx+1:]
			name = name[:idx]
			hasValue = true
		}

		switch name {
		case "port":
			if !hasValue && i < len(args) {
				value = args[i]
				i++
			}
			if p, err := strconv.Atoi(value); err == nil {
				opts.Port = p
			} else {
				return nil, fmt.Errorf("invalid port: %s", value)
			}
		case "log":
			opts.Log = true
		case "no-log":
			opts.NoLog = true
		case "ban":
			if !hasValue && i < len(args) {
				value = args[i]
				i++
			}
			opts.Ban = value
		case "all-models":
			opts.AllModels = true
		case "onboard":
			opts.Onboard = true
		case "best":
			opts.Best = true
		case "help", "h":
			opts.Help = true
		case "version", "v":
			opts.ShowVersion = true
		default:
			return nil, fmt.Errorf("unknown flag: %s", arg)
		}
	}

	if len(positional) > 0 {
		switch positional[0] {
		case "start":
			opts.Command = "start"
		case "onboard":
			opts.Onboard = true
		case "status":
			opts.Command = "status"
		case "update":
			opts.Command = "update"
		case "refresh-scores":
			opts.Command = "refresh-scores"
		case "best":
			opts.Best = true
		case "config":
			opts.Command = "config"
			if len(positional) > 1 {
				switch positional[1] {
				case "export":
					opts.ConfigExport = true
				case "import":
					if len(positional) > 2 {
						opts.ConfigImport = positional[2]
					}
				case "set-keys":
					opts.Command = "config-set-keys"
					if len(positional) > 3 {
						opts.SetKeys = positional[2] + " " + positional[3]
					}
				case "add-key":
					opts.Command = "config-add-key"
					if len(positional) > 3 {
						opts.AddKey = positional[2] + " " + positional[3]
					}
				case "remove-key":
					opts.Command = "config-remove-key"
					if len(positional) > 3 {
						opts.RemoveKey = positional[2] + " " + positional[3]
					}
				case "set-maxturns":
					opts.Command = "config-set-maxturns"
					if len(positional) > 3 {
						opts.SetMaxTurns = positional[2] + " " + positional[3]
					}
				}
			}
		case "autoupdate":
			opts.Command = "autoupdate"
			if len(positional) > 1 {
				opts.AutoUpdate = positional[1]
			}
		case "autostart":
			opts.Command = "autostart"
			if len(positional) > 1 {
				opts.AutoStart = positional[1]
			}
		default:
			return nil, fmt.Errorf("unknown command: %s", positional[0])
		}
	}

	if opts.Help {
		opts.Command = "help"
	}
	if opts.ShowVersion {
		opts.Command = "version"
	}

	return opts, nil
}

func PrintHelp() {
	fmt.Print(`FreeModel Router - free-tier AI model router

Usage:
  freemodel                           Interactive TUI (default)
  freemodel start [--port 7352]       Start router server (background mode)
  freemodel onboard                   Interactive key setup wizard
  freemodel --best                    Print best model ID to stdout
  freemodel status                    Show provider/account status
  freemodel update                    Manual update check & apply
  freemodel refresh-scores            Re-fetch model quality scores
  freemodel config export             Print config as transfer token
  freemodel config import <token>     Import config from token
  freemodel config set-keys <provider> <key1,key2,...>
  freemodel config add-key <provider> <key>
  freemodel config remove-key <provider> <key|index>
  freemodel config set-maxturns <provider> <number>
  freemodel autoupdate [enable|disable|status]
  freemodel autostart [install|start|uninstall|status]

Flags:
  --port <n>        Router HTTP port (default: 7352)
  --log             Enable request payload logging
  --no-log          Disable request logging (default)
  --ban <ids>       Comma-separated model IDs to ban
  --all-models      Disable coding-only filter
  --onboard         Same as onboard subcommand
  --help, -h        Show help
  --version, -v     Show version

Environment:
  FREMODEL_PORT           Router listen port (default 7352)
  FREMODEL_LOG            Enable request payload logging
  FREMODEL_CONFIG_PATH    Override config file path
`)
}

func PrintVersion() {
	fmt.Println("freemodel version " + Version)
}

// loadVersion reads the VERSION file next to the executable first (installed
// layout), then the CWD (source/dev layout), falling back to the default.
func loadVersion() string {
	candidates := []string{"VERSION"}
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "VERSION")}, candidates...)
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		v := strings.TrimSpace(string(data))
		if v != "" {
			return v
		}
	}
	return defaultVersion
}
