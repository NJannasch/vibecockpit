package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"vibecockpit/internal/memory"
	"vibecockpit/internal/provider"
)

// runMemory dispatches the `vibecockpit memory <subcmd>` family. Kept
// in its own file to avoid further bloating main.go now that the CLI
// surface is growing.
//
// Each subcommand is split into its own function that returns an error;
// runMemory does the os.Exit(1) at the top level so the deferred
// idx.Close() inside subcommands actually runs (otherwise gocritic's
// exitAfterDefer fires).
func runMemory(providers []provider.Provider, args []string) {
	if err := dispatchMemory(providers, args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func dispatchMemory(providers []provider.Provider, args []string) error {
	if len(args) == 0 {
		printMemoryUsage()
		return fmt.Errorf("no subcommand")
	}
	idxPath, err := memory.DefaultPath()
	if err != nil {
		return fmt.Errorf("could not resolve memory path: %w", err)
	}

	switch args[0] {
	case "stats":
		return cmdMemoryStats(idxPath)
	case "reindex":
		return cmdMemoryReindex(idxPath, providers)
	case "search":
		if len(args) < 2 {
			return fmt.Errorf("usage: vibecockpit memory search <query>")
		}
		return cmdMemorySearch(idxPath, args[1])
	case "export":
		if len(args) < 2 {
			return fmt.Errorf("usage: vibecockpit memory export <path>")
		}
		return cmdMemoryExport(idxPath, args[1])
	case "import":
		if len(args) < 2 {
			return fmt.Errorf("usage: vibecockpit memory import <path>")
		}
		return cmdMemoryImport(idxPath, args[1])
	default:
		printMemoryUsage()
		return fmt.Errorf("unknown subcommand: %q", args[0])
	}
}

func cmdMemoryStats(idxPath string) error {
	idx, err := memory.Open(idxPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = idx.Close() }()
	st, err := idx.Stats()
	if err != nil {
		return fmt.Errorf("stats: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(st)
}

func cmdMemoryReindex(idxPath string, providers []provider.Provider) error {
	idx, err := memory.Open(idxPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = idx.Close() }()
	fmt.Fprintln(os.Stderr, "Indexing every provider's transcripts (this may take a moment)…")
	run := memory.NewIndexer(idx).IndexAll(context.Background(), providers)
	fmt.Printf("indexed=%d  skipped=%d  removed=%d  errors=%d  duration=%s\n",
		run.Indexed, run.Skipped, run.Removed, run.Errors, run.Duration)
	if run.LastError != "" {
		fmt.Fprintf(os.Stderr, "last error: %s\n", run.LastError)
	}
	return nil
}

func cmdMemorySearch(idxPath, query string) error {
	idx, err := memory.Open(idxPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = idx.Close() }()
	results, err := idx.Search(query, memory.SearchOpts{Limit: 20})
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func cmdMemoryExport(idxPath, dest string) error {
	idx, err := memory.Open(idxPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = idx.Close() }()
	if err := idx.Export(dest); err != nil {
		return fmt.Errorf("export: %w", err)
	}
	fi, _ := os.Stat(dest)
	var size int64
	if fi != nil {
		size = fi.Size()
	}
	fmt.Printf("Exported %s (%d bytes)\n", dest, size)
	return nil
}

func cmdMemoryImport(idxPath, src string) error {
	idx, err := memory.Open(idxPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = idx.Close() }()
	added, err := idx.Import(src)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	fmt.Printf("Imported %s (%d new sessions; existing entries kept as-is)\n", src, added)
	return nil
}

func printMemoryUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  vibecockpit memory stats
  vibecockpit memory reindex
  vibecockpit memory search <query>
  vibecockpit memory export <path>     # snapshot to a file (e.g. for moving to another machine)
  vibecockpit memory import <path>     # merge another machine's memory.db (local entries kept on conflict)`)
}
