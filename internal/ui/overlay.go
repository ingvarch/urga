package ui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/ingvarch/urga/internal/nomad"
)

// overlay is what took the keyboard from the screen. The root model decides
// who gets a key, a component never installs a hook of its own.
type overlay int

const (
	overlayNone overlay = iota
	overlayPrompt
	overlayFilter
	overlayScale
	overlaySignal
	overlayHelp
	overlayConfirm
)

// asksForALine says the overlay is the line at the top, whatever it is
// asking for.
func (o overlay) asksForALine() bool {
	return o == overlayPrompt || o == overlayFilter || o == overlayScale || o == overlaySignal
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

	// group is the task group a count belongs to, as it was when the count
	// was asked for; task is the task a signal is for.
	group nomad.TaskGroup
	task  string

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
	if !strings.HasPrefix(offered, strings.ToLower(firstWord(p.text))) {
		return ""
	}

	return offered
}

// split is the line in three: the word of the resource as the prompt would
// write it, the rest of that word which was not typed, and whatever the line
// says after it.
func (p promptModel) split() (word, tail, rest string) {
	word, rest = p.text, ""
	if at := strings.IndexByte(p.text, ' '); at >= 0 {
		word, rest = p.text[:at], p.text[at:]
	}

	offered := p.choice()
	if offered == "" {
		return word, "", rest
	}

	// The resource is spelled the way the cluster spells it, whatever the
	// keyboard was in: the line has to read as one word.
	return offered[:len(word)], offered[len(word):], rest
}

// line is what the command line reads as.
func (p promptModel) line() string {
	word, tail, rest := p.split()

	return word + tail + rest
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

	// A resource that is still among them stays the one that is offered:
	// typing where to look must not walk the resource back.
	chosen := ""
	if p.at >= 0 && p.at < len(p.matches) {
		chosen = p.matches[p.at]
	}

	p.matches = matchingCommands(p.text)

	// What is typed is the first word; a line that has none of it, a space
	// for instance, names nothing yet.
	p.at = 0
	if firstWord(p.text) == "" || len(p.matches) == 0 {
		p.at = -1
	}

	// A word that is an alias of its own settles the question: typing it in
	// full outranks whatever was walked to before.
	if _, exact := commandAliases[strings.ToLower(firstWord(p.text))]; exact {
		return p
	}

	for i, match := range p.matches {
		if match == chosen {
			p.at = i
		}
	}

	return p
}

func (p promptModel) view(width int) string {
	word, tail, rest := p.split()

	// The cursor sits after the whole line, so the word the prompt offers
	// reads whole.
	line := styleTitle.Render(p.prefix) + styleText.Render(word) +
		styleMuted.Render(tail) + styleText.Render(rest) + styleSelected.Render(" ")

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
		m.prompt.text = m.prompt.line()
		m.prompt = m.prompt.narrow()

	// A terminal set to send ^H for backspace hands it over as ctrl+h. A
	// letter can take more than one byte, and goes as a whole.
	case "backspace", "ctrl+h":
		_, size := utf8.DecodeLastRuneInString(m.prompt.text)
		m.prompt.text = m.prompt.text[:len(m.prompt.text)-size]
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

	return m.followFilter(), nil
}

// followFilter narrows the table to the line being typed. Only the filter
// changes what is on the table under the line.
func (m Model) followFilter() Model {
	if m.overlay == overlayFilter {
		m.filter = m.prompt.text
		m.layout()
	}

	return m
}

// paste puts pasted text on the line being typed, as if it were typed. With
// no line open nothing takes text, and a paste presses no keys.
func (m Model) paste(content string) (Model, tea.Cmd) {
	if !m.overlay.asksForALine() {
		return m, nil
	}

	m.prompt.text += oneLine(content)
	m.prompt = m.prompt.narrow()

	return m.followFilter(), nil
}

