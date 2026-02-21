package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"

	"github.com/lucas-stellet/playbookd"
)

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	statusFlag := fs.String("status", "", "filter by status (draft, active, deprecated, archived)")
	categoryFlag := fs.String("category", "", "filter by category")
	jsonFlag := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}

	mgr, err := newManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	filter := playbookd.ListFilter{
		Category: *categoryFlag,
	}
	if *statusFlag != "" {
		s := playbookd.Status(*statusFlag)
		filter.Status = &s
	}

	playbooks, err := mgr.List(context.Background(), filter)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(playbooks, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(playbooks) == 0 {
		fmt.Println("No playbooks found.")
		return nil
	}

	fmt.Printf("%-36s  %-30s  %-12s  %-12s  %s\n", "ID", "Name", "Status", "Category", "Confidence")
	fmt.Printf("%-36s  %-30s  %-12s  %-12s  %s\n",
		"------------------------------------",
		"------------------------------",
		"------------",
		"------------",
		"----------",
	)
	for _, pb := range playbooks {
		fmt.Printf("%-36s  %-30s  %-12s  %-12s  %.2f\n",
			pb.ID, pb.Name, pb.Status, pb.Category, pb.Confidence)
	}
	return nil
}
