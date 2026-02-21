package main

import (
	"context"
	"flag"
	"fmt"
)

func runReindex(args []string) error {
	fs := flag.NewFlagSet("reindex", flag.ContinueOnError)

	if err := fs.Parse(args); err != nil {
		return err
	}

	mgr, err := newManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	fmt.Println("Rebuilding search index...")
	if err := mgr.Reindex(context.Background()); err != nil {
		return fmt.Errorf("reindex: %w", err)
	}

	fmt.Println("Reindex complete.")
	return nil
}
