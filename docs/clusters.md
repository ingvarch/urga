# Named clusters

This is optional. Without a settings file, urga connects to the cluster set by
the environment and flags, as described in [Configuration](configuration.md).

A settings file lets you:

- give names to the clusters you work with;
- start urga on one of them with `--cluster`;
- switch between them with `:ctx` without restarting urga;
- make a cluster read-only and give it a color, so that production does not
  look like development.

## Where the file is

The file is `urga/clusters.toml`, in the same directory as the saved session:

| System | Path |
| --- | --- |
| Any, with `XDG_CONFIG_HOME` set | `$XDG_CONFIG_HOME/urga/clusters.toml` |
| Linux | `~/.config/urga/clusters.toml` |
| macOS | `~/Library/Application Support/urga/clusters.toml` |
| Windows | `%AppData%\urga\clusters.toml` |

urga only reads this file. It never writes to it.

## Example

```toml
default = "dev"

[clusters.dev]
address   = "http://127.0.0.1:4646"
namespace = "default"

[clusters.prod]
address         = "https://nomad.prod.example.com:4646"
region          = "eu"
token_command   = ["op", "read", "op://ops/nomad/token"]
ca_cert         = "~/.nomad/prod-ca.pem"
tls_server_name = "server.eu.nomad"
read_only       = true
color           = "red"
```

## Settings

At the top level:

| Key | Description |
| --- | --- |
| `default` | Cluster to start on when `--cluster` is not given. Optional. |

For each cluster, in a `[clusters.<name>]` table:

| Key | Description |
| --- | --- |
| `address` | Address of the cluster. Required. |
| `region` | Region to use. Default: the region of the agent. |
| `namespace` | Namespace to open the first time you use this cluster. Default: all namespaces. |
| `token_env` | Name of an environment variable that holds the token. |
| `token_command` | Command that prints the token, as a list: `["op", "read", "..."]`. |
| `token` | The token itself. Avoid this if you can: the file is then a secret. |
| `ca_cert` | CA certificate used to verify the cluster. |
| `client_cert` | Client certificate, if the cluster requires one. |
| `client_key` | Key of the client certificate. |
| `tls_server_name` | Server name used to verify the certificate. |
| `read_only` | `true` works like `--readonly` for this cluster. |
| `color` | Color of the frame and the cluster name: `red`, `orange`, `yellow`, `green`, `cyan`, `blue` or `purple`. |

A path that starts with `~/` is read from your home directory.

## Which cluster urga starts on

1. The cluster given with `--cluster`.
2. The `default` cluster of the file.
3. If neither is set, the cluster from the environment, as without a file.

The header then shows `Cluster:`, the name and the address, instead of
`Address:`.

The flags `--address`, `--region`, `--namespace` and `--readonly` still
override the settings of the cluster urga starts on. `--readonly` also applies
to every cluster you switch to later.

## Tokens

A named cluster does not use any `NOMAD_*` environment variables. A token you
exported for one cluster is never sent to another.

The token comes from one of `token_env`, `token_command` or `token`. You can
set only one of them. If none is set, urga sends no token.

urga reads the token when it connects to the cluster: on start, and each time
you switch to it. A `token_command` must finish within one minute. It does not
get the terminal, so it cannot ask for input there. Tools that ask in their
own window, such as a password manager with fingerprint unlock, work.

If the variable in `token_env` is not set, or the command fails, urga does
not connect and shows the error.

## Certificates

`ca_cert`, `client_cert`, `client_key` and `tls_server_name` work like the
`NOMAD_*` variables of the same meaning. If a certificate file cannot be read,
urga does not connect to the cluster.

## Read-only and color

`read_only = true` hides every key that changes the cluster, the same as
`--readonly`. See [Read-only mode](configuration.md#read-only-mode).

`color` paints the frame around the screen and the cluster name in the header.
For example, make production red so that it is easy to see which cluster you
are on.

## Switching clusters

Type `:ctx prod` to switch to the cluster named `prod`. Type `:ctx` to see a
list of clusters, with the current one marked. Press `enter` to switch to the
cluster under the cursor.

When you switch, urga:

- reads the token of the new cluster again;
- opens the screen, namespace and number keys you last used on that cluster;
- applies its color and read-only setting;
- shows `Connected to <name>.` in the status line.

If urga cannot connect, it stays on the current cluster and shows the error.

Each cluster has its own saved session. Switching from dev to prod and back
keeps the namespace of each.

## Errors in the file

urga does not start, and shows which key is wrong, if the file has:

- a key it does not know, such as a typo in `read_only`;
- a `default` that is not a cluster in the file;
- a cluster without `address`;
- more than one of `token`, `token_env` and `token_command`;
- a color that is not in the list above.
