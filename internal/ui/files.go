package ui

import (
	"context"
	"fmt"
	"path"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// fileTitles are the columns of a directory of an allocation.
var fileTitles = []string{"Name", "Size", "Modified"}

// parentDir is the row that goes to the directory above.
const parentDir = ".."

// filesMsg is what a directory of an allocation holds.
type filesMsg []nomad.File

// filesPage is a directory of an allocation: what its tasks share, or what
// one of them holds.
type filesPage struct {
	namespace, allocID, path string

	// files are what the directory held when it was last listed, and listed
	// says it has been: until then there are no rows, not even the one that
	// goes up.
	files  []nomad.File
	listed bool
}

// browse opens the directory of the task under the cursor: what its
// templates rendered is in local/ there.
func browse(p tasksPage, e env) (tasksPage, outcome) {
	task, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	return p, then(openMsg{filesPage{namespace: p.namespace, allocID: p.allocID, path: path.Join("/", task.Name)}})
}

func (p filesPage) title(_ env, count int) string {
	return sprintf("Files (Allocation: %s, %s) [%d]", shortID(p.allocID), p.path, count)
}

func (filesPage) titles() []string { return fileTitles }
func (filesPage) topics() []string { return nil }

// fetch lists the directory the page is open on.
func (p filesPage) fetch(e env) tea.Cmd {
	client, namespace, allocID, dir := e.client, p.namespace, p.allocID, p.path

	return fetchList(func(ctx context.Context) ([]nomad.File, error) {
		return client.Files(ctx, namespace, allocID, dir)
	}, func(files []nomad.File) tea.Msg { return filesMsg(files) })
}

func (p filesPage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	switch msg := msg.(type) {
	case filesMsg:
		p.files, p.listed = msg, true

		return p, outcome{}, true

	case fileMsg:
		// A file of this directory, opened to be read: it is read on a page
		// of its own, on top. It is no answer to what the directory asked.
		if msg.allocID != p.allocID || path.Dir(msg.path) != p.path {
			return p, outcome{}, false
		}

		file := filePage{namespace: p.namespace, allocID: p.allocID, path: msg.path, first: msg.stream}

		return p, outcome{now: []tea.Msg{openMsg{file}}, reading: true}, true
	}

	return p, outcome{}, false
}

func (p filesPage) rows(env) []tableRow { return fileRows(p.entries()) }

// entries are the rows of the directory, the one above it first when there
// is one.
func (p filesPage) entries() []nomad.File {
	switch {
	case !p.listed:
		return nil
	case p.path == "/":
		return p.files
	}

	return append([]nomad.File{{Name: parentDir, Dir: true}}, p.files...)
}

var filesKeys = []pageKey[filesPage]{{press: "enter", label: "Open", do: openEntry}}

func (p filesPage) keys(e env) []keyHint { return hintsOf(p, e, filesKeys) }

func (p filesPage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, filesKeys, k)
}

// openEntry goes into the directory under the cursor, or up, or opens the
// file.
func openEntry(p filesPage, e env) (filesPage, outcome) {
	entry, ok := pickedFrom(e, p.entries())
	if !ok {
		return p, outcome{}
	}

	switch {
	case entry.Name == parentDir:
		return p, p.openDir(path.Dir(p.path))

	case entry.Dir:
		return p, p.openDir(path.Join(p.path, entry.Name))
	}

	return p, p.openFile(entry, e)
}

// openDir lists a directory of the same allocation, on top of this one:
// escape comes back to it.
func (p filesPage) openDir(dir string) outcome {
	return then(openMsg{filesPage{namespace: p.namespace, allocID: p.allocID, path: dir}})
}

// fileMsg is a file of an allocation, opened to be read.
type fileMsg struct {
	stream        *nomad.LogStream
	allocID, path string
}

