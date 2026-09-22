package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Editor hands a file to the editor of the user and comes back when it is
// closed.
type Editor interface {
	Edit(path string) tea.Cmd
}

// Messages of an edit.
type (
	editFileMsg struct {
		path     string
		original string
		submit   func(source string) tea.Cmd
	}

	editedMsg struct {
		path string
		err  error
	}
)

// terminalEditor runs what EDITOR names, with the terminal handed over to it.
type terminalEditor struct{}

// NewEditor is the editor of the user.
func NewEditor() Editor { return terminalEditor{} }

func (terminalEditor) Edit(path string) tea.Cmd {
	name := editorCommand()
	if name == "" {
		return func() tea.Msg {
			return editedMsg{path: path, err: errors.New("no editor: set EDITOR or VISUAL")}
		}
	}

	fields := strings.Fields(name)
	fields = append(fields, path)

	return tea.ExecProcess(exec.Command(fields[0], fields[1:]...), func(err error) tea.Msg {
		return editedMsg{path: path, err: err}
	})
}

func editorCommand() string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}

	return ""
}

// edit opens what the cursor is on in the editor of the user.
func (m Model) edit() (Model, tea.Cmd) {
	row, ok := m.selectedIndex()
	if !ok {
		return m, nil
	}

	client := m.client

	switch m.screen.kind {
	case screenJobs:
		job := m.jobs[row]

		return m, openEditor("hcl",
			func(ctx context.Context) (string, error) {
				return client.JobSpec(ctx, job.Namespace, job.ID)
			},
			func(source string) tea.Cmd {
				return act(fmt.Sprintf("Job %s submitted.", job.ID), func(ctx context.Context) error {
					return client.SubmitJob(ctx, job.Namespace, source)
				})
			})

	case screenNamespaces:
		namespace := m.namespaces[row]

		return m, openEditor("json",
			func(ctx context.Context) (string, error) {
				return client.NamespaceSpec(ctx, namespace.Name)
			},
			func(source string) tea.Cmd {
				return act(fmt.Sprintf("Namespace %s submitted.", namespace.Name), func(ctx context.Context) error {
					return client.SubmitNamespace(ctx, source)
				})
			})
	}

	return m, nil
}

// openEditor puts what the cluster has in a file and hands it over.
func openEditor(extension string, load func(ctx context.Context) (string, error), submit func(string) tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		content, err := load(ctx)
		if err != nil {
			return errMsg{err: err}
		}

		file, err := os.CreateTemp("", "urga-*."+extension)
		if err != nil {
			return errMsg{err: err}
		}

		if _, err := file.WriteString(content); err != nil {
			return errMsg{err: err}
		}

		if err := file.Close(); err != nil {
			return errMsg{err: err}
		}

		return editFileMsg{path: file.Name(), original: content, submit: submit}
	}
}

// startEdit hands the file over to the editor.
func (m Model) startEdit(msg editFileMsg) (Model, tea.Cmd) {
	if m.opts.Editor == nil {
		m.err = errors.New("no editor: set EDITOR or VISUAL")

		return m, nil
	}

	m.editing = msg

	return m, m.opts.Editor.Edit(msg.path)
}

// finishEdit reads what came back. A file that was not touched changes
// nothing, and saving is the decision, so nothing is asked again.
func (m Model) finishEdit(msg editedMsg) (Model, tea.Cmd) {
	edit := m.editing
	m.editing = editFileMsg{}

	defer func() { _ = os.Remove(msg.path) }()

	if msg.err != nil {
		m.err = msg.err

		return m, nil
	}

	data, err := os.ReadFile(msg.path)
	if err != nil {
		m.err = err

		return m, nil
	}

	source := string(data)
	if source == edit.original {
		m.said = fmt.Sprintf("%s unchanged.", filepath.Base(msg.path))

		return m, nil
	}

	if edit.submit == nil {
		return m, nil
	}

	return m, edit.submit(source)
}
