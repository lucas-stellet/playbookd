package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"sort"

	"github.com/lucas-stellet/playbookd"
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

	fmt.Println("\nBy Status:")
	statuses := []playbookd.Status{
		playbookd.StatusActive,
		playbookd.StatusDraft,
		playbookd.StatusDeprecated,
		playbookd.StatusArchived,
	}
	for _, s := range statuses {
		if n, ok := stats.ByStatus[s]; ok {
			fmt.Printf("  %-12s %d\n", s, n)
		}
	}

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
