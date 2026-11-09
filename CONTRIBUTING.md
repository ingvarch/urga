# Contributing to urga

Bug reports, ideas and pull requests are welcome. Everyone who takes part
follows the [Code of Conduct](CODE_OF_CONDUCT.md). A security problem is
reported privately, not in an issue: see [SECURITY.md](SECURITY.md).

## Report a bug or ask for a feature

Open an issue with one of the
[issue forms](https://github.com/ingvarch/urga/issues/new/choose). A bug
report needs what you did, what you saw and what you expected, the steps to
get there, and the versions of urga and Nomad: the form asks for each.

## Before you write code

For anything bigger than a small fix, open an issue first, so the shape of the
change is agreed before the work is done.

## Build and run

What the build needs, how to run urga against a local cluster and the checks
are in [docs/development.md](docs/development.md).

## How a change is made

- Work on a branch from `main` named `<type>/<short-name>`, for example
  `fix/cursor-after-filter`.
- The test comes first. Write the test that names the behaviour, watch it
  fail, then write the smallest code that passes it. This holds for small
  functions too.
- `make check` is green before every commit.
- A change a user can see updates, in the same commit, the guide in `docs/`
  that describes it.
- The [architecture rules](AGENTS.md#architecture-rules) are settled. A change
  that needs to break one starts as an issue.
- The interface is English only. There is no translation layer.
- Comments are short and say why, not what the line does.
- The code is written here. Do not paste code from projects under a license
  other than MIT.

## Commits and pull requests

Commit messages and pull request titles are conventional commits, with the
part of urga as the scope: `feat(ui): ...`, `fix(nomad): ...`,
`chore(ci): ...`. The scopes are `ui`, `nomad`, `config`, `settings`, `cmd`,
`release` and `ci`.

One pull request is one change. Fill in the template and link the issue it
closes with `Closes #N`. Every push runs the checks on Linux, the tests on
Linux, macOS and Windows, and the whole release without publishing it. A pull
request needs all of them green. Pull requests are merged with rebase or
squash, so `main` keeps a linear history.

## Releases

A maintainer pushes a `vX.Y.Z` tag. The tag publishes the release, the deb and
rpm packages and the Homebrew cask; a pull request changes nothing for that.

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE).
