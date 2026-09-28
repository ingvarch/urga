package ui

import (
	"errors"
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
		}))

	// A job that takes no payload has no place for one.
	forbidden := dispatchTemplate(report, nomad.DispatchForm{Optional: []string{"region"}, Payload: nomad.PayloadForbidden})
	r.Contains(forbidden, "# Optional meta: region.\n")
	r.Contains(forbidden, "# Payload: none, the job takes no payload.\n")
	r.NotContains(forbidden, payloadMarker)
}

// onReport is the job list with the cursor on the parameterized job.
func onReport(t *testing.T, client *fakeClient, editor Editor) Model {
	t.Helper()

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

	client := &fakeClient{jobs: periodicJobs(), dispatchForm: nomad.DispatchForm{Payload: nomad.PayloadForbidden}}
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

	client := &fakeClient{jobs: periodicJobs(), dispatchForm: nomad.DispatchForm{
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
