package ui

import (
	"go/ast"
	"go/token"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// waitsOnChannels are the functions that may wait on a channel: the one
// place a command waits on a stream, and the shell's watch of the terminal
// size, which runs in the shell's own goroutine and not in a command.
var waitsOnChannels = map[string]bool{"waitOn": true, "watchSize": true, "sendSize": true}

func TestStreams_AreWaitedOnInOnePlace(t *testing.T) {
	fset, files := sourceFiles(t)

	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || waitsOnChannels[fn.Name.Name] {
				continue
			}

			for _, at := range channelWaits(fn.Body) {
				t.Errorf("%s waits on a channel outside waitOn: %s", fn.Name.Name, fset.Position(at))
			}
		}
	}
}

// channelWaits are the receives in body that can block: every receive but
// the cases of a select that has a default. A range over a channel waits too,
// but finding one needs the types; playOut names such a command when it
// does not answer.
func channelWaits(body *ast.BlockStmt) []token.Pos {
	nonBlocking := map[ast.Node]bool{}

	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectStmt)
		if !ok || !hasDefault(sel) {
			return true
		}

		for _, stmt := range sel.Body.List {
			if comm := stmt.(*ast.CommClause).Comm; comm != nil {
				ast.Inspect(comm, func(n ast.Node) bool {
					if recv, ok := n.(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
						nonBlocking[recv] = true
					}

					return true
				})
			}
		}

		return true
	})

	var waits []token.Pos

	ast.Inspect(body, func(n ast.Node) bool {
		if recv, ok := n.(*ast.UnaryExpr); ok && recv.Op == token.ARROW && !nonBlocking[recv] {
			waits = append(waits, recv.Pos())
		}

		return true
	})

	return waits
}

func hasDefault(sel *ast.SelectStmt) bool {
	for _, stmt := range sel.Body.List {
		if stmt.(*ast.CommClause).Comm == nil {
			return true
		}
	}

	return false
}

// holdStreams makes every stream wait answer at once, for the length of the
// test.
func holdStreams(t *testing.T) {
	t.Helper()

	held := streamsHeld.Swap(true)

	t.Cleanup(func() { streamsHeld.Store(held) })
}

func TestStreams_AHeldWaitAnswersWithWhatCameOrAtOnce(t *testing.T) {
	r := require.New(t)

	holdStreams(t)

	lines := make(chan string, 1)
	lines <- "the task says something\n"
	stream := &nomad.LogStream{Lines: lines, Err: make(chan error)}

	msg, ok := within(waitForStream(stream), time.Second).(logLineMsg)
	r.True(ok)
	r.Equal("the task says something\n", msg.text)

	// Nothing more came: the wait answers that it would wait.
	r.Equal(heldStream{}, within(waitForStream(stream), time.Second))
}

func TestPlayOut_LeavesAStreamThatSentNothing(t *testing.T) {
	m := playOut(loadedModel(t), waitForStream(&nomad.LogStream{Lines: make(chan string)}))

	require.Contains(t, plain(m.render()), "web")
	require.False(t, streamsHeld.Load())
}
