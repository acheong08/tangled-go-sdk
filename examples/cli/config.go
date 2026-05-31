package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

const (
	appName     = "tangled"
	configFile  = "config.toml"
	envPrefix   = "TANGLED"
)

// configPath returns the full path to the config file,
// using XDG config directory on all platforms:
//   - Linux:   ~/.config/tangled/config.toml
//   - macOS:   ~/Library/Application Support/tangled/config.toml
//   - Windows: %AppData%/tangled/config.toml
func configPath() (string, error) {
	return xdg.ConfigFile(filepath.Join(appName, configFile))
}

// initConfig sets up Viper with the config file and env var bindings.
// Called by Cobra's PersistentPreRunE.
func initConfig() error {
	// Config file location
	cp, err := configPath()
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	viper.SetConfigFile(cp)
	viper.SetConfigType("toml")

	// Environment variable overrides (e.g., TANGLED_HANDLE)
	viper.SetEnvPrefix(envPrefix)
	viper.AutomaticEnv()

	// Read config — it's okay if it doesn't exist yet
	if err := viper.ReadInConfig(); err != nil {
		if !os.IsNotExist(err) {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return fmt.Errorf("failed to read config: %w", err)
			}
		}
	}
	return nil
}

// saveConfig writes the current Viper state to the config file.
func saveConfig() error {
	cp, err := configPath()
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(cp)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory %q: %w", dir, err)
	}

	if err := viper.WriteConfigAs(cp); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Config saved to %s\n", cp)
	return nil
}
