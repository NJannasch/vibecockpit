package install

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed assets/AppIcon.icns
var appIconICNS []byte

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

func stopRunningInstances(_ string, opts Options) error {
	out, err := exec.Command("pgrep", "-x", "vibecockpit").Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return nil
	}

	self := os.Getpid()
	var pids []int
	for _, line := range strings.Fields(strings.TrimSpace(string(out))) {
		pid, err := strconv.Atoi(line)
		if err != nil || pid == self {
			continue
		}
		pids = append(pids, pid)
	}
	if len(pids) == 0 {
		return nil
	}

	fmt.Fprintf(opts.Stdout, "Found %d running VibeCockpit process(es)\n", len(pids))
	if !opts.Force {
		fmt.Fprint(opts.Stdout, "Stop them to continue install? [Y/n] ")
		reader := bufio.NewReader(opts.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer == "n" || answer == "no" {
			return fmt.Errorf("cannot update while VibeCockpit is running — stop it first")
		}
	}

	for _, pid := range pids {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
	}
	time.Sleep(time.Second)
	for _, pid := range pids {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Signal(syscall.SIGKILL)
		}
	}
	time.Sleep(300 * time.Millisecond)
	fmt.Fprintln(opts.Stdout, "Stopped running processes")
	return nil
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
		if err := stopRunningInstances(dest, opts); err != nil {
			return err
		}

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

	if runtime.GOOS == "darwin" {
		if confirm(opts, "Create macOS app launcher (~/Applications/VibeCockpit.app)? [Y/n] ") {
			createMacApp(dest, home)
		}
	}

	fmt.Println("Done! Run 'vibecockpit --web' to launch the web UI")
	fmt.Println("     Run 'vibecockpit --autostart' to start on login")
	return nil
}

// createMacApp writes a minimal .app bundle to ~/Applications so the user
// can launch the web UI from Launchpad / Spotlight / Dock. The bundle's
// executable is a small bash launcher that opens the running web UI if
// one is already up, or starts the binary first if it isn't.
func createMacApp(binPath, home string) {
	appDir := filepath.Join(home, "Applications", "VibeCockpit.app")
	macosDir := filepath.Join(appDir, "Contents", "MacOS")
	if err := os.MkdirAll(macosDir, 0755); err != nil {
		fmt.Printf("Warning: could not create app bundle dir: %v\n", err)
		return
	}

	resourcesDir := filepath.Join(appDir, "Contents", "Resources")
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		fmt.Printf("Warning: could not create Resources dir: %v\n", err)
		return
	}
	iconRef := ""
	if len(appIconICNS) > 0 {
		iconPath := filepath.Join(resourcesDir, "AppIcon.icns")
		if err := os.WriteFile(iconPath, appIconICNS, 0644); err == nil {
			iconRef = "\n\t<key>CFBundleIconFile</key><string>AppIcon</string>"
		} else {
			fmt.Printf("Warning: could not write app icon: %v\n", err)
		}
	}

	plistPath := filepath.Join(appDir, "Contents", "Info.plist")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>VibeCockpit</string>
	<key>CFBundleDisplayName</key><string>VibeCockpit</string>
	<key>CFBundleIdentifier</key><string>com.vibecockpit.app</string>
	<key>CFBundleVersion</key><string>1.0</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
	<key>CFBundleExecutable</key><string>VibeCockpit</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleSignature</key><string>????</string>
	<key>LSUIElement</key><true/>%s
</dict>
</plist>
`, iconRef)
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		fmt.Printf("Warning: could not write Info.plist: %v\n", err)
		return
	}

	launcherPath := filepath.Join(macosDir, "VibeCockpit")
	launcher := fmt.Sprintf(`#!/bin/bash
# VibeCockpit Launchpad launcher.
# Opens the web UI if vibecockpit is already serving; starts it first if not.
PORT=3456
URL="http://localhost:$PORT"
BIN=%q

if nc -z localhost "$PORT" 2>/dev/null; then
  open "$URL"
  exit 0
fi

