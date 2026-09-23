// Command urga is a terminal interface for a HashiCorp Nomad cluster.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/config"
	"github.com/ingvarch/urga/internal/nomad"
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
	showVersion := flag.Bool("version", false, "print the version and exit")
	address := flag.String("address", "", "address of the Nomad cluster, defaults to NOMAD_ADDR")
	region := flag.String("region", "", "region to ask in, defaults to NOMAD_REGION, then to the one of the agent")
	namespace := flag.String("namespace", os.Getenv("NOMAD_NAMESPACE"), "namespace to look at, empty is all of them")
	flag.Parse()

	if *showVersion {
		fmt.Println("urga", version.Full())
		return nil
	}

	client, err := nomad.New(nomad.Config{Address: *address, Region: *region})
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
		Namespace: ui.NamespaceOrAll(*namespace),
		Version:   version.Current(),
		Config:    cfg,
		Editor:    ui.NewEditor(),
		Shell:     ui.NewShell(client),
		InRegion:  ui.InRegionOf(client),
	})

	_, err = tea.NewProgram(model).Run()

	return err
}
