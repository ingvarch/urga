package ui

import (
	"go/ast"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// holdTimers makes every timer answer at once with what it would send, for
// the length of the test.
func holdTimers(t *testing.T) {
	t.Helper()

	held := timersHeld.Swap(true)

	t.Cleanup(func() { timersHeld.Store(held) })
}

func TestTimers_StartInOnePlace(t *testing.T) {
	r := require.New(t)

	fset, files := sourceFiles(t)

	for _, file := range files {
		if fset.Position(file.Pos()).Filename == "timer.go" {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "tea" && (sel.Sel.Name == "Tick" || sel.Sel.Name == "Every") {
				r.Fail("a timer is started outside timer.go", "tea.%s in %s", sel.Sel.Name, fset.Position(call.Pos()))
			}

			return true
		})
	}
}

func TestTimers_AHeldTimerAnswersAtOnceWithWhatItWouldSend(t *testing.T) {
	holdTimers(t)

	at := time.Now()

	require.Equal(t, heldTimer{wait: flashFor, msg: flashOverMsg{at: at}}, within(flashTimer(at), 10*time.Millisecond))
}

func TestTimers_ATimerThatIsNotHeldSendsItsMessage(t *testing.T) {
	require.Equal(t, sayMsg("done"), startTimer(time.Millisecond, sayMsg("done"))())
}

func TestPlayOut_LeavesATimerHoweverShortItIs(t *testing.T) {
	m := playOut(loadedModel(t), startTimer(time.Millisecond, sayMsg("a timer went off")))

	require.NotContains(t, plain(m.render()), "a timer went off")
	require.False(t, timersHeld.Load())
}
