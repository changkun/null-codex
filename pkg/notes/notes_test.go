package notes

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseContentReadsMetadataAndAttachments(t *testing.T) {
	content := ParseContent(filepath.Join(Dir, "project.md"), "# Project\nTags: work, writing\nArchived: true\nAttachment: diagram.png | Diagram.png | image/png\n\nBody\n")

	if content.Title != "Project" || !content.Archived || content.Body != "Body" {
		t.Fatalf("unexpected content: %+v", content)
	}
	if !reflect.DeepEqual(content.Tags, []string{"work", "writing"}) {
		t.Fatalf("unexpected tags: %#v", content.Tags)
	}
	if len(content.Attachments) != 1 || content.Attachments[0].StoredName != "diagram.png" {
		t.Fatalf("unexpected attachments: %#v", content.Attachments)
	}
}

func TestInspectNotebookFindsBrokenLinksAndOrphans(t *testing.T) {
	loaded := []Meta{
		{ID: "hub", ModTime: time.Now()},
		{ID: "known", ModTime: time.Now()},
		{ID: "lonely", ModTime: time.Now()},
	}

	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	if err := os.MkdirAll(Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path("hub"), []byte("# Hub\n\nLinks to [[known]] and [[missing]].\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path("known"), []byte("# Known\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path("lonely"), []byte("# Lonely\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	broken, orphaned, err := InspectNotebook(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 1 || broken[0] != (BrokenLink{Source: "hub", Target: "missing"}) {
		t.Fatalf("unexpected broken links: %#v", broken)
	}
	if !reflect.DeepEqual(orphaned, []string{"hub", "lonely"}) {
		t.Fatalf("unexpected orphaned notes: %#v", orphaned)
	}
}
