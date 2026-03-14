package notes

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	Dir               = "notes"
	AttachmentDirName = ".attachments"
	HistoryDirName    = ".history"
	TemplateDirName   = "templates"
	InboxID           = "inbox"
	InboxTitle        = "Inbox"
)

var NoteLinkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

type Meta struct {
	ID       string
	Title    string
	Tags     []string
	Archived bool
	ModTime  time.Time
}

type Content struct {
	Title       string
	Tags        []string
	Archived    bool
	Attachments []Attachment
	BodyLine    int
	Body        string
}

type Attachment struct {
	Name       string
	StoredName string
	MediaType  string
}

type BrokenLink struct {
	Source string
	Target string
}

type Edge struct {
	Source string
	Target string
}

func Path(id string) string {
	return filepath.Join(Dir, id+".md")
}

func AttachmentsRoot() string {
	return filepath.Join(Dir, AttachmentDirName)
}

func AttachmentsDir(id string) string {
	return filepath.Join(AttachmentsRoot(), id)
}

func AttachmentPath(id, storedName string) string {
	return filepath.Join(AttachmentsDir(id), storedName)
}

func AttachmentMarkdownPath(id, storedName string) string {
	return filepath.ToSlash(filepath.Join(AttachmentDirName, id, storedName))
}

func CustomTemplateDir() string {
	return filepath.Join(Dir, TemplateDirName)
}

func CustomTemplatePath(name string) (string, error) {
	if name == "" {
		return "", errors.New("template name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid template name %q", name)
	}
	return filepath.Join(CustomTemplateDir(), name+".md"), nil
}

func ReadContent(path string) (Content, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Content{}, err
	}
	return ParseContent(path, string(data)), nil
}

func ParseContent(path, data string) Content {
	lines := strings.Split(data, "\n")
	content := Content{
		Title:    strings.TrimSuffix(filepath.Base(path), ".md"),
		BodyLine: 1,
	}

	bodyStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			content.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		} else {
			content.Title = trimmed
		}
		bodyStart = i + 1
		break
	}

	if bodyStart < len(lines) {
		for bodyStart < len(lines) {
			line := strings.TrimSpace(lines[bodyStart])
			lower := strings.ToLower(line)
			switch {
			case strings.HasPrefix(lower, "tags:"):
				content.Tags = NormalizeTags(strings.Split(strings.TrimSpace(line[5:]), ","))
				bodyStart++
			case strings.HasPrefix(lower, "archived:"):
				content.Archived = strings.EqualFold(strings.TrimSpace(line[9:]), "true")
				bodyStart++
			case strings.HasPrefix(lower, "attachment:"):
				if attachment, ok := ParseAttachmentLine(line); ok {
					content.Attachments = append(content.Attachments, attachment)
				}
				bodyStart++
			default:
				goto body
			}
		}
	}

body:
	for bodyStart < len(lines) && strings.TrimSpace(lines[bodyStart]) == "" {
		bodyStart++
	}
	content.BodyLine = bodyStart + 1
	content.Body = strings.TrimRight(strings.Join(lines[bodyStart:], "\n"), "\n")
	return content
}

func Format(note Content) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", note.Title)
	if len(note.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(note.Tags, ", "))
	}
	if note.Archived {
		b.WriteString("Archived: true\n")
	}
	for _, attachment := range note.Attachments {
		fmt.Fprintf(&b, "Attachment: %s | %s | %s\n", attachment.StoredName, attachment.Name, attachment.MediaType)
	}
	b.WriteString("\n")
	if note.Body != "" {
		b.WriteString(note.Body)
		b.WriteString("\n")
	}
	return b.String()
}

func ParseAttachmentLine(line string) (Attachment, bool) {
	value := strings.TrimSpace(line[len("Attachment:"):])
	parts := strings.Split(value, "|")
	if len(parts) != 3 {
		return Attachment{}, false
	}
	attachment := Attachment{
		StoredName: strings.TrimSpace(parts[0]),
		Name:       strings.TrimSpace(parts[1]),
		MediaType:  strings.TrimSpace(parts[2]),
	}
	if attachment.StoredName == "" || attachment.Name == "" || attachment.MediaType == "" {
		return Attachment{}, false
	}
	return attachment, true
}

