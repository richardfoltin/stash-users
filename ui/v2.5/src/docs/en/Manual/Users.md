# Users and Permissions

Stash always has at least one user. Each user has a role, which decides what they may do and which scenes they can see, and each user has their own viewing activity.

Administrators manage users in the `Users` section of `Settings > Security`. `Manage users` in the account menu opens it.

## Roles

| Role | Permissions |
|------|-------------|
| `Viewer` | Browse, search and play content. Record their own viewing activity: plays, O counts and resume times, and their own play and O history. See the system status and the queue of running tasks. Viewers cannot change library data, ratings or settings. |
| `Editor` | Everything a viewer can do, plus creating and editing library metadata (scenes, images, galleries, performers, studios, tags, groups, markers and gallery chapters), ratings, running the installed scrapers to fill in the edit forms, generating cover screenshots, using the `Tagger` view and saving filters. Editors cannot delete or merge objects, change settings, run tasks, or install and manage plugins, scrapers and users. |
| `Admin` | Everything, including settings, tasks, deleting and merging objects, managing users, scene visibility, the API key, deleting saved filters and setting default filters. |

The interface hides the actions that the role of the signed-in user does not allow.

The last `Admin` user cannot be removed or demoted.

Plugin hooks run with `Admin` rights, regardless of the role of the user whose change triggered them. Plugin settings, stash-box API keys and the Handy connection key are only visible to administrators.

## First admin and automatic login

When Stash starts without any users, it creates the user `admin` with the `Admin` role, no password and `Log in automatically from the local network` turned on. The users list shows it with `No password` and `Auto login`.

Requests from the local network that are not signed in are signed in as the automatic user, without a password. The local network means private, loopback and link-local addresses, and the `100.64.0.0/10` range. Requests from other addresses must sign in with a password, so a user without a password cannot sign in from them at all.

Only one user can log in automatically. Turn it on when adding a user, or with the `Log in automatically from the local network` switch under the user. Turning it on for one user turns it off for every other user.

> **⚠️ Note:** Automatic login is only for trusted local networks. Behind a reverse proxy that does not pass `X-Forwarded-For`, or behind container NAT, every request looks local, so everyone who can reach Stash is signed in automatically. The `100.64.0.0/10` range includes Tailscale and CGNAT clients. Do not turn on automatic login for an `Admin` user unless everyone who can reach Stash this way may have full access.

To require a password from everyone:

1. Set a password for the automatic user. While a user has no password, `Change password` in the account menu does not ask for the current one. Administrators can also use `Change password` for the user in the users list.
2. Turn off `Log in automatically from the local network` for that user.

Automatic login cannot be turned off for a user without a password. It also cannot be given to another user while the automatic user has no password. Set a password first.

Changing a password ends all sessions of that user. Passwords cannot be longer than 72 bytes.

## Signing in, switching account and logging out

The account menu is the user icon at the right end of the navigation bar. It shows the signed-in user and their role, and has `Change password`, `Manage users` (administrators only), `Switch account` and `Log out`. On phones, the menu button opens a drawer, which has the same items at its top.

The `Sign in` page asks for a username and password. On the local network, while a user is set to log in automatically, it also offers `Continue as` that user, which signs in without a password. The `Sign in` page itself is never signed in automatically, so another user can sign in there.

`Switch account` opens the sign-in page with the title `Switch account`. The current session stays until the new sign-in succeeds, and `Cancel` returns to the previous page. On the local network, it also offers `Continue as` the automatic user, unless you are already signed in as that user.

`Log out` ends the session and opens the `Sign in` page. On the local network, while a user is set to log in automatically, opening any other page signs you in as that user again.

A session ends after the `Maximum session age` of idle time, set in `Settings > Security`.

## Scene visibility

Each scene has a visibility, which decides who can see it:

| `Visible to` | Who can see the scene | `default_scene_visibility` value |
|--------------|-----------------------|----------------------------------|
| `Everyone` | All users | `READ` |
| `Editors and admins` | Editors and administrators | `MODIFY` |
| `Admins only` | Administrators | `ADMIN` |

A hidden scene is hidden everywhere for that user: in lists, searches, counts and statistics, on the pages of its performers, studio, tags, groups and galleries, and along with its markers, files and media. The user cannot change it either.

Only administrators can change the visibility, with the `Visible to` field when editing a scene, or when editing several selected scenes at once. Administrators find scenes by visibility with the `Visible to` filter.

Scenes that are not visible to everyone show a lock icon on their card and on the scene page. Its tooltip says who can see the scene.

New scenes, including scanned, imported and created scenes, get the `Default visibility of new scenes` from `Settings > Security`. It is stored as `default_scene_visibility` in `config.yml`, and is `Everyone` (`READ`) unless changed. Changing it does not change existing scenes. Exports do not include the visibility of scenes.

Scans and scheduled tasks see every scene. The DLNA server only serves scenes visible to everyone.

## Personal viewing activity

Each user has their own:

- `Play Count` and play history, and `Last Played At`
- `O Count` and O history, and `Last O At`
- `Resume Time`
- `Play Duration`
- `O Count` of images

Every count, filter, sort and statistic based on these shows the activity of the signed-in user. Ratings and all other data are shared.

Viewers record their own activity, and can edit their own play and O history.

Activity that is not recorded by a signed-in user belongs to the first admin: the first user with the `Admin` role in the users list. This includes the activity of tasks, of plugin hooks that no user triggered, and of the DLNA server. The activity recorded before users existed also belongs to the first admin. Requests made with the API key record the activity of the administrator who generated the key.

A scene merge keeps the play and O history of every user when the merge keeps that history. When the merge sets the play duration, the play durations of the other users are added to theirs.

Export and import carry the activity of the first admin only. A full import replaces the activity of every user with the imported activity, which belongs to the first admin. See [Tasks](/help/Tasks.md).

Removing a user deletes their activity. Renaming a user moves their activity to the new name.

## API key

There is one API key for the whole system. An administrator generates it with `Generate API key` in `Settings > Security`. Generating a new key replaces the old one, and `Clear API key` removes it.

External systems send the key in the `ApiKey` header to use Stash without signing in. The key has full `Admin` access, as the administrator who generated it. It only works while that user exists and has the `Admin` role.

## Recovering access

If an administrator can still sign in, they can set a new password for any user with `Change password` in the users list.

If no administrator can sign in:

1. Stop Stash.
2. Open `config.yml` in a text editor. Delete the `users` section, and the `username` and `password` lines if they exist. Save the file.
3. Start Stash. Because there are no users, it creates the user `admin` with no password and automatic login from the local network.
4. Open Stash from the local network, and set a password with `Change password` in the account menu.

All other users are removed along with the `users` section, so add them again. Their viewing activity stays in the database, and belongs again to a user added with the same name. The API key only works again if the administrator who generated it is added again with the same name and the `Admin` role. Otherwise, generate a new key.

## Upgrading

Installs that used Stash without credentials get the user `admin` with no password and automatic login on their first start, so the local network keeps working without signing in.

`public_whitelist` no longer allows access from public addresses without signing in. Requests from public addresses must sign in, and `admin` cannot sign in until it has a password. Set a password for `admin` from the local network, then sign in.

A `username` and `password` set in `config.yml` by an earlier version of Stash are treated as a user with the `Admin` role. This user is moved into the `users` section the first time users are changed.

The viewing activity recorded before the upgrade belongs to the first admin.
