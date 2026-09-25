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

	"github.com/ingvarch/urga/internal/nomad"
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

// The screens that can be edited each open what the cursor is on in the
// editor of the user.

func editJob(m Model) (Model, tea.Cmd) {
	job, ok := selectedOf(m, screenJobs, m.jobs)
	if !ok {
		return m, nil
	}

	return m, openEditor(jobFile(m.client, job))
}

func editMeta(m Model) (Model, tea.Cmd) {
	return m, openEditor(metaFile(m.client, m.screen.nodeID, m.screen.label))
}

func editNamespace(m Model) (Model, tea.Cmd) {
	namespace, ok := selectedOf(m, screenNamespaces, m.namespaces)
	if !ok {
		return m, nil
	}

	return m, openEditor(namespaceFile(m.client, namespace.Name))
}

// file is what the editor is given: the name to save it under and what is in
// it, and how what comes back goes to the cluster. How it goes back is decided
// when it is read: a job goes back with the values its variables had.
type file struct {
	extension string
	content   string
	submit    func(source string) tea.Cmd
}

// load reads a resource as a file.
type load func(ctx context.Context) (file, error)

// jobFile is the job as a file: what it was submitted with, and what the
// cluster does have of it when that was not kept.
func jobFile(client jobsClient, job nomad.Job) load {
	return func(ctx context.Context) (file, error) {
		spec, err := client.JobSpec(ctx, job.Namespace, job.ID)
		if errors.Is(err, nomad.ErrNoSource) {
			spec = nomad.JobSource{Format: nomad.FormatJSON}
			spec.Source, err = client.DescribeJob(ctx, job.Namespace, job.ID)
		}

		if err != nil {
			return file{}, err
		}

		// The editor holds the file only, the values of the variables are
		// sent back beside it. What comes back is planned first: a changed
		// file can restart every allocation of the job.
		extension := jobExtension(spec.Format)

		return file{extension: extension, content: spec.Source, submit: reopening(extension, func(source string) tea.Cmd {
			return planFor(client, planState{
				namespace: job.Namespace,
				jobID:     job.ID,
				source:    source,
				vars:      spec.Variables,
			})
		})}, nil
	}
}

// jobExtension names a job file after how it is written, which is what an
// editor colors and checks it by.
func jobExtension(format string) string {
	if format == nomad.FormatJSON {
		return "json"
	}

	return "hcl"
}

// namespaceFile is a namespace as a file.
func namespaceFile(client namespacesClient, name string) load {
	return func(ctx context.Context) (file, error) {
		content, err := client.NamespaceSpec(ctx, name)

		return file{extension: "json", content: content, submit: reopening("json", func(source string) tea.Cmd {
			return act(fmt.Sprintf("Namespace %s submitted.", name), func(ctx context.Context) error {
				return client.SubmitNamespace(ctx, source)
			})
		})}, err
	}
}

// refusedEditMsg is an edit the cluster refused, and the file to open it in
// again: what was typed, and how it is sent the next time. advice says what
// sending it again does, when that is not plain.
type refusedEditMsg struct {
	err    error
	advice string
	again  file
}

// reopening sends an edit as send does. When the cluster refuses it, the
// edit opens again instead of being lost, and is sent the same way.
func reopening(extension string, send func(source string) tea.Cmd) func(source string) tea.Cmd {
	var submit func(source string) tea.Cmd

	submit = func(source string) tea.Cmd {
		sent := send(source)

		return func() tea.Msg {
			msg := sent()
			if failed, ok := msg.(errMsg); ok {
				return refusedEditMsg{err: failed.err, again: file{extension: extension, content: source, submit: submit}}
			}

			return msg
		}
	}

	return submit
}

// reopenEdit opens a refused edit again, with why at the top. The reason
// comes off before the file is sent: JSON has no comments, and the comments
// at the top of a job are part of it.
func (m Model) reopenEdit(msg refusedEditMsg) tea.Cmd {
	reason, ok := m.refusedBecause(msg.err)
	if !ok {
		reason = msg.err.Error()
	}

	header := refusal(reason, msg.advice)

	again, submit := msg.again, msg.again.submit
	again.content = header + again.content
	again.submit = func(source string) tea.Cmd { return submit(strings.TrimPrefix(source, header)) }

	return openEditor(func(context.Context) (file, error) { return again, nil })
}

// refusal is why an edit was refused, as comment lines. A file that comes
// back as it went out changes nothing, so saving it again takes a change.
func refusal(reason, advice string) string {
	lines := strings.Split("Not saved: "+reason+".", "\n")
	if advice != "" {
		lines = append(lines, advice)
	}

	lines = append(lines, "To save, change the file: delete these lines at least.", "To drop your edit, quit without saving.")

	var b strings.Builder
	for _, line := range lines {
		b.WriteString("# " + line + "\n")
	}

	return b.String()
}

// openEditor puts what the cluster has in a file and hands it over.
func openEditor(read load) tea.Cmd {
	return request(func(ctx context.Context) (editFileMsg, error) {
		loaded, err := read(ctx)
		if err != nil {
			return editFileMsg{}, err
		}

		file, err := os.CreateTemp("", "urga-*."+loaded.extension)
		if err != nil {
			return editFileMsg{}, err
		}

		if _, err := file.WriteString(loaded.content); err != nil {
			return editFileMsg{}, err
		}

		if err := file.Close(); err != nil {
			return editFileMsg{}, err
		}

		return editFileMsg{path: file.Name(), original: loaded.content, submit: loaded.submit}, nil
	}, func(msg editFileMsg) tea.Msg { return msg })
}

// startEdit hands the file over to the editor.
func (m Model) startEdit(msg editFileMsg) (Model, tea.Cmd) {
	if m.opts.Editor == nil {
		return m.fail(errors.New("no editor: set EDITOR or VISUAL")), nil
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
		return m.fail(msg.err), nil
	}

	data, err := os.ReadFile(msg.path)
	if err != nil {
		return m.fail(err), nil
	}

	source := string(data)
	if source == edit.original {
		return m.say(fmt.Sprintf("%s unchanged.", filepath.Base(msg.path))), nil
	}

	if edit.submit == nil {
		return m, nil
	}

	return m, edit.submit(source)
}
