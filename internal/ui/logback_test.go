package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// midLog is where a log read from its last 64 KiB begins.
var midLog = nomad.LogMark{File: "alloc/logs/server.stdout.1", Offset: 70_000}

// numberedLines are n lines named after their place, from first on.
func numberedLines(prefix string, first, n int) []string {
	lines := make([]string, 0, n)
	for i := first; i < first+n; i++ {
		lines = append(lines, fmt.Sprintf("%s %05d", prefix, i))
	}

	return lines
}

// sentLog opens a log and has its stream send the lines, the first of which
// begins at start.
func sentLog(t *testing.T, client *fakeClient, start nomad.LogMark, lines []string) (Model, *nomad.LogStream) {
	t.Helper()

	stream := &nomad.LogStream{Lines: make(chan string)}
	m := openLog(t, client, stream)
	m, _ = m.update(logLineMsg{stream: stream, text: strings.Join(lines, "\n") + "\n", start: start})

	return m, stream
}

// pressEarlier presses e and runs the read it asks for.
func pressEarlier(m Model) Model {
	m, cmd := m.update(key('e'))

	return drain(m, cmd)
}

// shownRows are the rows of the text, as the window draws them.
func shownRows(m Model) []string {
	rows := m.text.rows()
	out := make([]string, 0, len(rows))

	for _, row := range rows {
		out = append(out, row.text)
	}

	return out
}

func TestLogEarlier_ALineOfAStreamCarriesWhereItsFirstLineBegins(t *testing.T) {
	r := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v1/allocation/a1":
			_, _ = w.Write([]byte(`{"ID": "a1", "Namespace": "production", "NodeID": "n1", "TaskStates": {"server": {"State": "running"}}}`))
		case "/v1/client/fs/logs/a1":
			_ = json.NewEncoder(w).Encode(map[string]any{"Data": []byte("cut\nwhole\n"), "File": midLog.File, "Offset": midLog.Offset})
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	r.NoError(err)

	stream, err := client.Logs(context.Background(), "production", "a1", "server", nomad.LogStdout)
	r.NoError(err)
	t.Cleanup(stream.Close)

	// The cut line is dropped: the text starts at the line after it.
	start := nomad.LogMark{File: midLog.File, Offset: midLog.Offset - int64(len("whole\n"))}
	r.Equal(logLineMsg{stream: stream, text: "whole\n", start: start}, waitForStream(stream)())
}

func TestLogEarlier_OfferedWhileTheLogStartsPastItsStart(t *testing.T) {
	tests := []struct {
		name  string
		start nomad.LogMark
		want  bool
	}{
		{"read from the middle", midLog, true},
		{"read from the start", nomad.LogMark{File: "alloc/logs/server.stdout.0"}, false},
		{"no mark", nomad.LogMark{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := sentLog(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}, tt.start, []string{"one", "two"})

			require.Equal(t, tt.want, offersLabel(m, "e", "Read Earlier"))
		})
	}
}

func TestLogEarlier_AsksFromTheMarkWithTheRoomLeft(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, numberedLines("late", 0, 60))

	client.logPart = nomad.LogPart{Lines: numberedLines("early", 0, 10), From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}
	pressEarlier(m)

	r.Equal(midLog, client.backAt)
	r.Equal("production", client.backNamespace)
	r.Equal("af1f37df-7b19-6b1c-da67-5e8f482b5a15", client.backAllocID)
	r.Equal(maxTextLines-60, client.backLimit)
}

func TestLogEarlier_PutsTheLinesAboveAndKeepsTheWindowOnItsLine(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, numberedLines("late", 0, 60))

	m, _ = m.update(key('s'))
	m.text.move(-100)
	m.text.move(5)
	r.Equal("late 00005", shownRows(m)[m.text.top])

	from := nomad.LogMark{File: midLog.File, Offset: 60_000}
	client.logPart = nomad.LogPart{Lines: numberedLines("early", 0, 812), From: from, Before: 1_153_434, Size: 5_557_452}
	m = pressEarlier(m)

	// The line at the top of the window is still at the top, with the lines
	// read back above it.
	r.Len(shownRows(m), 812+60)
	r.Equal("early 00000", shownRows(m)[0])
	r.Equal("late 00005", shownRows(m)[m.text.top])

	out := plain(m.render())
	r.Contains(out, "[stdout, last 4.2 MiB of 5.3 MiB]")
	r.Contains(out, "Read 812 lines further back.")
	r.Equal(from, logOf(m).before)
	r.True(offersLabel(m, "e", "Read Earlier"))
}

func TestLogEarlier_AWindowThatFollowsStaysAtTheEnd(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, numberedLines("late", 0, 60))

	client.logPart = nomad.LogPart{Lines: numberedLines("early", 0, 100), From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}
	m = pressEarlier(m)

	r.True(m.text.following)
	r.Len(shownRows(m), 160)
	r.Contains(plain(m.render()), "late 00059")
	r.Equal(m.text.length()-m.text.height, m.text.top)
}

