package web

import (
	"html"
	"html/template"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"null-codex/pkg/notes"
	"null-codex/tasks"
)

var (
	markdownImagePattern = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	markdownLinkPattern  = regexp.MustCompile(`\[(.*?)\]\(([^)]+)\)`)
	inlineCodePattern    = regexp.MustCompile("`([^`]+)`")
	boldPattern          = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicPattern        = regexp.MustCompile(`\*([^*]+)\*`)
)

type FilterOptions struct {
	Query        string
	Tags         []string
	TagsInput    string
	ArchivedMode string
	TaskView     string
}

type RenderOptions struct {
	NoteID    string
	ReturnURL string
}

func ParseFilterOptions(values url.Values, normalizeTags func([]string) []string) FilterOptions {
	filter := FilterOptions{
		Query:        strings.TrimSpace(values.Get("q")),
		ArchivedMode: queryArchivedMode(values),
		TaskView:     queryTaskView(values),
	}
	filter.Tags = parseWebTags(values.Get("tags"), normalizeTags)
	for _, tag := range values["tag"] {
		filter.Tags = normalizeTags(append(filter.Tags, tag))
	}
	filter.TagsInput = strings.Join(filter.Tags, ", ")
	return filter
}

func RenderMarkdownHTML(noteID string, content notes.Content, existing map[string]struct{}, taskOptions *RenderOptions, currentTime time.Time, noteURL func(string) string, attachmentURL func(string, string) string) template.HTML {
	lines := strings.Split(content.Body, "\n")
	var b strings.Builder
	inList := false
	inCode := false

	closeList := func() {
		if inList {
			b.WriteString("</ul>")
			inList = false
		}
	}

	for i, raw := range lines {
		lineNumber := content.BodyLine + i
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") {
			closeList()
			if inCode {
				b.WriteString("</code></pre>")
			} else {
				b.WriteString("<pre><code>")
			}
			inCode = !inCode
			continue
		}
		if inCode {
			b.WriteString(html.EscapeString(raw))
			b.WriteByte('\n')
			continue
		}
		if trimmed == "" {
			closeList()
			continue
		}

		if strings.HasPrefix(trimmed, "### ") {
			closeList()
			b.WriteString("<h3>" + renderInlineMarkdown(noteID, strings.TrimSpace(trimmed[4:]), existing, noteURL, attachmentURL) + "</h3>")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			closeList()
			b.WriteString("<h2>" + renderInlineMarkdown(noteID, strings.TrimSpace(trimmed[3:]), existing, noteURL, attachmentURL) + "</h2>")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			closeList()
			b.WriteString("<h1>" + renderInlineMarkdown(noteID, strings.TrimSpace(trimmed[2:]), existing, noteURL, attachmentURL) + "</h1>")
			continue
		}

		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			itemText := strings.TrimSpace(trimmed[2:])
			b.WriteString("<li>")
			b.WriteString(renderListItemMarkdown(noteID, itemText, existing, taskOptions, lineNumber, currentTime, noteURL, attachmentURL))
			b.WriteString("</li>")
			continue
		}

		closeList()
		b.WriteString("<p>" + renderInlineMarkdown(noteID, trimmed, existing, noteURL, attachmentURL) + "</p>")
	}

	closeList()
	if inCode {
		b.WriteString("</code></pre>")
	}
	return template.HTML(b.String())
}

func renderListItemMarkdown(noteID, text string, existing map[string]struct{}, taskOptions *RenderOptions, line int, currentTime time.Time, noteURL func(string) string, attachmentURL func(string, string) string) string {
	task, ok := tasks.ParseLine(text)
	if !ok {
		return renderInlineMarkdown(noteID, text, existing, noteURL, attachmentURL)
	}

	checked := ""
	if !task.Open {
		checked = " checked"
	}
	task = tasks.AnnotateDue(task, dateOnly(currentTime))
	body := renderInlineMarkdown(noteID, task.Text, existing, noteURL, attachmentURL) + renderTaskDueBadge(task)
	if taskOptions == nil || taskOptions.NoteID == "" {
		return `<label class="task-item"><input type="checkbox" disabled` + checked + `><span>` + body + `</span></label>`
	}

	return `<form class="task-toggle-form" method="post" action="/tasks/toggle">` +
		`<input type="hidden" name="note" value="` + html.EscapeString(taskOptions.NoteID) + `">` +
		`<input type="hidden" name="line" value="` + strconv.Itoa(line) + `">` +
		`<input type="hidden" name="return_to" value="` + html.EscapeString(taskOptions.ReturnURL) + `">` +
		`<label class="task-item task-item-toggle"><input type="checkbox" onchange="this.form.submit()"` + checked + `><span>` + body + `</span></label>` +
		`</form>`
}

