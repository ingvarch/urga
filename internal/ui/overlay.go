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
	overlayScale
	overlayHelp
	overlayConfirm
)

// asksForALine says the overlay is the line at the top, whatever it is
// asking for.
func (o overlay) asksForALine() bool {
	return o == overlayPrompt || o == overlayFilter || o == overlayScale
}

// promptHeight is the line plus the border around it.
const promptHeight = 3

const (
	promptPrefix = ":"
	filterPrefix = "/"
)

// promptModel is the line at the top: a prefix, what was typed, and the rest
// of the word the prompt would complete. What the line is for is the overlay
// it was opened as, it is not said twice.
type promptModel struct {
	prefix string
	text   string

	// group is the task group a count belongs to.
	group string

	// suggest says whether the rest of a word is offered, which only the
	// command line does.
	suggest bool

	// matches are the resources the line could be about and at is the one
	// it offers. A line with nothing typed offers nothing until the arrows
	// ask it to: -1 is that.
	matches []string
	at      int
}

// choice is the resource the line offers, empty when it offers none. It is
// only ever a word the line reads as: what is on the screen is what enter
// opens, whatever order the keys were pressed in.
func (p promptModel) choice() string {
	if p.at < 0 || p.at >= len(p.matches) {
		return ""
	}

	offered := p.matches[p.at]
	if !strings.HasPrefix(offered, strings.ToLower(p.text)) {
		return ""
	}

	return offered
}

// walk moves through what the line could be about, and comes back around at
// either end.
func (p promptModel) walk(by int) promptModel {
	if len(p.matches) == 0 {
		p.at = -1

		return p
	}

	// From nothing, a step forward lands on the first resource and a step
	// back on the last.
	if p.at < 0 {
		p.at = -1
		if by < 0 {
			p.at = len(p.matches)
		}
	}

	p.at = (p.at + by + len(p.matches)) % len(p.matches)

	return p
}

// narrow keeps the resources the line still fits and offers the first of
// them. A line with nothing typed offers nothing yet: pressing the key that
// opened it must not put a resource in it.
func (p promptModel) narrow() promptModel {
	if !p.suggest {
		return p
	}

	p.matches = matchingCommands(p.text)

	// What is typed is the first word; a line that has none of it, a space
	// for instance, names nothing yet.
	p.at = 0
	if firstWord(p.text) == "" || len(p.matches) == 0 {
		p.at = -1
	}

	return p
}

// suggestion is the rest of the resource the line offers, which reads as one
// word with what was typed.
func (p promptModel) suggestion() string {
	offered := p.choice()
	if offered == "" {
		return ""
	}

	return offered[len(p.text):]
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
	m.prompt = promptModel{prefix: prefix, suggest: true, at: -1}
	m.prompt = m.prompt.narrow()

	if prefix == filterPrefix {
		m.overlay = overlayFilter
		m.prompt = promptModel{prefix: prefix, text: m.filter}
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

	case "up":
		m.prompt = m.prompt.walk(-1)

	case "down":
		m.prompt = m.prompt.walk(1)

	case "tab", "right", "ctrl+f":
		// What the line offers is taken into it, and the line stays open:
		// a namespace can follow the word. The word settles what fits it,
		// like any other way of changing the line.
		m.prompt.text += m.prompt.suggestion()
		m.prompt = m.prompt.narrow()

	case "backspace":
		if n := len(m.prompt.text); n > 0 {
			m.prompt.text = m.prompt.text[:n-1]
		}

		m.prompt = m.prompt.narrow()

	case "ctrl+u":
		m.prompt.text = ""
		m.prompt = m.prompt.narrow()

	default:
		if msg.Text != "" {
			m.prompt.text += msg.Text
			m.prompt = m.prompt.narrow()
		}
	}

	if m.overlay == overlayFilter {
		m.filter = m.prompt.text
	}

	m.layout()

	return m, nil
}

// commit does what the line says, which is what it offers when it offers
// anything.
func (m Model) commit() (Model, tea.Cmd) {
	input := m.prompt.text
	if offered := m.prompt.choice(); offered != "" && m.overlay == overlayPrompt {
		input = offered
	}

	asked, group := m.overlay, m.prompt.group

	if asked == overlayFilter {
		m.filter = input

		return m.closePrompt()
	}

	m, _ = m.closePrompt()

	if asked == overlayScale {
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
		if !m.knowsNamespace(cmd.namespace) {
			m.err = fmt.Errorf("no such namespace: %s", cmd.namespace)

			return m, nil
		}

		m.namespace = cmd.namespace
		m.screen.namespace = cmd.namespace
	}

	return m.show(cmd.kind)
}

// knowsNamespace says whether the cluster has a namespace by that name.
func (m Model) knowsNamespace(namespace string) bool {
	for _, known := range m.namespaces {
		if known.Name == namespace {
			return true
		}
	}

	return false
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
