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

## Services, variables and node pools

- `:services` lists the services registered in the cluster. `d` describes
  one.
- `:variables` lists the variables by path, with their age and last change.
  urga does not show their values.
- `:nodepools` lists the node pools and their schedulers.

## Regions and datacenters

`:region` switches to another region of the cluster. `:dc` shows only one
datacenter in jobs, clients, servers and the header. See
[Keys and commands](keys.md#command-line).
