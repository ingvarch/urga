package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// payloadMarker is the line of a dispatch file the payload follows.
const payloadMarker = "--- payload ---"

// dispatchable says the job under the cursor is parameterized: a dispatch
// runs it.
func dispatchable(p jobsPage, e env) bool {
	job, ok := p.picked(e)

	return ok && job.Parameterized
}

// dispatchJob reads what the job under the cursor takes, then asks for it: in
// the editor, or, when the job takes nothing, with a question.
func dispatchJob(p jobsPage, e env) (jobsPage, outcome) {
	job, ok := p.picked(e)
	if !ok {
		return p, outcome{}
	}

	client := e.client

	return p, outcome{cmd: request(func(ctx context.Context) (nomad.DispatchForm, error) {
		return client.DispatchForm(ctx, job.Namespace, job.ID)
	}, func(form nomad.DispatchForm) tea.Msg { return askDispatch(client, job, form) })}
}

// askDispatch asks for what a dispatch gives the job.
func askDispatch(client jobsClient, job nomad.Job, form nomad.DispatchForm) tea.Msg {
	if len(form.Required)+len(form.Optional) == 0 && form.Payload == nomad.PayloadForbidden {
		return askMsg{
			question: fmt.Sprintf("Really dispatch the job %s?", job.ID),
			apply:    dispatch(client, job, nil, nil),
		}
	}

	typed := dispatchFile(client, job, form)

	return requestMsg(openEditor(func(context.Context) (file, error) { return typed, nil }))
}

// dispatchFile is the file a dispatch is typed in. It is sent once it reads;
// a file that does not, or that the cluster refuses, opens again with the
// reason above it.
func dispatchFile(client jobsClient, job nomad.Job, form nomad.DispatchForm) file {
	return file{extension: "txt", content: dispatchTemplate(job, form), submit: reopening("txt", func(source string) tea.Cmd {
		meta, payload, err := parseDispatch(source)
		if err != nil {
			return func() tea.Msg { return errMsg{err: err} }
		}

		return dispatch(client, job, meta, payload)
	})}
}

// dispatch dispatches a job, and says the job it launched.
func dispatch(client jobsClient, job nomad.Job, meta map[string]string, payload []byte) tea.Cmd {
	return request(func(ctx context.Context) (string, error) {
		return client.DispatchJob(ctx, job.Namespace, job.ID, meta, payload)
	}, func(id string) tea.Msg {
		return doneMsg{said: fmt.Sprintf("Job %s dispatched as %s.", job.ID, id)}
	})
}

// dispatchTemplate is the file a dispatch starts from: what the job takes, as
// comments, then a line for each meta key and a place for the payload. A
// file saved unchanged sends nothing, so the last comment says how to
// dispatch with nothing set.
func dispatchTemplate(job nomad.Job, form nomad.DispatchForm) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Dispatch %s in %s.\n", job.ID, job.Namespace)
	fmt.Fprintf(&b, "# %s\n", metaTaken(form))
	fmt.Fprintf(&b, "# %s\n", payloadTaken(form.Payload))
	b.WriteString("# Save a change to dispatch: at least delete this line. Quit without saving to drop it.\n")

	for _, key := range slices.Concat(form.Required, form.Optional) {
		fmt.Fprintf(&b, "%s =\n", key)
	}

	if form.Payload != nomad.PayloadForbidden {
		b.WriteString(payloadMarker + "\n")
	}

	return b.String()
}

// metaTaken says which meta keys the job needs, and which it takes.
func metaTaken(form nomad.DispatchForm) string {
	said := []string{}

	if len(form.Required) > 0 {
		said = append(said, "Required meta: "+strings.Join(form.Required, ", ")+".")
	}

	if len(form.Optional) > 0 {
		said = append(said, "Optional meta: "+strings.Join(form.Optional, ", ")+".")
	}

	if len(said) == 0 {
		return "Meta: none."
	}

	return strings.Join(said, " ")
}

// payloadTaken says whether the job takes a payload, and where it goes.
func payloadTaken(mode string) string {
	if mode == nomad.PayloadForbidden {
		return "Payload: none, the job takes no payload."
	}

	return fmt.Sprintf("Payload: %s. It is everything below the line %q, as it is.", mode, payloadMarker)
}

// parseDispatch reads a dispatch file: a key = value line for each meta key,
// with comments and blank lines left out, then the payload below the marker,
// as it is. A key left empty is not sent, so the cluster says it is missing.
func parseDispatch(source string) (map[string]string, []byte, error) {
	meta := map[string]string{}
	rest := source

	for n := 1; rest != ""; n++ {
		var line string

		line, rest, _ = strings.Cut(rest, "\n")
		text := strings.TrimSpace(line)

		switch {
		case text == payloadMarker:
			return metaOrNone(meta), payloadOrNone(rest), nil
		case text == "" || strings.HasPrefix(text, "#"):
			continue
		}

		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return nil, nil, fmt.Errorf("line %d: write key = value", n)
		}

		if key = strings.TrimSpace(key); key == "" {
			return nil, nil, fmt.Errorf("line %d: no key before =", n)
		}

		if value = strings.TrimSpace(value); value != "" {
			meta[key] = value
		}
	}

	return metaOrNone(meta), nil, nil
}

func metaOrNone(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}

	return meta
}

// payloadOrNone is a payload, or none for blank space.
func payloadOrNone(payload string) []byte {
	if strings.TrimSpace(payload) == "" {
		return nil
	}

	return []byte(payload)
}
