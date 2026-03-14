package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

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
		if err := editNote([]string{"daily-log", "New", "body"}); err != nil {
			t.Fatalf("editNote returned error: %v", err)
		}
	})

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "# Daily Log\n\nNew body\n" {
		t.Fatalf("unexpected file contents: %q", string(got))
	}

	if output != "edited daily-log\n" {
		t.Fatalf("unexpected stdout: %q", output)
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

func TestSearchNotesMatchesTitleAndBody(t *testing.T) {
	withTempDir(t)

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(notePath("project-ideas"), []byte("# Project Ideas\n\nBuild a search command.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(notePath("weekly-review"), []byte("# Weekly Review\n\nSearch indexing can wait.\n"), 0o644); err != nil {
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

	if !strings.Contains(lines[1], "project-ideas") || !strings.Contains(lines[1], "Project Ideas") {
		t.Fatalf("expected older note second, got %q", lines[1])
	}
}

func TestSearchNotesRequiresQuery(t *testing.T) {
	withTempDir(t)

	err := searchNotes(nil)
	if err == nil || err.Error() != "search requires a query" {
		t.Fatalf("expected missing query error, got %v", err)
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
