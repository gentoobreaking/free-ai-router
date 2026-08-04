package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/providers"
)

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

	// Build list from centralized provider registry, skipping providers
	// without a signup URL.
	var onboardList []providers.ProviderMeta
	for _, p := range providers.AllProviders {
		if p.SignupURL != "" {
			onboardList = append(onboardList, p)
		}
	}

	for _, p := range onboardList {
		fmt.Printf("  %s\n", p.Key)
		fmt.Print("  Choice (O/E/S): ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToUpper(choice))

		switch choice {
		case "O":
			fmt.Printf("    Opening %s ...\n", p.SignupURL)
			_ = openBrowser(p.SignupURL)
			key := promptKey(reader, p.Key, p.KeyPrefix)
			if key != "" {
				cfg.APIKeys[p.Key] = key
				setEnabled(cfg, p.Key)
			}
		case "E":
			key := promptKey(reader, p.Key, p.KeyPrefix)
			if key != "" {
				cfg.APIKeys[p.Key] = key
				setEnabled(cfg, p.Key)
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