// openFile reads the file under the cursor like a log. A pipe is not asked
// about: its client would wait on it for as long as the task writes nothing.
func (p filesPage) openFile(entry nomad.File, e env) outcome {
	if entry.Pipe() {
		return then(warnMsg(fmt.Sprintf("%s is a pipe: only the task on its other end can read it", entry.Name)))
	}

	return outcome{cmd: readFile(e.client, p.namespace, p.allocID, path.Join(p.path, entry.Name))}
}

// readFile opens the stream of a file. What is not text comes back as the
// reason it is not read, and the screen stays where it is.
func readFile(client filesClient, namespace, allocID, file string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		stream, err := client.File(ctx, namespace, allocID, file)
		if err != nil {
			return errMsg{err: err}
		}

		return fileMsg{stream: stream, allocID: allocID, path: file}
	}
}

// filePage is a file of an allocation, read like a log. A file is read from
// its top: what it grows by stays below.
type filePage struct {
	namespace, allocID, path string

	// first is the stream its directory opened the file with, read when the
	// page is entered the first time: a file that is not text never opens a
	// page of its own.
	first *nomad.LogStream

	read logState
}

// title says which file of which allocation it is, and when only its end
// was read, how much of it.
func (p filePage) title(env, int) string {
	what := p.path
	if p.read.from > 0 {
		what += fmt.Sprintf(", last %s of %s", sizeOf(p.read.size-p.read.from), sizeOf(p.read.size))
	}

	return fmt.Sprintf("File (Allocation: %s) [%s]", shortID(p.allocID), what)
}

func (filePage) titles() []string           { return nil }
func (filePage) topics() []string           { return nil }
func (filePage) fetch(env) tea.Cmd          { return nil }
func (filePage) rows(env) []tableRow        { return nil }
func (filePage) follows() bool              { return false }
func (p filePage) text(env) textContent     { return p.read.content }
func (p filePage) saveAs() (string, string) { return path.Base(p.path), "txt" }

// open reads the file through the stream its directory opened, the first
// time. Every time after that the file is asked for again, and read again
// from its top.
func (p filePage) open(e env) (page, tea.Cmd) {
	if p.first != nil {
		var cmd tea.Cmd
		p.read, cmd = logState{}.opened(p.first)
		p.first = nil

		return p, cmd
	}

	p.read.stop()
	p.read = logState{}

	return p, readFile(e.client, p.namespace, p.allocID, p.path)
}

func (p filePage) close() page {
	p.read.stop()

	return p
}

func (p filePage) take(msg tea.Msg, _ env) (page, outcome, bool) {
	var (
		out  outcome
		took bool
	)

	if opened, ok := msg.(fileMsg); ok {
		p.read, out, took = p.read.keep(opened.stream, opened.allocID == p.allocID && opened.path == p.path)
	} else {
		p.read, out, took = p.read.take(msg)
	}

	return p, out, took
}

// fileKeys are the keys of a log that a file has: when urga read a line is
// nothing to a file.
var fileKeys = append([]pageKey[filePage]{followKey[filePage]()}, textKeys[filePage]()...)

func (p filePage) keys(e env) []keyHint { return hintsOf(p, e, fileKeys) }

func (p filePage) press(k string, e env) (page, outcome, bool) {
	return pressOf(p, e, fileKeys, k)
}

func fileRows(files []nomad.File) []tableRow {
	rows := make([]tableRow, 0, len(files))

	for _, f := range files {
		row := tableRow{cells: []string{f.Name, sizeOf(f.Size), ageOf(f.Modified)}, ages: moments{2: f.Modified}}

		switch {
		case f.Name == parentDir:
			row.cells = []string{parentDir, "", ""}

		case f.Dir:
			row.cells = []string{f.Name + "/", "", ageOf(f.Modified)}
			row.color = colorTitle

		case f.Pipe():
			// Nothing to read here: it is read by whoever holds its
			// other end.
			row.color = colorSpent
		}

		rows = append(rows, row)
	}

	return rows
}

// sizeOf is a size the way a person reads it: 1.1 KiB, 5.2 MiB.
func sizeOf(bytes int64) string {
	const unit = 1024

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
