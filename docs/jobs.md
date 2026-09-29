# Jobs

This page covers jobs, task groups and their scaling, editing, plans,
versions, evaluations and deployments. For the keys of each screen, see
[Keys and commands](keys.md).

## Job list

The job list shows the ID, name, type, namespace, status, running and desired
allocations, and age of each job. The color of a row shows the state of the
job, for example running, pending or dead. A batch job that ended is grey, or
red when a task group of it failed: it ended with allocations failed or lost
and none complete.

From the list:

- `enter` opens the allocations of the job, or the launches of a periodic or
  parameterized job;
- `t` opens its task groups;
- `d` shows the job as the cluster describes it;
- `h` shows the job file it was submitted with;
- `v` opens its versions.

## Periodic and parameterized jobs

A periodic or parameterized job runs nothing itself. Each time it runs, Nomad
launches a job of its own, such as `backup/periodic-1758499200` or
`report/dispatch-1758499200-3f1c`. The job list does not show these launches;
the job that launched them stands for them.

While a periodic or parameterized job runs, its row takes the color of its
newest launch: red when that launch failed, yellow while it waits to be
placed. A launch that runs or did its work leaves the color of the job as it
is. `!` keeps a job whose newest launch failed.

`enter` on such a job opens its launches. They are jobs like any other: the
keys of the job list work on them. For a periodic job, the title says when it
launches next, for example `Launches (Job: backup, next in 4h) [2]`.

`r` runs a periodic job now, out of its schedule, after you confirm. The launch
appears among its launches.

On a parameterized job, `r` dispatches it. A job that takes no meta and no
payload is dispatched after you confirm. For any other, urga opens a file in
your editor:

```
# Dispatch report in production.
# Required meta: day. Optional meta: region.
# Payload: optional. It is everything below the line "--- payload ---", as it is.
# Save a change to dispatch: at least delete this line. Quit without saving to drop it.
day = monday
region =
--- payload ---
{"rows": 100}
```

Each line above the marker is a meta key and its value. A key left empty is
not sent. Everything below `--- payload ---` is the payload, as you typed it;
a job that takes no payload has no marker. Save and close the editor to
dispatch; the status line names the job the dispatch launched. A file saved
unchanged dispatches nothing. When a line does not read as `key = value`, or
the cluster refuses the dispatch, for example because a required key is
missing, the file opens again with the reason at the top.

## Starting and stopping

`ctrl-s` stops a running job and starts a stopped one, after you confirm.
With several jobs marked, it acts on all of them.

