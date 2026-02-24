package main

import (
	"fmt"
	"os"
)

const usage = `playbookd - Procedural memory manager for AI agents

Usage:
  playbookd <command> [options]

Commands:
  init      Generate a .playbookd.toml configuration file
  list      List playbooks
  search    Search for playbooks
  get       Get a specific playbook
  edit      Edit a playbook in an external editor
  stats     Show aggregate statistics
  prune     Archive stale playbooks
  reindex   Rebuild the search index

Use "playbookd <command> -help" for more information about a command.`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "list":
		err = runList(args)
	case "search":
		err = runSearch(args)
	case "get":
		err = runGet(args)
	case "edit":
		err = runEdit(args)
	case "stats":
		err = runStats(args)
	case "prune":
		err = runPrune(args)
	case "reindex":
		err = runReindex(args)
	case "-h", "-help", "--help", "help":
		fmt.Println(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
