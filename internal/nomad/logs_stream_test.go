package nomad

import (
	"strings"
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

	lines := streamLines(frames, neverCut, nil, nil, cancel)

	close(cancel)

	require.True(t, ended(lines))
}

func TestStreamLines_EndWhenCancelledOnNoFrames(t *testing.T) {
	cancel := make(chan struct{})

	lines := streamLines(nil, neverCut, nil, nil, cancel)

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

	lines := streamLines(frames, neverCut, nil, nil, cancel)

	close(cancel)

	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("the producer stayed blocked on its send")
	}

	require.True(t, ended(lines))
}

// chunksOf sends the data in one frame each, closes the frames, and collects
// what streamLines sends until it ends.
func chunksOf(t *testing.T, data ...string) []string {
	t.Helper()

	frames := make(chan *api.StreamFrame, len(data))
	for _, d := range data {
		frames <- &api.StreamFrame{Data: []byte(d)}
	}

	close(frames)

	lines := streamLines(frames, neverCut, nil, nil, make(chan struct{}))

	var chunks []string

	for {
		select {
		case chunk, ok := <-lines:
			if !ok {
				return chunks
			}

			chunks = append(chunks, chunk)
		case <-time.After(2 * time.Second):
			t.Fatal("the stream did not end")
		}
	}
}

func TestStreamLines_JoinALineSplitAcrossFrames(t *testing.T) {
	chunks := chunksOf(t, "first li", "ne\nsecond\nthi", "rd\n")

	require.Equal(t, "first line\nsecond\nthird\n", strings.Join(chunks, ""))

	for _, chunk := range chunks {
		require.True(t, strings.HasSuffix(chunk, "\n"), "chunk %q lacks its line break", chunk)
	}
}

func TestStreamLines_KeepAMultiByteCharacterWhole(t *testing.T) {
	snow := []byte("é")

	chunks := chunksOf(t, "caf"+string(snow[:1]), string(snow[1:])+"\n")

	require.Equal(t, []string{"café\n"}, chunks)
}

func TestStreamLines_SendTheLastLineWhenTheStreamEnds(t *testing.T) {
	chunks := chunksOf(t, "done\nno break")

	require.Equal(t, "done\nno break", strings.Join(chunks, ""))
}

func TestStreamLines_SendAHeldLineThatNeverEndsPastTheTail(t *testing.T) {
	frames := make(chan *api.StreamFrame, 1)
	cancel := make(chan struct{})

	lines := streamLines(frames, neverCut, nil, nil, cancel)

	frames <- &api.StreamFrame{Data: []byte(strings.Repeat("x", logTail+1))}

	select {
	case chunk := <-lines:
		require.Len(t, chunk, logTail+1)
	case <-time.After(2 * time.Second):
		t.Fatal("a line with no end was held back")
	}

	close(cancel)
}

func TestStreamLines_EndTheLineSentPastTheTailWithoutAnEmptyOne(t *testing.T) {
	frames := make(chan *api.StreamFrame, 2)
	cancel := make(chan struct{})

	defer close(cancel)

	lines := streamLines(frames, neverCut, nil, nil, cancel)

	frames <- &api.StreamFrame{Data: []byte(strings.Repeat("x", logTail+1))}
	frames <- &api.StreamFrame{Data: []byte("\nnext\n")}

	// The break that follows ends the line already sent.
	got := collect(t, lines, 2)
	require.Equal(t, "next\n", got[1])
}

func TestStreamLines_DropWhatIsHeldWhenCancelled(t *testing.T) {
	frames := make(chan *api.StreamFrame, 1)
	cancel := make(chan struct{})

	lines := streamLines(frames, neverCut, nil, nil, cancel)

	frames <- &api.StreamFrame{Data: []byte("part")}

	close(cancel)

	for chunk := range lines {
		t.Fatalf("sent %q after cancel", chunk)
	}
}

