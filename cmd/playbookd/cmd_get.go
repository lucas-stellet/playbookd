package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/lucas-stellet/playbookd"
)

func runGet(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	executionsFlag := fs.Int("executions", 0, "also show last N executions")
	jsonFlag := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: playbookd get ID [-executions N]")
	}
	id := fs.Arg(0)

	mgr, err := newManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	ctx := context.Background()
	pb, err := mgr.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get playbook %q: %w", id, err)
	}

	var execs []*playbookd.ExecutionRecord
	if *executionsFlag > 0 {
		execs, err = mgr.ListExecutions(ctx, id, *executionsFlag)
		if err != nil {
			return fmt.Errorf("list executions: %w", err)
		}
	}

	if *jsonFlag {
		out := map[string]any{"playbook": pb}
		if execs != nil {
			out["executions"] = execs
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
	fmt.Printf("Name:       %s\n", pb.Name)
	fmt.Printf("ID:         %s\n", pb.ID)
	fmt.Printf("Slug:       %s\n", pb.Slug)
	fmt.Printf("Category:   %s\n", pb.Category)
	if pb.Archived {
		fmt.Println("** ARCHIVED **")
	}
	fmt.Printf("Version:    %d\n", pb.Version)
	fmt.Printf("Confidence: %.2f\n", pb.Confidence)
	fmt.Printf("Success:    %d  Failure: %d\n", pb.SuccessCount, pb.FailureCount)
	if len(pb.Tags) > 0 {
		fmt.Printf("Tags:       %s\n", strings.Join(pb.Tags, ", "))
	}
	fmt.Printf("Created:    %s\n", pb.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated:    %s\n", pb.UpdatedAt.Format("2006-01-02 15:04:05"))
	if !pb.LastUsedAt.IsZero() {
		fmt.Printf("Last Used:  %s\n", pb.LastUsedAt.Format("2006-01-02 15:04:05"))
	}

	if pb.Description != "" {
		fmt.Printf("\nDescription:\n  %s\n", pb.Description)
	}

	if len(pb.Steps) > 0 {
		fmt.Printf("\nSteps (%d):\n", len(pb.Steps))
		for _, s := range pb.Steps {
			fmt.Printf("  %d. %s\n", s.Order, s.Action)
			if s.Tool != "" {
				fmt.Printf("     Tool: %s\n", s.Tool)
			}
		}
	}

	if len(pb.Lessons) > 0 {
		fmt.Printf("\nLessons (%d):\n", len(pb.Lessons))
		for _, l := range pb.Lessons {
			fmt.Printf("  - %s\n", l.Content)
		}
	}

	if len(execs) > 0 {
		fmt.Printf("\nRecent Executions (%d):\n", len(execs))
		for _, e := range execs {
			fmt.Printf("  [%s] %s  outcome=%s\n",
				e.StartedAt.Format("2006-01-02 15:04"),
				e.ID,
				e.Outcome,
			)
		}
	}

	return nil
}
