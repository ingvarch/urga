# Development

This page covers what you need to build urga, how to run it against a local
cluster, the checks and where the code lives. How a change is proposed and
sent is in [CONTRIBUTING.md](../CONTRIBUTING.md).

## Prerequisites

- Go, the version named in `go.mod`.
- Nomad, for a local dev agent to point urga at.
- golangci-lint, the version named in `.tool-versions`, which CI lints with.
  `make lint` fails when it is not installed.
- goreleaser, the version named in `.tool-versions`, for `make dist` only.
- nomad, tmux, curl and freeze, for `make screens` only.

## Run

A local Nomad dev agent is enough to point urga at, and urga finds it on
`http://127.0.0.1:4646` without flags. Start the agent in one terminal and
urga in another:

```sh
nomad agent -dev
make run
```

## Checks

```sh
make check   # fmt, vet, lint, licenses, test, build
make dist    # the whole release, published nowhere
make screens # the pictures in the README, from a cluster of their own
```

`make screens` takes about seven minutes: the charts of the client fill for
six.

`make licenses` fails when a module urga links has a license outside the
list in `internal/licenses`. `make notices` also writes `THIRD_PARTY_NOTICES`,
which the release ships.

## Project structure

| Path | What it holds |
| --- | --- |
| `cmd/urga` | The binary: flags, environment, the settings file, the start. |
| `internal/ui` | The terminal interface: one root model that owns the screen and the keyboard. |
| `internal/nomad` | Calls to the cluster. Each takes the namespace it asks in. |
| `internal/config` | What urga remembers between runs: the saved session. |
| `internal/settings` | The optional settings file of named clusters. urga reads it and never writes it. |
| `internal/release` | The check for a newer release and its notes. |
| `internal/version` | What the build stamps into the binary. |
| `internal/licenses` | The license check and the third-party notices. It is not linked into urga. |
| `internal/buildconfig` | Tests that read the Makefile, the workflows and the release config. |
| `scripts/screens.sh` | Takes the pictures in the README. |

The architecture rules the code keeps are in
[AGENTS.md](../AGENTS.md#architecture-rules).
