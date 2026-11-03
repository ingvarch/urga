package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func ctrlKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl}
}

// answerYes walks the cursor to the confirm button and presses it, the way a
// person answers the question. The cursor starts on cancel.
func answerYes(m Model) (Model, tea.Cmd) {
	m, _ = m.update(tea.KeyPressMsg{Code: tea.KeyRight})

	return m.update(enter())
}

func TestAction_StopAJobAsks(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('s'))

	// The cluster is not touched before the question is answered.
	r.Equal(overlayConfirm, m.overlay)
	r.Contains(plain(m.render()), "stop the job web")
	r.Zero(client.stopped)

	m, cmd := answerYes(m)
	r.Equal(overlayNone, m.overlay)

	m = drain(m, cmd)
	r.Equal(1, client.stopped)
	r.Equal("production", client.askedNamespace)

	// The result shows on one line, not in a window that has to be
	// closed.
	r.Contains(plain(m.render()), "Stopped the job web")
}

func TestAction_CancelLeavesTheClusterAlone(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('s'))
	m, cmd := m.update(escape())

	r.Equal(overlayNone, m.overlay)
	r.Nil(cmd)
	r.Zero(client.stopped)
}

func TestAction_StartADeadJob(t *testing.T) {
	r := require.New(t)

	jobs := []nomad.Job{{ID: "cron", Name: "cron", Namespace: "production", Type: "batch", Status: "dead"}}
	client := &fakeClient{jobs: jobs}

	m := newTestModel(client)
	m, _ = m.update(jobsMsg(jobs))

	m, _ = m.update(ctrlKey('s'))

	// A job that is already dead is started, not stopped.
	r.Contains(plain(m.render()), "start the job cron")

	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(1, client.started)
}

func TestAction_RestartAnAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	m, _ = m.update(key('r'))
	r.Contains(plain(m.render()), "restart the allocation af1f37df")

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Equal(1, client.restarted)
	r.Contains(plain(m.render()), "Restarted the allocation af1f37df")
}

func TestAction_RestartEveryTaskOfAnAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	m, _ = m.update(ctrlKey('r'))
	r.Contains(plain(m.render()), "Really restart every task of the allocation af1f37df, prestart and sidecar tasks too?")

	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Equal(1, client.restartedAll)
	r.Equal(0, client.restarted)
	r.Contains(plain(m.render()), "Restarted every task of the allocation af1f37df.")
}

func TestAction_RestartEveryTaskOfTheMarkedAllocations(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	m, _ = m.update(ctrlKey('a'))
	m, _ = m.update(ctrlKey('r'))
	r.Contains(plain(m.render()), "Really restart every task of 2 allocations, prestart and sidecar tasks too?")

	m, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(2, client.restartedAll)
}

func TestAction_StopAnAllocation(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))
	m, _ = m.update(enter())
	m, _ = m.update(allocsMsg(twoAllocs()))

	m, _ = m.update(ctrlKey('k'))
	_, cmd := answerYes(m)
	drain(m, cmd)

	r.Equal(1, client.stoppedAllocs)
}

func TestAction_RevertAJob(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), plan: nomad.Plan{To: 3, Version: 4, Index: 42}}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	// Going back is submitting an older version, and it is planned first
	// like any other submit: the version before the one that runs.
	m, cmd := m.update(key('u'))
	m = drain(m, cmd)

	r.Nil(client.plannedTo)
	r.IsType(planPage{}, m.screen.page)
	r.Contains(plain(m.render()), "Revert (Job: web, Version: 3)")

	// A revert reads no file, so there is no read to have changed since.
	r.NotContains(plain(m.render()), "Changed after the file was read")

	m, cmd = m.update(key('y'))
	m = drain(m, cmd)

	// From the version it was planned at: a job that changed since is not
	// reverted.
	r.Equal(uint64(3), client.revertedTo)
	r.Equal(uint64(4), client.revertedFrom)
	r.Contains(plain(m.render()), "Job web reverted to version 3")
}

func TestAction_FailureIsSaid(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), actionErr: errors.New("permission denied")}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('s'))
	m, cmd := answerYes(m)
	m = drain(m, cmd)

	r.Contains(plain(m.render()), "permission denied")
}

