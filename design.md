# bt Design

`bt` is a small CLI helper for keeping a timestamped blog/journal. Each invocation
lets the user write an entry in `$EDITOR` (`nvim`) and appends it to a per-day
Markdown file with a consistent header format.

## Overview

```
main.go            CLI definition (kong), single-instance lock, config loading
cmd/               Kong commands: add (default), view, edit, migrate
internal/          Core logic: timestamp parsing, entry files, editor, location, migration
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
6. A `Location: <location>` line is written right after the header. Since each
   entry is a self-contained file, the line is always present (possibly empty
   when no location is given); no deduplication against previous entries is
   performed.

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
- Each file holds exactly one entry; a day's entries are separate files, all
  printed one after another by `bt view`.

## File format

Each `.md` entry file contains a single entry and is plain Markdown:

```markdown
## Sunday 2026-09-20 9:19 AM PDT
Location: Somewhere

Things happened...
```

- Header line: `## <weekday> <YYYY-MM-DD> <H:MM AM/PM> <TZ>`, formatted with
  `timestampLayout = "Monday 2006-01-02 3:04 PM MST"` (`internal/edit.go:17`).
- The `Location: ...` line immediately follows the header and is always
  present, even when empty (`Location: ` with no value).
- Everything after the blank line following the location is the entry body;
  the temp file handed to the editor contains just the header, and the
  location line is prepended to the saved body.

## Configuration

`config.toml` (searched at `~/data/bt/config.toml` then
`~/.config/bt/config.toml`), loaded by kong's TOML loader so keys map to CLI
flags:

```toml
location  = "My Town"      # Default location; overridden by --location
data-dir  = "~/data/Blog"  # Base directory for entries
```

## Migration (`bt migrate`)

Earlier versions stored a whole day of entries in one file:
`<data-dir>/YYYY/YYYY-MM-DD.txt`, where each entry was appended as
`\n` + header + optional `Location:` line + body, and a `Location:` line was
only written when it differed from the previous entry's. `bt migrate`
converts these to the current one-file-per-entry layout.

**Safety model.** Migration is two-phase and dry-run by default. `bt migrate`
always scans and validates *everything* first and prints a report without
writing; `bt migrate --apply` performs the writes only if the entire plan
validated. Any malformed file aborts the whole run with a `file:line` error
before anything is written. Entries are written atomically (temp file +
rename), re-read to verify byte-exact content, and only then is the source
file deleted — so an interrupted run leaves sources intact and can be re-run
safely: destinations that already exist with identical content are skipped,
while a conflicting destination aborts.

**Discovery.** `internal.DiscoverLegacyFiles` walks `<data-dir>/YYYY/` for
files named `YYYY-MM-DD.txt` (year dir must match the filename date);
unrecognized files in year dirs are reported and skipped. The pattern is
disjoint from the current `YYYY/MM/DD/*.md` layout, so migrated files are
never re-processed.

**Headered files** (`internal.ParseLegacyDayFile`). Only `## ` lines whose
remainder parses with one of `headerLayouts` open a new entry — the current
`Monday 2006-01-02 3:04 PM MST` and the early slash-date
`Monday 01/02/2006 3:04 PM MST` — other lines starting with `## ` (e.g.
plain Markdown headings) are body text and pass through unchanged. Before
the first entry header, blank lines and `Location: ` lines are allowed: the
last pre-header location line becomes the first entry's location (unless the
entry has its own, which wins and keeps the pre-header line in the body),
and other pre-header lines are preserved at the top of the first entry's
body with a review flag; anything else before the first header is malformed
and aborts. Location carry-down
reproduces the old semantics: an entry with a `Location: ` line keeps it; an
entry without one inherits the most recent earlier location in the file.

**Headerless files.** Old files sometimes have no entry headers at all;
entry boundaries there are implied only by prose and cannot be parsed, so the
whole file becomes a single entry stamped at **noon local time on the file's
date**. (A file whose only `## ` lines are non-header text takes this path
too.) The first `Location: ` line is promoted to the structured location
line; any later ones are unattributable, so they remain in the body verbatim
and the entry is flagged `review: multiple location lines`.

**Timestamp caveat.** Header epochs are derived with `time.Parse` using
`timestampLayout`. A header whose zone abbreviation is not known to the local
zone is parsed into a fabricated zero-offset zone. The parsed wall clock
always equals the header text, so day placement and within-day ordering stay
correct; the header remains the authoritative display timestamp. Epochs for
headerless entries use the true local zone.

**Empty files** (whitespace only) are skipped; there is nothing to migrate.

## Editor integration

- `bt add`: temp file `blog*.md` (markdown highlighting in the editor),
  opened with `nvim <file> +star +`. On exit the header is stripped and the
  remainder is the entry. The temp file is always removed.
- `bt edit`: opens the actual entry file in place; changes persist directly.

## Concurrency

`~/.bt.lock` is created with `go-singleinstance`; a second concurrent `bt`
invocation prints a notice and exits. The lock is released when the process
exits.
