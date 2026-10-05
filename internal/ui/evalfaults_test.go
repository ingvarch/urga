package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// evalFaultsIDs are the IDs of the evaluations the page keeps.
func evalFaultsIDs(p evaluationsPage) []string {
	ids := []string{}
	for _, eval := range p.visible(env{}) {
		ids = append(ids, eval.ID)
	}

	return ids
}

func TestEvalFaults_Blocked(t *testing.T) {
	evals := []nomad.Evaluation{
		{ID: "a", Status: "complete"},
		{ID: "b", Status: "blocked"},
		{ID: "c", Status: "failed"},
		{ID: "d", Status: "blocked"},
	}

	got := evalFaultsIDs(evaluationsPage{evaluations: evals, fault: faultBlocked})

	require.Equal(t, []string{"b", "d"}, got)
}

func TestEvalFaults_FailedOnlyWhenNewestOfItsJob(t *testing.T) {
	now := time.Now()
	// One job lists its newest first, another last: the newest is found by
	// Created, not by position.
	evals := []nomad.Evaluation{
		{ID: "web-2", JobID: "web", Namespace: "production", Status: "complete", Created: now.Add(-time.Hour)},
		{ID: "web-1", JobID: "web", Namespace: "production", Status: "failed", Created: now.Add(-2 * time.Hour)},
		{ID: "api-1", JobID: "api", Namespace: "production", Status: "complete", Created: now.Add(-2 * time.Hour)},
		{ID: "api-2", JobID: "api", Namespace: "production", Status: "failed", Created: now.Add(-time.Hour)},
		{ID: "db-1", JobID: "db", Namespace: "production", Status: "failed", Created: now.Add(-time.Hour)},
		{ID: "web-s", JobID: "web", Namespace: "staging", Status: "failed", Created: now.Add(-2 * time.Hour)},
	}

	got := evalFaultsIDs(evaluationsPage{evaluations: evals, fault: faultFailed})

	// The same job ID in another namespace is another job.
	require.Equal(t, []string{"api-2", "db-1", "web-s"}, got)
}

func TestEvalFaults_EnterOpensTheOneUnderTheCursor(t *testing.T) {
	r := require.New(t)

	evals := []nomad.Evaluation{
		{ID: "eval-1", JobID: "web", Namespace: "production", Status: "complete"},
		{ID: "eval-2", JobID: "api", Namespace: "production", Status: "blocked"},
	}
	client := &fakeClient{evaluations: evals}

	m := newTestModel(client)
	m, _ = m.update(openMsg{evaluationsPage{evaluations: evals, fault: faultBlocked}})

	m, cmd := m.update(enter())
	drain(m, cmd)

	r.Equal("eval-2", client.askedID)
}

func TestEvalFaults_TitleSaysWhatTheListIsNarrowedTo(t *testing.T) {
	r := require.New(t)

	evals := []nomad.Evaluation{
		{ID: "a", Status: "complete"},
		{ID: "b", Status: "blocked"},
	}

	m := newTestModel(&fakeClient{})
	m, _ = m.update(openMsg{evaluationsPage{evaluations: evals, fault: faultBlocked}})

	r.Equal("Evaluations (production, blocked) [1]", m.title())
}
