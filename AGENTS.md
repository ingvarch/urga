# AGENTS.md

urga is a terminal UI for HashiCorp Nomad: one Go binary, `urga`, that lists
the resources of a cluster and acts on them. `cmd/urga` starts it,
`internal/ui` is the interface on Bubble Tea v2 and Lipgloss v2, and
`internal/nomad` talks to the cluster through `github.com/hashicorp/nomad/api`.
Every change starts with a failing test, keeps the architecture rules below
and leaves `make check` green. The interface is English only, with no
translation layer and no plan for one: user-visible strings are literals
where they are used.

- [CONTRIBUTING.md](CONTRIBUTING.md): read before writing a change. It has
  the test-first rule, the comment style, commit and PR titles, the checks a
  PR needs and how releases are published.
- [docs/development.md](docs/development.md): read before building, running
  urga against a local cluster or running the checks. It has the
  prerequisites, the dev agent, the make targets and the project structure.
- [docs/keys.md](docs/keys.md): read before adding or changing a key or a
  command. It has the command line, the overview, find, events, sort, the
  filter and the keys of every screen.
- [docs/configuration.md](docs/configuration.md): read before changing a
  flag, an environment variable, read-only mode, the token, the update check,
  the editor, how screens poll and follow the event stream, or the saved
  session.
- [docs/clusters.md](docs/clusters.md): read before changing the settings
  file or switching clusters. It has where the file is, its settings, tokens,
  certificates and its errors.
- [docs/jobs.md](docs/jobs.md): read before changing a screen of jobs. It has
  periodic and parameterized jobs, task groups and scaling, editing and plans,
  versions, evaluations, deployments and watching a deploy.
- [docs/allocations.md](docs/allocations.md): read before changing a screen
  of allocations or tasks. It has checks, task actions, logs and files.
- [docs/clients.md](docs/clients.md): read before changing a screen of
  clients, servers, agent logs, the scheduler, namespaces, services,
  variables or node pools.
- [docs/storage.md](docs/storage.md): read before changing a screen of
  volumes or CSI plugins.
- [docs/acl.md](docs/acl.md): read before changing a screen of ACL tokens,
  policies, roles, auth methods or binding rules.
- [docs/install.md](docs/install.md): read before changing the release config
  or the packages. It has each way to install urga and the platforms the
  release builds for.

## Commands

`make check` runs the tests without the race detector. CI also runs them with
it, on Linux, macOS and Windows:

```sh
go test -race ./...
```

## Rules

### Architecture rules

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
   (`internal/ui/timer.go`). A command waits on a stream only through `waitOn`
   (`internal/ui/stream.go`).
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

### Code

- Charm's v2 modules are imported from `charm.land/bubbletea/v2` and
  `charm.land/lipgloss/v2`: their GitHub paths resolve to v1, which does not
  build against this code.
- The code is written here. hashicorp/damon is MPL-2.0 with an IBM copyright
  header in every file: carry over its decisions and lessons, and write the
  code from scratch. The same holds for any project under a license other
  than MIT.
- A test in `internal/ui` whose code writes a temporary file calls
  `tempDir(t)` (`internal/ui/edit_test.go`). It points TMPDIR, TMP and TEMP
  at the test's own directory, so the file goes away with the test.
- A change a user can see updates, in the same commit, the guide in `docs/`
  that describes it.

### Comments

- Plain words that name what the code does ("closes the stream", "drops the
  answer", "shows the error"), not figures of speech ("lets go of it", "says
  so"). The terms of the code stay as they are: page, screen, stack, ask,
  poll, stream, watch.
- Comments and commit messages explain urga in its own terms and name no
  other program it resembles: not "the way k9s does it".
- Existing comments stay. A comment goes only together with the code it
  describes.

### Commits and releases

- A PR body fills `.github/pull_request_template.md`.
- Releases are the maintainer's: only they push a `vX.Y.Z` tag.
