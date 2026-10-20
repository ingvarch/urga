package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// savedMsg is the file the screen was saved to.
type savedMsg struct{ path string }

// askSave asks where to save what the screen shows, with a name already
// typed, so that a log or a description can be read after urga is closed.
func (m Model) askSave() (Model, tea.Cmd) {
	if !m.readsAsText() {
		return m, nil
	}

	return m.askLine(lineMsg{prefix: "save to: ", text: saveName(m.screen.page), answer: saveTo})
}

// saveTo saves what the screen shows when enter is pressed, not when the
// line was opened: a log goes on while it is typed.
func saveTo(m Model, typed string) (Model, tea.Cmd) {
	path := strings.TrimSpace(typed)
	if path == "" {
		return m.warn("Saving needs a path."), nil
	}

	return m, saveFile(path, strings.Join(m.shownLines(), "\n"), false)
}

// saveFile writes content to path. A file that is there is asked about
// first, unless overwrite says it was.
func saveFile(path, content string, overwrite bool) tea.Cmd {
	return request(func(context.Context) (tea.Msg, error) {
		abs, err := writeText(path, content, overwrite)
		if errors.Is(err, errThere) {
			return askMsg{question: "Really overwrite " + abs + "?", apply: saveFile(path, content, true)}, nil
		}

		if errors.Is(err, errFolder) {
			return nil, fmt.Errorf("cannot save to %s: %w", abs, reason(err))
		}

		if err != nil {
			return nil, fmt.Errorf("cannot save to %s: %w", path, reason(err))
		}

		return savedMsg{path: abs}, nil
	}, func(msg tea.Msg) tea.Msg { return msg })
}

var (
	errFolder = errors.New("it is a folder")
	errThere  = errors.New("a file is there")
)

// writeText makes the folders of path and writes the file. It returns
// errThere when the file is there and overwrite is not set: fs.ErrExist
// also comes from MkdirAll, for a folder that is a broken link.
func writeText(path, content string, overwrite bool) (string, error) {
	full, err := localPath(path)
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(abs)

	switch {
	case err == nil && info.IsDir():
		return abs, errFolder
	case err == nil && !overwrite:
		return abs, errThere
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return abs, err
	}

	return abs, os.WriteFile(abs, []byte(content), 0o600)
}

// shownLines is the text as the screen shows it. What is saved or copied is
// what is shown: the filter, the wrapping and the times are how the screen
// was narrowed down to what matters.
func (m Model) shownLines() []string {
	rows := m.text.rows()

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.text)
	}

	return lines
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
