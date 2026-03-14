# Notes CLI

A minimal terminal-based note-taking app in Go. Notes are stored as local Markdown files in the `notes/` directory.

## Commands

```bash
go run . create "Daily Log" "Shipped the first version."
go run . edit daily-log "Replaced the body from the CLI."
EDITOR=vim go run . edit daily-log
EDITOR=vim go run . today
go run . list
go run . search shipped
go run . view daily-log
go run . delete daily-log
```

## Behavior

- `create` writes a Markdown file with a heading based on the note title.
- `edit` updates an existing note either by replacing the body from the CLI or opening the file in `$EDITOR`.
- `list` shows note ID, last modified timestamp, and title.
- `search` performs case-insensitive full-text search across note titles and bodies.
- `today` creates `notes/YYYY-MM-DD.md` when missing and opens today's daily note in `$EDITOR`.
- `view` prints the raw Markdown note content.
- `delete` removes the note file from `notes/`.