func TestAction_ConfirmTakesTheKeys(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('s'))

	// While the question is shown, the list does not move and q does not
	// quit.
	m, cmd := m.update(key('q'))
	r.Nil(cmd)
	r.Equal(overlayConfirm, m.overlay)

	m, _ = m.update(key('j'))
	r.Zero(m.list.table.cursor)
}

const longQuestion = "Really collect the garbage of the client web-01? Logs of its ended allocations are deleted."

func askAt80(t *testing.T, question string) string {
	t.Helper()

	m := newTestModel(&fakeClient{})
	m, _ = m.update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m, _ = m.ask(question, nil)

	return dialogRows(plain(m.render()))
}

// dialogRows keeps the lines of the screen that belong to the question box,
// the header above it has its own cut-off text.
func dialogRows(screen string) string {
	var rows []string

	for _, line := range strings.Split(screen, "\n") {
		if strings.Count(line, "│")+strings.Count(line, "╭")+strings.Count(line, "╰") >= 4 {
			rows = append(rows, line)
		}
	}

	return strings.Join(rows, "\n")
}

func TestConfirmView_ALongQuestionIsShownWhole(t *testing.T) {
	r := require.New(t)

	screen := askAt80(t, longQuestion)

	r.NotContains(screen, "…")

	for _, word := range strings.Fields(longQuestion) {
		r.Contains(screen, word)
	}

	last := strings.Index(screen, "deleted.")
	r.Positive(last)
	r.Greater(strings.Index(screen, "cancel"), last)
}

func TestConfirmView_AShortQuestionIsOneLine(t *testing.T) {
	r := require.New(t)

	view := plain(confirmModel{question: "Really stop the job web?"}.view(74))

	// Border, blank, question, blank, buttons, blank, border.
	r.Len(strings.Split(view, "\n"), 7)
	r.Contains(view, "Really stop the job web?")
}

func TestConfirmView_AnUnbrokenWordIsBrokenInside(t *testing.T) {
	r := require.New(t)

	path := "nomad/jobs/" + strings.Repeat("web_frontend/", 7) + "config"
	screen := askAt80(t, "Really delete "+path+"?")

	r.NotContains(screen, "…")

	flat := strings.NewReplacer("\n", "", " ", "", "│", "").Replace(screen)
	r.Contains(flat, path+"?")
}

func TestConfirmView_TheQuestionKeepsAMarginFromTheBorder(t *testing.T) {
	r := require.New(t)

	rows := strings.Split(plain(confirmModel{question: longQuestion}.view(72)), "\n")
	r.Greater(len(rows), 2)

	pad := strings.Repeat(" ", modalPadX)

	for _, row := range rows[1 : len(rows)-1] {
		r.True(strings.HasPrefix(row, "│"+pad), row)
		r.True(strings.HasSuffix(row, pad+"│"), row)
	}
}

func TestAction_AnActionThatEndsAfterItsScreenIsLeftKeepsTheMarksThere(t *testing.T) {
	r := require.New(t)

	// A failed stop keeps the job marked: on its own screen, not on this one.
	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs(), actionErr: errors.New("no luck")}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(ctrlKey('s'))
	m, stop := answerYes(m)

	m, cmd := m.update(enter())
	m = drain(m, cmd)
	r.IsType(allocationsPage{}, m.screen.page)

	m, _ = m.update(space())
	r.Equal(map[string]bool{twoAllocs()[0].ID: true}, m.list.marks)

	m = drain(m, stop)

	r.IsType(allocationsPage{}, m.screen.page)
	r.Equal(map[string]bool{twoAllocs()[0].ID: true}, m.list.marks)
	r.Contains(plain(m.render()), "no luck")
}

