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
	switch {
	case m.opts.ReadOnly:
		return m.warn("read-only: :run is off"), nil

	case path == "":
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
	full := path

	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", path, err)
		}

		full = filepath.Join(home, rest)
	}

	content, err := os.ReadFile(full)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			err = pathErr.Err
		}

		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}

	return string(content), nil
}
