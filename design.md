# bt Design

`bt` is a small CLI helper for keeping a timestamped blog/journal. Each invocation
lets the user write an entry in `$EDITOR` (`nvim`) and appends it to a per-day
Markdown file with a consistent header format.

## Overview

```
main.go            CLI definition (kong), single-instance lock, config loading
cmd/               Kong commands: add (default), view, edit
internal/          Core logic: timestamp parsing, entry files, editor, location
data/              Example/sample data tree
```

Flow of `bt add` (the default command when run with no subcommand):

1. Acquire a single-instance lock at `~/.bt.lock` (via `go-singleinstance`), so
   only one `bt` process runs at a time.
2. Parse configuration and CLI flags with `kong`, loading config from
   `~/data/bt/config.toml` or `~/.config/bt/config.toml` (first one found wins).
3. Resolve the target timestamp with `internal.ParseTimestamp`
   (`internal/arguments.go`), using `go-dateparser` to accept fuzzy human
   strings like `"yesterday 3:00 PM"` or `"2025-03-05"` in the system's local
   timezone. Empty means "now".
4. `internal.RunEditor` (`internal/edit.go`) writes the `## <timestamp>` header
   into a temp `.md` file, opens `nvim` on it, and on save returns the content
   below the header (trimmed). An empty entry aborts.
5. The destination file is computed by `internal.DestinationFile`, created if
   needed, and the entry is appended (see Storage layout below).
6. A `Location: <location>` line is added only when the location differs from
   the last location already present in that day's file
   (`internal.ShouldAddLocation*`), so repeated entries in the same place do
   not repeat the location.

`bt view` collects all entry files for the resolved day via
`internal.EntryFiles`, sorts them by the timestamp parsed from each file's
header (falling back to the filename epoch when unparseable), and prints them
wrapped to the terminal width (`github.com/bbrks/wrap` + `golang.org/x/term`).

`bt edit` opens the most recent entry file for the resolved day in `nvim`
directly; if none exists, it opens a new empty file at the would-be destination
path.

## Storage layout

Entries live under a configurable base directory (default `~/data/Blog`,
overridable with `--data-dir` or `data-dir` in config.toml):

```
<data-dir>/YYYY/MM/DD/<seconds-since-unix-epoch>.md
```

For example, an entry created at unix time `1789921166` on 2026-09-20 is stored
at `~/data/Blog/2026/09/20/1789921166.md`.

- The directory path comes from the *target timestamp* (`at.Format("2006")`,
  `"01"`, `"02"`), so `bt --at "yesterday"` writes into yesterday's folder.
- The filename epoch is written once at creation; the authoritative display
  timestamp is the one in the file's header line, which the user may have
  adjusted while editing. `EntryFiles` prefers the header timestamp for
  ordering and falls back to the filename epoch.
- Multiple entries on the same day are separate files, all appended into the
  same day's file set and printed one after another by `bt view`.

## File format

Each `.md` entry file is plain Markdown:

```markdown

## Sunday 2026-09-20 9:19 AM PDT
Location: Somewhere

Things happened...

## Sunday 2026-09-20 11:13 AM PDT
Other things happened...
```

- Header line: `## <weekday> <YYYY-MM-DD> <H:MM AM/PM> <TZ>`, formatted with
  `timestampLayout = "Monday 2006-01-02 3:04 PM MST"` (`internal/edit.go:17`).
- The optional `Location: ...` line immediately follows a header and appears
  only when it differs from the last location in the file (case-insensitive
  comparison).
- The header (and location) for each entry is repeated per entry; `bt add`
  appends `\n## ...` blocks to the same file as the day fills up.
- The temp file handed to the editor contains just the header; everything the
  user adds below it becomes the entry body.

## Configuration

`config.toml` (searched at `~/data/bt/config.toml` then
`~/.config/bt/config.toml`), loaded by kong's TOML loader so keys map to CLI
flags:

```toml
location  = "My Town"      # Default location; overridden by --location
data-dir  = "~/data/Blog"  # Base directory for entries
```

## Editor integration

- `bt add`: temp file `blog*.md` (markdown highlighting in the editor),
  opened with `nvim <file> +star +`. On exit the header is stripped and the
  remainder is the entry. The temp file is always removed.
- `bt edit`: opens the actual entry file in place; changes persist directly.

## Concurrency

`~/.bt.lock` is created with `go-singleinstance`; a second concurrent `bt`
invocation prints a notice and exits. The lock is released when the process
exits.
