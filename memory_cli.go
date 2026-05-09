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
func runMemory(providers []provider.Provider, args []string) {
	if len(args) == 0 {
		printMemoryUsage()
		os.Exit(1)
	}
	idxPath, err := memory.DefaultPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not resolve memory path: %v\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "stats":
		idx, err := memory.Open(idxPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}
		defer idx.Close()
		st, err := idx.Stats()
		if err != nil {
			fmt.Fprintf(os.Stderr, "stats: %v\n", err)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(st)

	case "reindex":
		idx, err := memory.Open(idxPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}
		defer idx.Close()
		fmt.Fprintln(os.Stderr, "Indexing every provider's transcripts (this may take a moment)…")
		run := memory.NewIndexer(idx).IndexAll(context.Background(), providers)
		fmt.Printf("indexed=%d  skipped=%d  removed=%d  errors=%d  duration=%s\n",
			run.Indexed, run.Skipped, run.Removed, run.Errors, run.Duration)
		if run.LastError != "" {
			fmt.Fprintf(os.Stderr, "last error: %s\n", run.LastError)
		}

	case "search":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: vibecockpit memory search <query>")
			os.Exit(1)
		}
		idx, err := memory.Open(idxPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}
		defer idx.Close()
		results, err := idx.Search(args[1], memory.SearchOpts{Limit: 20})
		if err != nil {
			fmt.Fprintf(os.Stderr, "search: %v\n", err)
			os.Exit(1)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)

	case "export":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: vibecockpit memory export <path>")
			os.Exit(1)
		}
		idx, err := memory.Open(idxPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}
		defer idx.Close()
		if err := idx.Export(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "export: %v\n", err)
			os.Exit(1)
		}
		fi, _ := os.Stat(args[1])
		var size int64
		if fi != nil {
			size = fi.Size()
		}
		fmt.Printf("Exported %s (%d bytes)\n", args[1], size)

	case "import":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: vibecockpit memory import <path>")
			os.Exit(1)
		}
		idx, err := memory.Open(idxPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}
		defer idx.Close()
		added, err := idx.Import(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "import: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Imported %s (%d new sessions; existing entries kept as-is)\n", args[1], added)

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %q\n", args[0])
		printMemoryUsage()
		os.Exit(1)
	}
}

func printMemoryUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  vibecockpit memory stats
  vibecockpit memory reindex
  vibecockpit memory search <query>
  vibecockpit memory export <path>     # snapshot to a file (e.g. for moving to another machine)
  vibecockpit memory import <path>     # merge another machine's memory.db (local entries kept on conflict)`)
}
