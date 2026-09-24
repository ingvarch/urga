# Allocations and tasks

This page covers allocations, the tasks of an allocation, checks, logs and
files. For the keys of each screen, see [Keys and commands](keys.md).

## Allocation list

You open allocations from a job, from a task group, from a client, or with
`:allocations` for the whole namespace. The list shows the task group, job,
namespace, node, status and desired status of each allocation. For running
allocations it also shows CPU and memory use, updated every 5 seconds.

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
  `sh` if not.

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
in the current directory, as the filter shows it.

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
- `w` wraps long lines, `/` filters them, `ctrl-s` saves the file.

urga does not open:

- files that are not text, such as binaries. The status line shows the type
  of the file.
- named pipes, such as the log pipes in `alloc/logs`. Nomad reads the start
  of a file to find its type, and on a pipe this read waits until something
  writes to the pipe. urga does not ask Nomad about pipes at all.
