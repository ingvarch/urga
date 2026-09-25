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
	return len(m.bar(m.width - 2*screenPadX - 2))
}

// took stores what the open page made of an answer, and applies what it
// returns. The work it starts belongs to the ask of the page and ends with
// it. An answer to the ask marks the screen as answered; a reading from a
// timer of the page does not.
func (m Model) took(next page, out outcome) (Model, tea.Cmd) {
	out.cmd = askedFor(m.asked, out.cmd)

	if out.reading {
		m.screen.page = next

		return m.apply(out)
	}

	m, poll := m.applyWhen(true, func(m *Model) { m.screen.page = next })
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
// the press is one.
func (m Model) withheldKey(press string) (keyHint, bool) {
	for _, k := range m.pageKeys() {
		if k.press == press && m.withheld(k) {
			return k, true
		}
	}

	return keyHint{}, false
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
	return m.screen.page.title(m.env(), len(m.list.table.rows))
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
	msg   tea.Msg
}

// askedFor labels every message a command returns, the commands of a batch
// included.
func askedFor(asked int, cmd tea.Cmd) tea.Cmd {
	return labelled(cmd, func(msg tea.Msg) tea.Msg { return answerMsg{asked: asked, msg: msg} })
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
	switch opened := msg.(type) {
	case logStreamMsg:
		opened.stream.Close()
	case fileMsg:
		opened.stream.Close()
	case jobLogOpenedMsg:
		if opened.stream != nil {
			opened.stream.Close()
		}
	}
}

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

	// A stream is opened again: the command that read it before belonged to
	// an ask that is over.
	m, stream := m.openStream()

	m.layout()

	// The watch of the screen that was left is stopped: the new one watches
	// what it shows, if the cluster can stream.
	m.watch = m.watch.end()

	return m, tea.Batch(m.fetch(), stream, m.watchScreen())
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
		letGo(msg.msg)

		return m, nil
	}

	return m.update(msg.msg)
}
