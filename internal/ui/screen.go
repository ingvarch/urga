package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// screen is a page on the stack.
type screen struct {
	page page

	// view is the list the page was opened as by name; nil for a page
	// opened from another one, which the next run does not come back to: a
	// session comes back to a list, not to the allocations of a job it no
	// longer remembers.
	view *view

	// left is where the screen was when another one was opened on top of
	// it, which is where escape comes back to.
	left place
}

// place is where a list was left: the row under the cursor, how far down the
// window was, and the filter and the sort order it had. A row number means
// nothing in a list that is filtered or sorted another way.
type place struct {
	cursor int
	top    int
	filter string
	sort   sortState
}

// followsSession says the screen is a list of what the session looks at: it
// follows the session to another namespace or datacenter. A screen opened
// for one job, allocation or stream does not.
func (s screen) followsSession() bool {
	f, ok := s.page.(follower)

	return ok && f.followsSession()
}

// listsTheNamespace says the screen is a list of the namespace the session
// looks at, so what it read holds only in that namespace.
func (s screen) listsTheNamespace() bool {
	_, ok := s.page.(namespaced)

	return ok && s.followsSession()
}

// unread is the screen with nothing read into its page, when the page is a
// list of the session's namespace. The place it was left goes too, its row,
// filter and sort order: the list comes back the way the open one is entered
// after a switch.
func (s screen) unread() screen {
	if s.listsTheNamespace() {
		s.page = s.page.(namespaced).unread()
		s.left = place{sort: newSortState()}
	}

	return s
}

// pageKeys are the keys of the open page, in the order the header shows
// them, whether or not each one does something right now.
func (m Model) pageKeys() []keyHint {
	return m.screen.page.keys(m.env())
}

// pressPage passes a key to the open page and applies what it returns.
func (m Model) pressPage(press string) (Model, tea.Cmd) {
	next, out, ok := m.screen.page.press(press, m.env())
	if !ok {
		return m, nil
	}

	m.screen.page = next

	return m.apply(out)
}

// buttonKey passes a key to the buttons at the foot of the open page, and
// applies what they return.
func (m Model) buttonKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	b, ok := m.screen.page.(buttoned)
	if !ok {
		return m, nil, false
	}

	next, out, ok := b.button(msg.String(), m.env())
	if !ok {
		return m, nil, false
	}

	m.screen.page = next
	m, cmd := m.apply(out)

	return m, cmd, true
}

// bar is the question at the foot of the open page, when it has one.
func (m Model) bar(width int) []string {
	if b, ok := m.screen.page.(buttoned); ok {
		return b.bar(m.env(), width)
	}

	return nil
}

// barRows is how many rows of the box the question at the foot of the page
// takes.
func (m Model) barRows() int {
	return len(m.bar(m.innerWidth()))
}

// took stores what the open page made of an answer, and applies what it
// returns. The work it starts belongs to the ask of the page and ends with
// it. An answer to the ask marks the screen as answered; a reading from a
// timer of the page does not.
func (m Model) took(next page, out outcome) (Model, tea.Cmd) {
	out.cmd = askedFor(m.asked, out.cmd)
	m.screen.page = next

	if out.reading {
		return m.apply(out)
	}

	m, poll := m.markAnswered()
	m, cmd := m.apply(out)

	// Rows that show their resource usage are read once they are filled.
	return usageOnce(m, tea.Batch(poll, cmd))
}

// apply carries out what a page returned: its messages at once, in order,
// and its command in the background.
func (m Model) apply(out outcome) (Model, tea.Cmd) {
	cmds := []tea.Cmd{out.cmd}

	for _, msg := range out.now {
		var cmd tea.Cmd

		m, cmd = m.update(msg)
		cmds = append(cmds, cmd)
	}

	m.layout()

	return m, tea.Batch(cmds...)
}

