# Configuration

urga does not need a configuration file. It reads the same environment
variables as the `nomad` CLI, and command-line flags override them.

If you work with several clusters and want to switch between them inside urga,
you can add an optional settings file. See [Named clusters](clusters.md).

## Environment variables

| Variable | Description |
| --- | --- |
| `NOMAD_ADDR` | Address of the cluster. Default: `http://127.0.0.1:4646`. |
| `NOMAD_TOKEN` | ACL token. |
| `NOMAD_REGION` | Region to use. Default: the region of the agent urga connects to. |
| `NOMAD_NAMESPACE` | Namespace to open on the first run. Default: all namespaces. |
| `NOMAD_HTTP_AUTH` | `user:password` for a cluster behind HTTP basic auth. |
| `NOMAD_CACERT` | CA certificate used to verify the cluster. |
| `NOMAD_CAPATH` | Directory of CA certificates, instead of a single file. |
| `NOMAD_CLIENT_CERT` | Client certificate, if the cluster requires one. |
| `NOMAD_CLIENT_KEY` | Key of the client certificate. |
| `NOMAD_TLS_SERVER_NAME` | Server name used to verify the certificate. |
| `NOMAD_SKIP_VERIFY` | Set to `true` to skip certificate verification. |

## Flags

| Flag | Description |
| --- | --- |
| `--address` | Address of the cluster. Overrides `NOMAD_ADDR`. |
| `--region` | Region to use. Overrides `NOMAD_REGION`. |
| `--namespace` | Namespace to open. Overrides `NOMAD_NAMESPACE` and the saved session. An empty value means all namespaces. |
| `--readonly` | Start in read-only mode. |
| `--cluster` | Start on a cluster from the settings file. See [Named clusters](clusters.md). |
| `--version` | Print the version and exit. |

Example:

```sh
urga --address https://nomad.example.com:4646 --region eu --namespace production
```

## Read-only mode

With `--readonly`, urga does not change anything in the cluster. It hides
every key that would: start and stop a job, revert, edit, scale, restart or
stop an allocation, restart or signal a task, drain a client, change its
eligibility, promote or fail a deployment, and open a shell. If you press one
of these keys, the status line says that it is off. The header shows
`read-only` next to the address.

The shell is off in read-only mode because a shell in a task can change
anything the task can.

## Editor

urga opens jobs, namespaces and client metadata in the editor set in
`$VISUAL`. If `$VISUAL` is not set, it uses `$EDITOR`. When you save and close
the editor, urga sends the change to the cluster. A job is planned first, see
[Editing a job](jobs.md#editing-a-job).

## How urga stays up to date

Most screens, such as jobs, allocations and clients, follow the Nomad event
stream: urga reloads the screen when something on it changes. It also reloads
every 30 seconds in case an event is missed. Screens that the event stream
does not cover, such as namespaces, variables and servers, reload every 2
seconds.

If the event stream is not available, which usually means the ACL token does
not allow it, urga reloads every screen every 2 seconds instead. The status
line says so.

CPU and memory in the header are updated every 15 seconds. CPU and memory of
allocations and clients in a list are updated every 5 seconds.

## Saved session

urga saves where you were and restores it on the next run:

- the namespace;
- the last list screen, such as jobs or clients;
- which namespace each number key opens.

The session is saved to `urga/config.json` in:

- `$XDG_CONFIG_HOME`, if it is set;
- otherwise `~/.config` on Linux, `~/Library/Application Support` on macOS and
  `%AppData%` on Windows.

urga writes this file itself. You do not need to edit it.

On start, urga picks the namespace in this order: `--namespace`, the saved
session, `NOMAD_NAMESPACE`, all namespaces. `NOMAD_NAMESPACE` comes after the
saved session because it is set for every run, not chosen for this one.

With [named clusters](clusters.md), each cluster has its own session in the
same file.
