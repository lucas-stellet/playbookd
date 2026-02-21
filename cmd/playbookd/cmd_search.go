package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"

	"github.com/lucas-stellet/playbookd"
)

func runSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	modeFlag := fs.String("mode", "hybrid", "search mode: hybrid, bm25, or vector")
	limitFlag := fs.Int("limit", playbookd.DefaultSearchLimit, "maximum number of results")
	jsonFlag := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: playbookd search \"query\" [-mode hybrid|bm25|vector] [-limit N]")
	}
	query := fs.Arg(0)

	mgr, err := newManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	results, err := mgr.Search(context.Background(), playbookd.SearchQuery{
		Text:  query,
		Mode:  playbookd.SearchMode(*modeFlag),
		Limit: *limitFlag,
	})
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	fmt.Printf("Found %d result(s) for %q:\n\n", len(results), query)
	for i, r := range results {
		fmt.Printf("%d. [%.3f] %s\n", i+1, r.Score, r.Playbook.Name)
		fmt.Printf("   ID: %s\n", r.Playbook.ID)
		if r.Playbook.Description != "" {
			fmt.Printf("   %s\n", r.Playbook.Description)
		}
		fmt.Println()
	}
	return nil
}
