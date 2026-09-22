// Package config is what urga remembers between runs.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// MaxNamespaces is how many namespaces the number keys reach.
const MaxNamespaces = 9

// Config is the last session: where it was looking and what it had open.
type Config struct {
	// Namespace is the one in use. A pointer, because every namespace at
	// once is a choice of its own and must not read as "never chose".
	Namespace *string `json:"namespace"`

	// Screen is the resource that was open.
	Screen string `json:"screen"`

	// Namespaces is which namespace each number key stands for.
	Namespaces []string `json:"namespaces"`

	path string
}

// Load reads what the last session left. A first run, or a file that cannot
// be read, starts fresh instead of stopping urga.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	cfg := &Config{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil
	}

	stored := &Config{}
	if err := json.Unmarshal(data, stored); err != nil {
		return cfg, nil
	}

	stored.path = path

	return stored, nil
}

// Save writes the session down.
func (c *Config) Save() error {
	if c.path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.path, data, 0o600)
}

// UseNamespace records the namespace in use.
func (c *Config) UseNamespace(namespace string) {
	c.Namespace = &namespace
}

// Remember keeps the order the number keys were handed out in. A namespace
// that is already known stays where it is.
func (c *Config) Remember(namespaces []string) {
	for _, name := range namespaces {
		if len(c.Namespaces) >= MaxNamespaces {
			return
		}

		if !slices.Contains(c.Namespaces, name) {
			c.Namespaces = append(c.Namespaces, name)
		}
	}
}

func configPath() (string, error) {
	home := os.Getenv("XDG_CONFIG_HOME")
	if home == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}

		home = dir
	}

	return filepath.Join(home, "urga", "config.json"), nil
}
