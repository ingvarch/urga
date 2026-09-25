package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// update hands a message to whoever it is for: the page on top first, then
// the root, by what the message is about.
func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	// An answer the open page asked for is the page's to keep.
	if next, out, ok := m.screen.page.take(msg, m.env()); ok {
		return m.took(next, out)
	}

	for _, route := range []func(Model, tea.Msg) (Model, tea.Cmd, bool){
		Model.fromTerminal,
		Model.fromPage,
		Model.fromAction,
		Model.fromSession,
		Model.keepingUp,
	} {
		if next, cmd, ok := route(m, msg); ok {
			return next, cmd
		}
	}

	return m, nil
}

// taken is what a route did with a message it took.
func taken(m Model, cmd tea.Cmd) (Model, tea.Cmd, bool) { return m, cmd, true }

// fromTerminal takes the size of the window, a key and a paste.
func (m Model) fromTerminal(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg), nil, true

	case tea.KeyPressMsg:
		return taken(m.handleKey(msg))

	case tea.PasteMsg:
		return taken(m.paste(msg.Content))
	}

	return m, nil, false
}

// fromPage takes what a page asks of the screen it is on.
func (m Model) fromPage(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case backMsg:
		return taken(m.back())

	case openMsg:
		return taken(m.push(screen{page: msg.page}))

	case sayMsg:
		return m.say(string(msg)), nil, true

	case warnMsg:
		return m.warn(string(msg)), nil, true

	case failMsg:
		return m.fail(msg.err), nil, true

	case forgetMsg:
		return m.forget(), nil, true

	case wrapMsg:
		return taken(wrapLines(m))

	case followMsg:
		return taken(toggleAutoscroll(m))

	case timesMsg:
		return taken(showTimes(m))

	case reopenMsg:
		return taken(m.openStream())

	case saveMsg:
		return taken(saveScreen(m))

	case askMsg:
		return taken(m.ask(msg.question, msg.apply))

	case requestMsg:
		return m, askedFor(m.asked, tea.Cmd(msg)), true

	case markMsg:
		return taken(mark(m))

	case markAllMsg:
		return taken(markAll(m))
	}

	return m, nil, false
}

// fromAction takes what a key started, and how it ended.
func (m Model) fromAction(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case scaleMsg:
		return taken(m.askScale(groupRef(msg)))

	case signalMsg:
		return taken(m.askForSignal(taskRef(msg)))

	case shellMsg:
		return taken(m.openShell(shellCommand(msg)))

	case shellDoneMsg:
		return m.shellDone(msg), nil, true

	case describeMsg:
		return taken(m.showDescribe(msg))

	case editFileMsg:
		return taken(m.startEdit(msg))

	case editedMsg:
		return taken(m.finishEdit(msg))

	case refusedEditMsg:
		return m, m.reopenEdit(msg), true

	case savedMsg:
		return m.say(fmt.Sprintf("Saved to %s.", msg.path)), nil, true

	case doneMsg:
		return taken(m.done(msg))

	case logStreamMsg, fileMsg, jobLogOpenedMsg:
		// A stream no page took: the page it was opened for moved on.
		letGo(msg)

		return m, nil, true
	}

	return m, nil, false
}

// fromSession takes where the session is asked to look, and what it keeps
// knowing of the cluster wherever it looks.
func (m Model) fromSession(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case switchRegionMsg:
		return taken(m.switchRegion(string(msg)))

	case narrowMsg:
		return taken(m.narrow(string(msg), Model.back))

	case switchClusterMsg:
		return taken(m.switchCluster(string(msg)))

	case connectedMsg:
		return taken(m.connected(Connection(msg)))

	case connectionMsg:
		return taken(m.takeConnection(msg))

	case tokenMsg:
		return m.keepToken(msg), nil, true

	case agentMsg:
		return m.keepAgent(msg), nil, true

	case namespacesMsg:
		return taken(m.keepNamespaces(msg))

	case regionsMsg:
		return taken(m.keepRegions(msg))

	case datacentersMsg:
		return taken(m.keepDatacenters(msg))

	case newerReleaseMsg:
		return taken(m.keepRelease(msg))

	case checkReleaseMsg:
		return m, m.checkRelease(), true
	}

	return m, nil, false
}

// keepingUp takes the answers to what the screen asked, the timers, the
// readings and the event stream that keep it up to date.
func (m Model) keepingUp(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case answerMsg:
		return taken(m.takeAnswer(msg))

	case errMsg:
		return taken(m.takeError(msg.err))

	case pollMsg:
		return taken(m.poll())

	case usageMsg:
		return taken(m.keepClusterUsage(msg))

	case pollUsageMsg:
		return taken(m.readClusterUsage())

	case rowUsageMsg:
		return taken(m.keepRowUsage(msg))

	case pollUsageRow:
		return taken(m.readRows())

	case watchingMsg:
		return taken(m.startWatch(msg))

	case changeMsg:
		return taken(m.keepChange(msg))

	case settleMsg:
		return taken(m.settled())

	case watchEndedMsg:
		return m.watchEnded(msg), nil, true

	case flashOverMsg:
		return m.clearFlash(msg), nil, true
	}

	return m, nil, false
}
