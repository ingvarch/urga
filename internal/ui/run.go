package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// run reads a job file and opens the plan of it as a new job.
func (m Model) run(path string) (Model, tea.Cmd) {
	if path == "" {
		return m.warn(":run needs a job file"), nil
	}

	client, namespace := m.client, m.namespace

	return m, request(func(ctx context.Context) (planPage, error) {
		source, err := readJobFile(path)
		if err != nil {
			return planPage{}, err
		}

		return planNewJob(ctx, client, namespace, source)
	}, func(page planPage) tea.Msg { return openMsg{page} })
}

// readJobFile reads the file at path, which can start with ~/ for the home
// directory.
func readJobFile(path string) (string, error) {
	full, err := localPath(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}

	content, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, reason(err))
	}

	return string(content), nil
}

// localPath is a path typed by a person: ~/ is the home directory, anything
// else is as typed, so a relative path is from where urga runs.
func localPath(typed string) (string, error) {
	rest, ok := strings.CutPrefix(typed, "~/")
	if !ok {
		return typed, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, rest), nil
}

// reason leaves out the path a *fs.PathError repeats: the message already
// names it.
func reason(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}

	return err
}
