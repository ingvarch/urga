# Access control

This page covers the ACL objects of the cluster: tokens, policies, roles,
auth methods and binding rules. For the keys of each screen, see
[Keys and commands](keys.md#acl-lists).

ACL objects belong to the cluster, not to a namespace. Reading them needs a
management token; with another token, or on a cluster without ACLs, the
status line shows what the cluster answers.

## The lists

| Command | Also | Columns |
| --- | --- | --- |
| `:tokens` | `token` | Accessor (shortened), Name, Type, Policies, Roles, Global, Expires, Age |
| `:policies` | `policy`, `pol` | Name, Description |
| `:roles` | `role` | Name, Description, Policies |
| `:authmethods` | `authmethod`, `auth` | Name, Type, Default |
| `:bindingrules` | `bindingrule`, `br` | ID (shortened), Auth Method, Description |

The columns are what the cluster lists; `d` shows the rest.

A token is colored by how long it has left, the way the header colors the
token of the session: red when it has expired or has less than 5 days, the
attention color under 14 days, yellow under 30.

The cluster streams changes of ACL objects to a management token only, so
these lists are polled.

## Describe

`d` shows the object under the cursor as the cluster has it, as JSON. A token
is shown without its secret: describing is reading, and the screen may be
shared. A policy is shown as its file: a `# Description:` line, then its
rules as they are written.

## The secret of a token

`c` on the token list puts the secret of the token under the cursor on the
clipboard. It goes over OSC52, so it works through ssh. The secret is never
drawn on the screen.
