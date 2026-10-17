# Users and Permissions

Stash always has at least one user. Each user has a role, which decides what they may do.

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

## First admin and automatic login

When Stash starts without any users, it creates the user `admin` with the `Admin` role, no password and `Log in automatically from the local network` turned on. The users list shows it with `No password` and `Auto login`.

Requests from the local network that are not signed in are signed in as the automatic user, without a password. The local network means private, loopback and link-local addresses, and the `100.64.0.0/10` range. Requests from other addresses must sign in with a password, so a user without a password cannot sign in from them at all.

Only one user can log in automatically. Turn it on when adding a user, or with the `Log in automatically from the local network` switch under the user. Turning it on for one user turns it off for every other user.

> **⚠️ Note:** Automatic login is only for trusted local networks. Behind a reverse proxy that does not pass `X-Forwarded-For`, or behind container NAT, every request looks local, so everyone who can reach Stash is signed in automatically. The `100.64.0.0/10` range includes Tailscale and CGNAT clients. Do not turn on automatic login for an `Admin` user unless everyone who can reach Stash this way may have full access.

To require a password from everyone:

1. Set a password for the automatic user. While a user has no password, the `Change password` dialog of the user icon in the navigation bar does not ask for the current one. Administrators can also use `Change password` for the user in the users list.
2. Turn off `Log in automatically from the local network` for that user.

Automatic login cannot be turned off for a user without a password. It also cannot be given to another user while the automatic user has no password. Set a password first.

Changing a password ends all sessions of that user. Passwords cannot be longer than 72 bytes.

## Signing in and logging out

The user icon in the navigation bar shows the signed-in user and their role when you point at it, and opens `Change password`. The `Log out` button next to it ends the session and opens the `Sign in` page. On the local network, while a user is set to log in automatically, opening any other page signs you in as that user again.

The `Sign in` page asks for a username and password. On the local network, while a user is set to log in automatically, it also offers `Continue as` that user, which signs in without a password. The `Sign in` page itself is never signed in automatically, so another user can sign in there.

A session ends after the `Maximum session age` of idle time, set in `Settings > Security`.

## API key

There is one API key for the whole system. An administrator generates it with `Generate API key` in `Settings > Security`. Generating a new key replaces the old one, and `Clear API key` removes it.

External systems send the key in the `ApiKey` header to use Stash without signing in. The key has full `Admin` access, as the administrator who generated it. It only works while that user exists and has the `Admin` role.

## Recovering access

If an administrator can still sign in, they can set a new password for any user with `Change password` in the users list.

If no administrator can sign in:

1. Stop Stash.
2. Open `config.yml` in a text editor. Delete the `users` section, and the `username` and `password` lines if they exist. Save the file.
3. Start Stash. Because there are no users, it creates the user `admin` with no password and automatic login from the local network.
4. Open Stash from the local network, and set a password with the user icon in the navigation bar.

All other users are removed along with the `users` section, so add them again. The API key only works again if the administrator who generated it is added again with the same name and the `Admin` role. Otherwise, generate a new key.

## Upgrading

Installs that used Stash without credentials get the user `admin` with no password and automatic login on their first start, so the local network keeps working without signing in.

`public_whitelist` no longer allows access from public addresses without signing in. Requests from public addresses must sign in, and `admin` cannot sign in until it has a password. Set a password for `admin` from the local network, then sign in.

A `username` and `password` set in `config.yml` by an earlier version of Stash are treated as a user with the `Admin` role. This user is moved into the `users` section the first time users are changed.
