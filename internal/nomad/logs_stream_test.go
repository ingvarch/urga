package nomad

import (
	"testing"
	"time"

	"github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/require"
)

func neverCut(*api.StreamFrame) bool { return false }

// ended says the channel is closed within a short time.
func ended(lines <-chan string) bool {
	deadline := time.After(500 * time.Millisecond)

	for {
		select {
		case _, ok := <-lines:
			if !ok {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func TestStreamLines_EndWhenCancelledAndFramesStayOpen(t *testing.T) {
	frames := make(chan *api.StreamFrame)
	cancel := make(chan struct{})

	lines := streamLines(frames, neverCut, cancel)

	close(cancel)

	require.True(t, ended(lines))
}

func TestStreamLines_EndWhenCancelledOnNoFrames(t *testing.T) {
	cancel := make(chan struct{})

	lines := streamLines(nil, neverCut, cancel)

	close(cancel)

	require.True(t, ended(lines))
}

func TestStreamLines_UnblockAProducerWaitingOnFullFrames(t *testing.T) {
	frames := make(chan *api.StreamFrame, 10)
	cancel := make(chan struct{})
	sent := make(chan struct{})

	// Behaves like AllocFS.Logs: sends more frames than the buffer holds,
	// then waits for cancel and closes the channel.
	go func() {
		for range cap(frames) + 2 {
			frames <- &api.StreamFrame{Data: []byte("x\n")}
		}

		close(sent)

		<-cancel
		close(frames)
	}()

	lines := streamLines(frames, neverCut, cancel)

	close(cancel)

	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("the producer stayed blocked on its send")
	}

	require.True(t, ended(lines))
}
