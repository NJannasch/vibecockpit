package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"vibecockpit/internal/config"
	"vibecockpit/internal/provider"
)

func Launch(cfg *config.Config, prov provider.Provider, sess provider.Session) error {
	bin, args := prov.ResumeCommand(sess)
	return launch(cfg, prov.Name(), bin, args, sess.ProjectPath)
}

func LaunchNew(cfg *config.Config, prov provider.Provider, dir string) error {
	bin, args := prov.NewCommand(dir)
	return launch(cfg, prov.Name(), bin, args, dir)
}

func launch(cfg *config.Config, provName, bin string, args []string, dir string) error {
	binPath := resolveBinary(cfg, provName, bin)
	if binPath == "" {
		configPath := config.Path()
		return fmt.Errorf(
			"could not find %q in PATH. Configure its location in settings:\n\n"+
				"  provider_paths:\n"+
				"    %s: /path/to/%s\n\n"+
				"Config file: %s", bin, provName, bin, configPath)
	}

	switch cfg.Terminal {
	case "", "default":
		return execReplace(binPath, bin, args, dir)
	case "custom":
		return launchCustom(cfg.CustomTermCmd, binPath, args, dir)
	default:
		return launchTerminal(cfg.Terminal, binPath, args, dir)
	}
}

func resolveBinary(cfg *config.Config, provName, bin string) string {
	if p, ok := cfg.ProviderPaths[provName]; ok {
		p = expandHome(p)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	if p, err := exec.LookPath(bin); err == nil {
		return p
	}

	return ""
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return home + path[1:]
	}
	return path
}

func execReplace(binPath, bin string, args []string, dir string) error {
	if err := os.Chdir(dir); err != nil {
		return err
	}
	argv := append([]string{bin}, args...)
	return syscall.Exec(binPath, argv, os.Environ())
}

func launchTerminal(terminal, binPath string, args []string, dir string) error {
	fullCmd := binPath
	for _, a := range args {
		fullCmd += " " + a
	}

	var cmd *exec.Cmd
	switch terminal {
	case "kitty":
		cmd = exec.Command("kitty", append([]string{"--directory", dir}, append([]string{binPath}, args...)...)...)
	case "alacritty":
		cmd = exec.Command("alacritty", append([]string{"--working-directory", dir, "-e"}, append([]string{binPath}, args...)...)...)
	case "wezterm":
		cmd = exec.Command("wezterm", append([]string{"start", "--cwd", dir, "--"}, append([]string{binPath}, args...)...)...)
	case "ghostty":
		cmd = exec.Command("ghostty", append([]string{"--working-directory=" + dir, "-e"}, append([]string{binPath}, args...)...)...)
	case "hyper":
		cmd = exec.Command("hyper", append([]string{binPath}, args...)...)
		cmd.Dir = dir
	case "gnome-terminal":
		cmd = exec.Command("gnome-terminal", append([]string{"--working-directory=" + dir, "--"}, append([]string{binPath}, args...)...)...)
	case "konsole":
		cmd = exec.Command("konsole", append([]string{"--workdir", dir, "-e"}, append([]string{binPath}, args...)...)...)
	case "xterm":
		cmd = exec.Command("xterm", "-e", "cd "+dir+" && "+fullCmd)
	case "iterm2":
		script := fmt.Sprintf(`tell application "iTerm2"
			create window with default profile command "cd %s && %s"
		end tell`, dir, fullCmd)
		cmd = exec.Command("osascript", "-e", script)
	case "terminal.app":
		script := fmt.Sprintf(`tell application "Terminal"
			do script "cd %s && %s"
			activate
		end tell`, dir, fullCmd)
		cmd = exec.Command("osascript", "-e", script)
	default:
		return fmt.Errorf("unsupported terminal: %s", terminal)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

func launchCustom(tmpl, binPath string, args []string, dir string) error {
	fullCmd := binPath
	for _, a := range args {
		fullCmd += " " + a
	}

	expanded := strings.ReplaceAll(tmpl, "{dir}", dir)
	expanded = strings.ReplaceAll(expanded, "{cmd}", fullCmd)
	expanded = strings.ReplaceAll(expanded, "{bin}", binPath)

	parts := strings.Fields(expanded)
	if len(parts) == 0 {
		return fmt.Errorf("empty custom terminal command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
