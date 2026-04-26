package install

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Options controls interactive prompts during installation.
type Options struct {
	Stdin  io.Reader
	Stdout io.Writer
	Force  bool // skip prompts (--yes flag)
}

func confirm(opts Options, prompt string) bool {
	if opts.Force {
		return true
	}
	fmt.Fprint(opts.Stdout, prompt)
	scanner := bufio.NewScanner(opts.Stdin)
	if !scanner.Scan() {
		return false
	}
	line := strings.TrimSpace(scanner.Text())
	return line == "" || strings.EqualFold(line, "y")
}

func Install(opts Options) error {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	binDir := filepath.Join(home, ".local", "bin")
	dest := filepath.Join(binDir, "vibecockpit")

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	if exe != dest {
		src, err := os.Open(exe)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return err
		}
	}

	fmt.Printf("Installed to %s\n", dest)

	pathDirs := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	inPath := false
	for _, d := range pathDirs {
		if d == binDir {
			inPath = true
			break
		}
	}
	if !inPath {
		fmt.Printf("\nAdd to your shell profile:\n  export PATH=\"%s:$PATH\"\n\n", binDir)
	}

	if runtime.GOOS == "linux" {
		if confirm(opts, "Create desktop entry (~/.local/share/applications/vibecockpit.desktop)? [Y/n] ") {
			createDesktopEntry(dest, home)
		}
	}

	fmt.Println("Done! Run 'vibecockpit --web' to launch the web UI")
	fmt.Println("     Run 'vibecockpit --autostart' to start on login")
	return nil
}

func createDesktopEntry(binPath, home string) {
	desktopDir := filepath.Join(home, ".local", "share", "applications")
	_ = os.MkdirAll(desktopDir, 0755)

	desktop := fmt.Sprintf(`[Desktop Entry]
Name=VibeCockpit
Comment=AI coding session manager
Exec=%s --web
Icon=utilities-terminal
Terminal=false
Type=Application
Categories=Development;
StartupNotify=true
`, binPath)

	path := filepath.Join(desktopDir, "vibecockpit.desktop")
	if err := os.WriteFile(path, []byte(desktop), 0644); err != nil {
		fmt.Printf("Warning: could not create desktop entry: %v\n", err)
	} else {
		fmt.Printf("Desktop entry created: %s\n", path)
	}
}

func SetupAutostart(port int, opts Options) error {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}

	home, _ := os.UserHomeDir()
	binPath := filepath.Join(home, ".local", "bin", "vibecockpit")

	if _, err := os.Stat(binPath); err != nil {
		return fmt.Errorf("binary not found at %s — run --install first", binPath)
	}

	switch runtime.GOOS {
	case "linux":
		if !confirm(opts, "Create systemd user service for VibeCockpit web UI? [Y/n] ") {
			return nil
		}
		return setupSystemd(binPath, home, port)
	case "darwin":
		if !confirm(opts, "Create launchd agent for VibeCockpit web UI? [Y/n] ") {
			return nil
		}
		return setupLaunchd(binPath, home, port)
	default:
		return fmt.Errorf("autostart not supported on %s", runtime.GOOS)
	}
}

func RemoveAutostart() error {
	switch runtime.GOOS {
	case "linux":
		return removeSystemd()
	case "darwin":
		return removeLaunchd()
	default:
		return nil
	}
}

func setupSystemd(binPath, home string, port int) error {
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	_ = os.MkdirAll(serviceDir, 0755)

	service := fmt.Sprintf(`[Unit]
Description=VibeCockpit Web UI
After=network.target

[Service]
Type=simple
ExecStart=%s --web --port %d
Restart=on-failure
RestartSec=5
Environment=HOME=%s

[Install]
WantedBy=default.target
`, binPath, port, home)

	servicePath := filepath.Join(serviceDir, "vibecockpit.service")
	if err := os.WriteFile(servicePath, []byte(service), 0644); err != nil {
		return err
	}

	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	if err := exec.Command("systemctl", "--user", "enable", "--now", "vibecockpit").Run(); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	fmt.Printf("Service enabled and started\n")
	fmt.Printf("Web UI: http://localhost:%d\n", port)
	fmt.Println("Manage: systemctl --user {start|stop|status|disable} vibecockpit")
	return nil
}

func removeSystemd() error {
	_ = exec.Command("systemctl", "--user", "stop", "vibecockpit").Run()
	_ = exec.Command("systemctl", "--user", "disable", "vibecockpit").Run()

	home, _ := os.UserHomeDir()
	_ = os.Remove(filepath.Join(home, ".config", "systemd", "user", "vibecockpit.service"))
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()

	fmt.Println("Autostart removed")
	return nil
}

func setupLaunchd(binPath, home string, port int) error {
	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	_ = os.MkdirAll(plistDir, 0755)

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.vibecockpit.web</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>--web</string>
		<string>--port</string>
		<string>%d</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>`, binPath, port)

	plistPath := filepath.Join(plistDir, "com.vibecockpit.web.plist")
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return err
	}

	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to load agent: %w", err)
	}

	fmt.Printf("Launch agent loaded\n")
	fmt.Printf("Web UI: http://localhost:%d\n", port)
	fmt.Println("Manage: launchctl {load|unload} " + plistPath)
	return nil
}

func removeLaunchd() error {
	home, _ := os.UserHomeDir()
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.vibecockpit.web.plist")
	_ = exec.Command("launchctl", "unload", plistPath).Run()
	_ = os.Remove(plistPath)
	fmt.Println("Launch agent removed")
	return nil
}
