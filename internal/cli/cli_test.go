package cli

import "testing"

func TestParseArgsDefault(t *testing.T) {
	opts, err := ParseArgs(nil)
	if err != nil {
		t.Fatalf("ParseArgs(nil): %v", err)
	}
	if opts.Command != "" {
		t.Errorf("default command should be empty (TUI), got %q", opts.Command)
	}
	if opts.Port != 7352 {
		t.Errorf("default port should be 7352, got %d", opts.Port)
	}
}

func TestParseArgsStart(t *testing.T) {
	opts, err := ParseArgs([]string{"start", "--port", "9000"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.Command != "start" {
		t.Errorf("command should be start, got %q", opts.Command)
	}
	if opts.Port != 9000 {
		t.Errorf("port should be 9000, got %d", opts.Port)
	}
}

func TestParseArgsFlags(t *testing.T) {
	opts, err := ParseArgs([]string{"--log", "--all-models", "--ban", "a,b,c", "--port", "8000"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if !opts.Log {
		t.Error("--log should set Log")
	}
	if !opts.AllModels {
		t.Error("--all-models should set AllModels")
	}
	if opts.Ban != "a,b,c" {
		t.Errorf("--ban should be a,b,c, got %q", opts.Ban)
	}
	if opts.Port != 8000 {
		t.Errorf("port should be 8000, got %d", opts.Port)
	}
}

func TestParseArgsBest(t *testing.T) {
	opts, err := ParseArgs([]string{"--best"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if !opts.Best {
		t.Error("--best should set Best")
	}
}

func TestParseArgsVersionHelp(t *testing.T) {
	opts, _ := ParseArgs([]string{"--version"})
	if opts.Command != "version" {
		t.Errorf("--version should set command=version, got %q", opts.Command)
	}

	opts, _ = ParseArgs([]string{"--help"})
	if opts.Command != "help" {
		t.Errorf("--help should set command=help, got %q", opts.Command)
	}
}

func TestParseArgsSubcommands(t *testing.T) {
	opts, err := ParseArgs([]string{"onboard"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if !opts.Onboard {
		t.Error("onboard should set Onboard")
	}

	opts, err = ParseArgs([]string{"status"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.Command != "status" {
		t.Errorf("status command should be status, got %q", opts.Command)
	}

	opts, err = ParseArgs([]string{"update"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.Command != "update" {
		t.Errorf("update command should be update, got %q", opts.Command)
	}
}

func TestParseArgsConfigSubcommands(t *testing.T) {
	opts, err := ParseArgs([]string{"config", "export"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if !opts.ConfigExport {
		t.Error("config export should set ConfigExport")
	}

	opts, err = ParseArgs([]string{"config", "import", "mrconf:v1:abc"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.ConfigImport != "mrconf:v1:abc" {
		t.Errorf("config import should set token, got %q", opts.ConfigImport)
	}

	opts, err = ParseArgs([]string{"config", "set-keys", "nvidia", "key1,key2"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.SetKeys != "nvidia key1,key2" {
		t.Errorf("set-keys should capture provider+keys, got %q", opts.SetKeys)
	}
}

func TestParseArgsAutoupdateAutostart(t *testing.T) {
	opts, err := ParseArgs([]string{"autoupdate", "enable"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.Command != "autoupdate" || opts.AutoUpdate != "enable" {
		t.Errorf("autoupdate parsing wrong: %+v", opts)
	}

	opts, err = ParseArgs([]string{"autostart", "install"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.Command != "autostart" || opts.AutoStart != "install" {
		t.Errorf("autostart parsing wrong: %+v", opts)
	}
}

func TestParseArgsUnknownCommand(t *testing.T) {
	_, err := ParseArgs([]string{"bogus-command"})
	if err == nil {
		t.Error("unknown command should return error")
	}
}
