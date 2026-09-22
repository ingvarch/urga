package ui

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// overlay is what took the keyboard from the screen. The root model decides
// who gets a key, a component never installs a hook of its own.
type overlay int

const (
	overlayNone overlay = iota
	overlayPrompt
	overlayFilter
	overlayHelp
	overlayConfirm
)

// promptHeight is the line plus the border around it.
const promptHeight = 3

const (
	promptPrefix = ":"
	filterPrefix = "/"
)

// promptAction is what the line does when it is committed.
type promptAction int

const (
	promptCommand promptAction = iota
	promptFilter
	promptScale
)

// promptModel is the line at the top: a prefix, what was typed, and the rest
// of the word the prompt would complete.
type promptModel struct {
	prefix string
	text   string
	action promptAction

	// group is the task group a count belongs to.
	group string
}

func (p promptModel) suggestion() string {
	if p.action != promptCommand {
		return ""
	}

	return completeCommand(p.text)
}

func (p promptModel) view(width int) string {
	// The cursor sits after the suggestion, so the word the prompt offers
	// reads whole.
	line := styleTitle.Render(p.prefix) + styleText.Render(p.text) +
		styleMuted.Render(p.suggestion()) + styleSelected.Render(" ")

	return frame("", line, width, promptHeight)
}

// openPrompt puts the command line up.
func (m Model) openPrompt(prefix string) (Model, tea.Cmd) {
	m.overlay = overlayPrompt
	m.prompt = promptModel{prefix: prefix}

	if prefix == filterPrefix {
		m.overlay = overlayFilter
		m.prompt.action = promptFilter
		m.prompt.text = m.filter
	}

	m.layout()

	return m, nil
}

func (m Model) closePrompt() (Model, tea.Cmd) {
	m.overlay = overlayNone
	m.prompt = promptModel{}
	m.layout()

	return m, nil
}

// promptKey answers every key while the command line is open. Nothing of it
// reaches the screen underneath.
func (m Model) promptKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.overlay == overlayFilter {
			m.filter = ""
		}

		return m.closePrompt()

	case "enter":
		return m.commit()

	case "tab", "right", "ctrl+f":
		m.prompt.text += m.prompt.suggestion()

	case "backspace":
		if n := len(m.prompt.text); n > 0 {
			m.prompt.text = m.prompt.text[:n-1]
		}

	case "ctrl+u":
		m.prompt.text = ""

	default:
		if msg.Text != "" {
			m.prompt.text += msg.Text
		}
	}

	if m.overlay == overlayFilter {
		m.filter = m.prompt.text
		m.layout()
	}

	return m, nil
}

// commit does what the line says.
func (m Model) commit() (Model, tea.Cmd) {
	input := m.prompt.text
	action, group := m.prompt.action, m.prompt.group

	if action == promptFilter {
		m.filter = input

		return m.closePrompt()
	}

	m, _ = m.closePrompt()

	if action == promptScale {
		return m.scaleTo(group, input)
	}

	cmd, ok := parseCommand(input)
	if !ok {
		m.err = fmt.Errorf("no such resource: %s", firstWord(input))

		return m, nil
	}

	if cmd.bail {
		return m, tea.Quit
	}

	if cmd.namespace != "" {
		next, ok := m.useNamespace(cmd.namespace)
		if !ok {
			m.err = fmt.Errorf("no such namespace: %s", cmd.namespace)

			return m, nil
		}

		m = next
	}

	return m.show(cmd.kind)
}

// useNamespace switches the session to a namespace the cluster knows.
func (m Model) useNamespace(namespace string) (Model, bool) {
	for _, known := range m.namespaces {
		if known.Name == namespace {
			m.namespace = namespace
			m.screen.namespace = namespace
			m.filter = ""

			return m, true
		}
	}

	return m, false
}

func firstWord(input string) string {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return ""
	}

	return fields[0]
}

// troubledRows keeps the rows that are painted as not right: the color a row
// carries already says whether the cluster is happy with it.
func troubledRows(rows []tableRow, index []int) ([]tableRow, []int) {
	kept := make([]tableRow, 0, len(rows))
	keptIndex := make([]int, 0, len(index))

	for i, row := range rows {
		if row.color != colorDead && row.color != colorAttention {
			continue
		}

		kept = append(kept, row)

		if i < len(index) {
			keptIndex = append(keptIndex, index[i])
		}
	}

	return kept, keptIndex
}

// filterRows keeps the rows that say the text somewhere, and remembers where
// each of them came from, so that the cursor still points at the right
// resource.
func filterRows(rows []tableRow, filter string) ([]tableRow, []int) {
	index := make([]int, 0, len(rows))

	if filter == "" {
		for i := range rows {
			index = append(index, i)
		}

		return rows, index
	}

	match := matcher(filter)

	kept := make([]tableRow, 0, len(rows))
	for i, row := range rows {
		if match(strings.Join(row.cells, " ")) {
			kept = append(kept, row)
			index = append(index, i)
		}
	}

	return kept, index
}

// matcher reads the filter as a pattern, and as plain text when it is not
// one.
func matcher(filter string) func(string) bool {
	rx, err := regexp.Compile("(?i)" + filter)
	if err != nil {
		lower := strings.ToLower(filter)

		return func(s string) bool { return strings.Contains(strings.ToLower(s), lower) }
	}

	return rx.MatchString
}