func TestLogEarlier_ALineAfterAReadKeepsWhereTheTextStarts(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, stream := sentLog(t, client, midLog, numberedLines("late", 0, 5))

	from := nomad.LogMark{File: midLog.File, Offset: 60_000}
	client.logPart = nomad.LogPart{Lines: numberedLines("early", 0, 3), From: from, Before: 60_000, Size: 130_000}
	m = pressEarlier(m)

	// Every line of a stream carries the mark of its first line.
	m, _ = m.update(logLineMsg{stream: stream, text: "more\n", start: midLog})
	r.Equal(from, logOf(m).before)

	pressEarlier(m)
	r.Equal(from, client.backAt)
}

func TestLogEarlier_TheStartOfTheLogTakesTheKeyAway(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, numberedLines("late", 0, 5))

	client.logPart = nomad.LogPart{Lines: numberedLines("middle", 0, 3), From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}
	m = pressEarlier(m)
	r.Contains(plain(m.render()), "[stdout, last")

	client.logPart = nomad.LogPart{Lines: numberedLines("early", 0, 3), From: nomad.LogMark{File: "alloc/logs/server.stdout.0"}, Size: 130_000}
	m = pressEarlier(m)

	out := plain(m.render())
	r.False(offersLabel(m, "e", "Read Earlier"))
	r.NotContains(out, "last")
	r.Contains(out, "Read 3 lines further back, to the start of the log.")
}

func TestLogEarlier_NothingFurtherBack(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, numberedLines("late", 0, 5))

	client.logPart = nomad.LogPart{From: midLog, Size: 130_000}
	m = pressEarlier(m)

	r.Contains(plain(m.render()), "Nothing further back in the log.")
	r.Len(shownRows(m), 5)
}

func TestLogEarlier_AnAnswerForTheSameMarkTwiceIsTakenOnce(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, numberedLines("late", 0, 5))

	part := nomad.LogPart{Lines: numberedLines("early", 0, 3), From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}
	m, _ = m.update(logPartMsg{at: midLog, part: part})
	m, _ = m.update(logPartMsg{at: midLog, part: part})

	r.Len(shownRows(m), 8)
}

func TestLogEarlier_AnAnswerIsDroppedWhenTheStreamTookLinesOffTheFront(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, stream := sentLog(t, client, midLog, numberedLines("late", 0, maxTextLines-10))

	client.logPart = nomad.LogPart{Lines: numberedLines("early", 0, 10), From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}

	m, cmd := m.update(key('e'))
	m, _ = m.update(logLineMsg{stream: stream, text: strings.Join(numberedLines("more", 0, 30), "\n") + "\n"})
	m = drain(m, cmd)

	r.Equal(maxTextLines, len(shownRows(m)))
	r.Equal("late 00020", shownRows(m)[0])
	r.Equal(midLog, logOf(m).before)
}

func TestLogEarlier_AnEmptyAnswerIsDroppedWhenTheStreamTookLinesOffTheFront(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, stream := sentLog(t, client, midLog, numberedLines("late", 0, maxTextLines-10))

	client.logPart = nomad.LogPart{From: midLog, Size: 130_000}

	m, cmd := m.update(key('e'))
	m, _ = m.update(logLineMsg{stream: stream, text: strings.Join(numberedLines("more", 0, 30), "\n") + "\n", start: midLog})
	m = drain(m, cmd)

	// The text no longer starts at the mark, so the answer says nothing about
	// what is before it now.
	r.NotContains(plain(m.render()), "Nothing further back in the log.")

	read := logOf(m)
	r.Equal(midLog, read.before)
	r.True(read.more)
	r.Zero(read.size)
	r.Zero(read.from)
}

func TestLogEarlier_AnAnswerIsDroppedWhenItNoLongerFits(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, stream := sentLog(t, client, midLog, numberedLines("late", 0, maxTextLines-10))

	client.logPart = nomad.LogPart{Lines: numberedLines("early", 0, 10), From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}

	m, cmd := m.update(key('e'))
	m, _ = m.update(logLineMsg{stream: stream, text: strings.Join(numberedLines("more", 0, 5), "\n") + "\n"})
	m = drain(m, cmd)

	r.Equal(maxTextLines-5, len(shownRows(m)))
	r.Equal("late 00000", shownRows(m)[0])
	r.Equal(midLog, logOf(m).before)
	r.True(offersLabel(m, "e", "Read Earlier"))

	// The press would otherwise show nothing.
	r.Contains(status(m), "The lines read back no longer fit on the screen.")
	r.Equal(flashWarn, m.flash.level)
}

func TestLogEarlier_AFullTextOffersNoKey(t *testing.T) {
	m, _ := sentLog(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}, midLog, numberedLines("late", 0, maxTextLines))

	require.False(t, offersLabel(m, "e", "Read Earlier"))
}

func TestLogEarlier_TheLinesReadBackAreCleaned(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, numberedLines("late", 0, 5))

	client.logPart = nomad.LogPart{Lines: []string{"a\tb", "\x1b]0;title\x07x"}, From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}
	m = pressEarlier(m)

	r.Equal([]string{"a   b", "x", "late 00000"}, shownRows(m)[:3])
}