// atSize says a frame reached the size a file had when it was opened.
func atSize(size int64) func(*api.StreamFrame) bool {
	return func(frame *api.StreamFrame) bool { return frame.Offset >= size }
}

// collect reads n chunks from lines, or fails when they do not come.
func collect(t *testing.T, lines <-chan string, n int) []string {
	t.Helper()

	var got []string

	for len(got) < n {
		select {
		case chunk := <-lines:
			got = append(got, chunk)
		case <-time.After(2 * time.Second):
			t.Fatalf("lines did not show, got %q", got)
		}
	}

	return got
}

func TestStreamLines_SendTheEndOfAFileWhenItIsReadToItsSize(t *testing.T) {
	frames := make(chan *api.StreamFrame, 1)
	cancel := make(chan struct{})

	defer close(cancel)

	frames <- &api.StreamFrame{Data: []byte("a=1\nb=2"), Offset: 7}

	lines := streamLines(frames, neverCut, atSize(7), nil, cancel)

	require.Equal(t, "a=1\nb=2", strings.Join(collect(t, lines, 1), ""))
}

func TestStreamLines_EndTheLineSentAtTheSizeWithoutRepeatingIt(t *testing.T) {
	frames := make(chan *api.StreamFrame, 3)
	cancel := make(chan struct{})

	defer close(cancel)

	frames <- &api.StreamFrame{Data: []byte("a=1\nb=2"), Offset: 7}
	frames <- &api.StreamFrame{Data: []byte("\nc=3"), Offset: 11}
	frames <- &api.StreamFrame{Data: []byte("\n"), Offset: 12}

	lines := streamLines(frames, neverCut, atSize(7), nil, cancel)

	got := collect(t, lines, 2)

	require.Equal(t, []string{"a=1\nb=2", "c=3\n"}, got)

	select {
	case chunk := <-lines:
		t.Fatalf("sent %q again", chunk)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestStreamLines_JoinALineAfterTheSizeSplitAcrossFrames(t *testing.T) {
	frames := make(chan *api.StreamFrame, 3)
	cancel := make(chan struct{})

	defer close(cancel)

	frames <- &api.StreamFrame{Data: []byte("a=1\n"), Offset: 4}
	frames <- &api.StreamFrame{Data: []byte("first li"), Offset: 12}
	frames <- &api.StreamFrame{Data: []byte("ne\n"), Offset: 15}

	lines := streamLines(frames, neverCut, atSize(4), nil, cancel)

	require.Equal(t, []string{"a=1\n", "first line\n"}, collect(t, lines, 2))
}

func TestStreamLines_NoEndOfAFileThatWasEmpty(t *testing.T) {
	frames := make(chan *api.StreamFrame, 3)
	cancel := make(chan struct{})

	defer close(cancel)

	frames <- &api.StreamFrame{Data: []byte("first li"), Offset: 8}
	frames <- &api.StreamFrame{Data: []byte("ne\nsec"), Offset: 14}
	frames <- &api.StreamFrame{Data: []byte("ond\n"), Offset: 18}

	lines := streamLines(frames, neverCut, nil, nil, cancel)

	require.Equal(t, []string{"first line\n", "second\n"}, collect(t, lines, 2))
}

func TestStreamLines_HoldALogLineUntilItsBreakWhileFramesStayOpen(t *testing.T) {
	frames := make(chan *api.StreamFrame, 2)
	cancel := make(chan struct{})

	defer close(cancel)

	lines := streamLines(frames, neverCut, nil, nil, cancel)

	frames <- &api.StreamFrame{Data: []byte("waiting")}

	select {
	case chunk := <-lines:
		t.Fatalf("sent %q before the line ended", chunk)
	case <-time.After(300 * time.Millisecond):
	}

	frames <- &api.StreamFrame{Data: []byte(" done\n")}

	require.Equal(t, []string{"waiting done\n"}, collect(t, lines, 1))
}
