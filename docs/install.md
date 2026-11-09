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

## Check a download

Each release page has a file of checksums, `urga_<version>_checksums.txt`,
for every archive and package. Download it into the folder with your
download, and run there:

```sh
# Linux
sha256sum --check --ignore-missing urga_*_checksums.txt
# macOS
shasum -a 256 --check --ignore-missing urga_*_checksums.txt
```

Each file you downloaded should read `OK`. On Windows, in PowerShell, print
the hash of the archive and its line in the file, and compare the two:

```powershell
Get-FileHash urga_*_windows_amd64.zip
Select-String windows_amd64.zip urga_*_checksums.txt
```

The check shows that the file is the one the release published, not damaged
or cut short on the way.

The macOS binary is also signed and notarized by Apple. To see it, run in the
folder of the unpacked archive:

```sh
spctl --assess --type open --context context:primary-signature -v ./urga
```

It prints `accepted` and `source=Notarized Developer ID`.

## From source

```sh
make build && ./bin/urga
```

What the build needs and how to run the checks is in
[Development](development.md).
