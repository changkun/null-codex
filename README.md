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
go run . history daily-log
go run . history daily-log 20260314T090000.000000000Z-0001-edit
go run . restore daily-log 20260314T090000.000000000Z-0001-edit
EDITOR=vim go run . edit daily-log
EDITOR=vim go run . today
go run . list --tag work
go run . list --include-archived
go run . list --archived-only
go run . tasks --tag work
go run . tasks --include-archived
go run . tasks toggle daily-log 12
go run . search shipped --tag work
go run . search shipped --include-archived
go run . search --tag review
go run . view daily-log
go run . links daily-log
go run . backlinks daily-log
go run . graph
go run . serve
go run . serve --addr 127.0.0.1:9090
go run . delete daily-log
go run . doctor
go run . doctor --fix --report
go run . sync
```

## Behavior

- `create` writes a Markdown file with a heading based on the note title and optional normalized tags, or renders a built-in scaffold with `--template <name>`.
- `template` lists built-in and disk-backed templates with no arguments, or creates a note directly from a template with `template <name> [title]`.
- `edit` updates an existing note body and tags from the CLI, clears tags with `--clear-tags`, or opens the file in `$EDITOR`.
- Every create, edit, archive toggle, rename rewrite, delete, browser edit, and editor-driven change stores a local snapshot under `notes/.history/<note-id>/`.
- `history` lists saved note versions newest-first, or prints a diff between one saved version and the current note state when a version ID is supplied.
- `archive` and `unarchive` toggle a note's archived status by updating note metadata in place.
- `rename` changes a note's ID by renaming the Markdown file and updates `[[note-id]]` references across `notes/` without altering note titles, tags, archive status, or non-link body content.
- `restore` rewrites a note from one saved version so you can safely roll back or recover a deleted note locally.
- `list` shows note ID, last modified timestamp, title, and tags, filters with repeated `--tag` flags, and hides archived notes unless `--include-archived` or `--archived-only` is provided.
- `tasks` indexes Markdown checkbox items like `- [ ] follow up` across all notes, groups every open task by note, prints each task's file line number, filters with repeated `--tag` flags, and hides archived notes unless `--include-archived` or `--archived-only` is provided.
- `tasks toggle <id> <line>` flips the checkbox state for the Markdown task at that file line and records the change in note history.
- `search` performs case-insensitive full-text search across note titles and bodies, can be narrowed to notes matching all requested tags, and hides archived notes unless `--include-archived` or `--archived-only` is provided.
- `today` creates `notes/YYYY-MM-DD.md` from the built-in daily template when missing and opens today's daily note in `$EDITOR`.
- `view` prints the raw Markdown note content.
- `links` lists the note IDs referenced by `[[note-id]]` links in a note body.
- `backlinks` lists the note IDs that link to the requested note.
- `graph` emits the notebook's `[[note-id]]` link structure as Graphviz DOT, including dashed nodes for missing link targets.
- `serve` starts a local web UI that renders Markdown notes, rewrites `[[note-id]]` references into clickable note pages, shows backlinks and broken-link warnings, lets you filter the notebook by tag, surfaces a notebook-wide open task view at `/tasks`, supports toggling Markdown checkboxes from note pages and the task view, and supports creating and editing notes directly in the browser.
- `delete` removes the note file from `notes/`.
- `doctor` scans the notebook graph, reports broken `[[note-id]]` links, and flags notes with no backlinks so you can add links or create missing targets. `doctor --fix` creates stub notes for missing link targets, and `--report` lists each created stub note.
- `sync` treats `notes/` as its own Git repository, stages notebook changes, creates a `sync notebook <timestamp>` commit when needed, then runs `git pull --rebase` and `git push` against the branch upstream configured for `notes/`.

## Git Sync Setup

Initialize the notebook repository once and point it at a remote:

```bash
mkdir -p notes
git -C notes init -b main
git -C notes config user.name "Your Name"
git -C notes config user.email "you@example.com"
git -C notes remote add origin <remote-url>
git -C notes commit --allow-empty -m "init"
git -C notes push -u origin main
```

After that, `go run . sync` will back up local note changes and bring down remote notebook updates for multi-machine use.

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

Custom templates can be added as Markdown files under `notes/templates/<name>.md`. They use the normal note format, so the file title becomes the default note title, `Tags:` become default tags, and the remaining body becomes the template body.

Version IDs are filesystem-local snapshots with a UTC timestamp, sequence number, and action suffix, for example `20260314T090000.000000000Z-0001-edit`.
