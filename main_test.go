package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
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