A stopped job stays in the list until the cluster collects it. `ctrl-p`
purges it now, after you confirm: the job leaves the cluster with its
versions and history, and cannot be started again from urga. With several
jobs marked, it purges the stopped ones and leaves the running ones as they
are. `:gc` does the same for every dead job of the cluster at once, see
[Keys and commands](keys.md#command-line).

## Task groups and scaling

The task group list shows how many allocations of each group are running,
starting, queued, complete, failed and lost. The Scaling column shows the
bounds of the scaling policy of the group: `1-5`, `1-5 off` when the policy
is disabled, or `-` when the group has none.

`s` scales the group under the cursor. urga asks for the new count, with the
current count filled in, and then asks you to confirm. When an enabled policy
holds the group, the question says so: the autoscaler may change the count
back. A count outside the bounds is refused by the cluster.

`a` shows what was done to the count of the group, newest first: each change,
`2 → 3`, with its message and when it happened. A report of the autoscaler
that left the count shows `-`, and one that failed is red. The title shows
the count and the bounds: `Scaling (Job: web, Group: frontend, 3 in 1-5)`.
`d` shows the policy in full, with the settings the autoscaler reads. Nomad
keeps the last 20 events of a group, from the autoscaler and from a scale by
hand alike.

### Scaling policies

`:scaling` lists the scaling policies of the namespace: the job and the group
each one scales, its type, and whether it is enabled. A disabled policy is
grey. `enter` opens what was done to the count of the group, `d` shows the
policy in full.

A policy is part of its job. To change it, edit the job with `e`.

## Editing a job

`e` opens the job file in your editor. It is the file the job was submitted
with. If the cluster has no file for the job, urga opens the job as JSON.

When you save and close the editor, urga does not submit the job yet. It asks
the cluster for a plan and shows it. If the cluster cannot plan the file, for
example because of a syntax error, urga opens the editor again with your edit
and the reason at the top. See [Editor](configuration.md#editor).

## The plan

The plan shows, from top to bottom:

1. **Placement failures**, in red: task groups the scheduler cannot place, and
   why.
2. **Warnings** from the cluster.
3. **What will happen when you submit this job**: for each task group, how
   many allocations will be created, stopped, migrated, updated in place,
   recreated or deployed as canaries, and how many stay unchanged.
4. **Changes**: the difference between the running job and your file, laid out
   like the job file. Removed lines are red and start with `-`, added lines
   are green and start with `+`.

At the bottom, urga asks whether to submit, with **Cancel** and **Submit**
buttons. Use `left`, `right` or `tab` to choose a button and `enter` to press
it. `y` submits at once. `r` plans again. `esc` goes back without submitting.
The cursor starts on Cancel.

You cannot submit a plan with placement failures. The button then says why.

urga submits exactly what was planned. If someone changed the job after the
plan, the cluster refuses the submit, and urga asks you to plan again with
`r`.

## Versions and reverting

`v` lists the versions the cluster keeps for a job. `enter` shows what a
version changed, in the same format as the Changes part of a plan.

`u` reverts a job:

- on the job list, to the version before the current one;
- on the versions screen, to the version under the cursor.

A revert shows a plan first, like an edit. Its button says **Revert**.

`t` on the versions screen tags the version under the cursor: urga asks for
the name on the line at the top, and the name you type is the answer. A
version carries one tag, so on a tagged version the line starts with its tag,
and a new name renames it. `ctrl-t` takes the tag off, after you confirm.
Tags need Nomad 1.9 or later.

## Why a job is not placed

When a job or task group has allocations waiting to be placed, `p` opens the
newest evaluation that could not place them. For each task group, it shows
which nodes were filtered out and which ran out of resources, such as CPU or
memory.

`ctrl-e` asks the scheduler to evaluate a job again, as it stands, after you
confirm. Use it when what stopped the job has changed, for example when a
client joined or was freed: the scheduler places what it can now. With several
jobs marked, it evaluates all of them. It is not shown on a periodic or
parameterized job: the scheduler evaluates their launches.

## Evaluations

`:evaluations` lists the evaluations of the namespace. `enter` opens one: its
status, what triggered it, related evaluations, and the placement failures, if
there are any.

## Deployments

`:deployments` lists the deployments. `d` describes one, `p` promotes its
canaries, `f` fails it and `ctrl-s` pauses or resumes it.

`enter` opens a deployment. At the top, urga shows its status, job, job
version and status description, and a table with one row per task group:

| Column | Description |
| --- | --- |
| Desired | Allocations the group should have. |
| Placed | Allocations placed so far. |
| Healthy, Unhealthy | Placed allocations the deployment marked healthy or unhealthy. |
| Canaries | Placed and desired canaries, or `-` if the group has none. |
| Promoted | Whether the canaries were promoted, or `-` without canaries. |
| Auto Revert | Whether a failed deployment reverts the job to its last stable version. |
| Deadline | The progress deadline: how long an allocation has to become healthy. |
| Progress By | Time left before the deployment fails if nothing becomes healthy. Shown only while the deployment runs. |

A group with unhealthy allocations is red. A group with canaries waiting to
be promoted is yellow.

Below, urga lists the allocations of the deployment, with whether each one
is a canary and how the deployment judged its health: `healthy`, `unhealthy`
or `checking`. The keys of the allocation list work here too.

On the deployment screen:

- `p` promotes the canaries of the task group of the allocation under the
  cursor. The other groups keep their canaries. This key is shown when that
  group has canaries waiting to be promoted.
- `ctrl-p` promotes the canaries of every group.
- `f` fails the deployment. If Auto Revert is on, the job goes back to its
  last stable version.
- `ctrl-s` pauses a running deployment, or resumes a paused one.
- `h` and `u` mark the allocation under the cursor, or the marked ones,
  healthy or unhealthy. A group whose update block sets `health_check =
  "manual"` waits for this to go on. An unhealthy allocation fails the
  deployment.

Each of these asks you to confirm first. They are shown only while the
deployment is active: a deployment that succeeded, failed or was cancelled has
nothing left to promote, pause or fail.

System jobs have deployments since Nomad 1.11. They have no canaries, so
their Canaries and Promoted columns show `-`.
