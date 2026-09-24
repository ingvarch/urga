# Contributing to urga

Bug reports, ideas and pull requests are welcome. Everyone who takes part
follows the [Code of Conduct](CODE_OF_CONDUCT.md). A security problem is
reported privately, not in an issue: see [SECURITY.md](SECURITY.md).

## Before you write code

For anything bigger than a small fix, open an issue first, so the shape of the
change is agreed before the work is done.

## Build and run

You need Go, the version named in `go.mod`. A local Nomad dev agent is enough
to point urga at, and urga finds it on `http://127.0.0.1:4646` without flags:

```sh
nomad agent -dev
make run
```

```sh
make check   # fmt, vet, lint, test, build
make dist    # the whole release, published nowhere
```

`make lint` runs golangci-lint when it is installed; CI always runs it.
`make dist` needs goreleaser, the version named in `.tool-versions`.

## How a change is made

- The test comes first. Write the test that names the behaviour, watch it
  fail, then write the smallest code that passes it. This holds for small
  functions too.
- `make check` is green before every commit.
- The [architecture rules](CLAUDE.md#architecture-rules) are settled. A change
  that needs to break one starts as an issue.
- The interface is English only. There is no translation layer.
- Comments are short and say why, not what the line does.
- The code is written here. Do not paste code from projects under a license
  other than MIT.

## Commits and pull requests

Commit messages and pull request titles are conventional commits, with the
part of urga as the scope: `feat(ui): ...`, `fix(nomad): ...`,
`chore(ci): ...`.

One pull request is one change. Every push runs the checks on Linux, the tests
on Linux, macOS and Windows, and the whole release without publishing it. A
pull request needs all of them green.

## Releases

A maintainer pushes a `vX.Y.Z` tag. The tag publishes the release, the deb and
rpm packages and the Homebrew cask; a pull request changes nothing for that.
