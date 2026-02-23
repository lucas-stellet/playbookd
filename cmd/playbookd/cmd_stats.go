package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"sort"
)

func runStats(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	jsonFlag := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	mgr, err := newManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	stats, err := mgr.Stats(context.Background())
	if err != nil {
		return fmt.Errorf("stats: %w", err)
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Total Playbooks:  %d\n", stats.TotalPlaybooks)
	fmt.Printf("Total Executions: %d\n", stats.TotalExecs)
	fmt.Printf("Avg Confidence:   %.2f\n", stats.AvgConfidence)
	fmt.Printf("Archived:         %d\n", stats.TotalArchived)

	if len(stats.ByCategory) > 0 {
		fmt.Println("\nBy Category:")
		// Sort categories for stable output
		cats := make([]string, 0, len(stats.ByCategory))
		for c := range stats.ByCategory {
			cats = append(cats, c)
		}
		sort.Strings(cats)
		for _, c := range cats {
			fmt.Printf("  %-20s %d\n", c, stats.ByCategory[c])
		}
	}

	return nil
}
