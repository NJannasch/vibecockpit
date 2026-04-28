package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"

	"vibecockpit/internal/config"
	"vibecockpit/internal/costs"
	"vibecockpit/internal/demo"
	"vibecockpit/internal/install"
	"vibecockpit/internal/launcher"
	mcpserver "vibecockpit/internal/mcp"
	"vibecockpit/internal/plugin"
	"vibecockpit/internal/plugin/builtin"
	"vibecockpit/internal/plugin/remote"
	"vibecockpit/internal/provider"
	"vibecockpit/internal/provider/antigravity"
	"vibecockpit/internal/provider/claude"
	"vibecockpit/internal/provider/claudedesktop"
	"vibecockpit/internal/provider/codex"
	"vibecockpit/internal/provider/cursoragent"
	"vibecockpit/internal/provider/copilot"
	"vibecockpit/internal/provider/gemini"
	"vibecockpit/internal/provider/opencode"
	"vibecockpit/internal/tui"
	"vibecockpit/internal/web"
)

var version = "dev"

func main() {
	listFlag := flag.Bool("list", false, "list sessions non-interactively and exit")
	jsonFlag := flag.Bool("json", false, "output as JSON (with --list)")
	webFlag := flag.Bool("web", false, "start the web UI (opens in browser)")
	mcpFlag := flag.Bool("mcp", false, "start as MCP server (JSON-RPC over stdio)")
	portFlag := flag.Int("port", 3456, "port for the web UI")
	installFlag := flag.Bool("install", false, "install binary to ~/.local/bin and create desktop entry")
	uninstallFlag := flag.Bool("uninstall", false, "remove the installed binary, app launcher, and autostart service")
	autostartFlag := flag.Bool("autostart", false, "register as a login service (systemd/launchd)")
	removeAutostartFlag := flag.Bool("remove-autostart", false, "remove the login service")
	yesFlag := flag.Bool("yes", false, "skip confirmation prompts (for scripted installs)")
	demoFlag := flag.Bool("demo", false, "load demo data for screenshots and testing")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("vibecockpit " + version)
		return
	}

	installOpts := install.Options{Force: *yesFlag}

	if *installFlag {
		if err := install.Install(installOpts); err != nil {
			fmt.Fprintf(os.Stderr, "Install error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *uninstallFlag {
		if err := install.Uninstall(installOpts); err != nil {
			fmt.Fprintf(os.Stderr, "Uninstall error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *autostartFlag {
		if err := install.SetupAutostart(*portFlag, installOpts); err != nil {
			fmt.Fprintf(os.Stderr, "Autostart error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *removeAutostartFlag {
		if err := install.RemoveAutostart(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	cfg := config.Load()
	if cfg.Version != version {
		cfg.Version = version
		_ = cfg.Save()
	}
	registry := buildRegistry(cfg)

	var providers []provider.Provider
	if *demoFlag {
		providers = []provider.Provider{demo.New()}
	} else {
		providers = registry.Providers()
	}

	if *listFlag {
		if *jsonFlag {
			printSessionsJSON(providers)
		} else {
			printSessions(providers)
		}
		return
	}

	if *webFlag {
		if err := web.Start(cfg, providers, *portFlag, version); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *mcpFlag {
		if !cfg.EnableMCP {
			fmt.Fprintln(os.Stderr, "MCP server is disabled. Enable it in config.yaml:\n\n  enable_mcp: true")
			os.Exit(1)
		}
		srv := mcpserver.NewServer(providers, version)
		if err := srv.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	m := tui.New(cfg, providers)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	model := result.(tui.Model)
	action := model.GetAction()

	switch action.Kind {
	case "resume":
		s := action.Session
		prov := findProvider(providers, s.Provider)
		if prov == nil {
			fmt.Fprintf(os.Stderr, "Unknown provider: %s\n", s.Provider)
			os.Exit(1)
		}
		if err := launcher.Launch(cfg, prov, *s); err != nil {
			fmt.Fprintf(os.Stderr, "Launch error: %v\n", err)
			os.Exit(1)
		}

	case "new":
		if err := os.MkdirAll(action.Dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Could not create directory: %v\n", err)
			os.Exit(1)
		}
		prov := providers[0]
		if err := launcher.LaunchNew(cfg, prov, action.Dir); err != nil {
			fmt.Fprintf(os.Stderr, "Launch error: %v\n", err)
			os.Exit(1)
		}
	}
}

func buildRegistry(cfg *config.Config) *plugin.Registry {
	reg := plugin.NewRegistry(cfg)

	oc := opencode.New()
	cx := codex.New()

	reg.Register(builtin.New("claude", "Claude Code", "◆", claude.New(), func() bool {
		home, _ := os.UserHomeDir()
		_, err := os.Stat(home + "/.claude/projects")
		return err == nil
	}))
	reg.Register(builtin.New("claude-desktop", "Claude Desktop", "⬢", claudedesktop.New(), claudedesktop.Available))
	reg.Register(builtin.New("opencode", "OpenCode", "◇", oc, oc.Available))
	reg.Register(builtin.New("codex", "Codex CLI", "◈", cx, cx.Available))
	reg.Register(builtin.New("copilot", "Copilot CLI", "◉", copilot.New(), copilot.New().Available))
	reg.Register(builtin.New("gemini", "Gemini CLI", "✦", gemini.New(), gemini.New().Available))
	reg.Register(builtin.New("cursor", "Cursor Agent", "●", cursoragent.New(), cursoragent.Available))
	reg.Register(builtin.New("antigravity", "Antigravity", "▲", antigravity.New(cfg.NewProjectDir), antigravity.Available))

	for i, src := range cfg.RemoteSources {
		id := fmt.Sprintf("remote-%d", i)
		name := "Remote"
		if n, ok := src["name"].(string); ok {
			name = n
			id = "remote-" + n
		}
		rp := remote.New(id, name)
		if cfg.PluginConfigs == nil {
			cfg.PluginConfigs = make(map[string]map[string]any)
		}
		cfg.PluginConfigs[id] = src
		reg.Register(rp)
	}

	reg.InitAll()
	return reg
}

func printSessions(providers []provider.Provider) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tPROJECT\tSUMMARY\tMODEL\tBRANCH\tMSGS\tMODIFIED\tACTIVE")
	for _, prov := range providers {
		sessions, err := prov.ScanSessions(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", prov.Name(), err)
			continue
		}
		for _, s := range sessions {
			summary := s.Summary
			if summary == "" {
				summary = s.FirstPrompt
			}
			if len(summary) > 50 {
				summary = summary[:47] + "..."
			}
			active := ""
			if s.IsActive {
				active = fmt.Sprintf("PID %d", s.ActivePID)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
				prov.Name(), s.ProjectName, summary, s.Model,
				s.GitBranch, s.MessageCount, s.Modified.Format("2006-01-02 15:04"), active)
		}
	}
	w.Flush()
}

func printSessionsJSON(providers []provider.Provider) {
	var all []provider.Session
	for _, prov := range providers {
		sessions, err := prov.ScanSessions(context.Background())
		if err != nil {
			continue
		}
		for i := range sessions {
			sessions[i].EstCostUSD = costs.EstimateCost(sessions[i].Model, sessions[i].Tokens)
		}
		all = append(all, sessions...)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(all)
}

func findProvider(providers []provider.Provider, name string) provider.Provider {
	for _, p := range providers {
		if p.Name() == name {
			return p
		}
	}
	return nil
}
