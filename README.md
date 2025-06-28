# bt

Helper program to save blog entries with a consistent format. Example

```markdown
## Saturday 06/28/2025 10:45 AM CDT
Location: Somewhere
Things happened...

## Saturday 06/28/2025 10:45 AM CDT
Other things happened... But location is only added if it's different than the last
entry in the file.
```

Files are stored in `~/data/Blog/YYYY/YYYY-MM-DD.txt` (configureable)
Entries are edited with `nvim`

## Usage
```shell
bt                                 # Edit the current day's entry (alias for add)
bt --location "Somewhere Else" # Edit the current day's entry with a different location
bt --at "yesterday 3:00 PM"    # Set the timestamp to yesterday's date at 3:00 PM in the local (system) timezone

bt view --at "yesterday"  # Dump the contents of yesterday's file
```

## Configuration
Reads from `~/data/bt/config.toml` or `~/.config/bt/config.toml`
Example:

```toml
location = "My Town"    # Default location, override with --location on the command line
data-dir = "~/data/Blog" # Base directory where entries are stored
```

## Setup

- ~Have the [getCoreLocationData](https://www.icloud.com/shortcuts/1121da1aeece4d38aec5d38007944b6f) shortcut installed
  ([Source](https://benward.uk/blog/macos-location-cli))~
