# Clients, servers and other resources

This page covers clients, servers, namespaces, services, variables and node
pools. For the keys of each screen, see [Keys and commands](keys.md).

## Client list

`:clients` lists the clients of the cluster. Nomad also calls them nodes, and
`:nodes` works too. The list shows the name, datacenter, node pool, version,
status, eligibility, drain state, CPU and memory use, and address of each
client.

- `ctrl-d` drains a client, or stops draining it, after you confirm.
- `i` marks a client as eligible or ineligible for new work.

With several clients marked, these keys act on all of them.

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

`:servers` lists the servers and shows which one is the leader. `enter` on a
server shows what its agent reports: its addresses and ports, its gossip
settings, whether it is a voter in the Raft cluster, and the tags the cluster
was built with.

## Copying a value

On screens that show fields and values, such as a server, the attributes or
metadata of a client, and driver details, `c` copies the value under the
cursor to the clipboard. urga uses the OSC 52 terminal sequence, so copying
also works over SSH if your terminal supports it.

## Namespaces

`:namespaces` lists the namespaces. `e` opens a namespace in your editor, and
urga saves it to the cluster when you close the editor.

The number keys `1` to `9` switch to a namespace, and `0` shows all
namespaces. The header shows which namespace each key opens. urga keeps this
order between runs.

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

## Node pools

`:nodepools` lists the node pools and their schedulers.

## Regions and datacenters

`:region` switches to another region of the cluster. `:dc` shows only one
datacenter in jobs, clients, servers and the header. See
[Keys and commands](keys.md#command-line).
