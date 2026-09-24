package ui

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// savedMsg is where what was on the screen went.
type savedMsg struct{ path string }

// saveText writes what the screen shows to a file next to where urga runs,
// so that a log or a description can be read after urga is closed.
func (m Model) saveText() tea.Cmd {
	if !m.readsAsText() {
		return nil
	}

	name := saveName(m.screen)

	// What is kept is what is read: the filter, the wrapping and the times
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

// saveName says what the file is of and when it was taken, and keeps out
// everything a file name cannot hold.
func saveName(s screen) string {
	what, extension := s.label, "txt"

	switch s.kind {
	case screenLogs:
		what, extension = fmt.Sprintf("%s-%s", s.task, s.source), "log"
	case screenFile:
		what = path.Base(s.path)
	case screenJobLogs:
		what, extension = fmt.Sprintf("%s-%s-%s", s.jobID, s.task, s.source), "log"
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
