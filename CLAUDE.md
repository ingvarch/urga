# CLAUDE.md

Guidance for Claude Code working in this repository.

## What urga is

A terminal UI for HashiCorp Nomad. One binary, `urga`, that lists and acts on
cluster resources: jobs, allocations, tasks, task groups, logs, deployments,
namespaces, services, evaluations, nodes, variables and node pools.

The interface is English only. There is no translation layer and no plan for
one; write literals where they belong.

## Stack

- Go, module path `github.com/ingvarch/urga` (module path equals repo path, always)
- Bubble Tea v2 (`charm.land/bubbletea/v2`) for the program loop
- Lipgloss v2 (`charm.land/lipgloss/v2`) for styles
- Bubbles v2 (`charm.land/bubbles/v2`) for table, viewport and text input
- `github.com/hashicorp/nomad/api` for the cluster

Charm moved the v2 modules to `charm.land/...`. The GitHub path resolves to v1
and does not build against this code.

## Architecture rules

These are settled, not open for redesign per change. They come from a fork of
hashicorp/damon where the opposite of each one caused a bug that took hours.

1. **One owner for the keyboard.** The root model decides who gets a key press.
   Overlays (prompt, filter, help, dialog) are fields of the model with an
   explicit order. No component installs a global key hook, and no key is
   handled in two places.
2. **The namespace travels with the request.** Every call into `internal/nomad`
   takes the namespace as an argument. Nothing reads a "current namespace" out
   of global state to build a request.
3. **Polling lives in `tea.Cmd`.** A poll returns a message. No goroutine writes
   to the model, no goroutine draws, no blocking send on a channel that the UI
   also reads.
4. **Screen keys in the header, general keys in help.** The header lists only
   what the open resource can do. Navigation and global keys live in `?`.
5. **Render functions are pure.** A screen turns state plus a width into a
   string. That is what tests assert on.

## Do not copy damon

hashicorp/damon is MPL-2.0 with an IBM copyright header in every file. Carry
over decisions and lessons, write the code here from scratch.

## Process

- TDD, no exceptions. A failing test that names the behaviour comes first, then
  the smallest code that passes it. This holds for "simple" functions too.
- `make check` (fmt, vet, lint, test, build) is green before every commit.
- Conventional commits: `feat(ui): ...`, `fix(nomad): ...`, `chore: ...`.
- No mention of other tools by name in code comments or commit messages.
- Comments are short and say why, never what the line already says. Existing
  comments are not deleted.
