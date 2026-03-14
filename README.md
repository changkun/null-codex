# Notes CLI

A minimal terminal-based note-taking app in Go. Notes are stored as local Markdown files in the `notes/` directory.

## Commands

```bash
go run . create "Daily Log" --tag work --tag shipped "Shipped the first version."
go run . create --template daily
go run . create --template meeting "Weekly Sync"
go run . template
go run . template project "Null Codex"
go run . edit daily-log --tags work,review "Replaced the body from the CLI."
go run . edit daily-log --clear-tags
go run . archive daily-log
go run . unarchive daily-log
go run . rename daily-log project-log
EDITOR=vim go run . edit daily-log
EDITOR=vim go run . today
go run . list --tag work
go run . list --include-archived
go run . list --archived-only
go run . search shipped --tag work
go run . search shipped --include-archived
go run . search --tag review
go run . view daily-log
go run . links daily-log
go run . backlinks daily-log
go run . graph
go run . delete daily-log
go run . doctor
```

## Behavior

- `create` writes a Markdown file with a heading based on the note title and optional normalized tags, or renders a built-in scaffold with `--template daily|meeting|project`.
- `template` lists built-in templates with no arguments, or creates a note directly from a template with `template <name> [title]`.
- `edit` updates an existing note body and tags from the CLI, clears tags with `--clear-tags`, or opens the file in `$EDITOR`.
- `archive` and `unarchive` toggle a note's archived status by updating note metadata in place.
- `rename` changes a note's ID by renaming the Markdown file and updates `[[note-id]]` references across `notes/` without altering note titles, tags, archive status, or non-link body content.
- `list` shows note ID, last modified timestamp, title, and tags, filters with repeated `--tag` flags, and hides archived notes unless `--include-archived` or `--archived-only` is provided.
- `search` performs case-insensitive full-text search across note titles and bodies, can be narrowed to notes matching all requested tags, and hides archived notes unless `--include-archived` or `--archived-only` is provided.
- `today` creates `notes/YYYY-MM-DD.md` from the built-in daily template when missing and opens today's daily note in `$EDITOR`.
- `view` prints the raw Markdown note content.
- `links` lists the note IDs referenced by `[[note-id]]` links in a note body.
- `backlinks` lists the note IDs that link to the requested note.
- `graph` emits the notebook's `[[note-id]]` link structure as Graphviz DOT, including dashed nodes for missing link targets.
- `delete` removes the note file from `notes/`.
- `doctor` scans the notebook graph, reports broken `[[note-id]]` links, and flags notes with no backlinks so you can add links or create missing targets.

## Note Format

Tagged and archived notes are stored as Markdown with metadata lines under the title:

```md
# Daily Log
Tags: shipped, work
Archived: true

Shipped the first version.
```

Built-in templates add reusable body scaffolds and default tags:

- `daily`: top-of-mind, priorities, notes, wins, tomorrow
- `meeting`: details, notes, decisions, action items
- `project`: summary, goals, milestones, links, next actions
