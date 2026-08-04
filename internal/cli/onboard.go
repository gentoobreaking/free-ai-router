package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/freemodel/router/internal/config"
)

type providerSignup struct {
	key       string
	prefix    string
	signupURL string
}

var signupInfo = map[string]providerSignup{
	"nvidia":     {prefix: "nvapi-", signupURL: "https://build.nvidia.com/"},
	"groq":       {prefix: "gsk_", signupURL: "https://console.groq.com/keys"},
	"cerebras":   {prefix: "cerebras", signupURL: "https://cloud.cerebras.ai/"},
	"openrouter": {prefix: "sk-or-", signupURL: "https://openrouter.ai/keys"},
	"googleai":   {prefix: "AIza", signupURL: "https://aistudio.google.com/apikey"},
	"opencode":   {prefix: "oc-", signupURL: "https://opencode.ai/"},
	"codestral":  {prefix: "", signupURL: "https://codestral.mistral.ai/"},
	"scaleway":   {prefix: "", signupURL: "https://console.scaleway.com/"},
	"kilocode":   {prefix: "", signupURL: "https://kilocode.ai/"},
	"ollama":     {prefix: "", signupURL: "https://ollama.com/"},
}

func RunOnboard() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════╗")
	fmt.Println("  ║      FreeModel Router Onboarding      ║")
	fmt.Println("  ╚══════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("  For each provider, choose: [O]pen browser + enter key, [E]nter key manually, [S]kip")
	fmt.Println()

	providers := []string{"nvidia", "groq", "cerebras", "openrouter", "googleai", "opencode", "codestral", "scaleway", "kilocode"}

	for _, provider := range providers {
		info, ok := signupInfo[provider]
		if !ok {
			continue
		}

		fmt.Printf("  %s\n", provider)
		fmt.Print("  Choice (O/E/S): ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToUpper(choice))

		switch choice {
		case "O":
			fmt.Printf("    Opening %s ...\n", info.signupURL)
			_ = openBrowser(info.signupURL)
			key := promptKey(reader, provider, info.prefix)
			if key != "" {
				cfg.APIKeys[provider] = key
				setEnabled(cfg, provider)
			}
		case "E":
			key := promptKey(reader, provider, info.prefix)
			if key != "" {
				cfg.APIKeys[provider] = key
				setEnabled(cfg, provider)
			}
		default:
			fmt.Println("    Skipped.")
		}
		fmt.Println()
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println("  Config saved. Done.")
	return nil
}

func promptKey(reader *bufio.Reader, provider, prefix string) string {
	fmt.Printf("    Enter API key for %s: ", provider)
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if prefix != "" && !strings.HasPrefix(key, prefix) {
		fmt.Printf("    Warning: key does not start with expected prefix %q\n", prefix)
	}
	return key
}

func setEnabled(cfg *config.Config, provider string) {
	pcfg, ok := cfg.Providers[provider]
	if !ok {
		pcfg = config.ProviderConfig{}
	}
	pcfg.Enabled = true
	cfg.Providers[provider] = pcfg
}

// openBrowser opens url with the platform's default browser (best effort).
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Run()
}
