package main

import (
	"bytes"
	"io"
	"os"
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
	if err == nil || err.Error() != "--include-archived and --archived-only are only valid for list/search" {
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

	if !strings.Contains(lines[1], "project-ideas") || !strings.Contains(lines[1], "Project Ideas") {
		t.Fatalf("expected older note second, got %q", lines[1])
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

	if string(got) != "# 2026-03-14\n\n" {
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
