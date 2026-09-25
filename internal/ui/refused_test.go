package ui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// reasonFor is the top of the file a refused edit opens again with.
func reasonFor(err string) string {
	return "# Not saved: " + err + ".\n" +
		"# To save, change the file: delete these lines at least.\n" +
		"# To drop your edit, quit without saving.\n"
}

func TestRefusedEdit_AJobThatCannotBePlannedOpensAgain(t *testing.T) {
	r := require.New(t)

	refusal := "input.hcl:2,10-11: Invalid expression"
	broken := "# The web frontend.\njob \"web\" {\n  type = \n}"
	fixed := "# The web frontend.\njob \"web\" {\n  type = \"batch\"\n}"

	client := &fakeClient{jobs: twoJobs(), spec: nomad.JobSource{Source: "job \"web\" {}"}, plan: changedEnv(),
		refusals: []error{errors.New(refusal)}}
	m, editor := editModel(t, client)

	// Fixed the second time, with the reason left where it was.
	editor.edits = []string{broken, reasonFor(refusal) + fixed}

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 12)

	// The comments of the job are its own and stay.
	r.Len(editor.seen, 2)
	r.Equal(reasonFor(refusal)+broken, editor.seen[1])

	// The reason is not part of the job: what is planned is the fixed file.
	r.Equal(2, client.planCalls)
	r.Equal(fixed, client.plannedSource)
	r.Equal(screenPlan, m.screen.kind)
}

func TestRefusedEdit_ANamespaceOpensAgainUntilDropped(t *testing.T) {
	r := require.New(t)

	first, second := "the namespace is not valid JSON: unexpected end of JSON input", "namespace name is not valid"
	client := &fakeClient{refusals: []error{errors.New(first), errors.New(second)}}

	// JSON has no comments: the reason is taken off before it is sent.
	editor := &fakeEditor{edits: []string{`{"Name": "prod uction"`, reasonFor(first) + `{"Name": "prod uction"}`}}
	m := onNamespaces(t, client, editor)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 16)

	r.Equal(2, client.submittedNamespaces)
	r.Equal(`{"Name": "prod uction"}`, client.submittedSource)

	// A reason takes the place of the one before, and a file left as it
	// came back is dropped.
	r.Len(editor.seen, 3)
	r.Equal(reasonFor(second)+`{"Name": "prod uction"}`, editor.seen[2])
	r.Contains(plain(m.render()), "unchanged")
}

func TestRefusedEdit_MetadataIsSentAgainWithoutTheReason(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{refusals: []error{errors.New("no path to node")}}

	// The second time, only the reason is deleted.
	editor := &fakeEditor{edits: []string{`{"owner": "ingvar"}`, `{"owner": "ingvar"}`}}
	m := onMeta(t, client, editor)

	m, cmd := m.update(key('e'))
	m = follow(m, cmd, 12)

	r.Equal(reasonFor("no path to node")+`{"owner": "ingvar"}`, editor.seen[1])
	r.Equal([]string{"SubmitNodeMeta", "SubmitNodeMeta"}, client.writes)
	r.Equal(`{"owner": "ingvar"}`, client.metaSubmitted)
	r.Contains(plain(m.render()), "submitted")
}
