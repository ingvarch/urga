package ui

import tea "charm.land/bubbletea/v2"

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
}

// env is what a page may read of the session. It is built for every call,
// and no page keeps it: the session may be somewhere else by the next one.
type env struct {
	client    Client
	namespace string
}

// env is the session as the pages see it.
func (m Model) env() env {
	return env{client: m.client, namespace: m.namespace}
}
