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
| `services` | `svc` | Services |
| `namespaces` | `ns` | Namespaces |
| `variables` | `vars`, `var` | Variables |
| `clients` | `nodes`, `node`, `no` | Clients |
| `nodepools` | `np` | Node pools |
| `servers` | `srv` | Servers |

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
| `q`, `quit`, `exit` | Quit. |

In a list of regions, datacenters or clusters, the current one is marked.
`enter` switches to the one under the cursor, `esc` goes back without a
change.

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
| `A` to `Z` | Sort by the column that starts with this letter. Press again to reverse. |
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
| `enter` | Allocations of the job | |
| `t` | Task groups | |
| `d` | Describe | |
| `h` | Job file the job was submitted with | |
| `v` | Versions | |
| `e` | Edit the job in your editor | yes |
| `ctrl-s` | Start a stopped job, or stop a running one | yes |
| `u` | Revert to the previous version | yes |
| `p` | Why the job is not placed. Shown when allocations are waiting. | |
| `space`, `ctrl-a` | Mark | |

### Task groups

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Allocations of the task group | |
| `s` | Scale: set the number of allocations | yes |
| `p` | Why the group is not placed. Shown when allocations are waiting. | |

### Versions

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | What this version changed | |
| `u` | Revert to this version | yes |

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

### Deployments

| Key | What it does | Changes |
| --- | --- | --- |
| `d` | Describe | |
| `p` | Promote the canaries | yes |
| `f` | Fail the deployment | yes |

### Namespaces and services

| Key | What it does | Changes |
| --- | --- | --- |
| `e` | Edit a namespace in your editor | yes |
| `d` | Describe a service | |

### Clients

| Key | What it does | Changes |
| --- | --- | --- |
| `enter` | Open the client | |
| `ctrl-d` | Drain, or stop draining | yes |
| `i` | Allow or stop new work on the client | yes |
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

### Servers

| Key | What it does |
| --- | --- |
| `enter` | Details of the server |

### Text screens

Descriptions, job files and diffs:

| Key | What it does |
| --- | --- |
| `w` | Wrap on or off |
| `ctrl-s` | Save to a file |
