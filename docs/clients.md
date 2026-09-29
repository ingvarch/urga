# Clients, servers and other resources

This page covers clients, servers, the scheduler, namespaces, services,
variables and node pools. For the keys of each screen, see
[Keys and commands](keys.md).

## Client list

`:clients` lists the clients of the cluster. Nomad also calls them nodes, and
`:nodes` works too. The list shows the name, datacenter, node pool, version,
status, eligibility, drain state, CPU and memory use, and address of each
client.

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
- charts of its CPU and memory use, read every 5 seconds while the screen is
  open. The charts keep the last 20 minutes.

Below, it lists the allocations on the client, from every namespace. On a
small terminal, the charts are left out to keep room for the allocations.

From the client screen:

- `e` shows the events of the client;
- `ctrl-d` lists its drivers, and `enter` on a driver shows what it reports;
- `ctrl-h` lists its host volumes;
- `a` shows its attributes;
- `m` shows its metadata. `e` opens the metadata in your editor, and urga
  saves it to the client when you close the editor.

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

## Scheduler

`:scheduler` shows how the scheduler of the cluster places work: the
algorithm, binpack or spread, the types of job that may preempt others,
whether memory oversubscription is on, and whether the cluster takes new jobs
and schedules them.

`e` opens the configuration in your editor as JSON, and urga saves it when you
close the editor. It is saved with check-and-set: if someone changed the
configuration after you opened it, the cluster keeps theirs and the editor
opens again with the reason. Quit the editor without saving and press `e` to
start from the new one.

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
offered on it. When you delete the namespace the session is in, the session
goes to `default`.

The number keys `1` to `9` switch to a namespace, and `0` shows all
namespaces. The header shows which namespace each key opens. urga keeps this
order between runs. A namespace the cluster no longer has loses its key, and
the ones after it move up.

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

When the variable is held as a lock, a line above the values shows the ID of
the lock, its TTL and its lock delay, as Nomad reports them.

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

`ctrl-d` deletes the variable under the cursor, after you confirm, with
check-and-set: if the variable changed since the list was read, the cluster
keeps it and urga says so. A variable held as a lock cannot be deleted, so
`ctrl-d` is not offered on it.

## Node pools

`:nodepools` lists the node pools and their schedulers. `enter` on a pool
lists the jobs that run in it, of every namespace, and `c` lists its clients.
The pool `all` holds every client of the cluster.

## Regions and datacenters

`:region` switches to another region of the cluster. `:dc` shows only one
datacenter in jobs, clients, servers and the header. See
[Keys and commands](keys.md#command-line).