if [ ! -x "$BIN" ]; then
  osascript -e 'display alert "VibeCockpit not found" message "Run vibecockpit --install or reinstall via the install script."'
  exit 1
fi

nohup "$BIN" --web --port "$PORT" >/dev/null 2>&1 &
# Wait briefly for the server to bind before opening the browser.
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if nc -z localhost "$PORT" 2>/dev/null; then break; fi
  sleep 0.2
done
open "$URL"
`, binPath)

	if err := os.WriteFile(launcherPath, []byte(launcher), 0755); err != nil {
		fmt.Printf("Warning: could not write launcher: %v\n", err)
		return
	}

	// Clear *every* extended attribute on the bundle. com.apple.quarantine
	// alone isn't enough — com.apple.provenance has been observed to
	// SIGKILL the binary on first launch even after quarantine is stripped.
	// The Go toolchain already ad-hoc signs the binary at build time.
	_ = exec.Command("xattr", "-cr", appDir).Run()

	fmt.Printf("App launcher created: %s\n", appDir)
	fmt.Println("       Find VibeCockpit in Launchpad / Spotlight to start the web UI.")
}

// Uninstall reverses what Install + SetupAutostart created. It removes the
// binary at ~/.local/bin/vibecockpit, the macOS .app bundle (or Linux
// .desktop entry), and any autostart service. User config under
// ~/.config/vibecockpit is left in place — the user can rm it themselves
// if they want a fully clean slate.
func Uninstall(opts Options) error {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}

	home, _ := os.UserHomeDir()

	// Compose the work list up front so the user knows what's about to
	// disappear. Only items that actually exist are listed.
	type target struct {
		desc, path string
		isDir      bool
	}
	candidates := []target{
		{"binary", filepath.Join(home, ".local", "bin", "vibecockpit"), false},
	}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			target{"macOS app launcher", filepath.Join(home, "Applications", "VibeCockpit.app"), true},
			target{"launchd agent plist", filepath.Join(home, "Library", "LaunchAgents", "com.vibecockpit.web.plist"), false},
		)
	case "linux":
		candidates = append(candidates,
			target{"desktop entry", filepath.Join(home, ".local", "share", "applications", "vibecockpit.desktop"), false},
			target{"systemd unit", filepath.Join(home, ".config", "systemd", "user", "vibecockpit.service"), false},
		)
	}

	var present []target
	for _, t := range candidates {
		if _, err := os.Stat(t.path); err == nil {
			present = append(present, t)
		}
	}
	if len(present) == 0 {
		fmt.Fprintln(opts.Stdout, "Nothing to remove — vibecockpit is not installed under this user's home directory.")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "The following will be removed:")
	for _, t := range present {
		fmt.Fprintf(opts.Stdout, "  - %s: %s\n", t.desc, t.path)
	}
	if !confirm(opts, "Proceed? [Y/n] ") {
		fmt.Fprintln(opts.Stdout, "Aborted.")
		return nil
	}

	// Stop the autostart service first, otherwise removing the unit /
	// plist files races against the running daemon.
	switch runtime.GOOS {
	case "darwin":
		plist := filepath.Join(home, "Library", "LaunchAgents", "com.vibecockpit.web.plist")
		if _, err := os.Stat(plist); err == nil {
			_ = exec.Command("launchctl", "unload", plist).Run()
		}
	case "linux":
		_ = exec.Command("systemctl", "--user", "stop", "vibecockpit").Run()
		_ = exec.Command("systemctl", "--user", "disable", "vibecockpit").Run()
	}

	for _, t := range present {
		var err error
		if t.isDir {
			err = os.RemoveAll(t.path)
		} else {
			err = os.Remove(t.path)
		}
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(opts.Stdout, "Warning: could not remove %s: %v\n", t.path, err)
			continue
		}
		fmt.Fprintf(opts.Stdout, "Removed %s\n", t.path)
	}

	if runtime.GOOS == "linux" {
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	}

	fmt.Fprintln(opts.Stdout, "Done. Config under ~/.config/vibecockpit was left in place.")
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