func renderTaskDueBadge(task tasks.Task) string {
	if task.DueDate == "" {
		return ""
	}
	label := "Due " + task.DueDate
	className := "task-due"
	switch task.DueStatus {
	case "overdue":
		label += " overdue"
		className += " overdue"
	case "today":
		label += " today"
		className += " today"
	}
	return ` <span class="` + className + `">` + html.EscapeString(label) + `</span>`
}

func renderInlineMarkdown(noteID, text string, existing map[string]struct{}, noteURL func(string) string, attachmentURL func(string, string) string) string {
	escaped := html.EscapeString(text)
	escaped = renderMarkdownImages(noteID, escaped, attachmentURL)
	escaped = renderMarkdownLinks(escaped)
	escaped = renderWikiLinks(escaped, existing, noteURL)
	escaped = inlineCodePattern.ReplaceAllString(escaped, "<code>$1</code>")
	escaped = boldPattern.ReplaceAllString(escaped, "<strong>$1</strong>")
	escaped = italicPattern.ReplaceAllString(escaped, "<em>$1</em>")
	return escaped
}

func renderMarkdownImages(noteID, text string, attachmentURL func(string, string) string) string {
	return markdownImagePattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := markdownImagePattern.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		alt := html.EscapeString(parts[1])
		src := resolveAttachmentSrc(noteID, parts[2], attachmentURL)
		return `<img src="` + html.EscapeString(src) + `" alt="` + alt + `">`
	})
}

func renderMarkdownLinks(text string) string {
	return markdownLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := markdownLinkPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		label := parts[1]
		href := html.UnescapeString(parts[2])
		if strings.HasPrefix(href, "javascript:") {
			return label
		}
		return `<a href="` + html.EscapeString(href) + `">` + label + `</a>`
	})
}

func resolveAttachmentSrc(noteID, raw string, attachmentURL func(string, string) string) string {
	raw = strings.TrimSpace(raw)
	prefix := notes.AttachmentDirName + "/" + noteID + "/"
	if strings.HasPrefix(raw, prefix) {
		return attachmentURL(noteID, strings.TrimPrefix(raw, prefix))
	}
	return raw
}

func renderWikiLinks(text string, existing map[string]struct{}, noteURL func(string) string) string {
	return notes.NoteLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := notes.NoteLinkPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		target := strings.TrimSpace(parts[1])
		if _, ok := existing[target]; !ok {
			return `<span class="broken-link">[[` + html.EscapeString(target) + `]]</span>`
		}
		return `<a href="` + html.EscapeString(noteURL(target)) + `">[[` + html.EscapeString(target) + `]]</a>`
	})
}

func queryArchivedMode(values url.Values) string {
	archived := strings.TrimSpace(values.Get("archived"))
	switch {
	case archived == "1", strings.EqualFold(archived, "true"), strings.EqualFold(archived, "include"):
		return "include"
	case strings.EqualFold(archived, "only"):
		return "only"
	default:
		return "exclude"
	}
}

func queryTaskView(values url.Values) string {
	switch strings.ToLower(strings.TrimSpace(values.Get("view"))) {
	case "upcoming", "overdue":
		return strings.ToLower(strings.TrimSpace(values.Get("view")))
	default:
		return "all"
	}
}

func parseWebTags(tags string, normalizeTags func([]string) []string) []string {
	if strings.TrimSpace(tags) == "" {
		return nil
	}
	return normalizeTags(strings.Split(tags, ","))
}

func dateOnly(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
