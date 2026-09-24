# Jobs

This page covers jobs, task groups, editing, plans, versions, evaluations and
deployments. For the keys of each screen, see [Keys and commands](keys.md).

## Job list

The job list shows the ID, name, type, namespace, status, running and desired
allocations, and age of each job. The color of a row shows the state of the
job, for example running, pending or dead.

From the list:

- `enter` opens the allocations of the job;
- `t` opens its task groups;
- `d` shows the job as the cluster describes it;
- `h` shows the job file it was submitted with;
- `v` opens its versions.

## Starting and stopping

`ctrl-s` stops a running job and starts a stopped one, after you confirm.
With several jobs marked, it acts on all of them.

## Task groups and scaling

The task group list shows how many allocations of each group are running,
starting, queued, complete, failed and lost.

`s` scales the group under the cursor. urga asks for the new count, with the
current count filled in, and then asks you to confirm.

## Editing a job

`e` opens the job file in your editor. It is the file the job was submitted
with. If the cluster has no file for the job, urga opens the job as JSON.

When you save and close the editor, urga does not submit the job yet. It asks
the cluster for a plan and shows it.

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

## Why a job is not placed

When a job or task group has allocations waiting to be placed, `p` opens the
newest evaluation that could not place them. For each task group, it shows
which nodes were filtered out and which ran out of resources, such as CPU or
memory.

## Evaluations

`:evaluations` lists the evaluations of the namespace. `enter` opens one: its
status, what triggered it, related evaluations, and the placement failures, if
there are any.

## Deployments

`:deployments` lists the deployments. `d` describes one, `p` promotes its
canaries and `f` fails it.

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

System jobs have deployments since Nomad 1.11. They have no canaries, so
their Canaries and Promoted columns show `-`.
