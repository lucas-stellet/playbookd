package main

import (
	"fmt"
	"strings"

	"github.com/lucas-stellet/playbookd"
)

func printPlaybook(pb *playbookd.Playbook) {
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
}