// keys are what the open page offers now: a key that would do nothing in
// the state the page is in is not offered, nor, read-only, a key that
// changes the cluster.
func (m Model) keys() []keyHint {
	all := m.pageKeys()

	keys := make([]keyHint, 0, len(all))
	for _, k := range all {
		if k.offered && !m.withheld(k) {
			keys = append(keys, k)
		}
	}

	return keys
}

// withheld says read-only mode hides the key.
func (m Model) withheld(k keyHint) bool {
	return m.opts.ReadOnly && k.writes
}

// withheldKey is the key of the open page that read-only mode hides, when
// the press is one. Of several keys on one press, the one the page offers
// now comes first.
func (m Model) withheldKey(press string) (keyHint, bool) {
	var first keyHint

	found := false

	for _, k := range m.pageKeys() {
		if k.press != press || !m.withheld(k) {
			continue
		}

		if k.offered {
			return k, true
		}

		if !found {
			first, found = k, true
		}
	}

	return first, found
}

// hints are the keys the open resource answers. Keys that work everywhere
// are not here, they live in help.
func (m Model) hints() []hint {
	keys := m.keys()

	hints := make([]hint, 0, len(keys))
	for _, k := range keys {
		hints = append(hints, k.hint())
	}

	return hints
}

// offeredKey is what a key does on the page, when the page offers it.
func (m Model) offeredKey(press string) (keyHint, bool) {
	for _, k := range m.keys() {
		if k.press == press {
			return k, true
		}
	}

	return keyHint{}, false
}

// title labels the box with what it holds and how much of it. The count is
// what is on the screen, which the filter narrows.
func (m Model) title() string {
	title := m.screen.page.title(m.env(), len(m.list.table.rows))

	// A screen with a line of toggles says the column there: a log or file
	// title is long, and the edge would cut the column off.
	if m.readsAsText() && m.toggleRows() == 0 && m.text.shift > 0 {
		title += sprintf(", from column %d", m.text.shift+1)
	}

	if p, ok := m.screen.page.(endTitled); ok {
		title += p.titleEnd()
	}

	return title
}

// rows are what the open screen shows.
func (m Model) rows() []tableRow {
	return m.screen.page.rows(m.env())
}

// fetch asks the cluster for what the open screen shows. A screen whose
// content arrives another way, like a description or a log stream, asks for
// nothing.
func (m Model) fetch() tea.Cmd {
	return askedFor(m.asked, m.screen.page.fetch(m.env()))
}

// answerMsg is an answer with the ask it belongs to. The session can move
// on while a request is out; an answer to an earlier ask no longer matches
// the screen.
type answerMsg struct {
	asked int

	// wrote says the work changed the cluster: what it reports still shows
	// after its screen is left.
	wrote bool
	msg   tea.Msg
}

// askedFor labels every message a command returns, the commands of a batch
// included.
func askedFor(asked int, cmd tea.Cmd) tea.Cmd {
	return labelled(cmd, func(msg tea.Msg) tea.Msg { return answerMsg{asked: asked, msg: msg} })
}

// wroteFor labels the messages of work that changes the cluster. Once the
// screen is left they only report what was done.
func wroteFor(asked int, cmd tea.Cmd) tea.Cmd {
	return labelled(cmd, func(msg tea.Msg) tea.Msg { return answerMsg{asked: asked, wrote: true, msg: msg} })
}

// labelled wraps every message a command returns, the commands of a batch
// included.
func labelled(cmd tea.Cmd, label func(tea.Msg) tea.Msg) tea.Cmd {
	if cmd == nil {
		return nil
	}

	return func() tea.Msg {
		msg := cmd()

		if batch, ok := msg.(tea.BatchMsg); ok {
			out := make(tea.BatchMsg, 0, len(batch))
			for _, c := range batch {
				out = append(out, labelled(c, label))
			}

			return out
		}

		if msg == nil {
			return nil
		}

		return label(msg)
	}
}

