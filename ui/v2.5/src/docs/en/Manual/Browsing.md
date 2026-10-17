# Browsing

## Querying and filtering

### Keyword searching

The text field allows you to search using keywords. Keyword searching matches on different fields depending on the object type:

| Type | Fields searched |
|------|-----------------|
| Scene | Title, Details, Path, OSHash, Checksum, Marker titles |
| Image | Title, Details, Path, Checksum |
| Group | Title |
| Marker | Title, Scene title |
| Gallery | Title, Path, Checksum |
| Performer | Name, Aliases |
| Studio | Name, Aliases |
| Tag | Name, Aliases |

### Rules

Keyword matching uses the following rules:

- By default, all terms are required in the same matching field.
- Use the `or` keyword or `|` symbol to match either term.
- You can combine `or` sets in one query.
- Use the `-` symbol to exclude terms.
- The `-` symbol cannot be combined with an `or` operand.
- Use quotes (`"`) to match an exact phrase.
- Quotes can also escape keywords and symbols.
- `or` at the start or end of a query is treated literally.
- Keyword matching is case-insensitive.

#### Examples

| Query | Behavior | Explanation |
|---|---|---|
| `foo bar` | Requires both `foo` and `bar`. | Both terms must match in the same field. |
| `foo or bar` or `foo | bar` | Matches either `foo` or `bar`. | `or` and `|` are equivalent. |
| `foo or bar or baz xyz or zyx` | Matches one of `foo`, `bar`, or `baz`, and either `xyz` or `zyx`. | Multiple `or` sets can be combined. |
| `foo -bar` | Matches `foo`, excludes `bar`. | `-` excludes terms. |
| `-foo or bar` | Interpreted as `-foo` or `bar`. | `-` cannot be combined with an `or` operand. |
| `foo or bar -baz` | Matches `foo` or `bar`, excludes `baz`. | Exclusion is applied alongside the `or` set. |
| `"foo bar"` | Matches the exact phrase `foo bar`. | Quotes perform exact phrase matching. |
| `foo "-bar"` | Matches `foo` and the literal text `-bar`. | Quotes escape keyword/operator parsing. |
| `"foo bar" or baz -"xyz zyx"` | Matches `foo bar` or `baz`, excludes `xyz zyx`. | Quoted phrases can be used with `or` and `-`. |
| `or foo` | Matches literal `or` and `foo`. | `or` is literal at the start or end of a query. |

### Filters

Filters can be accessed by clicking the filter button on the right side of the query text field. 

Note that only one filter criterion per criterion type may be assigned.

#### Regex modifiers

Some filters have regex modifier as an option. Regex modifiers are case-sensitive by default.

### Sorting and page size

The current sorting field is shown next to the query text field, indicating the current sort field and order. The page size dropdown allows selecting from a standard set of objects per page, and allows setting a custom page size.

### Saved filters

Saved filters can be accessed with the bookmark button on the left of the query text field, and in the `Saved filters` section of the filter sidebar. The current filter can be saved by entering a filter name and clicking on the save button.

Each saved filter has an operations menu, opened with the button next to the filter name. It provides the following operations:

- `Overwrite` - replaces the saved filter with the current filter.
- `Rename` - changes the name of the saved filter, editing it in place in the list. Press `Enter` to confirm or `Escape` to cancel. The filter's criteria are left unchanged.
- `Set as default` - makes the saved filter the default filter for the page. See below.
- `Delete` - deletes the saved filter.

Saved filters are sorted alphabetically by title with capitalized titles sorted first.

### Default filter

The default filter for the top-level pages may be set to the current filter by clicking the `Set as default` button in the `Saved filters` section of the filter sidebar. An existing saved filter may be made the default by selecting `Set as default` from its operations menu.

### Permissions and shared filters

Saved filters, default filters, pinned criteria and pinned sort options are shared by all users. Saving a filter, including `Overwrite` and `Rename`, requires the `Save filters` permission. Deleting a saved filter requires `Delete saved filters`. `Set as default` and pinning criteria or sort options require `Shared list settings`. See [Users and Permissions](/help/Users.md).

### List memory

Each list page remembers how you left it, and opens that way the next time you open it without a query in the URL. It remembers the sort, page size, display mode, zoom, filter criteria and search. It does not remember the page number.

The memory is kept per list page, and separately for the list on each object's own page, such as the scenes of each performer. It is kept per user, in this browser: other users and other browsers have their own memory.

A URL with a query, such as a link to a filtered list, overrides the memory. Only the parts you change are remembered, so the criteria of a link are not remembered unless you change them.

The memory has two parts: the view (sort, page size, display mode and zoom) and the filter (criteria and search). A part is remembered as a whole once you change anything in it. The default filter applies only to the parts that you never changed.

## Reveal file in file manager

The `Reveal in file manager` action is available for file-based scenes, galleries and images in the `File Info` tab. This action will open the file manager to the location of the file on disk. The file will be selected if supported by the file manager.

This button will only be available to users whose role has `Manage files`, when accessing stash from a local loopback address (e.g. `localhost` or `127.0.0.1`), and will not be shown when accessing stash from a remote address.