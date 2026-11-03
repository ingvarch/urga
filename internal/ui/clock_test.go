package ui

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// fixedTime is a moment far from the wall clock, so a screen that reads the
// wall clock shows it.
func fixedTime() time.Time { return time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC) }

func TestRender_TheStatusLineCountsFromTheTimeOfTheModel(t *testing.T) {
	r := require.New(t)
	then := fixedTime()

	m := withToken(nomad.Token{Name: "deploy-bot", Type: "client", Expires: then.Add(12 * time.Minute)}, 120)
	m = m.say("Job web stopped.")
	m.flash.at = then.Add(-time.Second)
	m.now = then

	out := plain(m.render())

	r.Contains(out, "Job web stopped.")
	r.Contains(out, "expires in 12 minutes")
	r.Equal(out, plain(m.render()))
}

func TestRender_APageCountsFromTheTimeOfTheModel(t *testing.T) {
	then := fixedTime()

	t.Run("the rows of a list", func(t *testing.T) {
		r := require.New(t)

		m, _ := onAllocations(t)

		allocs := twoAllocs()
		allocs[0].Created = then.Add(-30 * time.Minute)
		allocs[0].Tasks[0].LastRestart = then.Add(-10 * time.Minute)

		m, _ = m.update(allocsMsg(allocs))
		m.now = then
		m.layout()

		r.Equal("30m", cellsOf(m.list.table, 12)[0])
		r.Equal(colorDead, m.list.table.rows[0].color)
	})

	t.Run("the title of the launches", func(t *testing.T) {
		r := require.New(t)

		m := onLaunches(t, &fakeClient{jobs: periodicJobs(), next: then.Add(4*time.Hour + 30*time.Minute), changes: newChanges()}, 1)
		m.now = then

		r.Equal("Launches (Job: backup, next in 4h) [2]", m.title())
	})

	t.Run("the checks of an allocation", func(t *testing.T) {
		r := require.New(t)

		checks := []nomad.Check{{Service: "web", Name: "http", Status: "success", Time: then.Add(-5 * time.Minute)}}

		r.Contains(plain(strings.Join(allocPanel(nomad.Alloc{ID: "a"}, checks, then, 80, 20), "\n")), "5m ago")
	})
}

// clockReaders are the functions that may read the wall clock: update, which
// keeps it on the model, and what runs in update or in a command, where the
// time is part of what changes. Everything else counts from the model.
var clockReaders = map[string]bool{
	// update and what it calls when a message comes in
	"Model.update":         true,
	"Model.flashed":        true,
	"allocationsPage.take": true,
	"logState.take":        true,
	"jobLogsPage.lines":    true,
	"jobLogsPage.chunk":    true,
	"saveName":             true,

	// commands
	"readPart":           true,
	"describeEvaluation": true,
	"placementOf":        true,
}

func TestClock_IsReadOnlyByUpdateAndItsCommands(t *testing.T) {
	fset, files := sourceFiles(t)

	for _, file := range files {
		for _, decl := range file.Decls {
			name := funcName(decl)
			if clockReaders[name] {
				continue
			}

			ast.Inspect(decl, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && readsClock(sel) {
					require.Fail(t, "the wall clock is read outside update", "time.%s in %s at %s", sel.Sel.Name, name, fset.Position(sel.Pos()))
				}

				return true
			})
		}
	}
}

// funcName is the name of a function, after its receiver: Model.update.
func funcName(decl ast.Decl) string {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok {
		return ""
	}

	if fn.Recv == nil {
		return fn.Name.Name
	}

	return types.ExprString(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func readsClock(sel *ast.SelectorExpr) bool {
	pkg, ok := sel.X.(*ast.Ident)

	return ok && pkg.Name == "time" && slices.Contains([]string{"Now", "Since", "Until"}, sel.Sel.Name)
}
