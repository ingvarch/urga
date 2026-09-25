package ui

import (
	"context"

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
	// What it asks of the session then belongs to the ask of the page.
	take(msg tea.Msg, e env) (page, outcome, bool)

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

	// marks are the resources an action is to take, by the ids the page
	// names its rows with.
	marks map[string]bool

	// index are the rows the filter and the order leave on the screen, by
	// their place among the rows the page built. usage is what the resource
	// of each row takes, by its id, as far as it was read.
	index []int
	usage map[string]nomad.ResourceUse

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

	// readOnly takes away every key that changes the cluster.
	readOnly bool
}

// env is the session as the pages see it.
func (m Model) env() env {
	row, onRow := m.list.selected()

	return env{
		client:      m.client,
		namespace:   m.namespace,
		row:         row,
		onRow:       onRow,
		marks:       m.list.marks,
		index:       m.list.index,
		usage:       m.usage.rows,
		region:      m.regionInUse(),
		regions:     m.regions,
		datacenter:  m.datacenter,
		datacenters: m.datacenterChoices(),
		cluster:     m.opts.Cluster,
		clusters:    m.opts.Clusters,
		namespaces:  m.namespaces,
		readOnly:    m.opts.ReadOnly,
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

// markedFrom are the items that carry a mark. A mark is on the resource,
// not on the line it sits on, so a filter or a sort does not change what an
// action takes. Without a mark anywhere, what the cursor is on is the
// answer, which is how every action reads a list.
func markedFrom[T any](e env, items []T, id func(T) string) []T {
	out := []T{}

	for _, item := range items {
		if e.marks[id(item)] {
			out = append(out, item)
		}
	}

	// Marks that name nothing on this page any more leave the cursor to
	// answer, rather than the key doing nothing at all.
	if len(out) > 0 {
		return out
	}

	one, ok := pickedFrom(e, items)
	if !ok {
		return nil
	}

	return []T{one}
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

	// reading says what the page took came from a timer of its own, not
	// the answer to its ask: the screen is no fresher for it, so the error
	// that is up stays and the poll keeps its time.
	reading bool
}

// then is an outcome of messages the root takes at once.
func then(msgs ...tea.Msg) outcome { return outcome{now: msgs} }

// What a page asks of the session.
type (
	// backMsg goes back to the screen the page was opened from.
	backMsg struct{}

	// openMsg opens a screen on top of the page.
	openMsg screen

	// sayMsg puts what came of a key on the status line; warnMsg puts
	// something worth knowing there.
	sayMsg  string
	warnMsg string

	// wrapMsg and saveMsg are the window over a text: wrap its lines, or
	// write what it shows to a file. followMsg follows the end of a stream,
	// or stops following it, and timesMsg puts when each line arrived in
	// front of it.
	wrapMsg   struct{}
	saveMsg   struct{}
	followMsg struct{}
	timesMsg  struct{}

	// reopenMsg reads the stream of the page again, the way entering the
	// page does, in the window it is read in: the page reads another
	// stream now.
	reopenMsg struct{}

	// switchRegionMsg, narrowMsg and switchClusterMsg move the session to
	// another region, datacenter (empty is every one) or cluster.
	switchRegionMsg  string
	narrowMsg        string
	switchClusterMsg string

	// askMsg puts a question up; yes runs apply.
	askMsg struct {
		question string
		apply    tea.Cmd
	}

	// requestMsg is work for the screen that is up: what it answers is
	// dropped once the screen is left, the way the answer to its fetch is.
	requestMsg tea.Cmd

	// markMsg takes the row under the cursor for an action, or lets it go;
	// markAllMsg every row on the screen.
	markMsg    struct{}
	markAllMsg struct{}

	// failMsg puts what went wrong on the status line; forgetMsg takes an
	// error off it, once what went wrong is over.
	failMsg   struct{ err error }
	forgetMsg struct{}
)

// What a page asks of the session that the session still does with code of
// its own.
type (
	// scaleMsg asks for the count of a group, and then whether to set it.
	scaleMsg nomad.TaskGroup

	// signalMsg asks which signal to send to a task, and then whether to.
	signalMsg taskRef

	// shellMsg opens a shell in a task, with the terminal handed over.
	shellMsg shellCommand
)

// copyKey copies the value of the field under the cursor, on a page that
// reads as a list of fields: that is how an address or an id gets out of the
// screen and into a command somewhere else. It goes over OSC52, so it works
// through ssh.
func copyKey[P page]() pageKey[P] {
	return pageKey[P]{press: "c", label: "Copy", do: func(p P, e env) (P, outcome) {
		// The value is the second column on every page of fields; a page
		// may carry more after it, like where the value came from.
		row, ok := pickedFrom(e, p.rows(e))
		if !ok || len(row.cells) < 2 {
			return p, outcome{}
		}

		return p, copying(row.cells[0], row.cells[1])
	}}
}

// copying puts a value on the clipboard and says whose it is.
func copying(field, value string) outcome {
	return outcome{now: []tea.Msg{sayMsg(sprintf("Copied %s.", field))}, cmd: tea.SetClipboard(value)}
}

// markKey and markAllKey take rows for an action, on a page whose rows name
// what they show.
func markKey[P page]() pageKey[P] {
	return pageKey[P]{press: "space", label: "Mark", do: func(p P, _ env) (P, outcome) { return p, then(markMsg{}) }}
}

func markAllKey[P page]() pageKey[P] {
	return pageKey[P]{press: "ctrl+a", label: "Mark All", do: func(p P, _ env) (P, outcome) { return p, then(markAllMsg{}) }}
}

// marking is a page whose rows can be marked: ids name the resource of each
// row, in the order of the rows, so that a mark belongs to the resource and
// not to the line it sits on.
type marking interface {
	ids(e env) []string
}

// measured is a page whose rows show what their resources take: readings
// are the resources of the rows on the screen, and reading reads one.
type measured interface {
	readings(e env) []rowRef
	reading(ctx context.Context, client Client, ref rowRef) (nomad.ResourceUse, error)
}

// panelled is a page with something to say above its rows, in no more than
// room rows.
type panelled interface {
	panel(e env, width, room int) []string
}

// buttoned is a page with a question at its foot: bar is the question and
// its buttons, in the rows it takes. The buttons are chosen and pressed the
// way the buttons of any question are, so their keys are the question's:
// help names them, the header does not.
type buttoned interface {
	bar(e env, width int) []string
	button(key string, e env) (page, outcome, bool)
}

// reader is a page that reads as text rather than as a list. The text is the
// page's; the window over it, the wrap, the filter and the times, is the
// root's, as the list is for a page of rows.
type reader interface {
	text(e env) textContent
}

// textKeys are the keys of a page that reads as text.
func textKeys[P page]() []pageKey[P] {
	return []pageKey[P]{wrapKey[P](), saveKey[P]()}
}

func wrapKey[P page]() pageKey[P] { return windowKey[P]("w", "Toggle Wrap", wrapMsg{}) }

func saveKey[P page]() pageKey[P] { return windowKey[P]("ctrl+s", "Save", saveMsg{}) }

// followKey follows the end of what a stream writes, or stops following it
// where it stands; timesKey puts when each line of it arrived in front of
// the line.
func followKey[P page]() pageKey[P] { return windowKey[P]("s", "Toggle Autoscroll", followMsg{}) }

func timesKey[P page]() pageKey[P] { return windowKey[P]("t", "Toggle Timestamps", timesMsg{}) }

// windowKey is a key of the window over a text, which the root holds.
func windowKey[P page](press, label string, msg tea.Msg) pageKey[P] {
	return pageKey[P]{press: press, label: label, do: func(p P, _ env) (P, outcome) { return p, then(msg) }}
}

// saving is a page of text that says what a file of it is called: what it
// is of, and the extension.
type saving interface {
	saveAs() (what, extension string)
}

// streamer is a page that reads what something writes as it is written: a
// log, or a file. The root opens the stream every time the page is entered,
// since every ask of the session ends the reading it had, and closes it
// whenever the page stops being the one on top. follows says the window
// keeps to the end of what arrives from the start.
type streamer interface {
	open(e env) (page, tea.Cmd)
	close() page
	follows() bool
}

// restarter is a page with timers of its own. Every ask of the session ends
// the chains it had, so restart lets go of the ones the page thinks are on
// their way.
type restarter interface {
	restart() page
}

// follower is a page that lists what the session looks at rather than one
// thing: it follows the session to another namespace or datacenter.
type follower interface {
	followsSession() bool
}

// ofTheSession is a list the session opens by name, of what it looks at.
type ofTheSession struct{}

func (ofTheSession) followsSession() bool { return true }

// noAnswers is a page that asks the cluster nothing: what it shows came
// with it.
type noAnswers struct{}

func (noAnswers) take(tea.Msg, env) (page, outcome, bool) { return nil, outcome{}, false }

// noKeys is a page without keys of its own.
type noKeys struct{}

func (noKeys) keys(env) []keyHint { return nil }

func (noKeys) press(string, env) (page, outcome, bool) { return nil, outcome{}, false }
