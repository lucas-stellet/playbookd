package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"github.com/lucas-stellet/playbookd"
)

func runPrune(args []string) error {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	maxAgeFlag := fs.String("max-age", "90d", "maximum age before pruning (e.g. 30d, 90d)")
	dryRunFlag := fs.Bool("dry-run", false, "show what would be pruned without making changes")
	jsonFlag := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	maxAge, err := parseDuration(*maxAgeFlag)
	if err != nil {
		return fmt.Errorf("invalid -max-age %q: %w", *maxAgeFlag, err)
	}

	mgr, err := newManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	result, err := mgr.Prune(context.Background(), playbookd.PruneOptions{
		MaxAge: maxAge,
		DryRun: *dryRunFlag,
	})
	if err != nil {
		return fmt.Errorf("prune: %w", err)
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if *dryRunFlag {
		fmt.Printf("Dry run: %d playbook(s) would be archived.\n", len(result.Archived))
	} else {
		fmt.Printf("Archived %d playbook(s).\n", len(result.Archived))
	}

	for _, id := range result.Archived {
		fmt.Printf("  - %s\n", id)
	}

	return nil
}

// parseDuration parses a duration string like "90d" or standard Go durations.
func parseDuration(s string) (time.Duration, error) {
	// Support "Nd" shorthand for days
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s[:len(s)-1], "%d", &days); err != nil {
			return 0, fmt.Errorf("cannot parse %q as days", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