func AddAttachments(id string, existing []Attachment, body string, paths []string, embed bool) ([]Attachment, string, error) {
	attachments := append([]Attachment(nil), existing...)
	if err := os.MkdirAll(AttachmentsDir(id), 0o755); err != nil {
		return nil, "", err
	}
	for _, sourcePath := range paths {
		attachment, err := CopyAttachmentFile(id, strings.TrimSpace(sourcePath))
		if err != nil {
			return nil, "", err
		}
		attachments = append(attachments, attachment)
		if embed && attachment.IsImage() {
			body = AppendEmbeddedAttachment(body, id, attachment)
		}
	}
	return attachments, body, nil
}

func CopyAttachmentFile(id, sourcePath string) (Attachment, error) {
	if sourcePath == "" {
		return Attachment{}, errors.New("attachment path cannot be empty")
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return Attachment{}, err
	}
	if info.IsDir() {
		return Attachment{}, fmt.Errorf("attachment %q is a directory", sourcePath)
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return Attachment{}, err
	}
	defer source.Close()

	originalName := filepath.Base(sourcePath)
	storedName := UniqueAttachmentStoredName(id, originalName)
	targetPath := AttachmentPath(id, storedName)
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return Attachment{}, err
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		_ = os.Remove(targetPath)
		return Attachment{}, err
	}

	return Attachment{
		Name:       originalName,
		StoredName: storedName,
		MediaType:  DetectAttachmentMediaType(originalName),
	}, nil
}

func UniqueAttachmentStoredName(id, originalName string) string {
	clean := SanitizeAttachmentName(originalName)
	if clean == "" {
		clean = "attachment"
	}
	ext := filepath.Ext(clean)
	base := strings.TrimSuffix(clean, ext)
	candidate := clean
	for i := 1; ; i++ {
		if _, err := os.Stat(AttachmentPath(id, candidate)); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
}

func SanitizeAttachmentName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case strings.ContainsRune("._-", r):
			return r
		default:
			return '-'
		}
	}, name)
	return strings.Trim(name, ".-")
}

func DetectAttachmentMediaType(name string) string {
	if mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); mediaType != "" {
		return mediaType
	}
	return "application/octet-stream"
}

func AppendEmbeddedAttachment(body, id string, attachment Attachment) string {
	embed := fmt.Sprintf("![%s](%s)", attachment.Name, AttachmentMarkdownPath(id, attachment.StoredName))
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return embed
	}
	return body + "\n\n" + embed
}

func (a Attachment) IsImage() bool {
	return strings.HasPrefix(strings.ToLower(a.MediaType), "image/")
}

func MaterializeMultipartFiles(files []*multipart.FileHeader) ([]string, func(), error) {
	tempDir, err := os.MkdirTemp("", "note-attachments-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	var paths []string
	for _, fileHeader := range files {
		source, err := fileHeader.Open()
		if err != nil {
			cleanup()
			return nil, nil, err
		}

		name := SanitizeAttachmentName(fileHeader.Filename)
		if name == "" {
			name = "attachment"
		}
		targetPath := filepath.Join(tempDir, uniqueLocalAttachmentName(tempDir, name))
		target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			source.Close()
			cleanup()
			return nil, nil, err
		}
		if _, err := io.Copy(target, source); err != nil {
			target.Close()
			source.Close()
			cleanup()
			return nil, nil, err
		}
		target.Close()
		source.Close()
		paths = append(paths, targetPath)
	}
	return paths, cleanup, nil
}

func uniqueLocalAttachmentName(dir, name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, candidate)); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
}

