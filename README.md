# bt

Helper program to save blog entries with a consistent format. Each entry is
saved to its own file:

```markdown
## Saturday 2025-06-28 10:45 AM CDT
Location: Somewhere
Things happened...

## Saturday 2025-06-28 11:13 AM CDT
Location: Somewhere
Other things happened...
```

Files are stored in `~/data/Blog/YYYY/MM/DD/<seconds since unix epoch>.md` (configurable)
Entries are edited with `nvim`

## Usage
```shell
# Edit the current day's entry (alias for bt add)
bt

# Edit the current day's entry with a different location
bt --location "Somewhere Else" 

# Set the timestamp to yesterday's date at 3:00 PM in the local (system) timezone
bt --at "yesterday 3:00 PM"

# Print all of yesterday's entries
bt view --at "yesterday"  

# Edit the entry for a specific date
bt edit --at "2023-03-11"
```

## Configuration
Reads from `~/data/bt/config.toml` or `~/.config/bt/config.toml`
Example:

```toml
location = "My Town"    # Default location, override with --location on the command line
data-dir = "~/data/Blog" # Base directory where entries are stored
```
