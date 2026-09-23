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
  Urga Rev:  v0.1.0                     <1> production  <t>      Task groups     @@!  @@@ @@!  @@@ !@@       @@!  @@@
  Nomad Rev: 2.0.5                      <2> staging     <d>      Describe        @!@  !@! @!@!!@!  !@! @!@!@ @!@!@!@!
  CPU:       15%                        <3> default     <h>      Job spec        !!:  !!! !!: :!!  :!!   !!: !!:  !!!
  MEM:       31%                                        <ctrl-s> Start or stop    :.:: :   :   : :  :: :: :   :   : :
 ╭───────────────────────────────────────── Jobs (all) [3] ──────────────────────────────────────────╮
 │ ID                       Name                     Type      Namespace    Status    Allocs   Age   │
 │ api                      api                      service   production   running   3/3      11d   │
 │ nightly-import           nightly-import           batch     production   dead      0/0      7h    │
 │ traefik                  traefik                  system    default      running   2/3      1h    │
 ╰───────────────────────────────────────────────────────────────────────────────────────────────────╯
  <:> command   </> filter   <?> help   <q> quit
```

## Install

Download the archive for your machine from the
[releases](https://github.com/ingvarch/urga/releases) — Linux, macOS, Windows
and FreeBSD, on amd64 and arm — or install it with Go:

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
`evaluations`, `clients`, `servers`, `variables`, `nodepools`, or the short
forms `jb`, `dp`, `ns`, `svc`, `ev`, `no`, `srv`, `vars`, `np`. Clients answer
to `nodes` as well, the word Nomad's own CLI uses. A letter is enough: the
line finishes the word it fits, `up` and `down` walk through the rest of
them, `tab` or `right` takes what is offered, `enter` opens it. A second word
switches the namespace with it, as in `jobs production`. `q` leaves.

| Key | What it does |
| --- | --- |
| `:` | Command line |
| `/` | Filter what is on the screen: a pattern, `!` for everything else, `-f ` for the letters in that order |
| `?` | Help, with the keys of the open resource |
| `0`–`9` | Switch namespace, `0` is all of them |
| `A`–`Z` | Sort by the column that starts with that letter, again to reverse |
| `!` | Only what needs attention, again for all of it |
| `enter` | Open what the cursor is on: a client opens what it runs, a server what the agent says about itself |
| `esc` | Back |
| `d` | Describe |
| `h` | The job file the job was submitted with |
| `e` | Edit a job, a namespace or the metadata of a client in `$EDITOR`; the events of a client or of a task |
| `t` | Task groups of a job |
| `s` | Scale a task group, or a shell in a task |
| `v` | Versions of a job, with what each one changed |
| `u` | Revert a job to its previous version, or on the versions screen to the one under the cursor |
| `space` | Mark a row, on jobs, allocations and clients; an action then takes every marked row |
| `ctrl-a` | Mark every row on the screen, or none |
| `ctrl-s` | Start or stop a job, or every marked one |
| `r` | Restart an allocation, or every marked one |
| `ctrl-k` | Stop an allocation, or every marked one |
| `ctrl-d` | Drain a client, or every marked one; on a client, what it can run |
| `ctrl-h` | Host volumes of a client |
| `a` | Attributes of a client |
| `m` | Metadata of a client |
| `i` | Let a client take new work, or stop it; every marked one at once |
| `p` | Promote the canaries of a deployment |
| `f` | Fail a deployment |
| `c` | Copy the value under the cursor, on a screen of fields |
| `ctrl-e` | Logs of a task, stderr |
| `w` | Wrap long lines, on logs and descriptions |
| `t` | Show when urga read each log line; a task writes no time of its own |
| `ctrl-s` | Save what is on the screen to a file, as the filter left it |
| `q` | Quit |

## What works

Jobs, allocations, tasks, task groups, deployments, namespaces, services,
evaluations, clients, servers, variables and node pools. A screen that the
cluster will talk about follows its event stream and is asked again the moment
something changes; the rest are asked on a timer. A cluster that will not
stream — an ACL that does not allow it, most often — is polled instead, and
the status line says so. A task says what happened to it, from the moment the client received it.
Logs follow a task as it writes; the filter lights up what it matched, long
lines wrap, and what is on the screen saves to a file. A job or a namespace opens in your
editor and goes back to the cluster when you save. Jobs start, stop, revert and
scale; a job keeps its versions, each saying what it changed, and goes back to
any of them; allocations restart and stop; jobs start and stop; clients drain and take work
again — one of them, or as many as are marked; a task opens a shell. Clients drain and
take work again, deployments promote their canaries or fail. The servers list
says which one leads, and a server opens on everything its agent carries: the
addresses and ports, the gossip it speaks, whether the raft still counts its
vote, and every tag the cluster was built with. One key copies the value under
the cursor, over OSC52, so it works through ssh as well.

A client opens on what the machine is doing: its CPU and memory as a chart of
the readings taken while the screen is open, and the allocations it runs, from
every namespace. From there one key each opens what happened to it, the drivers
it has and what every one of them reports, the volumes it lends out, the
attributes it is built from and its metadata, which opens in your editor and
goes back to the machine when you save.

Allocations and clients say what they are using, the list sorts by any column,
and one key leaves only what needs attention.

The session comes back where it was left: the namespace, the resource and which
namespace each number key stands for.

## Develop

```sh
make check   # fmt, vet, lint, test, build
make dist    # one platform, packed the way a release is downloaded
```

Every push runs the checks on Linux, the tests on Linux, macOS and Windows,
and a build for each platform urga is released for. A tag starting with `v`
builds the archives and opens a draft release.

Tests come before the code they cover. The architecture rules live in
`CLAUDE.md` and are not negotiated per change.

## License

MIT, see [LICENSE](LICENSE).

## About the name

An urga is a Mongolian catch pole: a long wooden shaft with a loop of rope at
the end. A rider holds it out at a gallop and takes the one horse he wants out
of a herd of hundreds that is still moving.

A cluster is that herd. This is what you reach into it with.