func TestText_LinesPutInFrontHaveNoTimeTagOrStyle(t *testing.T) {
	r := require.New(t)

	own := &tag{text: "own │ "}
	bold := lipgloss.NewStyle().Bold(true)

	text := textContent{}
	text.add(linesFrom(10, 1), arrivedAt, nil, nil)
	text.add(linesFrom(11, 2), arrivedAt, own, &bold)
	text.addBefore(linesFrom(0, 3))

	r.Equal([]string{"line 0", "line 1", "line 2", "line 10", "line 11", "line 12"}, text.lines)

	for i := range 4 {
		r.Nil(text.tagAt(i), i)
		r.Nil(text.paintAt(i), i)
	}

	for i := range 3 {
		r.Zero(text.stampAt(i), i)
	}

	r.Equal(arrivedAt, text.stampAt(3))

	for i := 4; i < 6; i++ {
		r.Equal(arrivedAt, text.stampAt(i), i)
		r.Same(own, text.tagAt(i), i)
		r.Same(&bold, text.paintAt(i), i)
	}
}

func TestLogEarlier_TheLinesReadBackHaveNoTime(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, numberedLines("late", 0, 5))

	m, _ = m.update(key('t'))

	client.logPart = nomad.LogPart{Lines: numberedLines("early", 0, 2), From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}
	m = pressEarlier(m)

	rows := shownRows(m)
	r.Equal(strings.Repeat(" ", len(logTimeFormat)+2)+"early 00000", rows[0])
	r.NotEqual(strings.Repeat(" ", len(logTimeFormat)+2), rows[2][:len(logTimeFormat)+2])
	r.Contains(rows[2], "late 00000")
}

func TestLogEarlier_TheWindowStaysOnItsLineWithWrapAndAFilter(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	late := numberedLines("late", 0, 60)
	m, _ := sentLog(t, client, midLog, late)

	m, _ = m.update(key('s'))
	m, _ = m.update(key('w'))
	m, _ = m.update(key('/'))
	m = typeIn(m, "0")
	m, _ = m.update(enter())
	m.text.move(-100)
	m.text.move(7)

	before, rowsBefore := shownRows(m)[m.text.top], len(shownRows(m))

	// Each line read back wraps into several rows, and some are filtered out.
	long := strings.Repeat("x", 300)
	client.logPart = nomad.LogPart{Lines: []string{"early 0 " + long, "skipped " + long, "early 00 " + long}, From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}
	m = pressEarlier(m)

	r.Greater(len(shownRows(m)), rowsBefore)
	r.Equal(before, shownRows(m)[m.text.top])
	r.Greater(m.text.top, 7)
}

func TestLogEarlier_NextGoesOnFromTheMatchItWasOn(t *testing.T) {
	r := require.New(t)

	late := numberedLines("late", 0, 100)
	for _, i := range []int{20, 50, 80} {
		late[i] += " error"
	}

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, late)
	m = foundIn(windowOf(m, 10), "error")

	m, _ = m.update(key('n'))
	r.Contains(status(m), `Match 2 of 3 for "error".`)

	client.logPart = nomad.LogPart{Lines: numberedLines("early", 0, 10), From: nomad.LogMark{File: midLog.File, Offset: 60_000}, Before: 60_000, Size: 130_000}
	m = pressEarlier(m)
	r.Equal("late 00050 error", shownRows(m)[m.text.at])

	m, _ = m.update(key('n'))
	r.Contains(status(m), `Match 3 of 3 for "error".`)
	r.Equal("late 00080 error", shownRows(m)[m.text.at])
}

func TestLogEarlier_ARefusedReadLeavesTheLogAsItWas(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}
	m, _ := sentLog(t, client, midLog, numberedLines("late", 0, 5))

	client.err = errors.New("Permission denied")
	m = pressEarlier(m)

	r.Contains(plain(m.render()), "Permission denied")
	r.Len(shownRows(m), 5)
	r.Equal(midLog, logOf(m).before)
	r.True(offersLabel(m, "e", "Read Earlier"))
}

func TestLogEarlier_TheLogsOfEveryAllocationAndAFileOfferNoKey(t *testing.T) {
	r := require.New(t)

	client := &fakeClient{
		jobs:        twoJobs(),
		allocs:      webAllocs("server"),
		logsByAlloc: map[string]*nomad.LogStream{newer: writing("one\n"), older: writing("two\n"), backend: writing()},
	}
	m := fromJobs(t, client)
	m, _ = m.update(key('j'))
	m, cmd := m.update(enter())
	m = playOut(m, cmd)

	r.IsType(jobLogsPage{}, m.screen.page)
	r.False(offers(m, "e"))

	stream := &nomad.LogStream{Lines: make(chan string), Err: make(chan error)}
	m = readingEnv(t, &fakeClient{jobs: twoJobs(), allocs: twoAllocs()}, stream)
	m, _ = m.update(logLineMsg{stream: stream, text: "a\nb\n", start: midLog})

	r.IsType(filePage{}, m.screen.page)
	r.False(offers(m, "e"))
}
