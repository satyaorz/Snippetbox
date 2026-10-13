package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// savedConfig is what `snip login` stores: the server and the API token.
type savedConfig struct {
	Server string `json:"server"`
	Token  string `json:"token"`
}

// configPath returns where the config lives: $SNIP_CONFIG, or snip/config.json
// in the user's config directory (~/.config on Linux).
func configPath(getenv func(string) string) (string, error) {
	if p := getenv("SNIP_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "snip", "config.json"), nil
}

func loadConfig(path string) (savedConfig, error) {
	var cfg savedConfig

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

// saveConfig writes the config readable only by the current user, since it
// contains a token that can act as them.
func saveConfig(path string, cfg savedConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	// Write to a temporary file and rename, so a crash can't leave a
	// half-written config behind.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
