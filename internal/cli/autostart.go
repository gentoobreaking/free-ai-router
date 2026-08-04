package cli

import (
	"fmt"
	"os"
	"runtime"
)

func RunAutostart(cmd string) error {
	switch cmd {
	case "install":
		return autostartInstall()
	case "start":
		return autostartStart()
	case "uninstall":
		return autostartUninstall()
	case "status":
		return autostartStatus()
	default:
		fmt.Println("Usage: freemodel autostart [--install|--start|--uninstall|--status]")
		return nil
	}
}

func autostartInstall() error {
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd()
	case "linux":
		return installXDG()
	default:
		return fmt.Errorf("autostart not supported on %s", runtime.GOOS)
	}
}

func installLaunchd() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	launchAgentsDir := home + "/Library/LaunchAgents"
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return err
	}

	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.freemodel.router</string>
    <key>ProgramArguments</key>
    <array>
        <string>` + binaryPath() + `</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
`

	plistPath := launchAgentsDir + "/com.freemodel.router.plist"
	if err := os.WriteFile(plistPath, []byte(plist), 0600); err != nil {
		return err
	}

	fmt.Println("Autostart installed:", plistPath)
	fmt.Println("Run: launchctl load", plistPath)
	return nil
}

func installXDG() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	autostartDir := home + "/.config/autostart"
	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return err
	}

	desktop := `[Desktop Entry]
Type=Application
Name=FreeModel Router
Exec=` + binaryPath() + ` start
X-GNOME-Autostart-enabled=true
`

	desktopPath := autostartDir + "/freemodel-router.desktop"
	if err := os.WriteFile(desktopPath, []byte(desktop), 0600); err != nil {
		return err
	}

	fmt.Println("Autostart installed:", desktopPath)
	return nil
}

func autostartStart() error {
	fmt.Println("Starting router...")
	return nil
}

func autostartUninstall() error {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		path := home + "/Library/LaunchAgents/com.freemodel.router.plist"
		if err := os.Remove(path); err == nil {
			fmt.Println("Removed:", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	case "linux":
		path := home + "/.config/autostart/freemodel-router.desktop"
		if err := os.Remove(path); err == nil {
			fmt.Println("Removed:", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	fmt.Println("Autostart uninstalled.")
	return nil
}

func autostartStatus() error {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		path := home + "/Library/LaunchAgents/com.freemodel.router.plist"
		if _, err := os.Stat(path); err == nil {
			fmt.Println("Autostart: installed")
		} else {
			fmt.Println("Autostart: not installed")
		}
	case "linux":
		path := home + "/.config/autostart/freemodel-router.desktop"
		if _, err := os.Stat(path); err == nil {
			fmt.Println("Autostart: installed")
		} else {
			fmt.Println("Autostart: not installed")
		}
	default:
		fmt.Println("Autostart: not supported on", runtime.GOOS)
	}
	return nil
}

func binaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "freemodel"
	}
	return exe
}
