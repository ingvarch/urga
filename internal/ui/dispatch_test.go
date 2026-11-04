package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

func TestParseDispatch(t *testing.T) {
	r := require.New(t)

	meta, payload, err := parseDispatch("# Dispatch report in production.\n" +
		"\n" +
		"day = monday\n" +
		"region =\n" +
		"note = a = b\n" +
		"--- payload ---\n" +
		"hello\n" +
		"world\n")
	r.NoError(err)

	// A key left empty is not sent; a value keeps what follows the first =.
	r.Equal(map[string]string{"day": "monday", "note": "a = b"}, meta)

	// The payload is the rest, as it is.
	r.Equal([]byte("hello\nworld\n"), payload)
}

func TestParseDispatch_Nothing(t *testing.T) {
	r := require.New(t)

	meta, payload, err := parseDispatch("day =\n--- payload ---\n  \n")
	r.NoError(err)
	r.Nil(meta)
	r.Nil(payload)
}

func TestParseDispatch_ALineThatIsNotAKey(t *testing.T) {
	r := require.New(t)

	_, _, err := parseDispatch("# Dispatch report.\nday = monday\njust words\n")
	r.EqualError(err, "line 3: write key = value")

	_, _, err = parseDispatch(" = monday\n")
	r.EqualError(err, "line 1: no key before =")
}

func TestDispatchTemplate(t *testing.T) {
	r := require.New(t)

	report := nomad.Job{ID: "report", Namespace: "production"}

	r.Equal("# Dispatch report in production.\n"+
		"# Required meta: day. Optional meta: region, zone.\n"+
		"# Payload: required. It is everything below the line \"--- payload ---\", as it is.\n"+
		"# Save a change to dispatch: at least delete this line. Quit without saving to drop it.\n"+
		"day =\n"+
		"region =\n"+
		"zone =\n"+
		"--- payload ---\n",
		dispatchTemplate(report, nomad.DispatchForm{
			Required: []string{"day"}, Optional: []string{"region", "zone"}, Payload: nomad.PayloadRequired,
		}, false))

	// A job that takes no payload has no place for one.
	forbidden := dispatchTemplate(report, nomad.DispatchForm{Optional: []string{"region"}, Payload: nomad.PayloadForbidden}, false)
	r.Contains(forbidden, "# Optional meta: region.\n")
	r.Contains(forbidden, "# Payload: none, the job takes no payload.\n")
	r.NotContains(forbidden, payloadMarker)
}

func TestDispatchTemplate_APriority(t *testing.T) {
	r := require.New(t)

	report := nomad.Job{ID: "report", Namespace: "production"}

	form := nomad.DispatchForm{Required: []string{"day"}, Priority: 50, Payload: nomad.PayloadForbidden}
	template := dispatchTemplate(report, form, true)

	r.Equal("# Priority: empty keeps the job's, 50.\n", strings.Split(template, "\n")[1]+"\n")
	r.Contains(template, "priority =\nday =\n")

	// A job that gives no priority has none to name.
	form.Priority = 0
	r.Equal("# Priority: empty keeps the job's.", strings.Split(dispatchTemplate(report, form, true), "\n")[1])
}

func TestTakePriority(t *testing.T) {
	for name, tc := range map[string]struct {
		meta     map[string]string
		priority int
		rest     map[string]string
	}{
		"a priority and meta": {map[string]string{"priority": "80", "day": "monday"}, 80, map[string]string{"day": "monday"}},
		"no priority":         {map[string]string{"day": "monday"}, 0, map[string]string{"day": "monday"}},
		"only a priority":     {map[string]string{"priority": "80"}, 80, nil},
	} {
		t.Run(name, func(t *testing.T) {
			priority, rest, err := takePriority(tc.meta)
			require.NoError(t, err)
			require.Equal(t, tc.priority, priority)
			require.Equal(t, tc.rest, rest)
		})
	}

	_, _, err := takePriority(map[string]string{"priority": "high"})
	require.EqualError(t, err, `priority: "high" is not a number`)
}

// onReport is the job list with the cursor on the parameterized job.
func onReport(t *testing.T, client *fakeClient, editor Editor) Model {
	t.Helper()
	tempDir(t)

	m := newTestModel(client)
	m.opts.Editor = editor
	m, _ = m.update(jobsMsg(client.jobs))
	m, _ = m.update(key('j'))
	m, _ = m.update(key('j'))

	return m
}

func TestDispatch_TheKey(t *testing.T) {
	r := require.New(t)

	dispatch := hint{Key: "<r>", Description: "Dispatch"}
	runNow := hint{Key: "<r>", Description: "Run Now"}

	m := onReport(t, &fakeClient{jobs: periodicJobs()}, nil)
	r.Contains(m.hints(), dispatch)
	r.NotContains(m.hints(), runNow)

	// A periodic job is run now, not dispatched.
	m, _ = m.update(key('k'))
	r.Contains(m.hints(), runNow)
	r.NotContains(m.hints(), dispatch)
}

func TestDispatch_AJobThatTakesNothingIsAsked(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), changes: newChanges(), dispatchForm: nomad.DispatchForm{Payload: nomad.PayloadForbidden}}
	editor := &fakeEditor{}
	m := onReport(t, client, editor)

	m, cmd := m.update(key('r'))
	m = follow(m, cmd, 4)

	// Nothing to type: no editor, a question.
	r.Empty(editor.opened)
	r.Contains(plain(m.render()), "Really dispatch the job report?")

	m, cmd = m.update(key('y'))
	m = playOut(m, cmd)

	r.Equal([]string{"DispatchJob"}, client.writes)
	r.Equal("report", client.askedID)
	r.Nil(client.dispatchedMeta)
	r.Nil(client.dispatchedPayload)
	r.Contains(plain(m.render()), "Job report dispatched as report/dispatch-1790611292-1fcb1371.")
}

