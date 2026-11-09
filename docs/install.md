# Install

This page covers installing urga from a package, a release archive, Go or
the source. To point it at a cluster once it is installed, see
[Configuration](configuration.md).

## macOS

With Homebrew:

```sh
brew install ingvarch/tap/urga
```

To update, run `brew upgrade ingvarch/tap/urga`.

## Debian, Ubuntu, Fedora and RHEL

Download the `.deb` or `.rpm` package from the
[releases](https://github.com/ingvarch/urga/releases) page and install it:

```sh
sudo apt install ./urga_*_amd64.deb
sudo dnf install ./urga-*.x86_64.rpm
```

The packages are built for amd64, arm64 and 32-bit ARM.

## Arch Linux

Install from the AUR:

```sh
# https://aur.archlinux.org/packages/urga
yay -S urga
```

A member of the community keeps the AUR package, and it can be a few releases
behind the latest one.

## Other systems

The [releases](https://github.com/ingvarch/urga/releases) page has an archive
for each of these:

| System | Architectures |
| --- | --- |
| Linux | amd64, arm64, 32-bit ARM |
| macOS | amd64, arm64 |
| Windows | amd64, arm64 |
| FreeBSD | amd64 |

Or install with Go, the version named in `go.mod`:

```sh
go install github.com/ingvarch/urga/cmd/urga@latest
```

A copy built with `go install` shows its version as `dev` and does not check
for [newer releases](configuration.md#newer-releases).

## From source

```sh
make build && ./bin/urga
```

What the build needs and how to run the checks is in
[Development](development.md).
