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

	return m, nil
}

func fileRows(files []nomad.File) []tableRow {
	rows := make([]tableRow, 0, len(files))

	for _, f := range files {
		row := tableRow{cells: []string{f.Name, sizeOf(f.Size), ageOf(f.Modified)}}

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
