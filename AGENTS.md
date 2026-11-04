# AGENTS.md

How to work on urga, for any coding agent. CLAUDE.md imports this file.

## What urga is

A terminal UI for HashiCorp Nomad: one binary, `urga`, that lists the
resources of a cluster and acts on them.

The interface is English only, with no translation layer and no plan for one.
User-visible strings are literals where they are used.

## Stack

Go, Bubble Tea v2 for the program loop, Lipgloss v2 for styles and
`github.com/hashicorp/nomad/api` for the cluster. Charm's v2 modules are
imported from `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2`: their
GitHub paths resolve to v1, which does not build against this code.

## Architecture rules

These are settled, not open for redesign per change. A change that needs to
break one starts as an issue. Each comes from a fork of hashicorp/damon, where
the opposite caused a bug that took hours to find.

1. **One owner for the keyboard.** The root model decides who gets a key
   press. Overlays (prompt, filter, help, dialog) are fields of the model, in
   an explicit order. Each key is handled in one place, and no component
   installs a global key hook.
2. **The namespace travels with the request.** Every call into
   `internal/nomad` takes the namespace as an argument. A request never reads
   a "current namespace" from global state.
3. **Polling lives in `tea.Cmd`.** A poll returns a message. A goroutine never
   writes to the model, never draws, and never blocks sending on a channel
   the UI also reads. A timer starts only through `startTimer`
   (`internal/ui/timer.go`).
4. **Events say when, reads say what.** The event stream marks a screen
   stale, and the screen is read again the usual way. A cluster that does not
   stream is polled as before. Event payloads are read in two places only:
   `:events`, which shows the events as they came, and the failure note on
   the status line. No screen is built out of them.
5. **Screen keys in the header, general keys in help.** The header lists only
   what the open resource can do. Navigation and global keys live in `?`.
6. **Render functions are pure.** A screen turns state plus a width into a
   string, and tests assert on that string. The time comes from the model
   (`m.now`, set at the top of update): `clockReaders` in
   `internal/ui/clock_test.go` lists the functions that may read the clock.
7. **The table is ours** (`internal/ui/table.go`). Every screen is a list that
   needs columns as wide as their content, a color per row for its state, and
   a cursor row that reads. A cell color nested inside a highlight leaves the
   row unreadable, so the cursor row is painted end to end.

## Code from elsewhere

The code is written here. hashicorp/damon is MPL-2.0 with an IBM copyright
header in every file: carry over its decisions and lessons, and write the
code from scratch. The same holds for any project under a license other
than MIT.

## Process

- TDD: a failing test that names the behaviour comes first, then the smallest
  code that passes it. Small functions too.
- `make check` (fmt, vet, lint, licenses, test, build) is green before every
  commit. It needs `golangci-lint`, the latest release, which CI lints with.
- A test in `internal/ui` whose code writes a temporary file calls
  `tempDir(t)` (`internal/ui/edit_test.go`). It points TMPDIR, TMP and TEMP
  at the test's own directory, so the file goes away with the test.
- Commit messages and PR titles are conventional commits, with the part of
  urga as the scope: `feat(ui): ...`, `fix(nomad): ...`, `chore(ci): ...`.
  A PR body fills `.github/pull_request_template.md`.
- Commits and PRs carry no AI attribution: no `Co-Authored-By` for an
  assistant, no "Generated with".
- Releases are the maintainer's: only they push a `vX.Y.Z` tag.

## Comments

- Short, and they say why. What the line does is left to the line.
- Plain words that name what the code does ("closes the stream", "drops the
  answer", "shows the error"), not figures of speech ("lets go of it", "says
  so"). The terms of the code stay as they are: page, screen, stack, ask,
  poll, stream, watch.
- Comments and commit messages explain urga in its own terms and name no
  other program it resembles: not "the way k9s does it".
- Existing comments stay. A comment goes only together with the code it
  describes.
