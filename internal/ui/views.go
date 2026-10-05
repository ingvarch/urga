package ui

import "github.com/ingvarch/urga/internal/nomad"

// view is a list the session opens by name: from the command line, or as
// the list the next run reopens.
type view struct {
	// stored is the name the view is saved under between runs. A view
	// without one is not reopened.
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

	volumesView = &view{
		stored:  "volumes",
		aliases: []string{"volumes", "volume", "vol"},
		open:    func() page { return volumesPage{} },
	}

	pluginsView = &view{
		stored:  "plugins",
		aliases: []string{"plugins", "plugin"},
		open:    func() page { return pluginsPage{} },
	}

	// The access control of the cluster, a list for each kind.
	tokensView       = aclView(nomad.ACLToken, "tokens", "token")
	policiesView     = aclView(nomad.ACLPolicy, "policies", "policy", "pol")
	rolesView        = aclView(nomad.ACLRole, "roles", "role")
	authMethodsView  = aclView(nomad.ACLAuthMethod, "authmethods", "authmethod", "auth")
	bindingRulesView = aclView(nomad.ACLBindingRule, "bindingrules", "bindingrule", "br")

	eventsView = &view{
		stored:  "events",
		aliases: []string{"events", "event"},
		open:    func() page { return feedPage{} },
	}

	scalingView = &view{
		stored:  "scaling",
		aliases: []string{"scaling", "scale"},
		open:    func() page { return scalingPoliciesPage{} },
	}

	schedulerView = &view{
		stored:  "scheduler",
		aliases: []string{"scheduler", "sched"},
		open:    func() page { return schedulerPage{} },
	}

	overviewView = &view{
		stored:  "overview",
		aliases: []string{"overview", "ov"},
		open:    func() page { return overviewPage{} },
	}

	// aboutView is this urga. It is not a list of the cluster, so the next
	// run does not reopen it.
	aboutView = &view{
		aliases: []string{"about", "version"},
		open:    func() page { return aboutPage{} },
	}
)

// views is the one place that knows which lists urga opens by name, so that
// adding one is adding a line here.
var views = []*view{
	jobsView, allocationsView, deploymentsView, servicesView, evaluationsView,
	nodesView, variablesView, regionsView, datacentersView, clustersView,
	namespacesView, serversView, nodePoolsView, volumesView, pluginsView,
	tokensView, policiesView, rolesView, authMethodsView, bindingRulesView, eventsView, scalingView, schedulerView, overviewView, aboutView,
}

// opened is the list as a screen just opened, with nothing read into it yet.
func (v *view) opened() screen { return screen{page: v.open(), view: v} }

// aclView is the list of a kind of ACL object, stored under its first alias.
func aclView(kind string, aliases ...string) *view {
	return &view{stored: aliases[0], aliases: aliases, open: func() page { return aclPage{kind: kind} }}
}
