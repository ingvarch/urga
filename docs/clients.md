# Clients, servers and other resources

This page covers clients, servers, agent logs, the scheduler, namespaces,
services, variables and node pools. For the keys of each screen, see
[Keys and commands](keys.md).

## Client list

`:clients` lists the clients of the cluster. Nomad also calls them nodes, and
`:nodes` works too. The list shows the name, datacenter, node pool, version,
status, eligibility, drain state, CPU and memory use, and address of each
client.

A client that holds as many allocations as its limit allows (`node_max_allocs`,
Nomad 1.10.2 and later) is full: the scheduler places nothing more on it, and
its row has the attention color, like a client that is draining or
ineligible. With the event stream on, the list shows a client that fills up
or frees a place as soon as an allocation is placed on it or stops. For that
it follows the allocations of every namespace, which needs a token that may
read the jobs of every namespace, such as a management token. With any other
token the list follows the clients alone, says nothing about it, and shows a
full client within 30 seconds.

- `ctrl-d` drains a client, or stops draining it, after you confirm.
- `i` marks a client as eligible or ineligible for new work.
- `ctrl-p` purges a client that is down, after you confirm: the cluster
  forgets it. If the machine comes back, it registers again.
- `ctrl-g` collects the garbage of a client that runs, after you confirm: it
  deletes the directories and logs of its allocations that ended, which frees
  its disk.

With several clients marked, these keys act on all of them that fit:
`ctrl-p` on the ones that are down, `ctrl-g` on the ones that run.

## A client

`enter` on a client opens it. At the top, urga shows:

- its status, address, datacenter, node pool, version and whether it takes
  new work;
- when the client has a limit, how many allocations it holds of it, like
  `3 of 10`. Allocations that ended or are being stopped do not count;
