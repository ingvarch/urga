package ui

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// savedMsg is the file the screen was saved to.
type savedMsg struct{ path string }

// saveText writes what the screen shows to a file next to where urga runs,
// so that a log or a description can be read after urga is closed.
func (m Model) saveText() tea.Cmd {
	if !m.readsAsText() {
		return nil
	}

	name := saveName(m.screen.page)

	// What is saved is what is shown: the filter, the wrapping and the times
	// are how the screen was narrowed down to what matters.
	rows := m.text.rows()

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.text)
	}

	content := strings.Join(lines, "\n")

	return request(func(context.Context) (savedMsg, error) {
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			return savedMsg{}, err
		}

		return savedMsg{path: name}, nil
	}, func(msg savedMsg) tea.Msg { return msg })
}

// saveName names the file by what it is of and when it was saved, and
// leaves out everything a file name cannot hold.
func saveName(p page) string {
	what, extension := "", "txt"
	if s, ok := p.(saving); ok {
		what, extension = s.saveAs()
	}

	return fmt.Sprintf("%s-%s.%s", plainName(what), time.Now().Format("20060102-150405"), extension)
}

var notInAName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func plainName(what string) string {
	name := strings.Trim(notInAName.ReplaceAllString(what, "-"), "-")
	if name == "" {
		return "urga"
	}

	return name
}
