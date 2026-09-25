// Command urga is a terminal interface for a HashiCorp Nomad cluster.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
	"github.com/ingvarch/urga/internal/release"
	"github.com/ingvarch/urga/internal/settings"
	"github.com/ingvarch/urga/internal/ui"
	"github.com/ingvarch/urga/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "urga:", err)
		os.Exit(1)
	}
}

func run() error {
	cl, err := parseFlags(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	if err != nil {
		return err
	}

	if cl.version {
		fmt.Println("urga", version.Full())
		return nil
	}

	model, err := open(cl)
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(model).Run()

	return err
}

// open builds urga from the command line and the settings, before any
// request goes to the cluster.
func open(cl cmdline) (ui.Model, error) {
	dir, err := config.Dir()
	if err != nil {
		return ui.Model{}, err
	}

	s, err := settings.Load(filepath.Join(dir, "clusters.toml"))
	if err != nil {
		return ui.Model{}, err
	}

	st, err := startOn(cl, s)
	if err != nil {
		return ui.Model{}, err
	}

	client, err := nomad.New(st.nomad)
	if err != nil {
		return ui.Model{}, err
	}

	// What the last session was looking at. A session that cannot be read
	// starts fresh.
	cfg, err := config.Load()
	if err != nil {
		return ui.Model{}, err
	}

	// Without clusters in the settings there is nothing to switch to.
	var connect func(string) (ui.Connection, error)
	if len(s.Clusters) > 0 {
		connect = connectWith(cl, s)
	}

	return ui.New(client, ui.Options{
		Cluster:        st.cluster,
		Color:          st.color,
		Clusters:       s.Names(),
		Connect:        connect,
		Namespace:      ui.NamespaceOrAll(st.namespace),
		NamespaceGiven: st.namespaceGiven,
		ReadOnly:       st.readOnly,
		Version:        version.Current(),
		Config:         cfg,
		Editor:         ui.NewEditor(),
		Shell:          ui.NewShell(client),
		InRegion:       ui.InRegionOf(client),
		NewerRelease:   newerRelease(os.Getenv, version.Version),
	}), nil
}

// cmdline is what the command line says.
type cmdline struct {
	version   bool
	cluster   string
	address   string
	region    string
	namespace string
	readOnly  bool

	namespaceGiven bool
}

func parseFlags(args []string) (cmdline, error) {
	var cl cmdline

	flags := flag.NewFlagSet("urga", flag.ContinueOnError)
	flags.BoolVar(&cl.version, "version", false, "print the version and exit")
	flags.StringVar(&cl.cluster, "cluster", "", "cluster of the settings file to start on, defaults to the default of the file")
	flags.StringVar(&cl.address, "address", "", "address of the Nomad cluster, defaults to NOMAD_ADDR")
	flags.StringVar(&cl.region, "region", "", "region to ask in, defaults to NOMAD_REGION, then to the one of the agent")
	flags.StringVar(&cl.namespace, "namespace", os.Getenv("NOMAD_NAMESPACE"), "namespace to look at, empty is all of them")
	flags.BoolVar(&cl.readOnly, "readonly", false, "change nothing in the cluster: the keys that would are taken away")

	err := flags.Parse(args)
	cl.namespaceGiven = given(flags, "namespace")

	return cl, err
}

// start is what urga starts with: the cluster, how to reach it, whether it
// is read-only, and the namespace to look at.
type start struct {
	cluster        string
	color          string
	nomad          nomad.Config
	readOnly       bool
	namespace      string
	namespaceGiven bool
}

// startOn picks the cluster: the one the command line names, then the
// default of the settings, then the environment. What the command line says
// wins over the settings of the cluster.
func startOn(cl cmdline, s settings.Settings) (start, error) {
	st := start{readOnly: cl.readOnly, namespace: cl.namespace, namespaceGiven: cl.namespaceGiven}

	name := cl.cluster
	if name == "" {
		name = s.Default
	}

	if name == "" {
		st.nomad = nomad.Config{Address: cl.address, Region: cl.region}

		return st, nil
	}

	cluster, cfg, err := clusterOf(s, name)
	if err != nil {
		return start{}, err
	}

	st.cluster, st.color = name, cluster.Color
	st.readOnly = st.readOnly || cluster.ReadOnly

	st.nomad = cfg
	st.nomad.Address = cmp.Or(cl.address, cfg.Address)
	st.nomad.Region = cmp.Or(cl.region, cfg.Region)

	// NOMAD_NAMESPACE is only for a run without settings.
	if !st.namespaceGiven {
		st.namespace = cluster.Namespace
	}

	return st, nil
}

// clusterOf is a cluster of the settings, and how to reach it. Its token is
// read here, which may run a command.
func clusterOf(s settings.Settings, name string) (settings.Cluster, nomad.Config, error) {
	cluster, ok := s.Clusters[name]
	if !ok {
		names := strings.Join(s.Names(), ", ")
		if names == "" {
			names = "the settings file names none"
		}

		return settings.Cluster{}, nomad.Config{}, fmt.Errorf("no cluster %q: %s", name, names)
	}

	token, err := cluster.ReadToken()
	if err != nil {
		return settings.Cluster{}, nomad.Config{}, fmt.Errorf("cluster %q: %w", name, err)
	}

	return cluster, nomad.Config{
		Named:   true,
		Address: cluster.Address,
		Region:  cluster.Region,
		Token:   token,
		TLS: nomad.TLS{
			CACert:     cluster.CACert,
			ClientCert: cluster.ClientCert,
			ClientKey:  cluster.ClientKey,
			ServerName: cluster.TLSServerName,
		},
	}, nil
}

// connectWith is how the session switches to a cluster of the settings.
// -readonly applies to every one of them; -address and -region only to the
// cluster urga started on.
func connectWith(cl cmdline, s settings.Settings) func(name string) (ui.Connection, error) {
	return func(name string) (ui.Connection, error) {
		cluster, cfg, err := clusterOf(s, name)
		if err != nil {
			return ui.Connection{}, err
		}

		client, err := nomad.New(cfg)
		if err != nil {
			return ui.Connection{}, fmt.Errorf("cluster %q: %w", name, err)
		}

		return ui.Connection{
			Name:      name,
			Color:     cluster.Color,
			ReadOnly:  cl.readOnly || cluster.ReadOnly,
			Namespace: ui.NamespaceOrAll(cluster.Namespace),
			Client:    client,
			InRegion:  ui.InRegionOf(client),
			Shell:     ui.NewShell(client),
		}, nil
	}
}

// given says the flag was named on the command line rather than left at its
// default.
func given(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(f *flag.Flag) { found = found || f.Name == name })

	return found
}

// noUpdateCheck set to any value stops urga from checking for a newer
// release, for a network that cannot or should not reach it.
const noUpdateCheck = "URGA_NO_UPDATE_CHECK"

// newerRelease asks whether a release newer than current is out. It is nil
// when it must not ask: the check is turned off, or current was built from
// source and has no release to be behind.
func newerRelease(getenv func(string) string, current string) func(ctx context.Context) (string, error) {
	if getenv(noUpdateCheck) != "" || !release.IsRelease(current) {
		return nil
	}

	return func(ctx context.Context) (string, error) {
		return release.NewerThan(ctx, http.DefaultClient, release.LatestURL, current)
	}
}
