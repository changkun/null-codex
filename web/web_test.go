package web

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"null-codex/pkg/notes"
)

func TestParseFilterOptionsNormalizesTagsAndModes(t *testing.T) {
	values := url.Values{
		"q":        {"search"},
		"tag":      {"Work", "urgent"},
		"archived": {"only"},
		"view":     {"overdue"},
	}

	filter := ParseFilterOptions(values, notes.NormalizeTags)
	if filter.Query != "search" || filter.ArchivedMode != "only" || filter.TaskView != "overdue" {
		t.Fatalf("unexpected filter: %+v", filter)
	}
	if strings.Join(filter.Tags, ",") != "urgent,work" {
		t.Fatalf("unexpected tags: %#v", filter.Tags)
	}
}

func TestRenderMarkdownHTMLRendersWikiLinksAndTaskForms(t *testing.T) {
	rendered := string(RenderMarkdownHTML(
		"",
		notes.Content{Body: "- [ ] Follow up\nSee [[target]] and [[missing]].", BodyLine: 1},
		map[string]struct{}{"target": {}},
		&RenderOptions{NoteID: "alpha", ReturnURL: "/notes/alpha"},
		time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC),
		func(id string) string { return "/notes/" + id },
		func(id, stored string) string { return "/attachments/" + id + "/" + stored },
	))

	if !strings.Contains(rendered, `action="/tasks/toggle"`) {
		t.Fatalf("expected toggle form, got %q", rendered)
	}
	if !strings.Contains(rendered, `<a href="/notes/target">[[target]]</a>`) {
		t.Fatalf("expected linked wiki target, got %q", rendered)
	}
	if !strings.Contains(rendered, `class="broken-link"`) {
		t.Fatalf("expected broken-link marker, got %q", rendered)
	}
}
