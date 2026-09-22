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
  Address:   https://nomad.example.com  <0> all         <enter>  Allocations     @@@  @@@ @@@@@@@   @@@@@@@   @@@@@@
  urga Rev:  v0.1.0                     <1> production  <t>      Task groups     @@!  @@@ @@!  @@@ !@@       @@!  @@@
  Nomad Rev: 1.11.1                     <2> staging     <d>      Describe        @!@  !@! @!@!!@!  !@! @!@!@ @!@!@!@!
  Namespace: all                        <3> default     <h>      Job spec        !!:  !!! !!: :!!  :!!   !!: !!:  !!!
  CPU:       15%                                        <ctrl-s> Start or stop    :.:: :   :   : :  :: :: :   :   : :
  MEM:       31%                                        <u>      Revert
 ╭────────────────────────────────────────── Jobs (all) [4] ──────────────────────────────────────────╮
 │ ID              Name            Type     Namespace   Status   Allocs  Age                          │
 │ api             api             service  production  running  3/3     11d                          │
 │ nightly-import  nightly-import  batch    production  dead     0/0     7h                           │
 │ traefik         traefik         system   default     running  2/3     1h                           │
 ╰────────────────────────────────────────────────────────────────────────────────────────────────────╯
  <:> command   </> filter   <?> help   <q> quit
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

## Keys

Type `:` for the command line: `jobs`, `deployments`, `namespaces`, `services`,
`evaluations`, `nodes`, `variables`, `nodepools`, or the short forms `jb`, `dp`,
`ns`, `svc`, `ev`, `no`, `vars`, `np`. A second word switches the namespace with
it, as in `jobs production`. `q` leaves.

| Key | What it does |
| --- | --- |
| `:` | Command line |
| `/` | Filter what is on the screen |
| `?` | Help, with the keys of the open resource |
| `0`–`9` | Switch namespace, `0` is all of them |
| `enter` | Open what the cursor is on |
| `esc` | Back |
| `d` | Describe |
| `h` | The job file the job was submitted with |
| `e` | Edit a job or a namespace in `$EDITOR` |
| `t` | Task groups of a job |
| `s` | Scale a task group, or a shell in a task |
| `ctrl-s` | Start or stop a job |
| `u` | Revert a job to its previous version |
| `r` | Restart an allocation |
| `ctrl-k` | Stop an allocation |
| `ctrl-e` | Logs of a task, stderr |
| `q` | Quit |

## What works

Jobs, allocations, tasks, task groups, deployments, namespaces, services,
evaluations, nodes, variables and node pools, refreshed while they are open.
Logs follow a task as it writes. A job or a namespace opens in your editor and
goes back to the cluster when you save. Jobs start, stop, revert and scale;
allocations restart and stop; a task opens a shell.

The session comes back where it was left: the namespace, the resource and which
namespace each number key stands for.

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
