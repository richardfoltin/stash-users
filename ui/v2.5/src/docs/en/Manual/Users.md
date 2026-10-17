# Users and Permissions

Without users, Stash does not require signing in. Once a user exists, only signed-in users can use Stash. Each user has a role, which decides what they may do.

Administrators manage users in the `Users` section of `Settings > Security`.

## Roles

| Role | Permissions |
|------|-------------|
| `Viewer` | Browse, search and play content. See the system status and the queue of running tasks. Viewers cannot change library data, ratings, play history or settings. |
| `Editor` | Everything a viewer can do, plus creating and editing library metadata (scenes, images, galleries, performers, studios, tags, groups, markers and gallery chapters), ratings, plays, O counts and resume times, and the play and O history, running the installed scrapers to fill in the edit forms, generating cover screenshots, using the `Tagger` view and saving filters. Editors cannot delete or merge objects, change settings, run tasks, or install and manage plugins, scrapers and users. |
| `Admin` | Everything, including settings, tasks, deleting and merging objects, managing users, the API key, deleting saved filters and setting default filters. |

The interface hides the actions that the role of the signed-in user does not allow.

The last `Admin` user cannot be removed or demoted.

Plugin hooks run with `Admin` rights, regardless of the role of the user whose change triggered them. Plugin settings, stash-box API keys and the Handy connection key are only visible to administrators.

## Automatic login

Requests from the local network that are not signed in are signed in as the automatic user, without a password. The local network means private, loopback and link-local addresses, and the `100.64.0.0/10` range. Requests from other addresses must sign in with a password.

Only one user can log in automatically. Turn it on when adding a user, or with the `Log in automatically from the local network` switch under the user. Turning it on for one user turns it off for every other user.

> **⚠️ Note:** Automatic login is only for trusted local networks. Behind a reverse proxy that does not pass `X-Forwarded-For`, or behind container NAT, every request looks local, so everyone who can reach Stash is signed in automatically. The `100.64.0.0/10` range includes Tailscale and CGNAT clients. Do not turn on automatic login for an `Admin` user unless everyone who can reach Stash this way may have full access.

## Signing in and logging out

The user icon in the navigation bar shows the signed-in user and their role when you point at it, and opens `Change password`. The `Log out` button next to it ends the session and opens the login page. On the local network, while a user is set to log in automatically, opening any other page signs you in as that user again.

The login page asks for a username and password. The login page itself is never signed in automatically, so another user can sign in there.

Changing a password ends all sessions of that user. Passwords cannot be longer than 72 bytes.

A session ends after the `Maximum session age` of idle time, set in `Settings > Security`.

## API key

There is one API key for the whole system. An administrator generates it with `Generate API key` in `Settings > Security`. Generating a new key replaces the old one, and `Clear API key` removes it.

External systems send the key in the `ApiKey` header to use Stash without signing in. The key has full `Admin` access, as the administrator who generated it. It only works while that user exists and has the `Admin` role.

## Recovering access

If an administrator can still sign in, they can set a new password for any user with `Change password` in the users list.

If no administrator can sign in:

1. Stop Stash.
2. Open `config.yml` in a text editor. Delete the `users` section, and the `username` and `password` lines if they exist. Save the file.
3. Start Stash. Because there are no users, Stash does not require signing in.
4. Open Stash from the local network, and add an administrator in the `Users` section of `Settings > Security`.

All other users are removed along with the `users` section, so add them again. The API key only works again if the administrator who generated it is added again with the same name and the `Admin` role. Otherwise, generate a new key.

## Upgrading

A `username` and `password` set in `config.yml` by an earlier version of Stash are treated as a user with the `Admin` role. This user is moved into the `users` section the first time users are changed.
