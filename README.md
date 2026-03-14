# Notes CLI

A minimal terminal-based note-taking app in Go. Notes are stored as local Markdown files in the `notes/` directory.

## Commands

```bash
go run . create "Daily Log" --tag work --tag shipped "Shipped the first version."
go run . edit daily-log --tags work,review "Replaced the body from the CLI."
go run . edit daily-log --clear-tags
EDITOR=vim go run . edit daily-log
EDITOR=vim go run . today
go run . list --tag work
go run . search shipped --tag work
go run . search --tag review
go run . view daily-log
go run . delete daily-log
```

## Behavior

- `create` writes a Markdown file with a heading based on the note title and optional normalized tags.
- `edit` updates an existing note body and tags from the CLI, clears tags with `--clear-tags`, or opens the file in `$EDITOR`.
- `list` shows note ID, last modified timestamp, title, and tags, and can be filtered with repeated `--tag` flags.
- `search` performs case-insensitive full-text search across note titles and bodies and can be narrowed to notes matching all requested tags.
- `today` creates `notes/YYYY-MM-DD.md` when missing and opens today's daily note in `$EDITOR`.
- `view` prints the raw Markdown note content.
- `delete` removes the note file from `notes/`.

## Note Format

Tagged notes are stored as Markdown with a `Tags:` metadata line under the title:

```md
# Daily Log
Tags: shipped, work

Shipped the first version.
```
