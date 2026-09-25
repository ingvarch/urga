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
| `URGA_NO_UPDATE_CHECK` | Set to any value to stop urga from checking for a newer release. See [Newer releases](#newer-releases). |

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

## Token

The right end of the status line shows the ACL token urga sends. The
`Token:` label has the color of the labels in the header; the value after it
has a color that shows how long the token has left:

| Status line | Meaning |
| --- | --- |
| `Token: deploy-bot`, the name in green | The token does not expire, or expires in more than 30 days. |
| `Token: deploy-bot`, the name in yellow | It expires in 14 to 30 days. |
| `Token: deploy-bot`, the name in orange | It expires in 5 to 14 days. |
| `Token: expires in 4 days` in red | It expires in less than 5 days. Under a day, in hours, then minutes. |
| `Token: expired` in red | It has expired. |
| `Token: not valid` in red | The cluster does not accept the token: it is wrong, deleted or expired. |
| `Token: anonymous` in grey | No token is set. The cluster answers with what anonymous access allows. |

On a cluster without ACLs there is no token, and the status line shows
nothing about it.

A token that may read only some namespaces gets an empty list for the
others, without an error. So when a list is empty and the token is not a
management token, urga adds a grey line under the column titles:

- `Nothing here that deploy-bot can read: its policies may not allow it.`
- `Nothing here: no token is set, and anonymous access may not allow it.`

When the cluster refuses a request with `403 Permission denied`, the status
line says which token it refused: `Permission denied: deploy-bot may not do
this`, `Permission denied: no token is set` or `Permission denied: the token
is not valid`. For a management token, urga shows the error of the cluster
as it is.

## Newer releases

When a newer release of urga is out, the header says so next to the version:

```
Urga Rev:  v0.5.0 (40f73c8) ↑ v0.5.1
```

urga asks GitHub for the latest release when it starts, and then once a day
while it runs. The request goes to `api.github.com` and says nothing about
your cluster. If GitHub cannot be reached within 3 seconds, urga shows
nothing and asks again the next day.

A build from source, such as `dev` or `v0.5.0-3-gabc1234`, does not ask.

To turn the check off, set `URGA_NO_UPDATE_CHECK` to any value:

```sh
export URGA_NO_UPDATE_CHECK=1
```

## Editor

urga opens jobs, namespaces, variables and client metadata in the editor set
in `$VISUAL`. If `$VISUAL` is not set, it uses `$EDITOR`. When you save and
close the editor, urga sends the change to the cluster. A job is planned
first, see [Editing a job](jobs.md#editing-a-job). A variable is saved with
check-and-set, see [Editing a variable](clients.md#editing-a-variable).

If you close the editor without a change, nothing is sent.

If the cluster refuses the change, for example because of a syntax error or
because the token may not make it, urga opens the editor again with your
edit and the reason at the top, as comment lines:

```
# Not saved: the namespace is not valid JSON: unexpected end of JSON input.
# To save, change the file: delete these lines at least.
# To drop your edit, quit without saving.
```

Fix the file and save it to send it again. urga takes these lines off before
it sends the file, so they do not break JSON and do not end up in a job.
Because a file that comes back unchanged is not sent, saving it again takes a
change: at least delete these lines. To drop your edit, close the editor
without saving.

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