- what its allocations reserve of the CPU, memory and disk the client gives to
  jobs, like `Allocated CPU 50%, memory 50%, disk 1%`. What a client gives to
  jobs is what it has less what it keeps for itself, and the tasks of an
  allocation are counted the way the header counts `CPU Alloc` (see
  [how urga stays up to date](configuration.md#how-urga-stays-up-to-date)).
  Allocations that ended or are being stopped do not count, nor do allocations
  in namespaces your token may not read. A resource the client does not
  report, such as a disk it could not measure, is left out, and so is the whole
  line when it reports none;
- while the client drains, how many of its allocations are left to move off
  it, of those it had when the drain started, like `Left to move 3 of 10`,
  and how long until the deadline, when the cluster stops the ones still on
  it, like `Deadline in 42m`. A drain that leaves system jobs does not count
  them, and a drain without a deadline shows none. Allocations that ended
  and that the cluster has since collected as garbage are no longer counted,
  so after a long drain or a garbage collection the second number can drop.
  A client that does not drain shows neither;
- when the client has an identity, when it expires, like `Identity expires in
  23h` (Nomad 1.11 and later). On an older cluster, or an older client, none
  of it shows;
- charts of its CPU and memory use, read every 5 seconds while the screen is
  open. The charts keep the last 20 minutes and start over when you come
  back to the client from another screen. `u` reads them and the allocated
  line in percents or in numbers, scaled up to gigahertz and gibibytes.

Below, it lists the allocations on the client, from every namespace. On a
small terminal, the charts are left out to keep room for the allocations.

From the client screen:

- `e` shows the events of the client;
- `v` lists its drivers, and `enter` on a driver shows what it reports;
- `ctrl-h` lists its host volumes;
- `a` shows its attributes;
- `m` shows its metadata. `e` opens the metadata in your editor, and urga
  saves it to the client when you close the editor; only the keys you add,
  change or take out are sent, so a key someone else sets while the file is
  open stays;
- `i` shows the claims of its identity;
- `ctrl-n` renews its identity after you confirm, and the client asks for a
  new one at its next heartbeat.

## Servers

`:servers` lists the servers and shows which one is the leader, and how each
stands in the Raft cluster:

| Column | Description |
| --- | --- |
| Health | Whether the server is healthy. An unhealthy server is red. |
| Voter | Whether it votes in the Raft cluster. |
| Contact | How long ago it heard from the leader. |
| Behind | How many Raft entries it lacks, compared with the leader. |

The title says whether the cluster is healthy, and how many servers it can
lose without an outage: `Servers (healthy, can lose 1) [3]`.

`enter` on a server shows what its agent reports: its addresses and ports, its
gossip settings, whether it is a voter in the Raft cluster, its health, when it
last heard from the leader, its Raft index, for how long it has been as
healthy as it is, and the tags the cluster was built with.

The health comes from the operator endpoint, which a token needs
`operator:read` for. Without it, the columns show `-` and the title leaves the
health out; the rest of the screen stays as it is.

## Agent logs

`:agentlogs` (or `:monitor`) follows what the agent urga talks to logs.
`ctrl-l` on a client or a server, in its list or on its screen, follows that
agent through the cluster. `ctrl-l` on a server shows only when urga talks to
a server: a client agent does not pass the request on to a server.

The log starts at `info`. `v` steps to more detail (`debug`, then `trace`),
then back to `error` and `warn`, and the log starts again at the new level.
The title shows the level.

First comes the last MiB of the log file of the agent, at the level and
above, then new lines as they are logged. The past lines need Nomad 1.10.4 or
later and an agent with `log_file` set. On an older cluster, with an agent
that logs only to journald, and on a server opened with `ctrl-l`, only new
lines show. A big log file takes a moment before its lines show.

The log needs `agent:read`. Without it, the status line shows the refusal of
the cluster.

Autoscroll, wrap, find, filter, saving and copying work as on a task log.
`esc` closes the log and its request.

## Scheduler

`:scheduler` shows how the scheduler of the cluster places work: the
algorithm, binpack or spread, the types of job that may preempt others,
whether memory oversubscription is on, and whether the cluster takes new jobs
and schedules them. When the node limit of feasibility checks is set (Nomad
2.0.1 and later), a row shows how many feasible clients the scheduler
considers for a job with spread or affinity. Unset, it considers 100.

`e` opens the configuration in your editor as JSON, and urga saves it when you
close the editor. It is saved with check-and-set: if someone changed the
configuration after you opened it, the cluster keeps theirs and the editor
opens again with the reason. Quit the editor without saving and press `e` to
start from the new one. The limit is the field `NodeLimitForFeasibilityChecks`;
0 leaves the default. If you leave the screen, or switch clusters, before the
cluster answers, the editor does not open again: the status line names the
file that keeps your text, see [Editor](configuration.md#editor).

Reading the configuration needs `operator:read`, saving it `operator:write`.

## Copying a value

On screens that show fields and values, such as a server, the scheduler, the
attributes or metadata of a client, and driver details, `c` copies the value
under the cursor to the clipboard. urga uses the OSC 52 terminal sequence, so
copying also works over SSH if your terminal supports it.

## Namespaces

`:namespaces` lists the namespaces. `e` opens a namespace in your editor, and
urga saves it to the cluster when you close the editor.

`n` opens a new namespace in your editor, with the fields it is made of:
`Name`, `Description` and `Meta`. urga creates it when you close the editor,
if there is no namespace of that name: saving one that exists would replace
it, so urga refuses and opens the file again with the reason.

`ctrl-d` deletes the namespace under the cursor, after you confirm. The
cluster refuses to delete a namespace that still holds jobs, variables or
volumes, and says why. `default` cannot be deleted, so `ctrl-d` is not
offered on it. When the cluster has deleted a namespace, a session that is
still in it goes to `default`. If you went to another namespace or cluster
before the answer, the session stays there.

The number keys `1` to `9` switch to a namespace, and `0` shows all
namespaces. The header shows which namespace each key opens. urga keeps this
order between runs. A namespace the cluster no longer has loses its key, and
the ones after it move up.

After a switch, a list shows no rows of the namespace you left, even when the
cluster does not answer; it fills once the new namespace answers. It starts at
the first row, with no filter or sort, and so does a list you go back to with
`esc`. Lists that are the same in every namespace, such as clients, keep their
rows.

## Services

`:services` lists the services registered in the cluster. `d` describes one.

`enter` on a service opens its instances: one row per registration, with the
address and port where it takes traffic, the allocation and client that
registered it, its tags, its checks and its status.

- **Checks** are read every 5 seconds from the client that runs each
  allocation: `2 passing`, `1 failing` or `1 pending`, only the checks of this
  service. urga shows the checks of services with `provider = "nomad"`.
- **Status** is the status of the allocation. A registration whose
  allocation is complete, failed or lost, or no longer exists, is **stale**:
  its status says `stale: alloc lost` or `stale: alloc gone`, and the row is
  red. Stale registrations happen when a client stops without removing them,
  and they send traffic to an address where nothing runs.

`ctrl-d` deletes the stale registration under the cursor, after you confirm.
It is not offered on a registration that still takes traffic. `enter` on an
instance opens the tasks of its allocation.

## Variables

`:variables` lists the variables by path, with their age and last change.
When a variable is held as a lock, the **Lock** column shows the short ID of
the lock. Nomad knows the holder of a lock by this ID only.

`enter` opens a variable on its values, one row per key. The values are
hidden until you press `v`, so a password does not show up on a shared
screen by accident. `v` hides them again, and they are hidden each time you
open the variable. A value of several lines, such as a certificate, shows its
first line and how many lines it has.

`c` copies the value under the cursor, the whole of it, even while it is
hidden.

When the variable is held as a lock, a line above the values shows the short
ID of the lock, its TTL and its lock delay, as Nomad reports them.

`ctrl-r` releases the lock, on the list or on the variable, after you
confirm. Use it when the holder is gone and left the lock held until its TTL
runs out. Whoever holds the lock loses it, and the values stay as they are.

### Editing a variable

`e` opens the variable in your editor, on the variable screen and on the
list. The file is TOML, one key per line:

```toml
# Variable nomad/jobs/web in namespace default.
CERT = """
-----BEGIN CERTIFICATE-----
MIIBfake
-----END CERTIFICATE-----
"""
DB_HOST = "10.0.0.5"
LIMITS = '{"cpu": 500}'
```

A value of several lines is written between `"""`, and a value with double
quotes, such as JSON, between single quotes, so both read as they are. Every
value is text: write a number in quotes, like `PORT = "5432"`. Comments are
not saved.

When you save and close the editor, urga saves the variable with
check-and-set: the cluster takes it only if the variable is still the version
you opened. If you close the editor without a change, nothing is saved.

If the cluster refuses the save, urga opens the editor again with your edit
and the reason at the top, see [Editor](configuration.md#editor). For a
variable, the reason may be:

- The file is not valid TOML, or a value is not text: fix it and save.
- The variable changed since you opened it: saving again replaces that
  change.
- The variable was deleted since you opened it: saving again creates it.
- The variable is locked: only the holder of the lock can change it.
- The token may not write the variable.

If you leave the screen, or switch clusters, before the cluster answers, urga
does not open the editor again. The status line names a file that keeps your
edit, see [Editor](configuration.md#editor). The file holds the values of the
variable: delete it once you have used it.

`e` is not offered on a variable held as a lock.

### Creating and deleting a variable

`n` on the list asks for the path of the new variable, then opens its file in
your editor, with a line to copy:

```toml
# Variable nomad/jobs/api in namespace production.
# KEY = "value"
```

The variable goes to the namespace of the session, or to `default` when the
session shows all namespaces; the line names it: `new variable in production
at:`. urga creates it only if there is no variable at that path. If there is
one, the editor opens again with `variable ... already exists`, and saving
again replaces it.

`ctrl-d` deletes the variable under the cursor after you confirm. The
question names its path and namespace. The delete uses check-and-set: if the
variable changed since the list was read, the cluster keeps it and urga says
so. A variable held as a lock cannot be deleted, so `ctrl-d` is not offered on
it.

## Node pools

`:nodepools` lists the node pools, their schedulers and batch job queues
(Nomad 2.1 and later). `enter` on a pool lists the jobs that run in it, of every
namespace, and `c` lists its clients. `b` opens the batch job queue of the pool
when it has one. The pool `all` holds every client of the cluster.

### Batch job queue

`:queue` lists the queued batch jobs across every pool that has a queue, sorted
by position. On `:nodepools`, `b` opens the queue of a single pool.

For a pool with a dynamic priority queue, the list shows priority adjustments
(base, fairshare, age, CPU, and memory) and, with `operator:read` permission, a
panel above the table shows tenant shares and fair share allocations.

`enter` on a queued job opens its allocations.

## Regions and datacenters

`:region` switches to another region of the cluster. `:dc` shows only one
datacenter in jobs, clients, servers and the header. See
[Keys and commands](keys.md#command-line).
