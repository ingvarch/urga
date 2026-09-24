<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/logo-dark.png">
  <img src="docs/images/logo-light.png" alt="urga" width="112">
</picture>

# urga

**A terminal UI for HashiCorp Nomad.**
One binary, no config to write, no browser.

[![ci](https://github.com/ingvarch/urga/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/ingvarch/urga/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/ingvarch/urga)](https://github.com/ingvarch/urga/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/ingvarch/urga)](go.mod)
[![license](https://img.shields.io/github/license/ingvarch/urga)](LICENSE)

</div>

![The job list, with each job colored by its state and the keys of the screen at the top](docs/images/jobs.png)

![A client, with charts of its CPU and memory use and the allocations it runs](docs/images/client.png)

## What it does

- Lists jobs, allocations, tasks, task groups, deployments, evaluations,
  services, namespaces, variables, clients, node pools and servers.
- Updates the screen when the cluster changes, using the Nomad event stream.
- Shows task logs and the files of an allocation, and follows them as they
  grow. Shows the log of a task from every allocation of a job at once.
- Edits a job in your editor and shows the plan before it submits anything.
- Shows the versions of a job and reverts to any of them.
- Explains why a job is not placed.
- Starts, stops, scales and restarts jobs and allocations, restarts and
  signals single tasks, and opens a shell in a task.
- Drains clients and shows their CPU and memory over time.
- Works in read-only mode, and with several named clusters.

## Install

On macOS, with Homebrew:

```sh
brew install ingvarch/tap/urga
```

On Debian, Ubuntu, Fedora or RHEL, download the `.deb` or `.rpm` package from
the [releases](https://github.com/ingvarch/urga/releases) page and install it:

```sh
sudo apt install ./urga_*_amd64.deb
sudo dnf install ./urga-*.x86_64.rpm
```

For other systems (Linux, macOS, Windows and FreeBSD, on amd64 and ARM),
download an archive from the same page, or install with Go:

```sh
go install github.com/ingvarch/urga/cmd/urga@latest
```

To build from source:

```sh
make build && ./bin/urga
```

## Quick start

urga uses the same environment variables as the `nomad` CLI. No
configuration file is needed.

```sh
export NOMAD_ADDR=https://nomad.example.com:4646
export NOMAD_TOKEN=<your token>
urga
```

Press `?` for help, `:` to open another screen, `/` to filter and `q` to
quit.

## Documentation

- [Configuration](docs/configuration.md): environment variables, flags,
  read-only mode, the editor, and the saved session.
- [Named clusters](docs/clusters.md): an optional settings file for several
  clusters, and switching between them.
- [Keys and commands](docs/keys.md): the command line, the filter, and the
  keys of every screen.
- [Jobs](docs/jobs.md): jobs, task groups, editing and plans, versions,
  evaluations and deployments.
- [Allocations and tasks](docs/allocations.md): allocations, checks, task
  actions, logs and files.
- [Clients, servers and other resources](docs/clients.md): clients,
  servers, namespaces, services, variables and node pools.

## Contributing

How to build urga, run the checks and send a change is in
[CONTRIBUTING.md](CONTRIBUTING.md). Security problems are reported privately,
see [SECURITY.md](SECURITY.md).

## License

MIT, see [LICENSE](LICENSE).

## About the name

An urga is a Mongolian catch pole: a long wooden shaft with a loop of rope at
the end. A rider holds it out at a gallop and takes the one horse he wants out
of a herd of hundreds that is still moving.

A cluster is that herd. This is what you reach into it with.