func TestAction_ALateAnswerOfAWriteOnlyReportsWhatItDid(t *testing.T) {
	failure := errors.New("no luck")

	for name, tc := range map[string]struct {
		msg  tea.Msg
		want string
	}{
		"done":        {doneMsg{said: "Stopped the job web."}, "Stopped the job web."},
		"done failed": {doneMsg{err: failure}, "no luck"},
		"error":       {errMsg{err: failure}, "no luck"},
		"saved":       {savedMsg{path: "/tmp/x"}, "Saved to /tmp/x."},
		"say":         {sayMsg("Did it."), "Did it."},
		"warn":        {warnMsg("Careful."), "Careful."},
		"fail":        {failMsg{err: failure}, "no luck"},
		"copy":        {copyMsg{field: "token", value: "v"}, "Copied token."},
		"refused":     {refusedEditMsg{err: failure}, "no luck"},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			m, _ := onAllocations(t)
			left := m.asked - 1

			m, _ = m.update(space())
			marks := m.list.marks
			r.NotEmpty(marks)

			m, _ = m.update(answerMsg{asked: left, wrote: true, msg: tc.msg})

			r.Contains(plain(m.render()), tc.want)
			r.IsType(allocationsPage{}, m.screen.page)
			r.Equal(marks, m.list.marks)
			r.Equal(overlayNone, m.overlay)
		})
	}
}

func TestAction_ALateAnswerOfAWriteChangesNothingElse(t *testing.T) {
	for name, msg := range map[string]tea.Msg{
		"open":                         openMsg{page: describePage{}},
		"describe":                     describeMsg{label: "x", content: "y"},
		"ask":                          askMsg{question: "Really?", apply: func() tea.Msg { return nil }},
		"namespace":                    switchNamespaceMsg("other"),
		"namespace of another cluster": connectionMsg{connection: 7, msg: switchNamespaceMsg("other")},
		"edit file":                    editFileMsg{path: "/tmp/x"},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			m, _ := onAllocations(t)
			m.opts.Editor = &fakeEditor{}
			left := m.asked - 1

			m, cmd := m.update(answerMsg{asked: left, wrote: true, msg: msg})

			r.Nil(cmd)
			r.Empty(m.editing.path)
			r.IsType(allocationsPage{}, m.screen.page)
			r.Equal("production", m.namespace)
			r.Equal(overlayNone, m.overlay)
		})
	}
}

func TestAction_ALateFailureOfAWriteStaysUntilItsTimeIsUp(t *testing.T) {
	for name, msg := range map[string]tea.Msg{
		"error": errMsg{err: errors.New("no luck")},
		"fail":  failMsg{err: errors.New("no luck")},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)

			m, _ := onAllocations(t)
			left := m.asked - 1

			m, _ = m.update(answerMsg{asked: left, wrote: true, msg: msg})

			// The screen now open answers: that is not the end of the failure.
			m, _ = m.update(answerMsg{asked: m.asked, msg: allocsMsg(twoAllocs())})

			r.Contains(plain(m.render()), "no luck")
		})
	}
}

func TestAction_ALateFailureTheTokenExplainsSaysWhy(t *testing.T) {
	refused := forbidden(t)

	for name, msg := range map[string]tea.Msg{
		"done":    doneMsg{err: refused},
		"error":   errMsg{err: refused},
		"fail":    failMsg{err: refused},
		"refused": refusedEditMsg{err: refused},
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := onAllocations(t)
			m, _ = m.update(tokenMsg(deployBot))
			left := m.asked - 1

			m, _ = m.update(answerMsg{asked: left, wrote: true, msg: msg})

			require.Contains(t, plain(statusLine(m)), "Permission denied: deploy-bot may not do this")
		})
	}
}

func TestAction_AnActionThatEndsOnTheSameListAfterItIsEnteredAgainUnmarksWhatItDid(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs()}
	m := newTestModel(client)
	m, _ = m.update(jobsMsg(twoJobs()))

	m, _ = m.update(space())
	m, _ = m.update(key('j'))
	m, _ = m.update(space())
	r.Len(m.list.marks, 2)

	m, _ = m.update(ctrlKey('s'))
	m, stop := answerYes(m)

	// The list is entered again before the answer comes.
	m = typeCommand(m, "jobs")

	m = drain(m, stop)

	r.Equal(1, client.stopped)
	r.Equal(1, client.started)
	r.Empty(m.list.marks)

	// What is drawn is what the next key acts on.
	r.Empty(markedRows(m))
	r.False(cursorOnMark(m))
}
