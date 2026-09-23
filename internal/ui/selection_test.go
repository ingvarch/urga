package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// everyActionKey is every key that acts on the row under the cursor.
func everyActionKey() []tea.KeyPressMsg {
	return []tea.KeyPressMsg{
		enter(),
		key('d'), key('h'), key('e'), key('t'), key('s'), key('u'),
		key('i'), key('p'), key('f'), key('r'),
		ctrlKey('s'), ctrlKey('k'), ctrlKey('d'), ctrlKey('e'),
	}
}

func TestSelection_ActionsOnAnEmptyList(t *testing.T) {
	r := require.New(t)

	screens := []string{"jobs", "deployments", "namespaces", "services", "evaluations", "nodes", "variables", "nodepools"}

	for _, name := range screens {
		m := newTestModel(&fakeClient{})
		m, _ = m.update(key(':'))
		m = typeIn(m, name)
		m, _ = m.update(enter())

		// A key that acts on a row, with no rows to act on, does nothing at
		// all.
		for _, k := range everyActionKey() {
			r.NotPanics(func() { m, _ = m.update(k) }, name+" "+k.String())
		}
	}
}

func TestSelection_ActionsAfterTheListShrinks(t *testing.T) {
	r := require.New(t)

	jobs := manyJobs(20)

	m := newTestModel(&fakeClient{jobs: jobs})
	m, _ = m.update(jobsMsg(jobs))
	m, _ = m.update(key('G'))

	// The cursor is at the end, then the cluster answers with one job.
	m, _ = m.update(jobsMsg(jobs[:1]))

	for _, k := range everyActionKey() {
		r.NotPanics(func() { m, _ = m.update(k) }, k.String())
	}
}
