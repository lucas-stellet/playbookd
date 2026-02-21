package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/lucas-stellet/playbookd"
)

func newManager() (*playbookd.PlaybookManager, error) {
	cfg, err := playbookd.LoadConfig(".playbookd.toml")
	if err == nil {
		// TOML config found — use it
		mgrCfg, err := cfg.BuildManagerConfig()
		if err != nil {
			return nil, fmt.Errorf("build config: %w", err)
		}
		// Env var overrides TOML data dir
		if envDir := os.Getenv("PLAYBOOKD_DATA"); envDir != "" {
			mgrCfg.DataDir = envDir
		}
		mgr, err := playbookd.NewPlaybookManager(mgrCfg)
		if err != nil {
			return nil, fmt.Errorf("init manager: %w", err)
		}
		return mgr, nil
	}

	// No config file — fall back to env var
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load config: %w", err)
	}

	dataDir := os.Getenv("PLAYBOOKD_DATA")
	if dataDir == "" {
		dataDir = "./playbooks"
	}

	mgr, err := playbookd.NewPlaybookManager(playbookd.ManagerConfig{
		DataDir: dataDir,
	})
	if err != nil {
		return nil, fmt.Errorf("init manager: %w", err)
	}
	return mgr, nil
}