// oneLine is pasted text as a line can hold it. A copied line brings its
// break along, which is dropped; breaks and tabs inside become spaces, and
// what would move the terminal around is left out.
func oneLine(text string) string {
	text = strings.TrimRight(text, "\r\n")
	text = strings.ReplaceAll(text, "\r\n", "\n")

	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r == utf8.RuneError || unicode.IsControl(r):
			return -1
		}

		return r
	}, text)
}

// commit does what the line says, which is what it offers when it offers
// anything.
func (m Model) commit() (Model, tea.Cmd) {
	input := m.prompt.text
	if m.overlay == overlayPrompt {
		input = m.prompt.line()
	}

	asked, group, task := m.overlay, m.prompt.group, m.prompt.task

	if asked == overlayFilter {
		m.filter = input

		return m.closePrompt()
	}

	m, _ = m.closePrompt()

	if asked == overlayScale {
		return m.scaleTo(group, input)
	}

	if asked == overlaySignal {
		return m.signalTask(task, input)
	}

	cmd, ok := parseCommand(input)
	if !ok {
		return m.fail(fmt.Errorf("no such resource: %s", firstWord(input))), nil
	}

	if cmd.bail {
		return m, tea.Quit
	}

	switch cmd.switching {
	case scopeRegion:
		return m.regionCommand(cmd.name)

	case scopeDatacenter:
		return m.datacenterCommand(cmd.name)

	case scopeCluster:
		return m.clusterCommand(cmd.name)
	}

	if cmd.namespace != "" {
		if !m.knowsNamespace(cmd.namespace) {
			return m.fail(fmt.Errorf("no such namespace: %s", cmd.namespace)), nil
		}

		m.namespace = cmd.namespace
		m.screen.namespace = cmd.namespace
	}

	return m.show(cmd.kind)
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

// matchIn is where a filter matches in a line, or nil when it does not. An
// empty match is nothing to light up.
func matchIn(line, filter string) []int {
	// A line kept because it does not say the word has nothing in it to
	// light up, and the letters of a fuzzy filter sit all over it.
	if strings.HasPrefix(filter, filterNot) || strings.HasPrefix(filter, filterFuzzy) {
		return nil
	}

	rx, err := regexp.Compile("(?i)" + filter)
	if err != nil {
		at := strings.Index(strings.ToLower(line), strings.ToLower(filter))
		if at < 0 || filter == "" {
			return nil
		}

		return []int{at, at + len(filter)}
	}

	at := rx.FindStringIndex(line)
	if at == nil || at[0] == at[1] {
		return nil
	}

	return at
}

// How a filter can be written, after the way k9s writes them.
const (
	// filterNot keeps what does not say it.
	filterNot = "!"

	// filterFuzzy keeps what has the letters in that order, with anything
	// between them.
	filterFuzzy = "-f "
)

// matcher reads the filter: what to keep out, what to find loosely, and
// otherwise a pattern, or plain text when the pattern does not compile.
func matcher(filter string) func(string) bool {
	if rest, ok := strings.CutPrefix(filter, filterNot); ok {
		if rest == "" {
			return everything
		}

		keep := matcher(rest)

		return func(s string) bool { return !keep(s) }
	}

	if rest, ok := strings.CutPrefix(filter, filterFuzzy); ok {
		return fuzzy(rest)
	}

	if filter == "" {
		return everything
	}

	rx, err := regexp.Compile("(?i)" + filter)
	if err != nil {
		lower := strings.ToLower(filter)

		return func(s string) bool { return strings.Contains(strings.ToLower(s), lower) }
	}

	return rx.MatchString
}

func everything(string) bool { return true }

// fuzzy keeps what carries the letters in that order, however far apart.
func fuzzy(letters string) func(string) bool {
	wanted := []rune(strings.ToLower(letters))

	return func(s string) bool {
		at := 0

		for _, r := range strings.ToLower(s) {
			if at < len(wanted) && r == wanted[at] {
				at++
			}
		}

		return at == len(wanted)
	}
}
