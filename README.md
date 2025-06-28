# bt

Helper program to save blog entries with a consistent format. Example

```markdown
## Saturday 06/28/2025 10:45 AM CDT
Location: Somewhere
Things happened...

## Saturday 06/28/2025 11:13 AM CDT
Other things happened... But location is only added if it's different than the last
entry in the file.
```

Files are stored in `~/data/Blog/YYYY/YYYY-MM-DD.txt` (configurable)
Entries are edited with `nvim`

## Usage
```shell
# Edit the current day's entry (alias for bt add)
bt

# Edit the current day's entry with a different location
bt --location "Somewhere Else" 

# Set the timestamp to yesterday's date at 3:00 PM in the local (system) timezone
bt --at "yesterday 3:00 PM"

# Print the contents of yesterday's file
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
