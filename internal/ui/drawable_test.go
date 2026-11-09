package ui

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// hostile is what a cluster or a task can put in a name, a path, a spec or
// an error: sequences a terminal acts on, between two letters of text.
const hostile = "x\x1bP+q544e\x1b\\\x1b_Gf=100;AAAA\x1b\\\x1b]52;c;ZWNobw==\x07\x1b[2J\ry"

var colourCode = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// requireOnlyText fails when the screen holds more than text, line breaks
// and colours.
func requireOnlyText(t *testing.T, screen string) {
	t.Helper()

	for _, r := range colourCode.ReplaceAllString(screen, "") {
		if r != '\n' && unicode.IsControl(r) {
			t.Fatalf("the screen holds %U:\n%q", r, screen)
		}
	}
}

func TestDrawn_WhatTheClusterWroteIsOnlyText(t *testing.T) {
	cases := []struct {
		name string
		open func(t *testing.T) Model
	}{
		{"the description of a node pool", func(t *testing.T) Model {
			m, _ := sessionModel(t, &fakeClient{nodePools: []nomad.NodePool{{Name: "pool", Description: hostile}}})

			return typeCommand(m, "nodepools")
		}},
		{"the path of a file in the title", func(t *testing.T) Model {
			m, _ := sessionModel(t, &fakeClient{})
			m, _ = m.update(openMsg{filesPage{namespace: "production", allocID: "abcdef1234", path: "/web/" + hostile}})

			return m
		}},
		{"a job spec", func(t *testing.T) Model {
			m, _ := sessionModel(t, &fakeClient{})
			m, _ = m.update(describeMsg{label: "Job spec: web", content: "job \"web\" {\n  # " + hostile + "\n}"})

			return m
		}},
		{"an error on the status line", func(t *testing.T) Model {
			m, _ := sessionModel(t, &fakeClient{})

			return m.fail(errors.New("refused: " + hostile))
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			screen := c.open(t).render()

			require.Contains(t, colourCode.ReplaceAllString(screen, ""), "x", "the text around stays")
			requireOnlyText(t, screen)
		})
	}
}

func TestTable_ACellIsOneLine(t *testing.T) {
	r := require.New(t)

	table := newTableModel([]string{"Key", "Value"})
	table.setSize(40, 10)
	table.show([]tableRow{{cells: []string{"key", "two\nlines\r"}}}, newSortState())

	lines := strings.Split(table.view(), "\n")

	r.Len(lines, 2, "the titles and one row")
	r.Contains(lines[1], "two lines")
}
