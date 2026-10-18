# Interface options

## Personal and shared settings

Each user has their own interface settings. Every setting on this page is personal, except the shared settings listed below, so changing it changes it for you only.

The settings that are saved where they are changed are personal too:
- default filters
- pinned filter criteria and sort options
- table columns
- the front page
- the Tagger settings
- "Advanced mode"
- the image lightbox options changed in the lightbox, which are the options of the "Image Lightbox" section

A setting you have never changed follows the default. A setting you changed shows an "Own" badge and a reset button, which returns it to the default. "Reset all my settings" returns every setting you changed to the default. The "My list settings" section lists your personal settings that have no option on this page, such as table columns or the front page, each with its reset button.

The following settings are shared: they apply to every user, and only administrators see and change them. On this page, their sections show the note "Shared: applies to every user".
- Custom CSS, Custom JavaScript and Custom localisation
- Interactive Options, including the Handy connection key
- Desktop Integration
- "Custom title"
- troubleshooting mode
- the task defaults, the options of the tasks on the `Tasks` page

### Publishing defaults

"Publish my settings as defaults…" makes some of your own settings the defaults of every user. Choose the settings to publish from the list of your own settings. Administrators can publish any of their own settings. Users whose role has `Shared list settings` can publish their list settings: default filters, pinned criteria and sort options, table columns, the front page and the Tagger settings.

A published setting becomes the default of every user who has not changed it. Users who changed it keep their own value. The published setting is removed from your own settings, so you follow the new default too.

When `config.yml` has no `users` section, for example while the `username` and `password` of an earlier version of Stash have not been moved into it yet, there are no personal settings: every setting is shared, as before. See [Users and Permissions](/help/Users.md).

## Language

Setting the language affects the formatting of numbers and dates.

## SFW content mode

SFW content mode is used to indicate that the content being managed is _not_ adult content. 

When SFW content mode is enabled, the following changes are made to the UI:
- default performer images are changed to less adult-oriented images
- certain adult-specific metadata fields are hidden (e.g. performer genital fields)
- `O`-Counter is replaced with `Like`-counter

## Scene/marker wall preview type

The Scene Wall and Marker pages display scene preview videos (mp4) by default. This can be changed to animated image (webp) or static image. 

> **⚠️ Note:** scene/marker preview videos must be generated to see them in the applicable wall page if Video preview type is selected. Likewise, if Animated image is selected, then Image Previews must be generated.

## Show studio overlay as text

By default, in the grid card view the studio will be shown as an image overlay of the studio logo. Checking this option changes this to display studios as a text name instead.

## Scene player options

By default, scene videos do not automatically start when navigating to the scenes page. Checking the "Auto-start video" option changes this to auto play scene videos.

The maximum loop duration option allows looping of shorter videos. Set this value to the maximum scene duration that scene videos should loop. Setting this to 0 disables this functionality.

### Activity tracking

The "Enable scene play history" option allows tracking of scene play count and duration, and sets the resume point when a scene video is not finished.

The play count, play duration, resume point and O count belong to the signed-in user: each user has their own, and records them if their role has `Record own viewing activity`. Requests that are signed in automatically from the local network record the activity of the user that logs in automatically. See [Users and Permissions](/help/Users.md).

The "Minimum play percent" gives the minimum proportion of a video that must be played before the play count of the scene is incremented.

By default, when a scene has a resume point, the scene player will automatically seek to this point when the scene is played. Setting "Always start video from beginning" to true disables this behaviour.

Like the other settings on this page, these options are personal: each user sets their own. DLNA, which plays without a signed-in user, follows the defaults of "Enable scene play history" and "Minimum play percent".

## Custom CSS

The stash UI can be customised using custom CSS. See [here](https://discourse.stashapp.cc/t/custom-css-snippets/4043) for a community-curated set of CSS snippets to customise your UI. 

There is also a [collection of community-created themes](https://discourse.stashapp.cc/tags/c/plugins/18/all/theme) available.

## Custom JavaScript

Stash supports the injection of custom JavaScript to assist with theming or adding additional functionality. Be aware that bad JavaScript could break the UI or worse.

## Custom locales

The localisation strings can be customised. The master list of default (en-GB) locale strings can be found [here](https://github.com/stashapp/stash/blob/develop/ui/v2.5/src/locales/en-GB.json). The custom locale format is the same as this json file.

For example, to override the `actions.add_directory` label (which is `Add directory` by default), you would have the following in the custom locale:

```
{
  "actions": {
    "add_directory": "Some other description"
  }
}
```

## Custom served folders

It is possible to expose specific folders to the UI. This configuration is performed manually in the `config.yml` file only.

Custom served content is exposed via the `/custom` URL path prefix.

For example, in the `config.yml` file:
```
custom_served_folders:
  /: D:\stash\static
  /foo: D:\bar
```

With the above configuration, a request for `/custom/foo/bar.png` would return `D:\bar\bar.png`. The `/` entry matches anything that is not otherwise mapped by the other entries. For example, `/custom/baz/xyz.png` would return `D:\stash\static\baz\xyz.png`.

Applications for this include using static images in custom css, like the Plex theme. For example, using the following config:
```yml
custom_served_folders:
  /: <stash folder>\custom
```

The `background.png` and `noise.png` files can be placed in the `custom` folder, then in the custom css, the `./background.png` and `./noise.png` strings can be replaced with `/custom/background.png` and `/custom/noise.png` respectively.

Other applications are to add custom UIs to stash, accessible via `/custom`.
