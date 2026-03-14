# Notes CLI

A minimal terminal-based note-taking app in Go. Notes are stored as local Markdown files in the `notes/` directory.

## Commands

```bash
go run . create "Daily Log" "Shipped the first version."
go run . list
go run . view daily-log
go run . delete daily-log
```

## Behavior

- `create` writes a Markdown file with a heading based on the note title.
- `list` shows note ID, last modified timestamp, and title.
- `view` prints the raw Markdown note content.
- `delete` removes the note file from `notes/`.
