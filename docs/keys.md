# Keys and commands

The header shows the keys of the screen you are on. Press `?` to see them
together with the keys that work on every screen.

Keys marked "yes" in the "Changes" column change the cluster, and they are
hidden in [read-only mode](configuration.md#read-only-mode). Actions such as
stop, restart, drain or scale ask you to confirm first. In the confirm dialog
the cursor starts on cancel, so pressing `enter` by habit changes nothing.

## Command line

Press `:` to open the command line, type a command and press `enter`.

| Command | Short forms | Opens |
| --- | --- | --- |
| `jobs` | `job`, `jb` | Jobs |
| `allocations` | `allocs`, `alloc` | Allocations of the namespace |
| `deployments` | `dp` | Deployments |
| `evaluations` | `evals`, `eval`, `ev` | Evaluations |
| `events` | `event` | What happens in the cluster, event by event. See [Events](#events). |
| `services` | `svc` | Services |
| `namespaces` | `ns` | Namespaces |
| `variables` | `vars`, `var` | Variables |
| `clients` | `nodes`, `node`, `no` | Clients |
| `nodepools` | `np` | Node pools |
| `servers` | `srv` | Servers |
| `scheduler` | `sched` | How the scheduler places work |
| `scaling` | `scale` | Scaling policies |
| `volumes` | `volume`, `vol` | CSI and host volumes |
| `plugins` | `plugin` | CSI plugins |
| `tokens` | `token` | ACL tokens |
| `policies` | `policy`, `pol` | ACL policies |
| `roles` | `role` | ACL roles |
| `authmethods` | `authmethod`, `auth` | ACL auth methods |
| `bindingrules` | `bindingrule`, `br` | ACL binding rules |
| `about` | `version` | This urga: its version, a newer release and what changed. See [Newer releases](configuration.md#newer-releases). |

You do not have to type the whole word. The line completes the command that
matches what you typed. `up` and `down` go through the other matches, `tab`
or `right` accepts the one shown, `enter` opens it.

A second word selects the namespace: `jobs production`.

Other commands:

| Command | What it does |
| --- | --- |
| `region eu` | Switch to another region of the cluster. |
| `region` | List the regions and pick one. |
| `dc dc2` | Show only one datacenter in jobs, clients, servers and the header. |
| `dc all` | Show all datacenters again. |
| `dc` | List the datacenters and pick one. |
| `ctx prod` | Switch to another cluster from the settings file. See [Named clusters](clusters.md). |
| `ctx` | List the clusters and pick one. |
| `find web` | Find anything in the cluster by its name or ID. `search` works too. See [Find](#find). |
| `gc` | Collect the garbage of the cluster now, after you confirm: dead jobs, evaluations and allocations that ended, and clients that are down. Not in read-only mode. |
| `q`, `quit`, `exit` | Quit. |

In a list of regions, datacenters or clusters, the current one is marked.
`enter` switches to the one under the cursor, `esc` goes back without a
change.

## Find

`:find web` searches the whole cluster, every namespace your token can read,
for names that contain `web`, or come close: jobs, their groups, tasks,
services, images and commands, allocations, clients, node pools, namespaces,
variables, volumes and CSI plugins. Text that can begin an ID, such as
`bb19f`, also finds the allocation, evaluation, deployment or client whose ID
begins with it. The rest of the line is what to find, spaces included. The
cluster needs at least 2 characters.

The screen lists what was found: its type, its name, its namespace, and where
it is, such as the job and group of a service or the ID of an allocation. It
is the search as the cluster answered it; run `:find` again to search again.
When the cluster cut a list short, the status line says to type more.

`enter` opens what is under the cursor:

| Found | Opens |
| --- | --- |
| job | The job list with only this job, and every key of a job |
| group, task, image, command | The allocations of the group |
| service | The instances of the service |
| allocation | Its tasks |
| client | The client screen |
| namespace | The job list of that namespace, which the session switches to |
| variable, volume, plugin | Its screen |
| evaluation | The evaluation |
| deployment | The deployment |

A node pool or a node class has no screen of its own, so `enter` does nothing
on it.

## Events

`:events` shows what happens in the cluster as it happens, newest first: the
topic, the type of the event, the namespace, the name of the object and the
state it reports, like `Allocation AllocationUpdated default web.frontend[0]
failed`. It starts with the events the cluster still keeps, the last 100 by
default, so it shows at once what just happened. urga keeps the last 1000.

What failed, was lost or went down is red. An evaluation that could not place
its work is in the attention color. `!` keeps both, and `/failed` shows only
the events that report a failure.

With the cursor on the top row, it stays there and shows each event as it
arrives. Moved down, it stays on its event. `enter` opens what the event is
about: a job, an allocation, a deployment, an evaluation, a client or a
service. `esc` comes back to the events as they were, and the ones that
happened meanwhile are added on top.

The list follows the namespace of the session. Events of clients belong to no
namespace and show in every one. In another namespace it starts again from
what the cluster keeps.

## Sort

Shift and a letter order the list by a column; the same key again turns the
order round, and the title of the column shows which way with an arrow.
Every column has a letter of its own. The common ones keep the letters you
know from other tools: `N` for a name, `A` for an age, `S` for a status or a
state, `P` for a namespace. Any other column takes the first letter of its
title that is free, so on `:events` `T` is the topic and `Y` the type. `?`
lists the letters of the screen that is open.

## Filter

Press `/` and type to show only the rows that match.

| Filter | Shows |
| --- | --- |
| `/web` | Rows that contain `web`. |
| `/!web` | Rows that do not contain `web`. |
| `/-f wb` | Rows that contain `w` and then `b`, with anything in between. |

On text screens, such as logs and files, the filter shows the matching lines
and highlights what matched.

## Keys on every screen

| Key | What it does |
| --- | --- |
| `:` | Command line |
| `/` | Filter |
| `?` | Help |
| `0` to `9` | Switch namespace. `0` is all namespaces. |
| `A` to `Z` | Sort by a column, see [Sort](#sort). Press again to reverse. |
| `!` | Show only rows that need attention. Press again to show all rows. |
| `enter` | Open the row under the cursor. |
| `esc` | Go back to the previous screen, to the same row, filter and sort order. |
| `up`, `k` / `down`, `j` | Move the cursor. |
| `pgup`, `ctrl-b` / `pgdn`, `ctrl-f` | Move one page. |
| `g`, `home` / `G`, `end` | Go to the top or the bottom. |
| `left`, `right` | Choose a button in a dialog. |
| `q`, `ctrl-c` | Quit. |

## Marking rows

On jobs, allocations and clients, `space` marks the row under the cursor and
`ctrl-a` marks every row on the screen, or clears all marks. An action such as
start, stop, restart or drain then applies to every marked row.

## Keys by screen

### Jobs

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Allocations of the job, or the launches of a periodic or parameterized job | |
| `t` | Task groups | |
| `d` | Describe | |
| `h` | Job file the job was submitted with | |
| `v` | Versions | |
| `l` | Logs of a task in every allocation of the job. Not shown for a periodic or parameterized job. | |
| `e` | Edit the job in your editor | yes |
| `ctrl-s` | Start a stopped job, or stop a running one | yes |
| `ctrl-p` | Purge a stopped job: it leaves the cluster with its versions. Shown on a stopped job. | yes |
| `ctrl-e` | Evaluate the job again: the scheduler places what it can now | yes |
| `r` | Run a periodic job now, out of its schedule, or dispatch a parameterized one. Shown for those jobs. | yes |
| `u` | Revert to the previous version | yes |
| `p` | Why the job is not placed. Shown when allocations are waiting. | |
| `space`, `ctrl-a` | Mark | |

### Task groups

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Allocations of the task group | |
| `s` | Scale: set the number of allocations | yes |
| `l` | Logs of a task in every allocation of the group | |
| `p` | Why the group is not placed. Shown when allocations are waiting. | |
| `a` | What was done to the count of the group | |

### Scaling policies

| Key | What it does |
| --- | --- |
| `enter` | What was done to the count of the group |
| `d` | Describe the policy |

On what was done to the count of a group, `d` describes its policy.

### Versions

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | What this version changed | |
| `u` | Revert to this version | yes |
| `t` | Tag the version, or rename its tag | yes |
| `ctrl-t` | Take the tag off the version. Shown on a tagged one. | yes |

### Plan

| Key | What it does | Changes |
| --- | --- | --- |
| `left`, `right`, `tab` | Choose Cancel or Submit | |
| `enter` | Press the chosen button | |
| `y` | Submit, or revert, at once | yes |
| `r` | Plan again | |
| `esc` | Go back without submitting | |
| `w` | Wrap long lines | |
| `ctrl-s` | Save the plan to a file | |

### Allocations

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Tasks of the allocation | |
| `d` | Describe | |
| `r` | Restart | yes |
| `ctrl-k` | Stop | yes |
| `l` | Logs of a task in every allocation of the list | |
| `space`, `ctrl-a` | Mark | |

### Tasks

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Logs, stdout | |
| `ctrl-e` | Logs, stderr | |
| `e` | Events of the task | |
| `b` | Files of the task | |
| `s` | Shell in the task | yes |
| `r` | Restart the task. Shown when it is running. | yes |
| `x` | Send a signal to the task. Shown when it is running. | yes |
| `c` | Client the allocation runs on | |
| `p` | Allocation this one replaced | |
| `n` | Allocation that replaced this one | |
| `f` | Evaluation that will place the allocation again | |

`p`, `n` and `f` are shown only when there is such an allocation or
evaluation.

### Logs

| Key | What it does |
| --- | --- |
| `s` | Autoscroll on or off |
| `t` | Timestamps on or off |
| `w` | Wrap on or off |
| `ctrl-e` | Switch between stdout and stderr |
| `p` | Same log in the allocation this one replaced |
| `ctrl-s` | Save the log to a file |

### Logs of every allocation

| Key | What it does |
| --- | --- |
| `r` | Read the allocations again and open the logs of the ones that run now |
| `ctrl-e` | Switch every log between stdout and stderr |
| `s` | Autoscroll on or off |
| `t` | Timestamps on or off |
| `w` | Wrap on or off |
| `ctrl-s` | Save the logs to a file |

In the list of tasks that opens first when there are several, `enter` opens
the logs of the task under the cursor.

### Files

| Key | What it does |
| --- | --- |
| `enter` | Open a directory or a file. `..` goes up. |

On an open file:

| Key | What it does |
| --- | --- |
| `s` | Autoscroll on or off |
| `w` | Wrap on or off |
| `ctrl-s` | Save the file to a local file |

### Evaluations

| Key | What it does |
| --- | --- |
| `enter` | Details of the evaluation |

### Events

| Key | What it does |
| --- | --- |
| `enter` | Open what the event is about. Not shown for a topic urga opens nothing of. |

### Deployments

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Open the deployment | |
| `d` | Describe | |
| `p` | Promote the canaries of every group | yes |
| `f` | Fail the deployment | yes |
| `ctrl-s` | Pause a running deployment, or resume a paused one | yes |

### A deployment

The deployment screen lists the allocations of the deployment. It has the
keys of [Allocations](#allocations), and these:

| Key | What it does | Changes |
| --- | --- | --- |
| `p` | Promote the canaries of the group of the allocation under the cursor | yes |
| `ctrl-p` | Promote the canaries of every group | yes |
| `f` | Fail the deployment | yes |
| `ctrl-s` | Pause or resume the deployment | yes |
| `h` | Mark the allocation under the cursor, or the marked ones, healthy | yes |
| `u` | Mark the allocation under the cursor, or the marked ones, unhealthy | yes |

These keys are shown only while the deployment is active. `p` is shown only
when the group under the cursor has canaries waiting to be promoted.

### Namespaces and services

| Key | What it does | Changes |
| --- | --- | --- |
| `n` | Create a namespace in your editor | yes |
| `e` | Edit a namespace in your editor | yes |
| `ctrl-d` | Delete a namespace. Not shown on `default`. | yes |
| `enter` | Instances of a service | |
| `d` | Describe a service | |

### Instances of a service

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Tasks of the allocation that registered the instance | |
| `ctrl-d` | Delete a stale registration. Shown only on a stale one. | yes |

### Variables

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Open the variable on its values | |
| `n` | Create a variable: its path is asked first | yes |
| `e` | Edit the variable in your editor. Not shown on a locked one. | yes |
| `ctrl-d` | Delete the variable. Not shown on a locked one. | yes |
| `ctrl-r` | Release the lock held on the variable. Shown on a locked one. | yes |

### A variable

| Key | What it does | Changes |
| --- | --- | --- |
| `v` | Show or hide the values | |
| `c` | Copy the value under the cursor, even when it is hidden | |
| `e` | Edit the variable in your editor. Not shown on a locked one. | yes |
| `ctrl-r` | Release the lock held on the variable. Shown on a locked one. | yes |

### Clients

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Open the client | |
| `ctrl-d` | Drain, or stop draining | yes |
| `i` | Allow or stop new work on the client | yes |
| `ctrl-p` | Purge a client that is down: the cluster forgets it. Shown on one that is down. | yes |
| `ctrl-g` | Collect the garbage of a client: the directories and logs of its allocations that ended. Shown on one that runs. | yes |
| `space`, `ctrl-a` | Mark | |

### A client

The client screen lists the allocations of the client. It has the keys of
[Allocations](#allocations), and these:

| Key | What it does | Changes |
| --- | --- | --- |
| `e` | Events of the client | |
| `ctrl-d` | Drivers | |
| `ctrl-h` | Host volumes | |
| `a` | Attributes | |
| `m` | Metadata | |

On drivers, `enter` opens the details of a driver. On attributes, metadata,
driver details and a server, `c` copies the value under the cursor. On
metadata, `e` edits it in your editor.

### Node pools

| Key | What it does |
| --- | --- |
| `enter` | Jobs that run in the pool, of every namespace |
| `c` | Clients of the pool |

### Servers

| Key | What it does |
| --- | --- |
| `enter` | Details of the server |

### Scheduler

| Key | What it does | Changes |
| --- | --- | --- |
| `e` | Edit the configuration in your editor | yes |
| `c` | Copy the value under the cursor | |

### Volumes

| Key | What it does |
| --- | --- |
| `enter` | Open the volume |
| `d` | Describe |

### A volume

The volume screen lists the allocations that use the volume. It has the keys
of [Allocations](#allocations), and these:

| Key | What it does | Changes |
| --- | --- | --- |
| `ctrl-d` | Detach a CSI volume from the node of the allocation under the cursor | yes |
| `ctrl-r` | Release the claim of a task group on a host volume. Shown when the volume is claimed. | yes |

### Plugins

| Key | What it does |
| --- | --- |
| `enter` | Open the plugin |
| `d` | Describe |

### A plugin

| Key | What it does |
| --- | --- |
| `enter` | Tasks of the allocation that runs the instance under the cursor |

### ACL lists

On every list of ACL objects:

| Key | What it does |
| --- | --- |
| `d` | Describe; a token without its secret, a policy as its file |
| `c` | Copy the secret of the token under the cursor. On the token list only. |
| `n` | Create one in your editor; a policy is named first. Changes the cluster. |
| `e` | Edit the one under the cursor in your editor. Changes the cluster. |
| `ctrl-d` | Delete the one under the cursor. Changes the cluster. |

### Text screens

Descriptions, job files and diffs:

| Key | What it does |
| --- | --- |
| `w` | Wrap on or off |
| `ctrl-s` | Save to a file |
