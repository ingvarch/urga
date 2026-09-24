// Package settings is what the user writes down for urga: the clusters it
// can talk to. urga reads the file and never writes it.
package settings

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Colours are the colours a cluster can be painted in.
var Colours = []string{"red", "orange", "yellow", "green", "cyan", "blue", "purple"}

// tokenTimeout is how long a token command may take: a password manager may
// ask for a fingerprint first.
const tokenTimeout = time.Minute

// Settings are the clusters of the file, and the one urga starts on.
type Settings struct {
	Default  string             `toml:"default"`
	Clusters map[string]Cluster `toml:"clusters"`
}

// Cluster is one cluster: where it is, how urga proves who it is, and how
// careful to be with it.
type Cluster struct {
	Address   string `toml:"address"`
	Region    string `toml:"region"`
	Namespace string `toml:"namespace"`

	// The token comes from one of these, or there is none.
	Token        string   `toml:"token"`
	TokenEnv     string   `toml:"token_env"`
	TokenCommand []string `toml:"token_command"`

	CACert        string `toml:"ca_cert"`
	ClientCert    string `toml:"client_cert"`
	ClientKey     string `toml:"client_key"`
	TLSServerName string `toml:"tls_server_name"`

	ReadOnly bool   `toml:"read_only"`
	Color    string `toml:"color"`
}

// Load reads the file. Without one there are no clusters, and urga runs
// from the environment. A file that is there is read in full or refused.
func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, nil
	}

	if err != nil {
		return Settings{}, err
	}

	var s Settings

	meta, err := toml.Decode(string(data), &s)
	if err != nil {
		return Settings{}, fmt.Errorf("%s: %w", path, err)
	}

	// A typo in a file that holds tokens must not pass unnoticed.
	if unknown := meta.Undecoded(); len(unknown) > 0 {
		return Settings{}, fmt.Errorf("%s: unknown key %s", path, unknown[0])
	}

	if err := s.check(); err != nil {
		return Settings{}, fmt.Errorf("%s: %w", path, err)
	}

	for name, cluster := range s.Clusters {
		s.Clusters[name] = cluster.fromHome()
	}

	return s, nil
}

// Names are the clusters in the order of their names.
func (s Settings) Names() []string {
	names := make([]string, 0, len(s.Clusters))
	for name := range s.Clusters {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// check refuses what would connect somewhere else than the file means.
func (s Settings) check() error {
	if _, ok := s.Clusters[s.Default]; s.Default != "" && !ok {
		return fmt.Errorf("default %q is not a cluster of the file", s.Default)
	}

	for _, name := range s.Names() {
		cluster := s.Clusters[name]

		if cluster.Address == "" {
			return fmt.Errorf("cluster %q has no address", name)
		}

		sources := 0
		for _, given := range []bool{cluster.Token != "", cluster.TokenEnv != "", len(cluster.TokenCommand) > 0} {
			if given {
				sources++
			}
		}

		if sources > 1 {
			return fmt.Errorf("cluster %q: one of token, token_env and token_command, not more", name)
		}

		if cluster.Color != "" && !slices.Contains(Colours, cluster.Color) {
			return fmt.Errorf("cluster %q: %q is not a colour: %s", name, cluster.Color, strings.Join(Colours, ", "))
		}
	}

	return nil
}

// fromHome reads a path that starts at ~/ from the home directory.
func (c Cluster) fromHome() Cluster {
	home, err := os.UserHomeDir()
	if err != nil {
		return c
	}

	for _, path := range []*string{&c.CACert, &c.ClientCert, &c.ClientKey} {
		if rest, ok := strings.CutPrefix(*path, "~/"); ok {
			*path = filepath.Join(home, rest)
		}
	}

	return c
}

// ReadToken is the token of the cluster, from where the file says it is.
// None is no token: the one of the environment belongs to another cluster.
func (c Cluster) ReadToken() (string, error) {
	switch {
	case c.TokenEnv != "":
		token, ok := os.LookupEnv(c.TokenEnv)
		if !ok {
			return "", fmt.Errorf("%s is not set", c.TokenEnv)
		}

		return token, nil

	case len(c.TokenCommand) > 0:
		return runToken(c.TokenCommand)
	}

	return c.Token, nil
}

// runToken runs the command that prints the token.
func runToken(command []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tokenTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w: %s", command[0], err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}