func Load() ([]Meta, error) {
	entries, err := os.ReadDir(Dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var notes []Meta
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		id := strings.TrimSuffix(entry.Name(), ".md")
		content, err := ReadContent(Path(id))
		if err != nil {
			return nil, err
		}
		notes = append(notes, Meta{
			ID:       id,
			Title:    content.Title,
			Tags:     content.Tags,
			Archived: content.Archived,
			ModTime:  info.ModTime(),
		})
	}

	sort.Slice(notes, func(i, j int) bool {
		return notes[i].ModTime.After(notes[j].ModTime)
	})
	return notes, nil
}

func ReadTitle(path string) (string, error) {
	content, err := ReadContent(path)
	if err != nil {
		return "", err
	}
	return content.Title, nil
}

func ExtractLinks(body string) []string {
	matches := NoteLinkPattern.FindAllStringSubmatch(body, -1)
	seen := make(map[string]struct{})
	var links []string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		id := strings.TrimSpace(match[1])
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		links = append(links, id)
	}
	return links
}

func RewriteLinks(content, oldID, newID string) string {
	return NoteLinkPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := NoteLinkPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		target := parts[1]
		if strings.TrimSpace(target) != oldID {
			return match
		}
		leading := len(target) - len(strings.TrimLeftFunc(target, unicode.IsSpace))
		trailing := len(target) - len(strings.TrimRightFunc(target, unicode.IsSpace))
		return "[[" + target[:leading] + newID + target[len(target)-trailing:] + "]]"
	})
}

func ReadExisting(id string) ([]byte, error) {
	data, err := os.ReadFile(Path(id))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("note %q not found", id)
		}
		return nil, err
	}
	return data, nil
}

func FindBacklinks(targetID string) ([]string, error) {
	loaded, err := Load()
	if err != nil {
		return nil, err
	}

	var backlinks []string
	for _, note := range loaded {
		if note.ID == targetID {
			continue
		}
		content, err := ReadContent(Path(note.ID))
		if err != nil {
			return nil, err
		}
		if containsLink(ExtractLinks(content.Body), targetID) {
			backlinks = append(backlinks, note.ID)
		}
	}
	sort.Strings(backlinks)
	return backlinks, nil
}

func CollectNotebookLinks(loaded []Meta) (map[string]struct{}, []Edge, error) {
	noteSet := make(map[string]struct{}, len(loaded))
	for _, note := range loaded {
		noteSet[note.ID] = struct{}{}
	}

	var edges []Edge
	for _, note := range loaded {
		content, err := ReadContent(Path(note.ID))
		if err != nil {
			return nil, nil, err
		}
		for _, target := range ExtractLinks(content.Body) {
			edges = append(edges, Edge{Source: note.ID, Target: target})
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source == edges[j].Source {
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Source < edges[j].Source
	})
	return noteSet, edges, nil
}

func InspectNotebook(loaded []Meta) ([]BrokenLink, []string, error) {
	noteSet, edges, err := CollectNotebookLinks(loaded)
	if err != nil {
		return nil, nil, err
	}

	backlinkCounts := make(map[string]int, len(loaded))
	var brokenLinks []BrokenLink
	for _, edge := range edges {
		if edge.Source != edge.Target {
			if _, ok := noteSet[edge.Target]; ok {
				backlinkCounts[edge.Target]++
			} else {
				brokenLinks = append(brokenLinks, BrokenLink{Source: edge.Source, Target: edge.Target})
			}
		}
	}

	var orphanedNotes []string
	for _, note := range loaded {
		if backlinkCounts[note.ID] == 0 {
			orphanedNotes = append(orphanedNotes, note.ID)
		}
	}

	sort.Slice(brokenLinks, func(i, j int) bool {
		if brokenLinks[i].Source == brokenLinks[j].Source {
			return brokenLinks[i].Target < brokenLinks[j].Target
		}
		return brokenLinks[i].Source < brokenLinks[j].Source
	})
	sort.Strings(orphanedNotes)
	return brokenLinks, orphanedNotes, nil
}

func NormalizeTags(tags []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func containsLink(links []string, targetID string) bool {
	for _, link := range links {
		if link == targetID {
			return true
		}
	}
	return false
}
