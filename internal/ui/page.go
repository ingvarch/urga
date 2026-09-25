package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// page is a screen as a type of its own: what it was opened for, what the
// cluster said about it, and how that reads as rows. How the rows are read,
// the filter, the order and the marks, is the list's; what belongs to the
// session is the root's.
type page interface {
	// title labels the box; count is what the filter leaves on the screen.
	title(e env, count int) string

	// titles are the columns of the rows.
	titles() []string

	// topics are what the cluster is asked to say about while the page is
	// up: a change in one of them is the page no longer being what it shows.
	topics() []string

	// fetch asks the cluster for what the page shows. Nil asks nothing.
	fetch(e env) tea.Cmd

	// take keeps an answer the page asked for, and says whether it was one.
	take(msg tea.Msg) (page, bool)

	// rows are what the page shows.
	rows(e env) []tableRow

	// keys are what the page answers, and press is one of them pressed; it
	// says whether the key did anything.
	keys(e env) []keyHint
	press(key string, e env) (page, outcome, bool)
}

// env is what a page may read of the session. It is built for every call,
// and no page keeps it: the session may be somewhere else by the next one.
type env struct {
	client    Client
	namespace string

	// row is the resource under the cursor, when onRow says there is one.
	row   int
	onRow bool

	// Where the session looks and what it can switch to: the region in use
	// and the others, the datacenter the lists are narrowed to (empty is
	// every one) and the choices, the cluster and those of the settings.
	region      string
	regions     []string
	datacenter  string
	datacenters []string
	cluster     string
	clusters    []string

	namespaces []nomad.Namespace
}

// env is the session as the pages see it.
func (m Model) env() env {
	row, onRow := m.list.selected()

	return env{
		client:      m.client,
		namespace:   m.namespace,
		row:         row,
		onRow:       onRow,
		region:      m.regionInUse(),
		regions:     m.regions,
		datacenter:  m.datacenter,
		datacenters: m.datacenterChoices(),
		cluster:     m.opts.Cluster,
		clusters:    m.opts.Clusters,
		namespaces:  m.namespaces,
	}
}

// pickedFrom is the item under the cursor, when there is one.
func pickedFrom[T any](e env, items []T) (T, bool) {
	var none T

	if !e.onRow || e.row >= len(items) {
		return none, false
	}

	return items[e.row], true
}

// pageKey is one key a page answers. A page keeps a table of them typed by the
// page, so what a key does reads and returns that page.
type pageKey[P any] struct {
	press string
	label string
	do    func(p P, e env) (P, outcome)

	// writes says the key changes the cluster, which read-only takes away.
	writes bool

	// offered says the key does something in the state the page is in. Nil
	// is always.
	offered func(p P, e env) bool
}

// keyHint is a key of a page as the root sees it: named, and whether it
// changes the cluster or does anything now.
type keyHint struct {
	press, label    string
	writes, offered bool
}

// hintsOf are the keys of a page's table as the root sees them.
func hintsOf[P any](p P, e env, table []pageKey[P]) []keyHint {
	out := make([]keyHint, 0, len(table))
	for _, k := range table {
		out = append(out, keyHint{
			press:   k.press,
			label:   k.label,
			writes:  k.writes,
			offered: k.offered == nil || k.offered(p, e),
		})
	}

	return out
}

// pressOf runs the key of a page's table that was pressed, when it does
// something now.
func pressOf[P page](p P, e env, table []pageKey[P], press string) (page, outcome, bool) {
	for _, k := range table {
		if k.press == press && (k.offered == nil || k.offered(p, e)) {
			next, out := k.do(p, e)

			return next, out, true
		}
	}

	return p, outcome{}, false
}

// outcome is what a page asks of the session after a key: messages the root
// takes at once, as though they had arrived, and work to do meanwhile. A
// page reaches the rest of the session this way only.
type outcome struct {
	now []tea.Msg
	cmd tea.Cmd
}

// then is an outcome of messages the root takes at once.
func then(msgs ...tea.Msg) outcome { return outcome{now: msgs} }

// What a page asks of the session.
type (
	// backMsg goes back to the screen the page was opened from.
	backMsg struct{}

	// switchRegionMsg, narrowMsg and switchClusterMsg move the session to
	// another region, datacenter (empty is every one) or cluster.
	switchRegionMsg  string
	narrowMsg        string
	switchClusterMsg string
)

// noKeys is a page without keys of its own.
type noKeys struct{}

func (noKeys) keys(env) []keyHint { return nil }

func (noKeys) press(string, env) (page, outcome, bool) { return nil, outcome{}, false }
