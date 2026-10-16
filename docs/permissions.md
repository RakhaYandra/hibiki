# Permissions

Hibiki reads the systemd journal and changes nothing. There is no write command,
no privileged path, and no code that adjusts a permission, a user, or a group.
The only files Hibiki writes are its own snapshot in the user's cache directory
and any file the user names with `--save`.

## Reading the journal

A user can read the journal through one of two routes. Either is enough.

**Group membership.** The user is in the group that owns the journal files:

```bash
id -nG | tr ' ' '\n' | grep -E '^(systemd-journal|adm)$'
```

On Arch and Fedora the group is usually `systemd-journal`, on Debian and Ubuntu
it is usually `systemd-journal` plus `adm`.

**A read ACL.** Access granted per directory or file:

```bash
getfacl -p /var/log/journal 2>/dev/null | grep -E '^(group|mask)'
```

`systemd-tmpfiles` grants `wheel` a read ACL on Arch by default, which is why
the journal is often readable without any group membership.

## Checking, without changing anything

```bash
journalctl -n 1 --no-pager -o json      # can this user read one record?
hibiki doctor                            # all six checks, with a suggested fix
hibiki sources                           # reachable range and source status
```

`hibiki doctor` prints what it found and what could be done about it, and then
stops. It never runs the fix.

## When the journal cannot be read

Hibiki exits with code 3 and names the reason:

```console
$ hibiki timeline --since 30m
hibiki: journalctl cannot read the journal: Failed to open /var/log/journal/...: Permission denied
        (check journal read access: systemd-journal or adm group membership, or a read ACL on /var/log/journal)
$ echo $?
3
```

The two fixes, for a human to apply:

```bash
sudo usermod -aG systemd-journal "$USER"     # then log out and back in
sudo setfacl -m g:wheel:r-x /var/log/journal # per machine policy
```

Hibiki will not do either of these, and will not suggest escalating its own
privileges.

## Files Hibiki writes

| Path                                        | Mode | Content                          |
| ------------------------------------------- | ---- | -------------------------------- |
| `$XDG_CACHE_HOME/hibiki/last.json`          | 0600 | The last query, for `inspect`    |
| The path given to `--save`                  | 0600 | The same envelope, where asked   |

The cache directory is created with mode 0700. The snapshot is written to a
temporary file in that directory and renamed into place, so a reader never sees
a half written file. A snapshot that cannot be written produces a warning on
stderr and does not fail the query.

Set `XDG_CACHE_HOME` to keep the snapshot somewhere else:

```bash
XDG_CACHE_HOME=/run/user/1000 hibiki timeline --since 30m
```

Both files contain journal content, so they are as sensitive as the journal
itself. `--save` is the way to hand a result to someone else; nothing is ever
uploaded, and there is no telemetry.