// request asks the cluster in the background and returns the answer as a
// message. Nothing here touches the model: every answer arrives through
// Update like any other message.
func request[T any](load func(ctx context.Context) (T, error), wrap func(T) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		answer, err := load(ctx)
		if err != nil {
			return errMsg{err: err}
		}

		return wrap(answer)
	}
}

// fetchList is a request for a list of resources.
func fetchList[T any](load func(ctx context.Context) ([]T, error), wrap func([]T) tea.Msg) tea.Cmd {
	return request(load, wrap)
}

// selectedIndex is the resource the cursor is on. The filter shifts the rows,
// so the row number is not the number of the resource.
func (m Model) selectedIndex() (int, bool) {
	return m.list.selected()
}

// show switches to a resource, which is what the command prompt does. Either
// way the session saves which list is open, so the next run opens the same
// one.
func (m Model) show(v *view) (Model, tea.Cmd) {
	if v != m.screen.view {
		return m.push(v.opened())
	}

	return m.arrive()
}

// push opens a list on top of the one that is there, which is where escape
// comes back to.
func (m Model) push(next screen) (Model, tea.Cmd) {
	return m.stack(next).arrive()
}

// stack puts a screen on top of the one that is there, and saves where that
// one was left for escape to come back to. Its filter and the order of its
// rows do not carry over to the screen that opens.
func (m Model) stack(next screen) Model {
	m = m.closeStream()
	m.screen.left = place{cursor: m.list.table.cursor, top: m.list.table.top, filter: m.list.filter, sort: m.list.sort}
	m.history = append(m.history, m.screen)
	m.screen = next
	m = m.forget()
	m.list.filter = ""
	m.list.sort = newSortState()
	m.list.marks = nil

	return m
}

// back is where escape goes.
func (m Model) back() (Model, tea.Cmd) {
	if len(m.history) == 0 {
		return m, nil
	}

	// The stream of the screen is closed before leaving it.
	m = m.closeStream()

	m.screen = m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m = m.forget()
	m.list.marks = nil

	arrived, cmd := m.arrive()

	return arrived.returnTo(arrived.screen.left), cmd
}

// closeStream closes the stream the page on top reads: it is no longer the
// one on top.
func (m Model) closeStream() Model {
	if s, ok := m.screen.page.(streamer); ok {
		m.screen.page = s.close()
	}

	return m
}

// openStream starts reading the stream of the page on top, and the window
// follows its end when the page wants that. The command that reads it
// belongs to the ask of the page and ends with it.
func (m Model) openStream() (Model, tea.Cmd) {
	s, ok := m.screen.page.(streamer)
	if !ok {
		return m, nil
	}

	var cmd tea.Cmd

	m.text.following = s.follows()
	m.screen.page, cmd = s.open(m.env())

	return m, askedFor(m.asked, cmd)
}

// letGo closes a stream that was opened for a page that is no longer open:
// nothing else would ever close it.
func letGo(msg tea.Msg) {
	if opened, ok := msg.(streamOpened); ok {
		opened.release()
	}
}

// streamOpened is a message that brings a stream: one no page takes is
// closed, since nothing else would close it.
type streamOpened interface{ release() }

// returnTo puts a list back the way it was left. Its rows are still there,
// so the cursor lands on the same one; the next answer keeps it there the
// way any refresh does.
func (m Model) returnTo(left place) Model {
	m.list.filter, m.list.sort = left.filter, left.sort
	m.layout()

	m.list.table.cursor, m.list.table.top = left.cursor, left.top
	m.list.table.follow()

	return m
}

// arrive shows the screen and saves the session. Every way of arriving at
// a screen goes through here, so the saved session is never one screen
// behind what is on display.
func (m Model) arrive() (Model, tea.Cmd) {
	entered, cmd := m.enter()

	return entered, tea.Batch(cmd, entered.remember())
}

