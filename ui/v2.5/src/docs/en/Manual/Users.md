# Users and Permissions

Stash always has at least one user. Each user has a role, which decides what they may do and which scenes they can see, and each user has their own viewing activity and personal saved filters.

Administrators manage users in the `Users` section of `Settings > Security`, and roles in the `Roles` section below it. `Manage users` in the account menu opens it.

## Roles and permissions

Every user has one role. A role is a named set of permissions.

`Admin` is built in. It has every permission, including the ones that no other role can have, and it cannot be changed or removed. `Editor` and `Viewer` are the default roles. They are examples: administrators can edit, rename and remove them like any other role.

| Permission | `Viewer` | `Editor` | `Admin` |
|------------|----------|----------|---------|
| `Record own viewing activity` | ✓ | ✓ | ✓ |
| `Edit` every kind of object | | ✓ | ✓ |
| `Delete` and `Merge` every kind of object | | | ✓ |
| `Use scrapers` | | ✓ | ✓ |
| `Save filters` | | ✓ | ✓ |
| `Delete saved filters` | | | ✓ |
| `Manage files` | | | ✓ |
| `See restricted scenes` | | ✓ | ✓ |
| `See private scenes` | | | ✓ |
| `Change scene visibility` | | | ✓ |
| `Run library tasks` | | | ✓ |
| `Shared list settings` | | | ✓ |

### Permissions

Each kind of library object - scenes, markers, images, galleries, gallery chapters, performers, studios, tags and groups - has an `Edit` and a `Delete` permission. Scenes, performers and tags also have a `Merge` permission. `Edit` includes creating objects, ratings, favourites and `Organized`. For galleries, it also includes adding and removing images and setting the cover. Creating an object from the edit form of another object, such as a new performer while editing a scene, needs the `Edit` permission of the new object.

| Permission | Allows |
|------------|--------|
| `Record own viewing activity` | Recording their own plays, O counts and resume times, and editing their own play and O history. |
| `Use scrapers` | Running the installed scrapers in the edit forms and the `Tagger` view, for the objects that the role can edit. |
| `Save filters` | Saving, overwriting and renaming shared filters, and making personal filters shared with `Share with everyone`. Every user can save their own personal filters without it. |
| `Delete saved filters` | Deleting shared filters. |
| `Manage files` | Moving, deleting and reassigning files, setting fingerprints and revealing files in the file manager. It is also needed for the `Delete file` option when deleting scenes, images and galleries. |
| `See restricted scenes` | Seeing the scenes whose visibility is `Restricted`. |
| `See private scenes` | Seeing the scenes whose visibility is `Private`. |
| `Change scene visibility` | Changing who can see a scene, together with `Edit` of scenes. |
| `Run library tasks` | Scanning, generating, auto tagging, identifying and cleaning, and stopping running tasks. `Settings` shows the `Tasks` page with these tasks only. |
| `Shared list settings` | Publishing their own list settings as the defaults of every user: default filters, pinned criteria and sort options, table columns, the front page and the `Tagger` settings. |

Only the `Admin` role can change settings, including the shared interface settings, whose custom JavaScript and CSS run in the browser of every user. It is also the only role that can manage users and roles, generate the API key, manage plugins, scrapers and packages, submit to stash-box and batch tag, import, export, back up and maintain the database, run SQL, read the logs, control DLNA and browse the folders of the server. Every user can change their own personal interface settings, described below.

### Managing roles

The `Roles` section of `Settings > Security` lists the roles with their descriptions and the number of their users. `New role` adds a role, with a name, a description, a table of `Edit`, `Delete` and `Merge` permissions for each kind of object, and the other permissions. `Edit` changes a role. The server applies the new permissions to its users from their next request, and the interface of a signed-in user shows them once the page is reloaded. Role names are unique, regardless of case.

A role that users have cannot be removed: give those users another role first. A user whose role does not exist, for example after `config.yml` was edited by hand, is locked and cannot sign in until an administrator gives them a role.

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
| `Everyone` | All users | `EVERYONE` |
| `Restricted` | Users whose role has `See restricted scenes` | `RESTRICTED` |
| `Private` | Users whose role has `See private scenes` | `PRIVATE` |

