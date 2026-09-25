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

// filesMsg is a directory of an allocation, and which one it is.
type filesMsg struct {
	path  string
	files []nomad.File
}

// dirState is the directory the files screen last listed.
type dirState struct {
	path  string
	files []nomad.File
}

var fileBindings = []binding{{press: "enter", label: "Open", do: openEntry}}

// browse opens the directory of the task under the cursor: what its
// templates rendered is in local/ there.
func browse(m Model) (Model, tea.Cmd) {
	task, ok := selectedOf(m, screenTasks, m.tasks())
	if !ok {
		return m, nil
	}

	return m.openDir(path.Join("/", task.Name))
}

// openDir lists a directory of the same allocation, on top of what is open:
// escape comes back to it.
func (m Model) openDir(dir string) (Model, tea.Cmd) {
	s := m.screen

	return m.push(screen{kind: screenFiles, namespace: s.namespace, jobID: s.jobID, allocID: s.allocID, path: dir})
}

// fetchFiles lists the directory the screen is open on.
func fetchFiles(m Model) tea.Cmd {
	client, screen := m.client, m.screen

	return fetchList(func(ctx context.Context) ([]nomad.File, error) {
		return client.Files(ctx, screen.namespace, screen.allocID, screen.path)
	}, func(files []nomad.File) tea.Msg { return filesMsg{path: screen.path, files: files} })
}

// entries are the rows of the directory on the screen, the one above it
// first when there is one. Until the directory is listed there are none: what
// was listed last may be the directory that was left.
func (m Model) entries() []nomad.File {
	if m.dir.path != m.screen.path {
		return nil
	}

	if m.screen.path == "/" {
		return m.dir.files
	}

	return append([]nomad.File{{Name: parentDir, Dir: true}}, m.dir.files...)
}

// openEntry goes into the directory under the cursor, or up.
func openEntry(m Model) (Model, tea.Cmd) {
	entry, ok := selectedOf(m, screenFiles, m.entries())
	if !ok {
		return m, nil
	}

	switch {
	case entry.Name == parentDir:
		return m.openDir(path.Dir(m.screen.path))

	case entry.Dir:
		return m.openDir(path.Join(m.screen.path, entry.Name))
	}

	return m.openFile(entry)
}

// fileMsg is a file opened to be read, and the screen it is read on.
type fileMsg struct {
	stream *nomad.LogStream
	file   screen
}

// fileTextBindings are the keys of a file that is read: the ones of a log
// that a file has.
var fileTextBindings = []binding{
	{press: "s", label: "Toggle Autoscroll", do: toggleAutoscroll},
	{press: "w", label: "Toggle Wrap", do: wrapLines},
	{press: "ctrl+s", label: "Save", do: saveScreen},
}

// openFile reads the file under the cursor like a log. A pipe is not asked
// about: its client would wait on it for as long as the task writes nothing.
func (m Model) openFile(entry nomad.File) (Model, tea.Cmd) {
	if entry.Pipe() {
		return m.warn(fmt.Sprintf("%s is a pipe: only the task on its other end can read it", entry.Name)), nil
	}

	s := m.screen

	return m, readFile(m.client, screen{
		kind:      screenFile,
		namespace: s.namespace,
		jobID:     s.jobID,
		allocID:   s.allocID,
		path:      path.Join(s.path, entry.Name),
	})
}

// readFile opens the stream of a file. What is not text comes back as the
// reason it is not read, and the screen stays where it is.
func readFile(client Client, file screen) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		stream, err := client.File(ctx, file.namespace, file.allocID, file.path)
		if err != nil {
			return errMsg{err: err}
		}

		return fileMsg{stream: stream, file: file}
	}
}

// openedFile puts a file on the screen it was opened from: on top of its
// directory, or on its own screen come back to. One that arrives after the
// screen moved on is let go of: nothing else would ever close it.
func (m Model) openedFile(msg fileMsg) (Model, tea.Cmd) {
	s, file := m.screen, msg.file

	onFile := s.kind == screenFile && s.allocID == file.allocID && s.path == file.path
	inDir := s.kind == screenFiles && s.allocID == file.allocID && s.path == path.Dir(file.path)

	if !onFile && !inDir || m.logs.stream != nil {
		msg.stream.Close()

		return m, nil
	}

	// A file is read from its top: what it grows by stays below.
	if inDir {
		m = m.stackText(file, textModel{})
		m.logs = logState{}
	}

	var cmd tea.Cmd
	m.logs, cmd = m.logs.opened(msg.stream)

	return m, cmd
}

// readFileAgain opens the stream of a file screen that is come back to.
func (m Model) readFileAgain() (Model, tea.Cmd) {
	m.text = m.text.emptied()
	m.logs = logState{}

	return m, readFile(m.client, m.screen)
}

// fileTitle says which file of which allocation it is, and when only its end
// was read, how much of it.
func fileTitle(s screen, logs logState) string {
	what := s.path
	if logs.from > 0 {
		what += fmt.Sprintf(", last %s of %s", sizeOf(logs.size-logs.from), sizeOf(logs.size))
	}

	return fmt.Sprintf("File (Allocation: %s) [%s]", shortID(s.allocID), what)
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