// enter sets up the table for the screen and asks the cluster for its rows.
func (m Model) enter() (Model, tea.Cmd) {
	// Any request still out was made for the screen before.
	m.asked++
	m.answered = false
	m = m.closeQuestion()

	// So were the readings, and their chain ends with that ask.
	m.usage = m.usage.forgetRows()

	if p, ok := m.screen.page.(restarter); ok {
		m.screen.page = p.restart()
	}

	m.list.table = newTableModel(m.screen.page.titles())
	m.list.filter = ""
	m.list.sort = newSortState()

	// A page of text opens on a window of its own, from the top.
	if _, ok := m.screen.page.(reader); ok {
		m.text = textModel{}
	}

	// What was going on belonged to the screen before.
	if m.flash.held {
		m = m.quiet()
	}

	// A stream is opened again: the command that read it before belonged to
	// an ask that is over.
	m, stream := m.openStream()

	m.layout()

	// The watch of the screen that was left is stopped: the new one watches
	// what it shows, if the cluster can stream.
	m.watch = m.watch.end()

	// The timer of the screen that was left may be a slow one.
	m = m.restartPoll()

	return m, tea.Batch(m.fetch(), stream, m.watchScreen())
}

// closeQuestion takes a question off the screen when the screen under it
// changes: its answer would be sent for the screen that is gone.
func (m Model) closeQuestion() Model {
	switch m.overlay {
	case overlayConfirm:
		return m.closeConfirm()

	case overlayAnswer:
		m, _ = m.closePrompt()
	}

	return m
}

// shortID keeps a UUID readable in a title.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}

	return id
}

// takeAnswer passes an answer to the open screen. An answer to an ask that
// is over is dropped, and a stream it opened is closed.
func (m Model) takeAnswer(msg answerMsg) (Model, tea.Cmd) {
	if msg.asked != m.asked {
		if msg.wrote {
			return m.reportLate(msg.asked, msg.msg)
		}

		letGo(msg.msg)

		return m, nil
	}

	return m.update(msg.msg)
}

// reportLate shows what a write did after its screen was left: on the status
// line only. Marks, pages and questions of the screen now open stay as they
// are, except that what the write did on the client in use is no longer
// marked. A switch of the session to another namespace still happens on the
// cluster it was made for.
func (m Model) reportLate(asked int, msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case doneMsg:
		return m.doneLate(asked, msg), nil

	case errMsg:
		return m.failLate(msg.err), nil

	case failMsg:
		return m.failLate(msg.err), nil

	case describeMsg:
		if msg.ended != "" {
			return m.say(msg.ended), nil
		}

	case refusedEditMsg:
		return m.failLate(msg.err), nil

	case connectionMsg:
		if _, ok := msg.msg.(switchNamespaceMsg); ok {
			return m.update(msg)
		}

	case sayMsg, warnMsg, copyMsg, savedMsg:
		return m.update(msg)
	}

	letGo(msg)

	return m, nil
}

// doneLate reports an action that ended after its screen was left.
func (m Model) doneLate(asked int, msg doneMsg) Model {
	// A mark is a name, and the cluster or region that was left has the same
	// names for other resources.
	if asked >= m.firstAsk {
		m = m.unmark(msg.went)
	}

	if msg.err != nil {
		return m.failLate(msg.err)
	}

	return m.say(msg.said)
}

// failLate shows the error of a write that ended after its screen was left.
// The screen now open answers on its own, and that must not clear it.
func (m Model) failLate(err error) Model {
	if why, ok := m.refusedBecause(err); ok {
		m = m.flashed(why, flashErr)
	} else {
		m = m.fail(err)
	}

	m.flash.ofAction = true

	return m
}

// unmark takes the marks off what an action did. The map is copied: the
// model before may still be read.
func (m Model) unmark(ids []string) Model {
	if len(ids) == 0 {
		return m
	}

	marks := make(map[string]bool, len(m.list.marks))
	for id := range m.list.marks {
		marks[id] = true
	}

	for _, id := range ids {
		delete(marks, id)
	}

	m.list.marks = marks

	// The rows are drawn from the marks only when they are laid out.
	m.layout()

	return m
}