func TestDispatch_ThroughTheEditor(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), changes: newChanges(), dispatchForm: nomad.DispatchForm{
		Required: []string{"day"}, Optional: []string{"region"}, Payload: nomad.PayloadOptional,
	}}
	editor := &fakeEditor{edits: []string{"day = monday\nregion =\n--- payload ---\nhello\n"}}
	m := onReport(t, client, editor)

	m, cmd := m.update(key('r'))
	m = playOut(m, cmd)

	// The file names what the job takes.
	r.NotEmpty(editor.seen)
	r.Contains(editor.seen[0], "day =\nregion =\n--- payload ---\n")

	r.Equal([]string{"DispatchJob"}, client.writes)
	r.Equal("production", client.askedNamespace)
	r.Equal(map[string]string{"day": "monday"}, client.dispatchedMeta)
	r.Equal([]byte("hello\n"), client.dispatchedPayload)
	r.Contains(plain(m.render()), "Job report dispatched as report/dispatch-1790611292-1fcb1371.")
}

func TestDispatch_WithAPriority(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), dispatchForm: nomad.DispatchForm{Required: []string{"day"}, Priority: 50}}
	editor := &fakeEditor{edits: []string{"priority = 80\nday = monday\n"}}
	m := onReport(t, client, editor)
	m.nomadVersion = "1.10.1"

	m, cmd := m.update(key('r'))
	playOut(m, cmd)

	r.NotEmpty(editor.seen)
	r.Contains(editor.seen[0], "priority =\nday =\n")
	r.Equal(80, client.dispatchedPriority)
	r.Equal(map[string]string{"day": "monday"}, client.dispatchedMeta)
}

func TestDispatch_NoPriorityOnAnOlderCluster(t *testing.T) {
	for _, version := range []string{"1.10.0", ""} {
		t.Run(version, func(t *testing.T) {
			r := require.New(t)

			client := &fakeClient{jobs: periodicJobs(), dispatchForm: nomad.DispatchForm{Required: []string{"day"}, Priority: 50}}
			editor := &fakeEditor{edits: []string{"day = monday\n"}}
			m := onReport(t, client, editor)
			m.nomadVersion = version

			m, cmd := m.update(key('r'))
			playOut(m, cmd)

			r.NotEmpty(editor.seen)
			r.NotContains(editor.seen[0], "riority")
			r.Equal(0, client.dispatchedPriority)
		})
	}
}

func TestDispatch_AMetaKeyNamedPriority(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), dispatchForm: nomad.DispatchForm{Optional: []string{"priority"}, Priority: 50}}
	editor := &fakeEditor{edits: []string{"priority = high\n"}}
	m := onReport(t, client, editor)
	m.nomadVersion = "1.10.1"

	m, cmd := m.update(key('r'))
	playOut(m, cmd)

	r.NotEmpty(editor.seen)
	r.NotContains(editor.seen[0], "# Priority:")
	r.Equal(map[string]string{"priority": "high"}, client.dispatchedMeta)
	r.Equal(0, client.dispatchedPriority)
}

func TestDispatch_APriorityThatIsNotANumber(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), dispatchForm: nomad.DispatchForm{Required: []string{"day"}, Priority: 50}}
	editor := &fakeEditor{edits: []string{"priority = high\nday = monday\n"}}
	m := onReport(t, client, editor)
	m.nomadVersion = "1.10.1"

	m, cmd := m.update(key('r'))
	playOut(m, cmd)

	r.Empty(client.writes)
	r.Len(editor.seen, 2)
	r.Contains(editor.seen[1], `# Not saved: priority: "high" is not a number.`)
}

func TestDispatch_ARefusalOpensTheFileAgain(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:         periodicJobs(),
		dispatchForm: nomad.DispatchForm{Required: []string{"day"}, Optional: []string{"region"}},
		refusals:     []error{errors.New("missing required meta keys [day]")},
	}
	editor := &fakeEditor{edits: []string{"region = eu\n", "day = monday\nregion = eu\n"}}
	m := onReport(t, client, editor)

	m, cmd := m.update(key('r'))
	playOut(m, cmd)

	// The edit is not lost: it opens again with the reason above it.
	r.Len(editor.seen, 2)
	r.Contains(editor.seen[1], "# Not saved: missing required meta keys [day].")
	r.Contains(editor.seen[1], "region = eu\n")

	r.Equal([]string{"DispatchJob", "DispatchJob"}, client.writes)
	r.Equal(map[string]string{"day": "monday", "region": "eu"}, client.dispatchedMeta)
}

func TestDispatch_AFileThatDoesNotReadIsNotSent(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), dispatchForm: nomad.DispatchForm{Required: []string{"day"}}}
	editor := &fakeEditor{edits: []string{"day monday\n"}}
	m := onReport(t, client, editor)

	m, cmd := m.update(key('r'))
	playOut(m, cmd)

	r.Empty(client.writes)
	r.Len(editor.seen, 2)
	r.Contains(editor.seen[1], "# Not saved: line 1: write key = value.")
}

func TestDispatch_AFormAnsweredAfterLeavingAsksNothing(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: periodicJobs(), changes: newChanges(), dispatchForm: nomad.DispatchForm{Payload: nomad.PayloadForbidden}}
	m := onReport(t, client, &fakeEditor{})

	m, form := m.update(key('r'))
	m, _ = m.update(enter())
	m = drain(m, form)

	r.Equal(overlayNone, m.overlay)
	r.NotContains(plain(m.render()), "Really dispatch")
}
