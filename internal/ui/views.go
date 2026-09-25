package ui

// view is a list the session opens by name: from the command line, or as
// the list the next run comes back to.
type view struct {
	// stored is how the view is written down between runs. A view without
	// one is not come back to.
	stored string

	// aliases are the words the command line takes for it. The first one is
	// what the prompt suggests.
	aliases []string

	// open makes its page, with nothing read into it yet.
	open func() page
}

// The lists urga opens by name.
var (
	jobsView = &view{
		stored:  "jobs",
		aliases: []string{"jobs", "job", "jb"},
		open:    func() page { return jobsPage{} },
	}

	allocationsView = &view{
		aliases: []string{"allocations", "allocation", "allocs", "alloc"},
		open:    func() page { return allocationsPage{} },
	}

	deploymentsView = &view{
		stored:  "deployments",
		aliases: []string{"deployments", "deployment", "dp"},
		open:    func() page { return deploymentsPage{} },
	}

	servicesView = &view{
		stored:  "services",
		aliases: []string{"services", "service", "svc"},
		open:    func() page { return servicesPage{} },
	}

	evaluationsView = &view{
		stored:  "evaluations",
		aliases: []string{"evaluations", "evaluation", "evals", "eval", "ev"},
		open:    func() page { return evaluationsPage{} },
	}

	nodesView = &view{
		// Nomad calls them clients in its own interface; the command line
		// takes either word.
		stored:  "nodes",
		aliases: []string{"clients", "client", "nodes", "node", "no"},
		open:    func() page { return nodesPage{} },
	}

	variablesView = &view{
		stored:  "variables",
		aliases: []string{"variables", "variable", "vars", "var"},
		open:    func() page { return variablesPage{} },
	}

	// The lists a region and a datacenter are picked from. The words that
	// switch them open them, so they have no alias of their own.
	regionsView     = &view{open: func() page { return regionsPage{} }}
	datacentersView = &view{open: func() page { return datacentersPage{} }}
	clustersView    = &view{open: func() page { return clustersPage{} }}

	namespacesView = &view{
		stored:  "namespaces",
		aliases: []string{"namespaces", "namespace", "ns"},
		open:    func() page { return namespacesPage{} },
	}

	serversView = &view{
		stored:  "servers",
		aliases: []string{"servers", "server", "srv"},
		open:    func() page { return serversPage{} },
	}

	nodePoolsView = &view{
		stored:  "nodepools",
		aliases: []string{"nodepools", "nodepool", "np"},
		open:    func() page { return nodePoolsPage{} },
	}
)

// views is the one place that knows which lists urga opens by name, so that
// adding one is adding a line here.
var views = []*view{
	jobsView, allocationsView, deploymentsView, servicesView, evaluationsView,
	nodesView, variablesView, regionsView, datacentersView, clustersView,
	namespacesView, serversView, nodePoolsView,
}

// opened is the list as a screen just opened, with nothing read into it yet.
func (v *view) opened() screen { return screen{page: v.open(), view: v} }
