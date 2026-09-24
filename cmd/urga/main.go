// Command urga is a terminal interface for a HashiCorp Nomad cluster.
package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
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

	dir, err := config.Dir()
	if err != nil {
		return err
	}

	s, err := settings.Load(filepath.Join(dir, "clusters.toml"))
	if err != nil {
		return err
	}

	st, err := startOn(cl, s)
	if err != nil {
		return err
	}

	client, err := nomad.New(st.nomad)
	if err != nil {
		return err
	}

	// What the last session was looking at. A session that cannot be read
	// starts fresh.
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	model := ui.New(client, ui.Options{
		Cluster:        st.cluster,
		Color:          st.color,
		Namespace:      ui.NamespaceOrAll(st.namespace),
		NamespaceGiven: st.namespaceGiven,
		ReadOnly:       st.readOnly,
		Version:        version.Current(),
		Config:         cfg,
		Editor:         ui.NewEditor(),
		Shell:          ui.NewShell(client),
		InRegion:       ui.InRegionOf(client),
	})

	_, err = tea.NewProgram(model).Run()

	return err
}

// cmdline is what the command line says.
type cmdline struct {
	version   bool
	cluster   string
	address   string
	region    string
	namespace string
	readOnly  bool

	flags *flag.FlagSet
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

	cl.flags = flags

	return cl, flags.Parse(args)
}

// start is where urga starts: the cluster, how to reach it, how careful to
// be with it, and the namespace to look at.
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
	st := start{readOnly: cl.readOnly, namespace: cl.namespace, namespaceGiven: given(cl.flags, "namespace")}

	name := cl.cluster
	if name == "" {
		name = s.Default
	}

	if name == "" {
		st.nomad = nomad.Config{Address: cl.address, Region: cl.region}

		return st, nil
	}

	cluster, ok := s.Clusters[name]
	if !ok {
		names := strings.Join(s.Names(), ", ")
		if names == "" {
			names = "the settings file names none"
		}

		return start{}, fmt.Errorf("no cluster %q: %s", name, names)
	}

	token, err := cluster.ReadToken()
	if err != nil {
		return start{}, fmt.Errorf("cluster %q: %w", name, err)
	}

	st.cluster, st.color = name, cluster.Color
	st.readOnly = st.readOnly || cluster.ReadOnly
	st.nomad = nomad.Config{
		Named:   true,
		Address: cmp.Or(cl.address, cluster.Address),
		Region:  cmp.Or(cl.region, cluster.Region),
		Token:   token,
		TLS: nomad.TLS{
			CACert:     cluster.CACert,
			ClientCert: cluster.ClientCert,
			ClientKey:  cluster.ClientKey,
			ServerName: cluster.TLSServerName,
		},
	}

	// The environment is for the cluster urga runs without settings.
	if !st.namespaceGiven {
		st.namespace = cluster.Namespace
	}

	return st, nil
}

// given says the flag was named on the command line rather than left at its
// default.
func given(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(f *flag.Flag) { found = found || f.Name == name })

	return found
}
