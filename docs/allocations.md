# Allocations and tasks

This page covers allocations, the tasks of an allocation, checks, logs and
files. For the keys of each screen, see [Keys and commands](keys.md).

## Allocation list

You open allocations from a job, from a task group, from a client, or with
`:allocations` for the whole namespace. The list shows the task group, job,
namespace, node, status and desired status of each allocation. For running
allocations it also shows CPU and memory use, updated every 5 seconds.

CPU and memory are a share of what the running tasks of the allocation asked
for. A task that has ended, or has not started yet, is left out. Memory is
what the kernel counts against the limit of a task. For a Docker task that
includes tmpfs and the file cache, so a task that reads and writes many files
can stay close to 100% without being short of memory.

Three columns show how the allocation is doing:

| Column | Description |
| --- | --- |
| Ver | The version of the job the allocation runs. Allocations on different versions show a deployment that has not finished, or one that failed half way. |
| Rst | How many times its tasks were restarted, all together. |
| OOM | `yes` when the kernel killed a task for the memory it used, in the events the cluster still keeps. |

A running allocation whose task was restarted or killed for its memory in the
last hour is painted in the attention color, and `!` keeps it: that is a
crash loop going on now. A restart of a month ago leaves the row as it is.

`r` restarts an allocation and `ctrl-k` stops it, after you confirm. With
several allocations marked, they act on all of them. When you stop an
allocation, the scheduler places a new one if the job still needs it.

## Tasks of an allocation

`enter` on an allocation opens its tasks. Above the tasks, a panel shows:

- the status and desired status;
- the client it runs on;
- the job version;
- the deployment health: `healthy`, `unhealthy` or `checking`, and whether it
  is a canary;
- the ports it listens on, as `label address`, with `->port` when the port
  inside the task is different;
- how many times it was rescheduled;
- the previous allocation (the one it replaced), the next allocation (the one
  that replaced it), and the follow-up evaluation (the one that will place it
  again);
- its checks, see below.

A line with nothing to show is left out. On a small terminal, the panel shows
only what fits and always leaves room for at least three tasks.

For each running task, the list shows CPU and memory use as a share of what
the task asked for, updated every 5 seconds. Memory is read the same way as
in the allocation list.

From here:

- `c` opens the client the allocation runs on;
- `p` opens the previous allocation, `n` the next one;
- `f` opens the follow-up evaluation.

## Checks

The panel lists the checks of the allocation while it is running:

- failing checks first, each with the output that explains the failure;
- then checks that have not run yet;
- then passing checks.

Checks are read every 5 seconds. If there is not enough room for all of them,
the last line says how many more there are.

urga shows the checks of services registered with `provider = "nomad"`. The
Nomad API does not return the checks of Consul services.

## Acting on a task

These keys work on the task under the cursor while it is running:

- `r` restarts the task. The other tasks of the allocation keep running.
- `x` sends a signal. urga asks for the signal name, with `SIGHUP` filled in.
  You can type it in short form or in lower case: `hup`, `usr1`, `SIGTERM`.
- `s` opens a shell in the task. urga starts `bash` if the task has it, and
  `sh` if not. The shell opens on a clean screen. What the terminal showed
  before, an earlier shell included, is still in its scrollback.

urga only sends a signal name that Nomad knows. When a Nomad client gets a
name it does not know, it sends `SIGINT` instead, which stops most tasks. So
urga refuses unknown names before anything is sent. The list of names is the
one Nomad uses on Linux and macOS. Nomad clients on Windows know fewer
signals.

`e` shows the events of a task: what the client did with it and why it
stopped.

## Logs

`enter` on a task opens its stdout, `ctrl-e` its stderr.

urga shows the last 64 KiB of the log and follows it as the task writes. For
a task that has finished, it reads the log to the end and stops. When a task
crashes and is placed again, the new allocation starts with an empty log: `p`
opens the same log in the allocation it replaced.

A line under the title shows the state of three toggles:

- **Autoscroll**: follow the end of the log. `s` turns it on or off.
  Scrolling up turns it off.
- **Timestamps**: show when urga received each line. Nomad does not store a
  time for log lines, so this is the only time there is. `t` turns it on or
  off.
- **Wrap**: wrap long lines. `w` turns it on or off.

`ctrl-e` switches between stdout and stderr. `ctrl-s` saves the log to a file
in the current directory, as the filter shows it. `c` copies it to the
clipboard.

## Logs of every allocation

`l` on a job, a task group or a list of allocations opens the log of one task
in every allocation that runs it, on one screen. If the allocations run more
than one task, urga first asks which one, with a list of the tasks and how
many allocations run each.

Each line starts with the short ID of its allocation, in a color of its own,
so you can tell the allocations apart. The filter matches the ID too: `/9a1b`
shows the lines of one allocation.

- urga reads only allocations that are running, the newest first. It reads at
  most 20 of them. If more are running, the title says how many it read, for
  example `20 of 27 allocations`.
- When the log of an allocation ends, for example because the allocation
  stopped, urga adds a line that says `stopped`. The other logs go on.
- The set of allocations is fixed when the screen opens. `r` reads the
  allocations again and opens the logs of the ones that run now, for example
  after a deployment.
- `ctrl-e` switches every log between stdout and stderr.
- Autoscroll, timestamps, wrap and saving work as on the log of one task.
- `esc` closes every log and goes back.

## Files

`b` opens the directory of the task under the cursor. It contains:

- `local/`: files the task writes, including rendered templates;
- `secrets/`: secrets of the task. Nomad does not allow reading this
  directory through the API, and urga shows the error it returns.
- `tmp/` and `private/`.

`..` goes up to the directory of the allocation. It contains `alloc/`, which
the tasks share (`data/`, `logs/`, `tmp/`), and a directory for each task.

The list shows the name, size and age of each file. Directories come first.

`enter` on a file opens it like a log:

- It opens at the top, with Autoscroll off. `s` turns Autoscroll on to follow
  the end as the file grows.
- A file larger than 1 MiB opens at its last 1 MiB. The title says so.
- `w` wraps long lines, `/` filters them, `ctrl-s` saves the file, `c` copies it.

urga does not open:

- files that are not text, such as binaries. The status line shows the type
  of the file.
- named pipes, such as the log pipes in `alloc/logs`. Nomad reads the start
  of a file to find its type, and on a pipe this read waits until something
  writes to the pipe. urga does not ask Nomad about pipes at all.
