# Storage

This page covers CSI volumes, dynamic host volumes and CSI plugins. For the
keys of each screen, see [Keys and commands](keys.md#volumes).

Host volumes that a client declares in its configuration are on the screen of
that client, see [A client](clients.md).

## Volume list

`:volumes` lists the CSI volumes and the dynamic host volumes of the namespace
in one table. `:volume` and `:vol` work too.

| Column | Description |
| --- | --- |
| ID | The ID of the volume. A host volume has an ID the cluster made up, shortened. |
| Name | The name of the volume. |
| Type | `csi` or `host`. |
| Namespace | The namespace of the volume. |
| Plugin | The plugin that provides the volume. |
| Node | The client a host volume lives on. A CSI volume is attached where it is claimed, so it shows `-`. |
| Capacity | The size of a host volume. The list of CSI volumes does not have it: see the volume screen. |
| State | `schedulable` or `unschedulable` for a CSI volume; `pending`, `ready` or `unavailable` for a host volume. |
| Claims | How many allocations write to a CSI volume and read it, like `1W 0R`. A host volume shows `-`. |
| Age | How long ago the volume was created. |

A volume nobody can use (`unschedulable`, `unavailable`) is red. A host volume
still being made (`pending`) is yellow. A CSI volume whose plugin has fewer
healthy controllers or node plugins than expected has the attention color:
attaching it may fail. `!` shows only those volumes.

The cluster reports no change of a CSI volume in its event stream, so the
storage screens are polled, not watched.

## A volume

`enter` opens a volume. At the top, urga shows what the volume is. Below, it
lists the allocations that use it, with the keys of any allocation list.

For a CSI volume: its plugin with the provider and its version, how many
controllers and node plugins are healthy, the access and attachment mode, the
capacity, whether it is schedulable, its claims, its ID in the storage behind
it, and its topology.

For a host volume: its plugin, its state, the client and the node pool it
lives in, its directory on the client, its capacity and the capacity it asked
for, and the task group that claims it.

Two keys fix a volume that is stuck. Both ask you to confirm first, and
neither touches the data on the volume.

- `ctrl-d` detaches a CSI volume from the node of the allocation under the
  cursor. Use it when the node is gone and its claim stays behind: until it is
  released, the volume cannot be attached anywhere else.
- `ctrl-r` releases the claim of a task group on a host volume. A task group
  with `sticky = true` keeps the volume it was given; after the release, its
  next allocation may be placed with another volume. The key is shown only
  when the volume is claimed.

urga does not create, register or delete volumes.

## CSI plugins

`:plugins` lists the CSI plugins of the cluster, with how many controllers and
node plugins are healthy out of how many are expected. `:plugin` works too.
A plugin with fewer healthy instances than expected has the attention color.
One with no healthy controller or node plugin is red: no volume of it can be
attached.

`enter` opens a plugin. At the top, urga shows its provider and version,
whether it needs a controller, and the counts of healthy instances. Below, it
lists each instance: whether it is a controller or a node plugin, its client,
whether it is healthy, why not, and when it last reported. An instance that is
not healthy is red. `enter` opens the tasks of the allocation that runs the
instance, where its logs are.
