package cmd

import (
	"fmt"

	"github.com/steipete/gogcli/internal/tracking"
)

func loadTrackingConfigForAccount(flags *RootFlags) (string, *tracking.Config, error) {
	return loadTrackingConfigForAccountWith(flags, tracking.LoadConfig)
}

func loadTrackingConfigMetadataForAccount(flags *RootFlags) (string, *tracking.Config, error) {
	return loadTrackingConfigForAccountWith(flags, tracking.LoadConfigMetadata)
}

func loadTrackingConfigForAccountWith(flags *RootFlags, loadConfig func(string) (*tracking.Config, error)) (string, *tracking.Config, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return "", nil, err
	}

	cfg, err := loadConfig(account)
	if err != nil {
		return "", nil, fmt.Errorf("load tracking config: %w", err)
	}

	return account, cfg, nil
}