Administrators see every scene. The two permissions are independent: a role can see private scenes without seeing restricted ones.

A hidden scene is hidden everywhere for that user: in lists, searches, counts and statistics, on the pages of its performers, studio, tags, groups and galleries, and along with its markers, files and media. The user cannot change it either.

Users whose role has `Change scene visibility` and `Edit` of scenes change it with the `Visible to` field when editing a scene, or when editing several selected scenes at once. They find scenes by visibility with the `Visible to` filter.

Scenes that are not visible to everyone show a lock icon on their card and on the scene page. Its tooltip says who can see the scene.

New scenes, including scanned, imported and created scenes, get the `Default visibility of new scenes` from `Settings > Security`. It is stored as `default_scene_visibility` in `config.yml`, and is `Everyone` (`EVERYONE`) unless changed. Changing it does not change existing scenes. Exports do not include the visibility of scenes.

Scans and scheduled tasks see every scene. The DLNA server only serves scenes visible to everyone.

## Personal viewing activity

Each user has their own:

- `Play Count` and play history, and `Last Played At`
- `O Count` and O history, and `Last O At`
- `Resume Time`
- `Play Duration`
- `O Count` of images

Every count, filter, sort and statistic based on these shows the activity of the signed-in user. Ratings and all other data are shared.

Users whose role has `Record own viewing activity` record their own activity, and can edit their own play and O history.

Activity that is not recorded by a signed-in user belongs to the first admin: the first user with the `Admin` role in the users list. This includes the activity of tasks, of plugin hooks that no user triggered, and of the DLNA server. The activity recorded before users existed also belongs to the first admin. Requests made with the API key record the activity of the administrator who generated the key.

A scene merge keeps the play and O history of every user when the merge keeps that history. When the merge sets the play duration, the play durations of the other users are added to theirs.

Export and import carry the activity of the first admin only. A full import replaces the activity of every user with the imported activity, which belongs to the first admin. See [Tasks](/help/Tasks.md).

Removing a user deletes their activity and personal saved filters. Renaming a user moves them to the new name.

## Personal interface settings

Each user has their own interface settings, including list settings such as default filters and table columns. Every user can open `Settings > Interface` to change their own. The other pages of `Settings` stay with administrators, apart from the `Tasks` page for `Run library tasks`. Which settings are personal, which are shared, and how defaults are published is described in [Interface Options](/help/Interface.md).

Personal settings are stored with the user, in their entry of the `users` section of `config.yml`. Renaming a user keeps them, and removing a user deletes them.

When adding a user, `Copy from` copies from an existing user:

- `Settings`: their personal interface settings. Checked by default.
- `Viewing history`: their plays and play history, O counts and resume times. Not checked by default.

If copying fails, the user is not created.

> **⚠️ Note for maintainers:** An upstream database migration that rewrites UI configuration keys, as migrations 58 and 60 did, only rewrites the shared defaults. Check the personal values of the same keys when merging upstream.

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

All other users are removed along with the `users` section, so add them again. Their personal interface settings are kept in the `users` section, so they are lost with it. Their viewing activity and personal saved filters stay in the database, and belong again to a user added with the same name. The API key only works again if the administrator who generated it is added again with the same name and the `Admin` role. Otherwise, generate a new key.

## Upgrading

Installs that used Stash without credentials get the user `admin` with no password and automatic login on their first start, so the local network keeps working without signing in.

`public_whitelist` no longer allows access from public addresses without signing in. Requests from public addresses must sign in, and `admin` cannot sign in until it has a password. Set a password for `admin` from the local network, then sign in.

A `username` and `password` set in `config.yml` by an earlier version of Stash are treated as a user with the `Admin` role. This user is moved into the `users` section the first time users are changed.

The viewing activity recorded before the upgrade belongs to the first admin.

Users keep their roles when roles become configurable: `Viewer`, `Editor` and `Admin` have the same rights as before. `config.yml` lists the `Editor` and `Viewer` roles only once roles are changed. The scene visibilities `Editors and admins` and `Admins only` became `Restricted` and `Private`. The stored values do not change, and saved filters with the old values keep working.
