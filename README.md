<div align="center">

<pre>
@@@  @@@ @@@@@@@   @@@@@@@   @@@@@@  
@@!  @@@ @@!  @@@ !@@       @@!  @@@ 
@!@  !@! @!@!!@!  !@! @!@!@ @!@!@!@! 
!!:  !!! !!: :!!  :!!   !!: !!:  !!! 
 :.:: :   :   : :  :: :: :   :   : : 
</pre>

**A terminal UI for HashiCorp Nomad.**
One binary, no config to write, no browser.

</div>

```
  Address:   https://nomad.example.com                           @@@  @@@ @@@@@@@   @@@@@@@   @@@@@@
  urga Rev:  v0.1.0                                              @@!  @@@ @@!  @@@ !@@       @@!  @@@
  Nomad Rev: 1.11.1                                              @!@  !@! @!@!!@!  !@! @!@!@ @!@!@!@!
  Namespace: all                                                 !!:  !!! !!: :!!  :!!   !!: !!:  !!!
                                                                  :.:: :   :   : :  :: :: :   :   : :
 ╭────────────────────────────────────────── Jobs (all) [3] ──────────────────────────────────────────╮
 │ ID              Name            Type     Namespace   Status   Allocs  Age                          │
 │ api             api             service  production  running  3/3     11d                          │
 │ nightly-import  nightly-import  batch    production  dead     0/0     7h                           │
 │ traefik         traefik         system   default     running  2/3     1h                           │
 ╰────────────────────────────────────────────────────────────────────────────────────────────────────╯
  q quit
```

## Install

```sh
go install github.com/ingvarch/urga/cmd/urga@latest
```

Or build from a clone:

```sh
make build && ./bin/urga
```

## Use

urga reads the same environment as the `nomad` command:

| Variable | What it is |
| --- | --- |
| `NOMAD_ADDR` | Address of the cluster, defaults to `http://127.0.0.1:4646` |
| `NOMAD_TOKEN` | ACL token, when the cluster asks for one |
| `NOMAD_NAMESPACE` | Namespace to start in, every namespace when unset |

Flags override it:

```sh
urga --address https://nomad.example.com --namespace production
```

Keys today: `↑`/`↓` or `k`/`j` to move, `PgUp`/`PgDn`, `g`/`G` for the ends,
`q` to quit.

## What works

The job list, refreshed from the cluster while it is open. That is the first
screen of a longer list: allocations, tasks, logs, deployments, namespaces,
services, evaluations, nodes, variables and node pools are coming, along with
the command prompt, the filter and the actions.

## Develop

```sh
make check   # fmt, vet, lint, test, build
```

Tests come before the code they cover. The architecture rules live in
`CLAUDE.md` and are not negotiated per change.

## License

MIT, see [LICENSE](LICENSE).

## About the name

An urga is a Mongolian catch pole: a long wooden shaft with a loop of rope at
the end. A rider holds it out at a gallop and takes the one horse he wants out
of a herd of hundreds that is still moving.

A cluster is that herd. This is what you reach into it with.
