package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateNoteStoresTags(t *testing.T) {
	withTempDir(t)

	output := captureStdout(t, func() {
		if err := createNote([]string{"Project Ideas", "--tag", "Work", "--tags", "Research, writing", "Build", "a", "tagged", "note."}); err != nil {
			t.Fatalf("createNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(notePath("project-ideas"))
	if err != nil {
		t.Fatal(err)
	}

	want := "# Project Ideas\nTags: research, work, writing\n\nBuild a tagged note.\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}

	if output != "created project-ideas\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestCreateNoteFromTemplateUsesDefaultTitleAndBody(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	output := captureStdout(t, func() {
		if err := createNote([]string{"--template", "daily"}); err != nil {
			t.Fatalf("createNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(notePath("2026-03-14"))
	if err != nil {
		t.Fatal(err)
	}

	want := "# 2026-03-14\nTags: daily\n\n## Top of Mind\n\n## Priorities\n- [ ]\n\n## Notes\n\n## Wins\n\n## Tomorrow\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
	if output != "created 2026-03-14\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestTemplateCommandCreatesMeetingNote(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	output := captureStdout(t, func() {
		if err := templateNote([]string{"meeting", "Weekly Sync", "--tag", "team"}); err != nil {
			t.Fatalf("templateNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(notePath("weekly-sync"))
	if err != nil {
		t.Fatal(err)
	}

	want := "# Weekly Sync\nTags: meeting, team\n\n## Details\n- Date: 2026-03-14\n- Attendees:\n- Agenda:\n\n## Notes\n\n## Decisions\n\n## Action Items\n- [ ]\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
	if output != "created weekly-sync\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestTemplateCommandListsBuiltIns(t *testing.T) {
	withTempDir(t)

	output := captureStdout(t, func() {
		if err := templateNote(nil); err != nil {
			t.Fatalf("templateNote returned error: %v", err)
		}
	})

	if !strings.Contains(output, "daily") || !strings.Contains(output, "meeting") || !strings.Contains(output, "project") {
		t.Fatalf("expected built-in templates in output, got %q", output)
	}
}

func TestCreateNoteFromCustomTemplateLoadsFromDisk(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(customTemplateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	templateFile, err := customTemplatePath("standup")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templateFile, []byte("# Team Standup\nTags: team, sync\n\n## Updates\n\n## Blockers\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := createNote([]string{"--template", "standup"}); err != nil {
			t.Fatalf("createNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(notePath("team-standup"))
	if err != nil {
		t.Fatal(err)
	}

	want := "# Team Standup\nTags: sync, team\n\n## Updates\n\n## Blockers\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
	if output != "created team-standup\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestTemplateCommandCreatesCustomTemplateNoteWithExplicitTitle(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(customTemplateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	templateFile, err := customTemplatePath("retro")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templateFile, []byte("# Sprint Retro\nTags: team\n\n## Went Well\n\n## Needs Work\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := templateNote([]string{"retro", "Sprint 42 Retro", "--tag", "eng"}); err != nil {
			t.Fatalf("templateNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(notePath("sprint-42-retro"))
	if err != nil {
		t.Fatal(err)
	}

	want := "# Sprint 42 Retro\nTags: eng, team\n\n## Went Well\n\n## Needs Work\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
	if output != "created sprint-42-retro\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestTemplateCommandListsCustomTemplates(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(customTemplateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	templateFile, err := customTemplatePath("retro")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templateFile, []byte("# Sprint Retro\n\n## Went Well\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := templateNote(nil); err != nil {
			t.Fatalf("templateNote returned error: %v", err)
		}
	})

	if !strings.Contains(output, "retro") {
		t.Fatalf("expected custom template in output, got %q", output)
	}
}

func TestRunIncludesTemplateCommand(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := run([]string{"template", "project", "Roadmap"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	got, err := os.ReadFile(notePath("roadmap"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(got), "Tags: project") || !strings.Contains(string(got), "## Milestones") {
		t.Fatalf("unexpected template-generated note: %q", string(got))
	}
}

func TestEditRejectsTemplateFlag(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("daily-log")
	if err := os.WriteFile(path, []byte("# Daily Log\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := editNote([]string{"daily-log", "--template", "meeting"})
	if err == nil || err.Error() != "--template is only valid for create/template" {
		t.Fatalf("expected template flag error, got %v", err)
	}
}

func TestEditNoteReplacesBodyFromCLI(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("daily-log")
	if err := os.WriteFile(path, []byte("# Daily Log\n\nOld body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := editNote([]string{"daily-log", "--tag", "journal", "New", "body"}); err != nil {
			t.Fatalf("editNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "# Daily Log\nTags: journal\n\nNew body\n" {
		t.Fatalf("unexpected file contents: %q", string(got))
	}

	if output != "edited daily-log\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestEditNoteUpdatesOnlyTagsFromCLI(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("daily-log")
	if err := os.WriteFile(path, []byte("# Daily Log\n\nExisting body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := editNote([]string{"daily-log", "--tags", "work, urgent"}); err != nil {
		t.Fatalf("editNote returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "# Daily Log\nTags: urgent, work\n\nExisting body\n" {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
}

func TestEditNoteCanClearTags(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("daily-log")
	if err := os.WriteFile(path, []byte("# Daily Log\nTags: urgent, work\n\nExisting body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := editNote([]string{"daily-log", "--clear-tags"}); err != nil {
		t.Fatalf("editNote returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "# Daily Log\n\nExisting body\n" {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
}

func TestEditNoteRequiresEditorWhenNoBodyProvided(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("daily-log")
	if err := os.WriteFile(path, []byte("# Daily Log\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("EDITOR", "")

	err := editNote([]string{"daily-log"})
	if err == nil || err.Error() != "EDITOR is not set" {
		t.Fatalf("expected EDITOR error, got %v", err)
	}
}

func TestCreateNoteRejectsArchiveFilterFlags(t *testing.T) {
	withTempDir(t)

	err := createNote([]string{"Daily Log", "--include-archived"})
	if err == nil || err.Error() != "--include-archived and --archived-only are only valid for list/search/tasks" {
		t.Fatalf("expected filter flag error, got %v", err)
	}
}

func TestRunIncludesEditCommand(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("daily-log")
	if err := os.WriteFile(path, []byte("# Daily Log\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"edit", "daily-log", "Updated"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(got), "Updated\n") {
		t.Fatalf("expected updated body, got %q", string(got))
	}
}

func TestHistoryNoteListsVersionsAndPrintsDiff(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nOld body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := editNote([]string{"daily-log", "New body"}); err != nil {
		t.Fatalf("editNote returned error: %v", err)
	}

	historyOutput := captureStdout(t, func() {
		if err := historyNote([]string{"daily-log"}); err != nil {
			t.Fatalf("historyNote returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(historyOutput), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 history entry, got %d in %q", len(lines), historyOutput)
	}
	fields := strings.Split(lines[0], "\t")
	if len(fields) != 3 {
		t.Fatalf("expected version fields, got %q", lines[0])
	}
	if fields[2] != "edit" {
		t.Fatalf("expected edit action, got %q", fields[2])
	}

	diffOutput := captureStdout(t, func() {
		if err := historyNote([]string{"daily-log", fields[0]}); err != nil {
			t.Fatalf("historyNote diff returned error: %v", err)
		}
	})

	if !strings.Contains(diffOutput, "--- history:"+fields[0]) {
		t.Fatalf("expected diff header, got %q", diffOutput)
	}
	if !strings.Contains(diffOutput, "-Old body") || !strings.Contains(diffOutput, "+New body") {
		t.Fatalf("expected changed body in diff, got %q", diffOutput)
	}
}

func TestRestoreNoteRevertsToHistoricalVersion(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nOld body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := editNote([]string{"daily-log", "New body"}); err != nil {
		t.Fatalf("editNote returned error: %v", err)
	}

	versionID := firstHistoryVersionID(t, "daily-log")
	output := captureStdout(t, func() {
		if err := restoreNote([]string{"daily-log", versionID}); err != nil {
			t.Fatalf("restoreNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(notePath("daily-log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Daily Log\n\nOld body\n" {
		t.Fatalf("unexpected restored content: %q", string(got))
	}
	if output != "restored daily-log to "+versionID+"\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}

	historyOutput := captureStdout(t, func() {
		if err := historyNote([]string{"daily-log"}); err != nil {
			t.Fatalf("historyNote returned error: %v", err)
		}
	})
	if !strings.Contains(historyOutput, "\trestore\n") {
		t.Fatalf("expected restore snapshot after rollback, got %q", historyOutput)
	}
}

func TestRestoreNoteCanRecoverDeletedNote(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := createNote([]string{"Daily Log", "Body"}); err != nil {
		t.Fatalf("createNote returned error: %v", err)
	}
	if err := deleteNote([]string{"daily-log"}); err != nil {
		t.Fatalf("deleteNote returned error: %v", err)
	}

	versionID := firstHistoryVersionID(t, "daily-log")
	if err := restoreNote([]string{"daily-log", versionID}); err != nil {
		t.Fatalf("restoreNote returned error: %v", err)
	}

	got, err := os.ReadFile(notePath("daily-log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Daily Log\n\nBody\n" {
		t.Fatalf("unexpected restored deleted note: %q", string(got))
	}
}

func TestEditorEditsCreateHistorySnapshot(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	editorDir := t.TempDir()
	editorPath := filepath.Join(editorDir, "editor.sh")
	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nprintf '# Daily Log\\n\\nEdited in editor\\n' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editorPath)

	if err := editNote([]string{"daily-log"}); err != nil {
		t.Fatalf("editNote returned error: %v", err)
	}

	got, err := os.ReadFile(notePath("daily-log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Daily Log\n\nEdited in editor\n" {
		t.Fatalf("unexpected edited content: %q", string(got))
	}

	historyOutput := captureStdout(t, func() {
		if err := historyNote([]string{"daily-log"}); err != nil {
			t.Fatalf("historyNote returned error: %v", err)
		}
	})
	if !strings.Contains(historyOutput, "\tedit\n") {
		t.Fatalf("expected editor change to be versioned, got %q", historyOutput)
	}
}

func TestRunIncludesHistoryAndRestoreCommands(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nOld body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"edit", "daily-log", "New body"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	historyOutput := captureStdout(t, func() {
		if err := run([]string{"history", "daily-log"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})
	versionID := strings.Split(strings.TrimSpace(historyOutput), "\t")[0]

	if err := run([]string{"restore", "daily-log", versionID}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	got, err := os.ReadFile(notePath("daily-log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Daily Log\n\nOld body\n" {
		t.Fatalf("unexpected restored content: %q", string(got))
	}
}

func TestSyncCommitsAndPushesNotebookChanges(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	remotePath := setupGitSyncRepo(t)
	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nSynced body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := syncNotes(nil); err != nil {
			t.Fatalf("syncNotes returned error: %v", err)
		}
	})

	if output != "synced notes with origin/main\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}

	logOutput := strings.TrimSpace(runGitCommand(t, notesDir, "log", "--format=%s", "-1"))
	if logOutput != "sync notebook 2026-03-14T09:00:00Z" {
		t.Fatalf("unexpected commit message: %q", logOutput)
	}

	cloneDir := t.TempDir()
	runGitCommand(t, "", "clone", remotePath, cloneDir)
	got, err := os.ReadFile(filepath.Join(cloneDir, "daily-log.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Daily Log\n\nSynced body\n" {
		t.Fatalf("unexpected remote note content: %q", string(got))
	}
}

func TestSyncPullsRemoteChangesBeforePushing(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	remotePath := setupGitSyncRepo(t)
	if err := os.WriteFile(notePath("local-note"), []byte("# Local Note\n\nLocal body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	otherClone := t.TempDir()
	runGitCommand(t, "", "clone", remotePath, otherClone)
	configureGitIdentity(t, otherClone)
	if err := os.WriteFile(filepath.Join(otherClone, "remote-note.md"), []byte("# Remote Note\n\nRemote body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, otherClone, "add", "--all", ".")
	runGitCommand(t, otherClone, "commit", "-m", "remote change")
	runGitCommand(t, otherClone, "push", "origin", "main")

	if err := syncNotes(nil); err != nil {
		t.Fatalf("syncNotes returned error: %v", err)
	}

	if _, err := os.Stat(notePath("remote-note")); err != nil {
		t.Fatalf("expected pulled remote note, got %v", err)
	}

	cloneDir := t.TempDir()
	runGitCommand(t, "", "clone", remotePath, cloneDir)
	if _, err := os.Stat(filepath.Join(cloneDir, "local-note.md")); err != nil {
		t.Fatalf("expected pushed local note, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, "remote-note.md")); err != nil {
		t.Fatalf("expected remote note to remain after sync, got %v", err)
	}
}

func TestSyncRequiresConfiguredUpstream(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, notesDir, "init", "-b", "main")
	configureGitIdentity(t, notesDir)

	err := syncNotes(nil)
	if err == nil || err.Error() != "notes Git remote is not configured; set an upstream branch for notes/ before syncing" {
		t.Fatalf("expected upstream configuration error, got %v", err)
	}
}

func TestListNotesFiltersByTag(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("project-ideas"), []byte("# Project Ideas\nTags: work, writing\n\nPlan docs.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("garden-log"), []byte("# Garden Log\nTags: home\n\nWater tomatoes.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listNotes([]string{"--tag", "work"}); err != nil {
			t.Fatalf("listNotes returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 matching note, got %d in %q", len(lines), output)
	}
	if !strings.Contains(lines[0], "project-ideas") || !strings.Contains(lines[0], "work,writing") {
		t.Fatalf("unexpected list output: %q", lines[0])
	}
}

func TestListNotesHidesArchivedByDefault(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("active-note"), []byte("# Active Note\n\nStill current.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("old-note"), []byte("# Old Note\nArchived: true\n\nNo longer active.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listNotes(nil); err != nil {
			t.Fatalf("listNotes returned error: %v", err)
		}
	})

	if strings.Contains(output, "old-note") {
		t.Fatalf("expected archived note to be hidden, got %q", output)
	}
	if !strings.Contains(output, "active-note") {
		t.Fatalf("expected active note in output, got %q", output)
	}
}

func TestListNotesCanIncludeArchived(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("active-note"), []byte("# Active Note\n\nStill current.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("old-note"), []byte("# Old Note\nArchived: true\n\nNo longer active.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listNotes([]string{"--include-archived"}); err != nil {
			t.Fatalf("listNotes returned error: %v", err)
		}
	})

	if !strings.Contains(output, "active-note") || !strings.Contains(output, "old-note") {
		t.Fatalf("expected both notes in output, got %q", output)
	}
}

func TestListNotesCanShowOnlyArchived(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("active-note"), []byte("# Active Note\n\nStill current.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("old-note"), []byte("# Old Note\nArchived: true\n\nNo longer active.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listNotes([]string{"--archived-only"}); err != nil {
			t.Fatalf("listNotes returned error: %v", err)
		}
	})

	if strings.Contains(output, "active-note") {
		t.Fatalf("expected active note to be excluded, got %q", output)
	}
	if !strings.Contains(output, "old-note") {
		t.Fatalf("expected archived note in output, got %q", output)
	}
}

func TestSearchNotesMatchesTitleAndBody(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("project-ideas"), []byte("# Project Ideas\nTags: work\n\nBuild a search command.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("weekly-review"), []byte("# Weekly Review\nTags: review\n\nSearch indexing can wait.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := searchNotes([]string{"search"}); err != nil {
			t.Fatalf("searchNotes returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 matching notes, got %d in %q", len(lines), output)
	}

	if !strings.Contains(lines[0], "weekly-review") || !strings.Contains(lines[0], "Weekly Review") {
		t.Fatalf("expected newest note first, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "review") {
		t.Fatalf("expected tag column in output, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "Search indexing can wait.") {
		t.Fatalf("expected matching body snippet in output, got %q", lines[0])
	}

	if !strings.Contains(lines[1], "project-ideas") || !strings.Contains(lines[1], "Project Ideas") {
		t.Fatalf("expected older note second, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "Build a search command.") {
		t.Fatalf("expected matching body snippet in output, got %q", lines[1])
	}
}

func TestSearchNotesRequiresQuery(t *testing.T) {
	withTempDir(t)

	err := searchNotes(nil)
	if err == nil || err.Error() != "search requires a query or at least one --tag" {
		t.Fatalf("expected missing query error, got %v", err)
	}
}

func TestSearchNotesCanFilterByTagOnly(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("project-ideas"), []byte("# Project Ideas\nTags: work, writing\n\nBuild docs.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("garden-log"), []byte("# Garden Log\nTags: home\n\nPlant herbs.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := searchNotes([]string{"--tag", "writing"}); err != nil {
			t.Fatalf("searchNotes returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 matching note, got %d in %q", len(lines), output)
	}
	if !strings.Contains(lines[0], "project-ideas") || !strings.Contains(lines[0], "work,writing") {
		t.Fatalf("unexpected search output: %q", lines[0])
	}
	if !strings.Contains(lines[0], "Build docs.") {
		t.Fatalf("expected body snippet in output, got %q", lines[0])
	}
}

func TestSearchNotesAppliesTagFilterAlongsideQuery(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("project-ideas"), []byte("# Project Ideas\nTags: work\n\nBuild a search command.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("weekly-review"), []byte("# Weekly Review\nTags: review\n\nSearch indexing can wait.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := searchNotes([]string{"search", "--tag", "work"}); err != nil {
			t.Fatalf("searchNotes returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 matching note, got %d in %q", len(lines), output)
	}
	if !strings.Contains(lines[0], "project-ideas") {
		t.Fatalf("unexpected search output: %q", lines[0])
	}
	if !strings.Contains(lines[0], "Build a search command.") {
		t.Fatalf("expected matching body snippet in output, got %q", lines[0])
	}
}

func TestSearchNotesHidesArchivedByDefault(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("active-note"), []byte("# Active Note\n\nSearch is live.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("old-note"), []byte("# Old Note\nArchived: true\n\nSearch is retired.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := searchNotes([]string{"search"}); err != nil {
			t.Fatalf("searchNotes returned error: %v", err)
		}
	})

	if strings.Contains(output, "old-note") {
		t.Fatalf("expected archived note to be hidden, got %q", output)
	}
	if !strings.Contains(output, "active-note") {
		t.Fatalf("expected active note in output, got %q", output)
	}
}

func TestSearchNotesCanIncludeArchived(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("active-note"), []byte("# Active Note\n\nSearch is live.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("old-note"), []byte("# Old Note\nArchived: true\n\nSearch is retired.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := searchNotes([]string{"search", "--include-archived"}); err != nil {
			t.Fatalf("searchNotes returned error: %v", err)
		}
	})

	if !strings.Contains(output, "active-note") || !strings.Contains(output, "old-note") {
		t.Fatalf("expected both notes in output, got %q", output)
	}
}

func TestSearchNotesCanShowOnlyArchived(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("active-note"), []byte("# Active Note\n\nSearch is live.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("old-note"), []byte("# Old Note\nArchived: true\n\nSearch is retired.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := searchNotes([]string{"search", "--archived-only"}); err != nil {
			t.Fatalf("searchNotes returned error: %v", err)
		}
	})

	if strings.Contains(output, "active-note") {
		t.Fatalf("expected active note to be excluded, got %q", output)
	}
	if !strings.Contains(output, "old-note") {
		t.Fatalf("expected archived note in output, got %q", output)
	}
}

func TestSearchNotesPrintsNoMatches(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nWorked on docs.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := searchNotes([]string{"search"}); err != nil {
			t.Fatalf("searchNotes returned error: %v", err)
		}
	})

	if output != "no matching notes found\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestRunIncludesSearchCommand(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nImplemented search today.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := run([]string{"search", "implemented"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "daily-log") || !strings.Contains(output, "Daily Log") {
		t.Fatalf("expected search output to include matching note, got %q", output)
	}
	if !strings.Contains(output, "Implemented search today.") {
		t.Fatalf("expected search output to include matching body snippet, got %q", output)
	}
}

func TestSearchNotesSnippetUsesParsedBodyInsteadOfMetadata(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("release-plan"), []byte("# Release Plan\nTags: search\nArchived: true\n\nShip the body excerpt.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := searchNotes([]string{"search", "--include-archived"}); err != nil {
			t.Fatalf("searchNotes returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Ship the body excerpt.") {
		t.Fatalf("expected snippet from note body, got %q", output)
	}
	if strings.Contains(output, "Archived: true") {
		t.Fatalf("expected metadata to be excluded from snippet, got %q", output)
	}
}

func TestListTasksGroupsOpenCheckboxesByNote(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("project"), []byte("# Project\nTags: work\n\n## Tasks\n- [ ] Ship release\n- [x] Closed item\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("review"), []byte("# Review\nTags: work, review\nArchived: true\n\n- [ ] Write recap\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listTasks([]string{"--include-archived", "--tag", "work"}); err != nil {
			t.Fatalf("listTasks returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected grouped task output, got %d lines in %q", len(lines), output)
	}
	if !strings.Contains(lines[0], "review") || !strings.Contains(lines[0], "Review") {
		t.Fatalf("expected newest matching note first, got %q", lines[0])
	}
	if lines[1] != "\t1\t[ ] Write recap" {
		t.Fatalf("unexpected task line: %q", lines[1])
	}
	if !strings.Contains(lines[2], "project") || !strings.Contains(lines[2], "Project") {
		t.Fatalf("expected second task group, got %q", lines[2])
	}
	if lines[3] != "\t2\t[ ] Ship release" {
		t.Fatalf("unexpected open task line: %q", lines[3])
	}
	if strings.Contains(output, "Closed item") {
		t.Fatalf("expected completed tasks to be hidden, got %q", output)
	}
}

func TestListTasksAppliesArchivedFilterByDefault(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("active"), []byte("# Active\n\n- [ ] Visible task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("archived"), []byte("# Archived\nArchived: true\n\n- [ ] Hidden task\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listTasks(nil); err != nil {
			t.Fatalf("listTasks returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Visible task") {
		t.Fatalf("expected active task in output, got %q", output)
	}
	if strings.Contains(output, "Hidden task") {
		t.Fatalf("expected archived task to be hidden, got %q", output)
	}
}

func TestListTasksPrintsNoOpenTasksMessage(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("done"), []byte("# Done\n\n- [x] Finished\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listTasks(nil); err != nil {
			t.Fatalf("listTasks returned error: %v", err)
		}
	})

	if output != "no open tasks found\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestRunIncludesTasksCommand(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\n- [ ] Follow up\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := run([]string{"tasks"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "daily-log") || !strings.Contains(output, "Follow up") {
		t.Fatalf("expected tasks output, got %q", output)
	}
}

func TestToggleTaskUpdatesCheckboxByFileLine(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("project"), []byte("# Project\nTags: work\n\n- [ ] Ship release\n- [x] Write recap\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listTasks([]string{"toggle", "project", "4"}); err != nil {
			t.Fatalf("listTasks returned error: %v", err)
		}
	})

	if output != "toggled task 4 in project to done\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}

	got, err := os.ReadFile(notePath("project"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Project\nTags: work\n\n- [x] Ship release\n- [x] Write recap\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
}

func TestToggleTaskRejectsMissingTaskLine(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("project"), []byte("# Project\n\nPlain text\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := listTasks([]string{"toggle", "project", "3"})
	if err == nil || err.Error() != "line 3 is not a Markdown task" {
		t.Fatalf("expected task-line error, got %v", err)
	}
}

func TestDoctorNotesReportsBrokenLinksAndOrphans(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("hub"), []byte("# Hub\n\nLinks to [[known-note]] and [[missing-note]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("known-note"), []byte("# Known Note\n\nExisting body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("lonely-note"), []byte("# Lonely Note\n\nUnlinked body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := doctorNotes(nil); err != nil {
			t.Fatalf("doctorNotes returned error: %v", err)
		}
	})

	want := "Broken links:\n" +
		"- hub links to missing [[missing-note]]; fix the link or create notes/missing-note.md\n\n" +
		"Orphaned notes:\n" +
		"- hub has no backlinks; add [[hub]] from a related note\n" +
		"- lonely-note has no backlinks; add [[lonely-note]] from a related note\n\n" +
		"Summary: 1 broken links, 2 orphaned notes\n"
	if output != want {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestDoctorNotesFixCreatesStubNotes(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("hub"), []byte("# Hub\n\nLinks to [[known-note]] and [[missing-note]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("known-note"), []byte("# Known Note\n\nExisting body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("lonely-note"), []byte("# Lonely Note\n\nUnlinked body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := doctorNotes([]string{"--fix"}); err != nil {
			t.Fatalf("doctorNotes returned error: %v", err)
		}
	})

	want := "doctor: fixed 1 broken link by creating 1 stub note\n\n" +
		"Orphaned notes:\n" +
		"- hub has no backlinks; add [[hub]] from a related note\n" +
		"- lonely-note has no backlinks; add [[lonely-note]] from a related note\n\n" +
		"Summary: 0 broken links, 2 orphaned notes\n"
	if output != want {
		t.Fatalf("unexpected stdout: %q", output)
	}

	got, err := os.ReadFile(notePath("missing-note"))
	if err != nil {
		t.Fatalf("expected missing stub note to be created: %v", err)
	}

	wantNote := "# Missing Note\n\nStub note created by doctor --fix.\n"
	if string(got) != wantNote {
		t.Fatalf("unexpected stub note content: %q", got)
	}
}

func TestDoctorNotesFixReportListsCreatedStubNotes(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("hub"), []byte("# Hub\n\nLinks to [[missing-a]], [[missing-b]], and [[missing-a]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := doctorNotes([]string{"--fix", "--report"}); err != nil {
			t.Fatalf("doctorNotes returned error: %v", err)
		}
	})

	want := "doctor: fixed 2 broken links by creating 2 stub notes\n" +
		"Applied fixes:\n" +
		"- created notes/missing-a.md for [[missing-a]]\n" +
		"- created notes/missing-b.md for [[missing-b]]\n\n" +
		"Orphaned notes:\n" +
		"- hub has no backlinks; add [[hub]] from a related note\n\n" +
		"Summary: 0 broken links, 1 orphaned notes\n"
	if output != want {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestDoctorNotesPrintsCleanNotebook(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\n\nPoints at [[beta]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("beta"), []byte("# Beta\n\nPoints at [[alpha]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := doctorNotes(nil); err != nil {
			t.Fatalf("doctorNotes returned error: %v", err)
		}
	})

	if output != "doctor: no issues found\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestDoctorNotesRejectsArguments(t *testing.T) {
	withTempDir(t)

	err := doctorNotes([]string{"extra"})
	if err == nil || err.Error() != "unknown doctor argument \"extra\"" {
		t.Fatalf("expected argument error, got %v", err)
	}
}

func TestDoctorNotesRejectsReportWithoutFix(t *testing.T) {
	withTempDir(t)

	err := doctorNotes([]string{"--report"})
	if err == nil || err.Error() != "--report requires --fix" {
		t.Fatalf("expected argument error, got %v", err)
	}
}

func TestGraphNotesPrintsDOT(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("hub"), []byte("# Hub\n\nLinks to [[beta]] and [[missing-note]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("beta"), []byte("# Beta\n\nPoints at [[hub]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("lonely-note"), []byte("# Lonely Note\n\nNo links here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := graphNotes(nil); err != nil {
			t.Fatalf("graphNotes returned error: %v", err)
		}
	})

	want := "digraph notes {\n" +
		"  \"beta\";\n" +
		"  \"hub\";\n" +
		"  \"lonely-note\";\n" +
		"  \"missing-note\" [style=dashed];\n" +
		"  \"beta\" -> \"hub\";\n" +
		"  \"hub\" -> \"beta\";\n" +
		"  \"hub\" -> \"missing-note\";\n" +
		"}\n"
	if output != want {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestGraphNotesRejectsArguments(t *testing.T) {
	withTempDir(t)

	err := graphNotes([]string{"extra"})
	if err == nil || err.Error() != "graph does not take any arguments" {
		t.Fatalf("expected argument error, got %v", err)
	}
}

func TestRunIncludesGraphCommand(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\n\nPoints at [[beta]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("beta"), []byte("# Beta\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := run([]string{"graph"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	want := "digraph notes {\n" +
		"  \"alpha\";\n" +
		"  \"beta\";\n" +
		"  \"alpha\" -> \"beta\";\n" +
		"}\n"
	if output != want {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestRunIncludesDoctorCommand(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\n\nPoints at [[beta]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("beta"), []byte("# Beta\n\nPoints at [[alpha]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := run([]string{"doctor"}); err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	})

	if output != "doctor: no issues found\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestOpenTodayNoteCreatesDailyNote(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))
	t.Setenv("EDITOR", "true")

	if err := openTodayNote(); err != nil {
		t.Fatalf("openTodayNote returned error: %v", err)
	}

	got, err := os.ReadFile(notePath("2026-03-14"))
	if err != nil {
		t.Fatal(err)
	}

	want := "# 2026-03-14\nTags: daily\n\n## Top of Mind\n\n## Priorities\n- [ ]\n\n## Notes\n\n## Wins\n\n## Tomorrow\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
}

func TestOpenTodayNoteKeepsExistingBody(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("2026-03-14")
	want := "# 2026-03-14\n\nExisting entry\n"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("EDITOR", "true")
	if err := openTodayNote(); err != nil {
		t.Fatalf("openTodayNote returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != want {
		t.Fatalf("expected existing note to be preserved, got %q", string(got))
	}
}

func TestCaptureInboxCreatesDedicatedInboxNote(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	output := captureStdout(t, func() {
		if err := captureInbox([]string{"Remember", "the", "milk"}); err != nil {
			t.Fatalf("captureInbox returned error: %v", err)
		}
	})

	got, err := os.ReadFile(notePath("inbox"))
	if err != nil {
		t.Fatal(err)
	}

	want := "# Inbox\nTags: inbox\n\n- [2026-03-14 09:00 UTC] Remember the milk\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
	if output != "captured to inbox\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}

	historyOutput := captureStdout(t, func() {
		if err := historyNote([]string{"inbox"}); err != nil {
			t.Fatalf("historyNote returned error: %v", err)
		}
	})
	if !strings.Contains(historyOutput, "\tcreate\n") {
		t.Fatalf("expected inbox creation history, got %q", historyOutput)
	}
}

func TestCaptureInboxAppendsTasksToExistingNote(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 5, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("inbox"), []byte("# Inbox\nTags: inbox\n\n- [2026-03-14 09:00 UTC] Existing thought\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := captureInbox([]string{"--task", "Follow", "up", "with", "design"}); err != nil {
		t.Fatalf("captureInbox returned error: %v", err)
	}

	got, err := os.ReadFile(notePath("inbox"))
	if err != nil {
		t.Fatal(err)
	}

	want := "# Inbox\nTags: inbox\n\n- [2026-03-14 09:00 UTC] Existing thought\n- [ ] [2026-03-14 09:05 UTC] Follow up with design\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}

	historyOutput := captureStdout(t, func() {
		if err := historyNote([]string{"inbox"}); err != nil {
			t.Fatalf("historyNote returned error: %v", err)
		}
	})
	if !strings.Contains(historyOutput, "\tinbox-capture\n") {
		t.Fatalf("expected inbox capture history, got %q", historyOutput)
	}
}

func TestRunIncludesInboxCommand(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := run([]string{"inbox", "https://example.com"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	got, err := os.ReadFile(notePath("inbox"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "https://example.com") {
		t.Fatalf("expected inbox note to contain captured link, got %q", string(got))
	}
}

func TestRunIncludesTodayCommand(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	editorDir := t.TempDir()
	editorArgPath := filepath.Join(editorDir, "editor-arg")
	editorPath := filepath.Join(editorDir, "editor.sh")
	if err := os.WriteFile(editorPath, []byte("#!/bin/sh\nprintf '%s' \"$1\" > \""+editorArgPath+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editorPath)

	if err := run([]string{"today"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	gotArg, err := os.ReadFile(editorArgPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(gotArg) != notePath("2026-03-14") {
		t.Fatalf("expected editor to receive today's note path, got %q", string(gotArg))
	}
}

func TestArchiveNoteMarksNoteArchived(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("daily-log")
	if err := os.WriteFile(path, []byte("# Daily Log\nTags: work\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := archiveNote([]string{"daily-log"}); err != nil {
			t.Fatalf("archiveNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "# Daily Log\nTags: work\nArchived: true\n\nBody\n" {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
	if output != "archived daily-log\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestUnarchiveNoteClearsArchivedMetadata(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("daily-log")
	if err := os.WriteFile(path, []byte("# Daily Log\nTags: work\nArchived: true\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := unarchiveNote([]string{"daily-log"}); err != nil {
			t.Fatalf("unarchiveNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "# Daily Log\nTags: work\n\nBody\n" {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
	if output != "unarchived daily-log\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestRenameNotePreservesContentAndMetadata(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldPath := notePath("daily-log")
	want := "# Daily Log\nTags: urgent, work\nArchived: true\n\nBody\n"
	if err := os.WriteFile(oldPath, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := renameNote([]string{"daily-log", "project-log"}); err != nil {
			t.Fatalf("renameNote returned error: %v", err)
		}
	})

	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected old path to be gone, got err=%v", err)
	}

	got, err := os.ReadFile(notePath("project-log"))
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
	if output != "renamed daily-log to project-log\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestRenameNoteUpdatesListAndSearchDiscoverability(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\nTags: work\nArchived: true\n\nSearchable body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := renameNote([]string{"daily-log", "project-log"}); err != nil {
		t.Fatalf("renameNote returned error: %v", err)
	}

	listOutput := captureStdout(t, func() {
		if err := listNotes([]string{"--include-archived", "--tag", "work"}); err != nil {
			t.Fatalf("listNotes returned error: %v", err)
		}
	})

	if !strings.Contains(listOutput, "project-log") || strings.Contains(listOutput, "daily-log") {
		t.Fatalf("expected list output to include only renamed id, got %q", listOutput)
	}
	if !strings.Contains(listOutput, "Daily Log") || !strings.Contains(listOutput, "work") {
		t.Fatalf("expected preserved title/tags in list output, got %q", listOutput)
	}

	searchOutput := captureStdout(t, func() {
		if err := searchNotes([]string{"searchable", "--include-archived"}); err != nil {
			t.Fatalf("searchNotes returned error: %v", err)
		}
	})

	if !strings.Contains(searchOutput, "project-log") || strings.Contains(searchOutput, "daily-log") {
		t.Fatalf("expected search output to include only renamed id, got %q", searchOutput)
	}
	if !strings.Contains(searchOutput, "Daily Log") {
		t.Fatalf("expected preserved title in search output, got %q", searchOutput)
	}
}

func TestRenameNoteUpdatesLinkedReferencesAcrossNotes(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nSee [[daily-log]] and [[other-note]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("source-a"), []byte("# Source A\n\nPoints at [[daily-log]] twice: [[daily-log]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("source-b"), []byte("# Source B\nArchived: true\n\nKeeps spacing [[ daily-log ]] and ignores [[daily-log-2]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := renameNote([]string{"daily-log", "project-log"}); err != nil {
		t.Fatalf("renameNote returned error: %v", err)
	}

	got, err := os.ReadFile(notePath("project-log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Daily Log\n\nSee [[project-log]] and [[other-note]].\n" {
		t.Fatalf("unexpected renamed note contents: %q", string(got))
	}

	got, err = os.ReadFile(notePath("source-a"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Source A\n\nPoints at [[project-log]] twice: [[project-log]].\n" {
		t.Fatalf("unexpected source-a contents: %q", string(got))
	}

	got, err = os.ReadFile(notePath("source-b"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Source B\nArchived: true\n\nKeeps spacing [[ project-log ]] and ignores [[daily-log-2]].\n" {
		t.Fatalf("unexpected source-b contents: %q", string(got))
	}
}

func TestRenameNoteUpdatesBacklinksAfterReferenceRewrite(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("target"), []byte("# Target\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("source"), []byte("# Source\n\nPoints at [[target]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := renameNote([]string{"target", "renamed-target"}); err != nil {
		t.Fatalf("renameNote returned error: %v", err)
	}

	output := captureStdout(t, func() {
		if err := viewNote([]string{"renamed-target"}); err != nil {
			t.Fatalf("viewNote returned error: %v", err)
		}
	})

	want := "# Target\n\nBody\n\nBacklinks: source\n"
	if output != want {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestRenameNoteRejectsExistingTarget(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("project-log"), []byte("# Project Log\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := renameNote([]string{"daily-log", "project-log"})
	if err == nil || err.Error() != "note \"project-log\" already exists" {
		t.Fatalf("expected collision error, got %v", err)
	}
}

func TestRenameNoteRejectsInvalidTargetID(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := renameNote([]string{"daily-log", "../escape"})
	if err == nil || err.Error() != "new id cannot contain path separators" {
		t.Fatalf("expected invalid id error, got %v", err)
	}
}

func TestRunIncludesRenameCommand(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("daily-log"), []byte("# Daily Log\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"rename", "daily-log", "project-log"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if _, err := os.Stat(notePath("project-log")); err != nil {
		t.Fatalf("expected renamed note to exist, got %v", err)
	}
}

func TestRunIncludesArchiveCommands(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := notePath("daily-log")
	if err := os.WriteFile(path, []byte("# Daily Log\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"archive", "daily-log"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Archived: true\n") {
		t.Fatalf("expected archived metadata, got %q", string(got))
	}

	if err := run([]string{"unarchive", "daily-log"}); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "Archived: true\n") {
		t.Fatalf("expected archived metadata to be removed, got %q", string(got))
	}
}

func TestExtractNoteLinksDeduplicatesAndPreservesOrder(t *testing.T) {
	body := "See [[project-log]] and [[daily-log]]. Repeat [[project-log]] and ignore [[]]."

	got := extractNoteLinks(body)
	want := []string{"project-log", "daily-log"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected links: got %v want %v", got, want)
	}
}

func TestRenderMarkdownHTMLLinksExistingAndBrokenNotes(t *testing.T) {
	existing := map[string]struct{}{
		"target": {},
	}

	got := string(renderMarkdownHTML("", noteContent{Body: "See [[target]] and [[missing]].", BodyLine: 1}, existing, nil))

	if !strings.Contains(got, `<a class="wiki-link" href="/notes/target">[[target]]</a>`) {
		t.Fatalf("expected existing wiki link to render as anchor, got %q", got)
	}
	if !strings.Contains(got, `<span class="broken-link">[[missing]]</span>`) {
		t.Fatalf("expected missing wiki link to render as warning span, got %q", got)
	}
}

func TestRenderMarkdownHTMLRendersCheckboxItems(t *testing.T) {
	got := string(renderMarkdownHTML("", noteContent{Body: "- [ ] Open\n- [x] Done", BodyLine: 1}, nil, nil))

	if !strings.Contains(got, `<label class="task-item"><input type="checkbox" disabled><span>Open</span></label>`) {
		t.Fatalf("expected open checkbox item, got %q", got)
	}
	if !strings.Contains(got, `<label class="task-item"><input type="checkbox" disabled checked><span>Done</span></label>`) {
		t.Fatalf("expected closed checkbox item, got %q", got)
	}
}

func TestRenderMarkdownHTMLRendersToggleFormsWhenTaskContextPresent(t *testing.T) {
	got := string(renderMarkdownHTML(
		"project",
		noteContent{Body: "- [ ] Open", BodyLine: 4},
		nil,
		&taskRenderOptions{NoteID: "project", ReturnURL: "/notes/project"},
	))

	if !strings.Contains(got, `action="/tasks/toggle"`) {
		t.Fatalf("expected task toggle form, got %q", got)
	}
	if !strings.Contains(got, `name="line" value="4"`) {
		t.Fatalf("expected file line in form, got %q", got)
	}
	if strings.Contains(got, `disabled`) {
		t.Fatalf("expected interactive checkbox, got %q", got)
	}
}

func TestParseNoteContentReadsAttachments(t *testing.T) {
	got := parseNoteContent(notePath("project"), "# Project\nAttachment: diagram.png | Architecture Diagram.png | image/png\nAttachment: spec.pdf | Spec.pdf | application/pdf\n\nBody\n")

	if len(got.Attachments) != 2 {
		t.Fatalf("expected attachments, got %#v", got.Attachments)
	}
	if got.Attachments[0].StoredName != "diagram.png" || got.Attachments[1].MediaType != "application/pdf" {
		t.Fatalf("unexpected attachments: %#v", got.Attachments)
	}
}

func TestAttachNoteFilesCopiesFilesAndEmbedsImages(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("project"), []byte("# Project\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	imagePath := filepath.Join(t.TempDir(), "diagram.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := attachNoteFiles([]string{"project", imagePath, "--embed"}); err != nil {
			t.Fatalf("attachNoteFiles returned error: %v", err)
		}
	})

	if output != "attached 1 file(s) to project\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
	content, err := readNoteContent(notePath("project"))
	if err != nil {
		t.Fatal(err)
	}
	if len(content.Attachments) != 1 {
		t.Fatalf("expected stored attachment, got %#v", content.Attachments)
	}
	if !strings.Contains(content.Body, "![diagram.png](.attachments/project/diagram.png)") {
		t.Fatalf("expected embedded image markdown, got %q", content.Body)
	}
	if _, err := os.Stat(noteAttachmentPath("project", "diagram.png")); err != nil {
		t.Fatalf("expected copied attachment, got %v", err)
	}
}

func TestRenderMarkdownHTMLRendersAttachedImages(t *testing.T) {
	got := string(renderMarkdownHTML("project", noteContent{
		Body:     "![Diagram](.attachments/project/diagram.png)",
		BodyLine: 1,
	}, nil, nil))

	if !strings.Contains(got, `<img src="/attachments/project/diagram.png" alt="Diagram">`) {
		t.Fatalf("expected attachment image rendering, got %q", got)
	}
}

func TestParseServeOptions(t *testing.T) {
	opts, err := parseServeOptions([]string{"--addr", ":9999", "--watch"})
	if err != nil {
		t.Fatalf("parseServeOptions returned error: %v", err)
	}
	if opts.Addr != ":9999" {
		t.Fatalf("unexpected addr: %q", opts.Addr)
	}
	if !opts.Watch {
		t.Fatal("expected watch mode to be enabled")
	}

	_, err = parseServeOptions([]string{"--addr"})
	if err == nil || err.Error() != "--addr requires a value" {
		t.Fatalf("expected addr value error, got %v", err)
	}
}

func TestNotebookServerRefreshUpdatesSnapshot(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("project"), []byte("# Project\n\nInitial body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server, err := newNotebookServer(true)
	if err != nil {
		t.Fatalf("newNotebookServer returned error: %v", err)
	}
	if got := server.snapshotCopy().RenderedNotes["project"].RenderedBody; !strings.Contains(string(got), "Initial body.") {
		t.Fatalf("expected initial snapshot body, got %q", string(got))
	}

	updated := "# Project\nTags: work\n\nUpdated body.\n- [ ] Follow up\n"
	if err := os.WriteFile(notePath("project"), []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := server.refresh(); err != nil {
		t.Fatalf("refresh returned error: %v", err)
	}

	snapshot := server.snapshotCopy()
	if !strings.Contains(snapshot.NotesByID["project"].Body, "Updated body.") {
		t.Fatalf("expected refreshed note body, got %#v", snapshot.NotesByID["project"])
	}
	if len(snapshot.TaskGroups) != 1 || len(snapshot.TaskGroups[0].Tasks) != 1 {
		t.Fatalf("expected refreshed task index, got %#v", snapshot.TaskGroups)
	}
}

func TestServeEventsStreamsRefreshNotifications(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("project"), []byte("# Project\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	serverState, err := newNotebookServer(true)
	if err != nil {
		t.Fatalf("newNotebookServer returned error: %v", err)
	}
	testServer := httptest.NewServer(newServeMux(serverState))
	defer testServer.Close()

	resp, err := testServer.Client().Get(testServer.URL + "/events")
	if err != nil {
		t.Fatalf("GET /events returned error: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("unexpected content type: %q", got)
	}

	lines := make(chan string, 8)
	readErr := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				readErr <- err
				return
			}
			lines <- line
		}
	}()

	if err := os.WriteFile(notePath("project"), []byte("# Project\n\nChanged.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := serverState.refresh(); err != nil {
		t.Fatalf("refresh returned error: %v", err)
	}

	deadline := time.After(2 * time.Second)
	var body strings.Builder
	for {
		select {
		case line := <-lines:
			body.WriteString(line)
			if strings.Contains(body.String(), "event: refresh") {
				return
			}
		case err := <-readErr:
			t.Fatalf("reading event stream failed: %v", err)
		case <-deadline:
			t.Fatalf("timed out waiting for refresh event, got %q", body.String())
		}
	}
}

func TestViewNoteShowsLinksAndBacklinks(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("target"), []byte("# Target\n\nLinks to [[other-note]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("source-a"), []byte("# Source A\n\nPoints at [[target]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("source-b"), []byte("# Source B\nArchived: true\n\nAlso points at [[target]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := viewNote([]string{"target"}); err != nil {
			t.Fatalf("viewNote returned error: %v", err)
		}
	})

	want := "# Target\n\nLinks to [[other-note]].\n\nLinks: other-note\nBacklinks: source-a, source-b\n"
	if output != want {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestViewNotePrintsRawMarkdownWhenNoLinkMetadataExists(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("solo"), []byte("# Solo\n\nNo references here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := viewNote([]string{"solo"}); err != nil {
			t.Fatalf("viewNote returned error: %v", err)
		}
	})

	if output != "# Solo\n\nNo references here.\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestListNoteLinksPrintsOutgoingLinks(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("target"), []byte("# Target\n\nSee [[beta]] and [[ alpha ]] and [[beta]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listNoteLinks([]string{"target"}); err != nil {
			t.Fatalf("listNoteLinks returned error: %v", err)
		}
	})

	if output != "beta\nalpha\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestListNoteLinksPrintsEmptyMessage(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("solo"), []byte("# Solo\n\nNo references here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listNoteLinks([]string{"solo"}); err != nil {
			t.Fatalf("listNoteLinks returned error: %v", err)
		}
	})

	if output != "no links found\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestListNoteBacklinksPrintsSources(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("target"), []byte("# Target\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("source-b"), []byte("# Source B\n\nPoints at [[target]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("source-a"), []byte("# Source A\nArchived: true\n\nPoints at [[target]] twice: [[target]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listNoteBacklinks([]string{"target"}); err != nil {
			t.Fatalf("listNoteBacklinks returned error: %v", err)
		}
	})

	if output != "source-a\nsource-b\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestListNoteBacklinksPrintsEmptyMessage(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("solo"), []byte("# Solo\n\nNo references here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := listNoteBacklinks([]string{"solo"}); err != nil {
			t.Fatalf("listNoteBacklinks returned error: %v", err)
		}
	})

	if output != "no backlinks found\n" {
		t.Fatalf("unexpected stdout: %q", output)
	}
}

func TestLinksAndBacklinksRequireExistingNote(t *testing.T) {
	withTempDir(t)

	err := listNoteLinks([]string{"missing"})
	if err == nil || err.Error() != "note \"missing\" not found" {
		t.Fatalf("expected missing note error from links, got %v", err)
	}

	err = listNoteBacklinks([]string{"missing"})
	if err == nil || err.Error() != "note \"missing\" not found" {
		t.Fatalf("expected missing note error from backlinks, got %v", err)
	}
}

func TestRunIncludesLinksAndBacklinksCommands(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("target"), []byte("# Target\n\nLinks to [[outgoing]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("source"), []byte("# Source\n\nPoints at [[target]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	linksOutput := captureStdout(t, func() {
		if err := run([]string{"links", "target"}); err != nil {
			t.Fatalf("run returned error for links: %v", err)
		}
	})
	if linksOutput != "outgoing\n" {
		t.Fatalf("unexpected links stdout: %q", linksOutput)
	}

	backlinksOutput := captureStdout(t, func() {
		if err := run([]string{"backlinks", "target"}); err != nil {
			t.Fatalf("run returned error for backlinks: %v", err)
		}
	})
	if backlinksOutput != "source\n" {
		t.Fatalf("unexpected backlinks stdout: %q", backlinksOutput)
	}
}

func TestServeIndexPageRendersTagFilterAndWarnings(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\nTags: work\n\n- [ ] Follow up\nSee [[beta]] and [[missing]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("beta"), []byte("# Beta\nTags: home\n\nBack to [[alpha]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/?tag=work", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Alpha") {
		t.Fatalf("expected filtered note in body, got %q", body)
	}
	if strings.Contains(body, "Beta</strong>") {
		t.Fatalf("expected non-matching note to be filtered out, got %q", body)
	}
	if !strings.Contains(body, "#work") {
		t.Fatalf("expected tag filter controls, got %q", body)
	}
	if !strings.Contains(body, "1 broken") {
		t.Fatalf("expected broken-link summary in sidebar, got %q", body)
	}
	if !strings.Contains(body, `name="q" type="search"`) {
		t.Fatalf("expected search form in sidebar, got %q", body)
	}
}

func TestServeIndexPageSearchMatchesBodyAndPreservesFiltersInLinks(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("project-ideas"), []byte("# Project Ideas\nTags: work, writing\n\nBuild a search command.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("weekly-review"), []byte("# Weekly Review\nTags: review\n\nSearch indexing can wait.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/?q=search&tags=work,writing", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Project Ideas") {
		t.Fatalf("expected matching note in body, got %q", body)
	}
	if strings.Contains(body, "Weekly Review</strong>") {
		t.Fatalf("expected non-matching tag-filtered note to be hidden, got %q", body)
	}
	if !strings.Contains(body, "Build a search command.") {
		t.Fatalf("expected search snippet in sidebar, got %q", body)
	}
	if !strings.Contains(body, `href="/notes/project-ideas?q=search&amp;tag=work&amp;tag=writing"`) {
		t.Fatalf("expected note links to preserve active search filters, got %q", body)
	}
}

func TestServeIndexPageCanShowArchivedOnlySearchResults(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("active-note"), []byte("# Active Note\nTags: search\n\nSearch is live.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("old-note"), []byte("# Old Note\nTags: search\nArchived: true\n\nSearch is retired.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/?q=search&archived=only", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "Active Note</strong>") {
		t.Fatalf("expected active note to be filtered out, got %q", body)
	}
	if !strings.Contains(body, "Old Note") || !strings.Contains(body, "archived") {
		t.Fatalf("expected archived search result, got %q", body)
	}
}

func TestServeNotePageRendersBacklinksAndBrokenWarning(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\nTags: work\n\n- [ ] Follow up\nSee [[beta]] and [[missing]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("beta"), []byte("# Beta\n\nBack to [[alpha]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/notes/alpha", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Broken links in this note") {
		t.Fatalf("expected broken-link warning, got %q", body)
	}
	if !strings.Contains(body, `<a class="wiki-link" href="/notes/beta">[[beta]]</a>`) {
		t.Fatalf("expected rendered outgoing wiki link, got %q", body)
	}
	if !strings.Contains(body, `<span class="broken-link">[[missing]]</span>`) {
		t.Fatalf("expected rendered broken wiki link, got %q", body)
	}
	if !strings.Contains(body, `href="/notes/beta">beta</a>`) {
		t.Fatalf("expected outgoing links section, got %q", body)
	}
	if !strings.Contains(body, `href="/notes/beta">beta</a>`) || !strings.Contains(body, "Backlinks") {
		t.Fatalf("expected backlinks section, got %q", body)
	}
	if !strings.Contains(body, `action="/tasks/toggle"`) {
		t.Fatalf("expected interactive task toggle form on note page, got %q", body)
	}
}

func TestServeNotePageReturnsNotFoundForMissingNote(t *testing.T) {
	withTempDir(t)

	req := httptest.NewRequest(http.MethodGet, "/notes/missing", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestServeCreateNotePageRendersForm(t *testing.T) {
	withTempDir(t)

	req := httptest.NewRequest(http.MethodGet, "/new", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `<form class="form-grid" method="post" action="/notes">`) {
		t.Fatalf("expected create form, got %q", body)
	}
	if !strings.Contains(body, "Create note") {
		t.Fatalf("expected create action text, got %q", body)
	}
}

func TestServeTasksPageRendersGroupedOpenTasks(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\nTags: work\n\n- [ ] Alpha task\n- [x] Closed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("beta"), []byte("# Beta\nTags: home\nArchived: true\n\n- [ ] Archived task\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks?tag=work", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Open Tasks") {
		t.Fatalf("expected tasks heading, got %q", body)
	}
	if !strings.Contains(body, "Alpha task") {
		t.Fatalf("expected open task in body, got %q", body)
	}
	if !strings.Contains(body, `action="/tasks/toggle"`) || !strings.Contains(body, `name="line" value="3"`) {
		t.Fatalf("expected task toggle form in body, got %q", body)
	}
	if strings.Contains(body, "Closed") {
		t.Fatalf("expected completed task to be hidden, got %q", body)
	}
	if strings.Contains(body, "Archived task") {
		t.Fatalf("expected archived task filtered by default, got %q", body)
	}
	if !strings.Contains(body, `href="/tasks?tag=work"`) {
		t.Fatalf("expected task-page filter links to stay on /tasks, got %q", body)
	}
}

func TestServeToggleTaskPostUpdatesNoteAndRedirects(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\n\n- [ ] Alpha task\n- [x] Closed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	form := url.Values{
		"note":      {"alpha"},
		"line":      {"3"},
		"return_to": {"/tasks?tag=work"},
	}
	req := httptest.NewRequest(http.MethodPost, "/tasks/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d with body %q", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/tasks?tag=work" {
		t.Fatalf("unexpected redirect target: %q", location)
	}

	got, err := os.ReadFile(notePath("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Alpha\n\n- [x] Alpha task\n- [x] Closed\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
}

func TestServeTasksPageShowsEmptyState(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\n\n- [x] Done\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No open tasks match this filter.") {
		t.Fatalf("expected empty tasks state, got %q", rec.Body.String())
	}
}

func TestServeCreateNotePostCreatesNote(t *testing.T) {
	withTempDir(t)

	form := url.Values{
		"title":    {"Browser Draft"},
		"tags":     {"Work, Writing"},
		"body":     {"Draft from the browser.\nWith a second line."},
		"archived": {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/notes", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d with body %q", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/notes/browser-draft" {
		t.Fatalf("unexpected redirect target: %q", location)
	}

	got, err := os.ReadFile(notePath("browser-draft"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Browser Draft\nTags: work, writing\nArchived: true\n\nDraft from the browser.\nWith a second line.\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
}

func TestServeEditNotePageRendersExistingContent(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\nTags: work\nArchived: true\n\nBody text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/notes/alpha/edit", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `action="/notes/alpha"`) {
		t.Fatalf("expected edit form action, got %q", body)
	}
	if !strings.Contains(body, `value="Alpha"`) || !strings.Contains(body, `value="work"`) {
		t.Fatalf("expected existing title and tags, got %q", body)
	}
	if !strings.Contains(body, ">Body text.</textarea>") {
		t.Fatalf("expected existing body, got %q", body)
	}
	if !strings.Contains(body, `name="archived" type="checkbox" value="1" checked`) {
		t.Fatalf("expected archived checkbox to be checked, got %q", body)
	}
}

func TestServeAttachNotePostUploadsAndEmbedsImage(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("attachment", "diagram.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(fileWriter, "png"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("embed", "1"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("return_to", "/notes/alpha"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/notes/alpha/attachments", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d with body %q", rec.Code, rec.Body.String())
	}
	content, err := readNoteContent(notePath("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if len(content.Attachments) != 1 {
		t.Fatalf("expected stored attachment, got %#v", content.Attachments)
	}
	if !strings.Contains(content.Body, "![diagram.png](.attachments/alpha/diagram.png)") {
		t.Fatalf("expected embedded markdown, got %q", content.Body)
	}
}

func TestServeAttachmentServesUploadedFile(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(noteAttachmentsDir("alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\nAttachment: diagram.png | Diagram.png | image/png\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noteAttachmentPath("alpha", "diagram.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/attachments/alpha/diagram.png", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "image/png") {
		t.Fatalf("unexpected content type: %q", got)
	}
	if rec.Body.String() != "png" {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestServeUpdateNotePostEditsNote(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\nTags: work\nArchived: true\n\nOld body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	form := url.Values{
		"title": {"Alpha Revised"},
		"tags":  {"home, urgent"},
		"body":  {"Updated body with [[beta]]."},
	}
	req := httptest.NewRequest(http.MethodPost, "/notes/alpha", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d with body %q", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/notes/alpha" {
		t.Fatalf("unexpected redirect target: %q", location)
	}

	got, err := os.ReadFile(notePath("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Alpha Revised\nTags: home, urgent\n\nUpdated body with [[beta]].\n"
	if string(got) != want {
		t.Fatalf("unexpected file contents: %q", string(got))
	}
}

func TestServeNoteHistoryPageRendersVersionsAndDiff(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\n\nOld body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := editNote([]string{"alpha", "New body."}); err != nil {
		t.Fatalf("editNote returned error: %v", err)
	}

	versionID := firstHistoryVersionID(t, "alpha")
	req := httptest.NewRequest(http.MethodGet, "/notes/alpha/history?version="+url.QueryEscape(versionID), nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Restore this version") {
		t.Fatalf("expected restore action, got %q", body)
	}
	if !strings.Contains(body, `action="/notes/alpha/history/restore"`) {
		t.Fatalf("expected restore form action, got %q", body)
	}
	if !strings.Contains(body, `name="version" value="`+versionID+`"`) {
		t.Fatalf("expected selected version in form, got %q", body)
	}
	if !strings.Contains(body, "-Old body.") || !strings.Contains(body, "+New body.") {
		t.Fatalf("expected version diff in body, got %q", body)
	}
}

func TestServeRestoreNotePostRestoresSelectedVersion(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("alpha"), []byte("# Alpha\n\nOld body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := editNote([]string{"alpha", "New body."}); err != nil {
		t.Fatalf("editNote returned error: %v", err)
	}

	versionID := firstHistoryVersionID(t, "alpha")
	form := url.Values{"version": {versionID}}
	req := httptest.NewRequest(http.MethodPost, "/notes/alpha/history/restore", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d with body %q", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/notes/alpha" {
		t.Fatalf("unexpected redirect target: %q", location)
	}

	got, err := os.ReadFile(notePath("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Alpha\n\nOld body.\n" {
		t.Fatalf("unexpected restored content: %q", string(got))
	}
}

func TestServeJournalPageRendersCalendarAndDailyEntry(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("2026-03-14"), []byte("# 2026-03-14\nTags: daily\n\n## Notes\n\nJournal body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("2026-03-10"), []byte("# 2026-03-10\nTags: daily\n\nEarlier entry.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/journal?date=2026-03-14", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Journal") || !strings.Contains(body, "March 2026") {
		t.Fatalf("expected journal heading, got %q", body)
	}
	if !strings.Contains(body, `href="/journal?date=2026-03-14"`) {
		t.Fatalf("expected today navigation link, got %q", body)
	}
	if !strings.Contains(body, `href="/journal?date=2026-02-14"`) || !strings.Contains(body, `href="/journal?date=2026-04-14"`) {
		t.Fatalf("expected month navigation links, got %q", body)
	}
	if !strings.Contains(body, `calendar-day`) || !strings.Contains(body, `selected`) {
		t.Fatalf("expected selected calendar day, got %q", body)
	}
	if !strings.Contains(body, `calendar-dot`) {
		t.Fatalf("expected entry marker in calendar, got %q", body)
	}
	if !strings.Contains(body, "Journal body.") {
		t.Fatalf("expected selected daily note body, got %q", body)
	}
}

func TestServeJournalPageShowsEmptyStateForMissingDate(t *testing.T) {
	withTempDir(t)
	setFixedNow(t, time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC))

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath("2026-03-14"), []byte("# 2026-03-14\nTags: daily\n\nEntry.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/journal?date=2026-03-15", nil)
	rec := httptest.NewRecorder()

	newTestServeMux(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "No journal entry exists for this day yet.") {
		t.Fatalf("expected empty journal state, got %q", body)
	}
	if strings.Contains(body, "Entry.</p>") {
		t.Fatalf("expected missing date to avoid rendering another note, got %q", body)
	}
}

func newTestServeMux(t *testing.T) http.Handler {
	t.Helper()

	serverState, err := newNotebookServer(false)
	if err != nil {
		t.Fatalf("newNotebookServer returned error: %v", err)
	}
	return newServeMux(serverState)
}

func withTempDir(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})
}

func setupGitSyncRepo(t *testing.T) string {
	t.Helper()

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	remotePath := filepath.Join(t.TempDir(), "remote.git")
	runGitCommand(t, "", "init", "--bare", remotePath)
	runGitCommand(t, notesDir, "init", "-b", "main")
	configureGitIdentity(t, notesDir)
	runGitCommand(t, notesDir, "remote", "add", "origin", remotePath)
	runGitCommand(t, notesDir, "commit", "--allow-empty", "-m", "init")
	runGitCommand(t, notesDir, "push", "-u", "origin", "main")
	return remotePath
}

func configureGitIdentity(t *testing.T, dir string) {
	t.Helper()
	runGitCommand(t, dir, "config", "user.name", "Notes Test")
	runGitCommand(t, dir, "config", "user.email", "notes@example.com")
}

func runGitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}

	return buf.String()
}

func setFixedNow(t *testing.T, ts time.Time) {
	t.Helper()

	previous := now
	now = func() time.Time {
		return ts
	}

	t.Cleanup(func() {
		now = previous
	})
}

func firstHistoryVersionID(t *testing.T, id string) string {
	t.Helper()

	output := captureStdout(t, func() {
		if err := historyNote([]string{id}); err != nil {
			t.Fatalf("historyNote returned error: %v", err)
		}
	})

	line := strings.Split(strings.TrimSpace(output), "\n")[0]
	return strings.Split(line, "\t")[0]
}
