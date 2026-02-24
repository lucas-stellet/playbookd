package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"

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

	printPlaybook(pb)

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
