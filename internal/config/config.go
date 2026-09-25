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

// Config holds the saved session of each cluster.
type Config struct {
	// Session is the one of the cluster the environment names. It sits at
	// the top level of the file, where older versions kept their only one.
	Session

	// Clusters are the sessions of the clusters the settings name.
	Clusters map[string]*Session `json:"clusters,omitempty"`

	path string
}

// Session is the namespace and the screen one cluster had open.
type Session struct {
	// Namespace is the one in use. A pointer, because all namespaces at once
	// is a choice too, and must not look like no choice at all.
	Namespace *string `json:"namespace"`

	// Screen is the resource that was open.
	Screen string `json:"screen"`

	// Namespaces is which namespace each number key selects.
	Namespaces []string `json:"namespaces"`
}

// Of is the session of a cluster; the empty name gives the one the
// environment names. A cluster seen for the first time gets an empty one.
func (c *Config) Of(cluster string) *Session {
	if cluster == "" {
		return &c.Session
	}

	if c.Clusters == nil {
		c.Clusters = map[string]*Session{}
	}

	if _, ok := c.Clusters[cluster]; !ok {
		c.Clusters[cluster] = &Session{}
	}

	return c.Clusters[cluster]
}

// Load reads the saved sessions. A first run, or a file that cannot be
// read, gives an empty config instead of an error that would stop urga.
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
func (c *Session) UseNamespace(namespace string) {
	c.Namespace = &namespace
}

// Remember keeps the order the number keys were handed out in.
func (c *Session) Remember(namespaces []string) {
	c.Namespaces = Ordered(c.Namespaces, namespaces)
}

// Ordered is the rule the number keys follow: first seen keeps its number, a
// namespace already known stays where it is, and no more than MaxNamespaces
// get a key.
func Ordered(known, seen []string) []string {
	order := slices.Clone(known)

	for _, name := range seen {
		if len(order) >= MaxNamespaces {
			break
		}

		if !slices.Contains(order, name) {
			order = append(order, name)
		}
	}

	return order
}

func configPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "config.json"), nil
}

// Dir is where urga keeps what it remembers, and where the user writes its
// settings.
func Dir() (string, error) {
	home := os.Getenv("XDG_CONFIG_HOME")
	if home == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}

		home = dir
	}

	return filepath.Join(home, "urga"), nil
}
