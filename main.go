package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const notesDir = "notes"

var noteLinkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

var now = time.Now

type noteMeta struct {
	ID       string
	Title    string
	Tags     []string
	Archived bool
	ModTime  time.Time
}

type noteContent struct {
	Title    string
	Tags     []string
	Archived bool
	Body     string
}

type searchResult struct {
	Note    noteMeta
	Snippet string
}

type brokenLink struct {
	Source string
	Target string
}

type noteEdge struct {
	Source string
	Target string
}

type doctorOptions struct {
	Fix    bool
	Report bool
}

type noteOptions struct {
	Tags            []string
	ClearTags       bool
	IncludeArchived bool
	ArchivedOnly    bool
	Body            string
	Template        string
}

type createOptions struct {
	noteOptions
	Title string
}

type noteTemplate struct {
	Name         string
	Description  string
	DefaultTitle func() string
	DefaultTags  []string
	Body         func(title string) string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "create", "new":
		return createNote(args[1:])
	case "template":
		return templateNote(args[1:])
	case "edit":
		return editNote(args[1:])
	case "list", "ls":
		return listNotes(args[1:])
	case "search":
		return searchNotes(args[1:])
	case "archive":
		return archiveNote(args[1:])
	case "unarchive":
		return unarchiveNote(args[1:])
	case "rename", "mv":
		return renameNote(args[1:])
	case "today":
		return openTodayNote()
	case "view", "show":
		return viewNote(args[1:])
	case "links":
		return listNoteLinks(args[1:])
	case "backlinks":
		return listNoteBacklinks(args[1:])
	case "graph":
		return graphNotes(args[1:])
	case "delete", "rm":
		return deleteNote(args[1:])
	case "doctor":
		return doctorNotes(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func createNote(args []string) error {
	opts, err := parseCreateOptions(args)
	if err != nil {
		return err
	}
	return createNoteFromOptions(opts)
}

func templateNote(args []string) error {
	if len(args) == 0 {
		printTemplates()
		return nil
	}
	if len(args) == 1 && args[0] == "list" {
		printTemplates()
		return nil
	}

	templateName := strings.TrimSpace(args[0])
	if templateName == "" {
		return errors.New("template requires a template name")
	}

	opts, err := parseCreateOptions(append([]string{"--template", templateName}, args[1:]...))
	if err != nil {
		return err
	}

	return createNoteFromOptions(opts)
}

func listNotes(args []string) error {
	opts, err := parseFilterOptions(args)
	if err != nil {
		return err
	}

	notes, err := loadNotes()
	if err != nil {
		return err
	}

	notes = filterNotesByTags(notes, opts.Tags)
	notes = filterArchivedNotes(notes, opts)
	if len(notes) == 0 {
		fmt.Println("no notes found")
		return nil
	}

	for _, note := range notes {
		fmt.Printf("%s\t%s\t%s\t%s\n", note.ID, note.ModTime.Format(time.RFC3339), note.Title, formatTags(note.Tags))
	}

	return nil
}

func searchNotes(args []string) error {
	opts, err := parseSearchOptions(args)
	if err != nil {
		return err
	}

	notes, err := loadNotes()
	if err != nil {
		return err
	}

	notes = filterNotesByTags(notes, opts.Tags)
	notes = filterArchivedNotes(notes, opts)
	query := strings.ToLower(opts.Body)
	var matches []searchResult
	for _, note := range notes {
		content, err := readNoteContent(notePath(note.ID))
		if err != nil {
			return err
		}

		if query == "" {
			matches = append(matches, searchResult{
				Note:    note,
				Snippet: buildSearchSnippet(content.Body, query),
			})
			continue
		}

		searchText := strings.ToLower(note.Title + "\n" + content.Body)
		if strings.Contains(searchText, query) {
			matches = append(matches, searchResult{
				Note:    note,
				Snippet: buildSearchSnippet(content.Body, query),
			})
		}
	}

	if len(matches) == 0 {
		fmt.Println("no matching notes found")
		return nil
	}

	for _, match := range matches {
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
			match.Note.ID,
			match.Note.ModTime.Format(time.RFC3339),
			match.Note.Title,
			formatTags(match.Note.Tags),
			match.Snippet,
		)
	}

	return nil
}

func buildSearchSnippet(body, query string) string {
	lines := strings.Split(body, "\n")
	if query != "" {
		for _, line := range lines {
			index := strings.Index(strings.ToLower(line), query)
			if index >= 0 {
				return clipSearchSnippet(line, index, len(query))
			}
		}
	}

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return clipSearchSnippet(line, -1, 0)
		}
	}

	return "-"
}

func clipSearchSnippet(line string, matchStart, matchLen int) string {
	line = normalizeSearchSnippetWhitespace(line)
	if line == "" {
		return "-"
	}

	const maxSnippetLen = 80
	const matchContext = 30

	if len(line) <= maxSnippetLen {
		return line
	}

	start := 0
	if matchStart >= 0 {
		start = matchStart - matchContext
		if start < 0 {
			start = 0
		}
	}
	end := start + maxSnippetLen
	if end > len(line) {
		end = len(line)
		start = max(0, end-maxSnippetLen)
	}
	if matchStart >= 0 && matchStart+matchLen > end {
		end = min(len(line), matchStart+matchLen+matchContext)
		start = max(0, end-maxSnippetLen)
	}

	snippet := strings.TrimSpace(line[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(line) {
		snippet += "..."
	}
	return snippet
}

func normalizeSearchSnippetWhitespace(line string) string {
	line = strings.ReplaceAll(line, "\t", " ")
	return strings.Join(strings.Fields(line), " ")
}

func openTodayNote() error {
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return err
	}

	today := defaultDailyTitle()
	path := notePath(today)
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		_, content, err := buildNoteFromOptions(createOptions{
			noteOptions: noteOptions{Template: "daily"},
		})
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(formatNote(content)), 0o644); err != nil {
			return err
		}
	}

	return openInEditor(path)
}

func editNote(args []string) error {
	if len(args) == 0 {
		return errors.New("edit requires a note id")
	}

	id := args[0]
	path := notePath(id)

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("note %q not found", id)
		}
		return err
	}

	if len(args) == 1 {
		return openInEditor(path)
	}

	content, err := readNoteContent(path)
	if err != nil {
		return err
	}

	opts, err := parseNoteOptions(args[1:])
	if err != nil {
		return err
	}
	if err := validateMutationOptions(opts); err != nil {
		return err
	}

	if opts.Body == "" && len(opts.Tags) == 0 && !opts.ClearTags {
		return openInEditor(path)
	}

	if opts.Body != "" {
		content.Body = opts.Body
	}
	if opts.ClearTags {
		content.Tags = nil
	}
	if len(opts.Tags) > 0 {
		content.Tags = opts.Tags
	}

	if err := os.WriteFile(path, []byte(formatNote(content)), 0o644); err != nil {
		return err
	}

	fmt.Printf("edited %s\n", id)
	return nil
}

func viewNote(args []string) error {
	if len(args) != 1 {
		return errors.New("view requires a note id")
	}

	id := args[0]
	data, err := readExistingNote(id)
	if err != nil {
		return err
	}

	content := parseNoteContent(notePath(id), string(data))
	links := extractNoteLinks(content.Body)
	backlinks, err := findBacklinks(id)
	if err != nil {
		return err
	}

	fmt.Print(string(data))
	if len(links) == 0 && len(backlinks) == 0 {
		return nil
	}

	if !strings.HasSuffix(string(data), "\n") {
		fmt.Println()
	}
	fmt.Println()
	if len(links) > 0 {
		fmt.Printf("Links: %s\n", strings.Join(links, ", "))
	}
	if len(backlinks) > 0 {
		fmt.Printf("Backlinks: %s\n", strings.Join(backlinks, ", "))
	}
	return nil
}

func listNoteLinks(args []string) error {
	if len(args) != 1 {
		return errors.New("links requires a note id")
	}

	id := args[0]
	data, err := readExistingNote(id)
	if err != nil {
		return err
	}

	content := parseNoteContent(notePath(id), string(data))
	links := extractNoteLinks(content.Body)
	if len(links) == 0 {
		fmt.Println("no links found")
		return nil
	}

	for _, link := range links {
		fmt.Println(link)
	}
	return nil
}

func listNoteBacklinks(args []string) error {
	if len(args) != 1 {
		return errors.New("backlinks requires a note id")
	}

	id := args[0]
	if _, err := readExistingNote(id); err != nil {
		return err
	}

	backlinks, err := findBacklinks(id)
	if err != nil {
		return err
	}
	if len(backlinks) == 0 {
		fmt.Println("no backlinks found")
		return nil
	}

	for _, backlink := range backlinks {
		fmt.Println(backlink)
	}
	return nil
}

func graphNotes(args []string) error {
	if len(args) != 0 {
		return errors.New("graph does not take any arguments")
	}

	notes, err := loadNotes()
	if err != nil {
		return err
	}

	noteSet, edges, err := collectNotebookLinks(notes)
	if err != nil {
		return err
	}

	var existingNodes []string
	for _, note := range notes {
		existingNodes = append(existingNodes, note.ID)
	}
	sort.Strings(existingNodes)

	missingNodes := make(map[string]struct{})
	for _, edge := range edges {
		if _, ok := noteSet[edge.Target]; !ok {
			missingNodes[edge.Target] = struct{}{}
		}
	}

	var missingNodeIDs []string
	for id := range missingNodes {
		missingNodeIDs = append(missingNodeIDs, id)
	}
	sort.Strings(missingNodeIDs)

	fmt.Println("digraph notes {")
	for _, id := range existingNodes {
		fmt.Printf("  %s;\n", dotQuote(id))
	}
	for _, id := range missingNodeIDs {
		fmt.Printf("  %s [style=dashed];\n", dotQuote(id))
	}
	for _, edge := range edges {
		fmt.Printf("  %s -> %s;\n", dotQuote(edge.Source), dotQuote(edge.Target))
	}
	fmt.Println("}")
	return nil
}

func deleteNote(args []string) error {
	if len(args) != 1 {
		return errors.New("delete requires a note id")
	}

	path := notePath(args[0])
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("note %q not found", args[0])
		}
		return err
	}

	fmt.Printf("deleted %s\n", args[0])
	return nil
}

func doctorNotes(args []string) error {
	opts, err := parseDoctorOptions(args)
	if err != nil {
		return err
	}

	notes, err := loadNotes()
	if err != nil {
		return err
	}

	brokenLinks, orphanedNotes, err := inspectNotebook(notes)
	if err != nil {
		return err
	}

	initialBrokenCount := len(brokenLinks)
	var created []string
	if opts.Fix && len(brokenLinks) > 0 {
		created, err = repairBrokenLinks(brokenLinks)
		if err != nil {
			return err
		}
		notes, err = loadNotes()
		if err != nil {
			return err
		}
		brokenLinks, orphanedNotes, err = inspectNotebook(notes)
		if err != nil {
			return err
		}
	}

	if len(brokenLinks) == 0 && len(orphanedNotes) == 0 {
		if len(created) > 0 {
			fmt.Printf("doctor: fixed %d broken %s by creating %d stub %s\n",
				initialBrokenCount,
				pluralize(initialBrokenCount, "link", "links"),
				len(created),
				pluralize(len(created), "note", "notes"),
			)
			if opts.Report {
				printDoctorFixes(created)
			}
			return nil
		}
		fmt.Println("doctor: no issues found")
		return nil
	}

	if len(created) > 0 {
		fmt.Printf("doctor: fixed %d broken %s by creating %d stub %s\n",
			initialBrokenCount,
			pluralize(initialBrokenCount, "link", "links"),
			len(created),
			pluralize(len(created), "note", "notes"),
		)
		if opts.Report {
			printDoctorFixes(created)
		}
		fmt.Println()
	}

	printDoctorFindings(brokenLinks, orphanedNotes)
	return nil
}

func renameNote(args []string) error {
	if len(args) != 2 {
		return errors.New("rename requires an old id and new id")
	}

	oldID := strings.TrimSpace(args[0])
	newID := strings.TrimSpace(args[1])
	if err := validateRenameID(newID); err != nil {
		return err
	}

	oldPath := notePath(oldID)
	if _, err := os.Stat(oldPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("note %q not found", oldID)
		}
		return err
	}

	newPath := notePath(newID)
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("note %q already exists", newID)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	if err := renameLinkedReferences(oldID, newID); err != nil {
		if rollbackErr := os.Rename(newPath, oldPath); rollbackErr != nil {
			return fmt.Errorf("update links after rename: %v (rollback failed: %v)", err, rollbackErr)
		}
		return err
	}

	fmt.Printf("renamed %s to %s\n", oldID, newID)
	return nil
}

func archiveNote(args []string) error {
	return setArchived(args, true)
}

func unarchiveNote(args []string) error {
	return setArchived(args, false)
}

func setArchived(args []string, archived bool) error {
	if len(args) != 1 {
		if archived {
			return errors.New("archive requires a note id")
		}
		return errors.New("unarchive requires a note id")
	}

	id := args[0]
	path := notePath(id)
	content, err := readNoteContent(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("note %q not found", id)
		}
		return err
	}

	content.Archived = archived
	if err := os.WriteFile(path, []byte(formatNote(content)), 0o644); err != nil {
		return err
	}

	if archived {
		fmt.Printf("archived %s\n", id)
	} else {
		fmt.Printf("unarchived %s\n", id)
	}
	return nil
}

func openInEditor(path string) error {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return errors.New("EDITOR is not set")
	}

	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return errors.New("EDITOR is not set")
	}

	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func createNoteFromOptions(opts createOptions) error {
	if err := validateCreateOptions(opts.noteOptions); err != nil {
		return err
	}

	id, content, err := buildNoteFromOptions(opts)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(notePath(id), []byte(formatNote(content)), 0o644); err != nil {
		return err
	}

	fmt.Printf("created %s\n", id)
	return nil
}

func buildNoteFromOptions(opts createOptions) (string, noteContent, error) {
	title := strings.TrimSpace(opts.Title)
	content := noteContent{
		Title: title,
		Tags:  append([]string(nil), opts.Tags...),
		Body:  opts.Body,
	}

	if opts.Template != "" {
		template, ok := builtInTemplates()[opts.Template]
		if !ok {
			return "", noteContent{}, fmt.Errorf("unknown template %q", opts.Template)
		}
		if title == "" {
			title = template.DefaultTitle()
		}
		if title == "" {
			return "", noteContent{}, errors.New("title cannot be empty")
		}
		content.Title = title
		if opts.Body == "" {
			content.Body = template.Body(title)
		}
		content.Tags = normalizeTags(append(append([]string(nil), template.DefaultTags...), opts.Tags...))
	} else {
		if title == "" {
			return "", noteContent{}, errors.New("create requires a title")
		}
	}

	id, err := nextNoteID(title)
	if err != nil {
		return "", noteContent{}, err
	}
	return id, content, nil
}

func nextNoteID(title string) (string, error) {
	base := slugify(title)
	if base == "" {
		base = "note"
	}

	id := base
	for i := 1; ; i++ {
		_, err := os.Stat(notePath(id))
		if errors.Is(err, fs.ErrNotExist) {
			return id, nil
		}
		if err != nil {
			return "", err
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

func readTitle(path string) (string, error) {
	content, err := readNoteContent(path)
	if err != nil {
		return "", err
	}
	return content.Title, nil
}

func readNoteContent(path string) (noteContent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return noteContent{}, err
	}

	return parseNoteContent(path, string(data)), nil
}

func parseNoteContent(path, data string) noteContent {
	lines := strings.Split(data, "\n")
	content := noteContent{
		Title: strings.TrimSuffix(filepath.Base(path), ".md"),
	}

	bodyStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			content.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			bodyStart = i + 1
		} else {
			content.Title = trimmed
			bodyStart = i + 1
		}
		break
	}

	if bodyStart < len(lines) {
		for bodyStart < len(lines) {
			line := strings.TrimSpace(lines[bodyStart])
			lower := strings.ToLower(line)
			switch {
			case strings.HasPrefix(lower, "tags:"):
				content.Tags = normalizeTags(strings.Split(strings.TrimSpace(line[5:]), ","))
				bodyStart++
			case strings.HasPrefix(lower, "archived:"):
				content.Archived = strings.EqualFold(strings.TrimSpace(line[9:]), "true")
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
	content.Body = strings.TrimRight(strings.Join(lines[bodyStart:], "\n"), "\n")

	return content
}

func loadNotes() ([]noteMeta, error) {
	entries, err := os.ReadDir(notesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var notes []noteMeta
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		id := strings.TrimSuffix(entry.Name(), ".md")
		content, err := readNoteContent(notePath(id))
		if err != nil {
			return nil, err
		}

		notes = append(notes, noteMeta{
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

func notePath(id string) string {
	return filepath.Join(notesDir, id+".md")
}

func formatNote(note noteContent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", note.Title)
	if len(note.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(note.Tags, ", "))
	}
	if note.Archived {
		b.WriteString("Archived: true\n")
	}
	b.WriteString("\n")
	if note.Body != "" {
		b.WriteString(note.Body)
		b.WriteString("\n")
	}
	return b.String()
}

func extractNoteLinks(body string) []string {
	matches := noteLinkPattern.FindAllStringSubmatch(body, -1)
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

func renameLinkedReferences(oldID, newID string) error {
	entries, err := os.ReadDir(notesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		path := filepath.Join(notesDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		updated := rewriteNoteLinks(string(data), oldID, newID)
		if updated == string(data) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(updated), info.Mode()); err != nil {
			return err
		}
	}

	return nil
}

func rewriteNoteLinks(content, oldID, newID string) string {
	return noteLinkPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := noteLinkPattern.FindStringSubmatch(match)
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

func findBacklinks(targetID string) ([]string, error) {
	notes, err := loadNotes()
	if err != nil {
		return nil, err
	}

	var backlinks []string
	for _, note := range notes {
		if note.ID == targetID {
			continue
		}

		content, err := readNoteContent(notePath(note.ID))
		if err != nil {
			return nil, err
		}

		if containsLink(extractNoteLinks(content.Body), targetID) {
			backlinks = append(backlinks, note.ID)
		}
	}

	sort.Strings(backlinks)
	return backlinks, nil
}

func collectNotebookLinks(notes []noteMeta) (map[string]struct{}, []noteEdge, error) {
	noteSet := make(map[string]struct{}, len(notes))
	for _, note := range notes {
		noteSet[note.ID] = struct{}{}
	}

	var edges []noteEdge
	for _, note := range notes {
		content, err := readNoteContent(notePath(note.ID))
		if err != nil {
			return nil, nil, err
		}

		for _, target := range extractNoteLinks(content.Body) {
			edges = append(edges, noteEdge{
				Source: note.ID,
				Target: target,
			})
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

func readExistingNote(id string) ([]byte, error) {
	data, err := os.ReadFile(notePath(id))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("note %q not found", id)
		}
		return nil, err
	}
	return data, nil
}

func inspectNotebook(notes []noteMeta) ([]brokenLink, []string, error) {
	noteSet, edges, err := collectNotebookLinks(notes)
	if err != nil {
		return nil, nil, err
	}

	backlinkCounts := make(map[string]int, len(notes))
	var brokenLinks []brokenLink
	for _, edge := range edges {
		if edge.Source != edge.Target {
			if _, ok := noteSet[edge.Target]; ok {
				backlinkCounts[edge.Target]++
			} else {
				brokenLinks = append(brokenLinks, brokenLink{
					Source: edge.Source,
					Target: edge.Target,
				})
			}
		}
	}

	var orphanedNotes []string
	for _, note := range notes {
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

func parseDoctorOptions(args []string) (doctorOptions, error) {
	var opts doctorOptions
	for _, arg := range args {
		switch arg {
		case "--fix":
			opts.Fix = true
		case "--report":
			opts.Report = true
		default:
			return doctorOptions{}, fmt.Errorf("unknown doctor argument %q", arg)
		}
	}
	if opts.Report && !opts.Fix {
		return doctorOptions{}, errors.New("--report requires --fix")
	}
	return opts, nil
}

func repairBrokenLinks(brokenLinks []brokenLink) ([]string, error) {
	targetSet := make(map[string]struct{})
	for _, link := range brokenLinks {
		targetSet[link.Target] = struct{}{}
	}

	var targets []string
	for target := range targetSet {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	if len(targets) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return nil, err
	}

	for _, target := range targets {
		content := noteContent{
			Title: stubTitleFromID(target),
			Body:  "Stub note created by doctor --fix.",
		}
		if err := os.WriteFile(notePath(target), []byte(formatNote(content)), 0o644); err != nil {
			return nil, err
		}
	}

	return targets, nil
}

func stubTitleFromID(id string) string {
	parts := strings.FieldsFunc(id, func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsSpace(r)
	})
	if len(parts) == 0 {
		return id
	}

	for i, part := range parts {
		runes := []rune(strings.ToLower(part))
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func printDoctorFixes(created []string) {
	fmt.Println("Applied fixes:")
	for _, id := range created {
		fmt.Printf("- created %s for [[%s]]\n", notePath(id), id)
	}
}

func printDoctorFindings(brokenLinks []brokenLink, orphanedNotes []string) {
	if len(brokenLinks) > 0 {
		fmt.Println("Broken links:")
		for _, link := range brokenLinks {
			fmt.Printf("- %s links to missing [[%s]]; fix the link or create %s\n", link.Source, link.Target, notePath(link.Target))
		}
	}

	if len(orphanedNotes) > 0 {
		if len(brokenLinks) > 0 {
			fmt.Println()
		}
		fmt.Println("Orphaned notes:")
		for _, id := range orphanedNotes {
			fmt.Printf("- %s has no backlinks; add [[%s]] from a related note\n", id, id)
		}
	}

	fmt.Printf("\nSummary: %d broken links, %d orphaned notes\n", len(brokenLinks), len(orphanedNotes))
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func dotQuote(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func containsLink(links []string, targetID string) bool {
	for _, link := range links {
		if link == targetID {
			return true
		}
	}
	return false
}

func slugify(input string) string {
	var b strings.Builder
	lastDash := false

	for _, r := range strings.ToLower(strings.TrimSpace(input)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{})
	var normalized []string
	for _, tag := range tags {
		tag = slugify(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	sort.Strings(normalized)
	return normalized
}

func parseCreateOptions(args []string) (createOptions, error) {
	var opts createOptions
	var positionals []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--clear-tags":
			opts.ClearTags = true
		case arg == "--include-archived":
			opts.IncludeArchived = true
		case arg == "--archived-only":
			opts.ArchivedOnly = true
		case arg == "--tag":
			if i+1 >= len(args) {
				return createOptions{}, errors.New("--tag requires a value")
			}
			opts.Tags = append(opts.Tags, args[i+1])
			i++
		case strings.HasPrefix(arg, "--tag="):
			opts.Tags = append(opts.Tags, strings.TrimPrefix(arg, "--tag="))
		case arg == "--tags":
			if i+1 >= len(args) {
				return createOptions{}, errors.New("--tags requires a value")
			}
			opts.Tags = append(opts.Tags, strings.Split(args[i+1], ",")...)
			i++
		case strings.HasPrefix(arg, "--tags="):
			opts.Tags = append(opts.Tags, strings.Split(strings.TrimPrefix(arg, "--tags="), ",")...)
		case arg == "--template":
			if i+1 >= len(args) {
				return createOptions{}, errors.New("--template requires a value")
			}
			opts.Template = strings.TrimSpace(strings.ToLower(args[i+1]))
			i++
		case strings.HasPrefix(arg, "--template="):
			opts.Template = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(arg, "--template=")))
		default:
			positionals = append(positionals, arg)
		}
	}

	opts.Tags = normalizeTags(opts.Tags)
	if len(positionals) > 0 {
		opts.Title = strings.TrimSpace(positionals[0])
	}
	if len(positionals) > 1 {
		opts.Body = strings.TrimSpace(strings.Join(positionals[1:], " "))
	}
	return opts, nil
}

func parseNoteOptions(args []string) (noteOptions, error) {
	var opts noteOptions
	var bodyParts []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--clear-tags":
			opts.ClearTags = true
		case arg == "--include-archived":
			opts.IncludeArchived = true
		case arg == "--archived-only":
			opts.ArchivedOnly = true
		case arg == "--tag":
			if i+1 >= len(args) {
				return noteOptions{}, errors.New("--tag requires a value")
			}
			opts.Tags = append(opts.Tags, args[i+1])
			i++
		case strings.HasPrefix(arg, "--tag="):
			opts.Tags = append(opts.Tags, strings.TrimPrefix(arg, "--tag="))
		case arg == "--tags":
			if i+1 >= len(args) {
				return noteOptions{}, errors.New("--tags requires a value")
			}
			opts.Tags = append(opts.Tags, strings.Split(args[i+1], ",")...)
			i++
		case strings.HasPrefix(arg, "--tags="):
			opts.Tags = append(opts.Tags, strings.Split(strings.TrimPrefix(arg, "--tags="), ",")...)
		case arg == "--template":
			if i+1 >= len(args) {
				return noteOptions{}, errors.New("--template requires a value")
			}
			opts.Template = strings.TrimSpace(strings.ToLower(args[i+1]))
			i++
		case strings.HasPrefix(arg, "--template="):
			opts.Template = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(arg, "--template=")))
		default:
			bodyParts = append(bodyParts, arg)
		}
	}

	opts.Tags = normalizeTags(opts.Tags)
	opts.Body = strings.TrimSpace(strings.Join(bodyParts, " "))
	return opts, nil
}

func parseFilterOptions(args []string) (noteOptions, error) {
	opts, err := parseNoteOptions(args)
	if err != nil {
		return noteOptions{}, err
	}
	if opts.Body != "" {
		return noteOptions{}, fmt.Errorf("unexpected argument %q", opts.Body)
	}
	if opts.ClearTags {
		return noteOptions{}, errors.New("--clear-tags is only valid for create/edit")
	}
	if opts.Template != "" {
		return noteOptions{}, errors.New("--template is only valid for create/template")
	}
	if opts.ArchivedOnly {
		opts.IncludeArchived = true
	}
	return opts, nil
}

func validateMutationOptions(opts noteOptions) error {
	if opts.IncludeArchived || opts.ArchivedOnly {
		return errors.New("--include-archived and --archived-only are only valid for list/search")
	}
	if opts.Template != "" {
		return errors.New("--template is only valid for create/template")
	}
	return nil
}

func validateCreateOptions(opts noteOptions) error {
	if opts.IncludeArchived || opts.ArchivedOnly {
		return errors.New("--include-archived and --archived-only are only valid for list/search")
	}
	return nil
}

func parseSearchOptions(args []string) (noteOptions, error) {
	opts, err := parseNoteOptions(args)
	if err != nil {
		return noteOptions{}, err
	}
	if opts.ClearTags {
		return noteOptions{}, errors.New("--clear-tags is only valid for create/edit")
	}
	if opts.Template != "" {
		return noteOptions{}, errors.New("--template is only valid for create/template")
	}
	if opts.ArchivedOnly {
		opts.IncludeArchived = true
	}
	if opts.Body == "" && len(opts.Tags) == 0 {
		return noteOptions{}, errors.New("search requires a query or at least one --tag")
	}
	return opts, nil
}

func filterNotesByTags(notes []noteMeta, tags []string) []noteMeta {
	if len(tags) == 0 {
		return notes
	}

	var filtered []noteMeta
	for _, note := range notes {
		if hasAllTags(note.Tags, tags) {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

func filterArchivedNotes(notes []noteMeta, opts noteOptions) []noteMeta {
	if opts.ArchivedOnly {
		var filtered []noteMeta
		for _, note := range notes {
			if note.Archived {
				filtered = append(filtered, note)
			}
		}
		return filtered
	}

	if opts.IncludeArchived {
		return notes
	}

	var filtered []noteMeta
	for _, note := range notes {
		if !note.Archived {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

func hasAllTags(noteTags, filterTags []string) bool {
	if len(filterTags) == 0 {
		return true
	}

	tagSet := make(map[string]struct{}, len(noteTags))
	for _, tag := range noteTags {
		tagSet[tag] = struct{}{}
	}
	for _, tag := range filterTags {
		if _, ok := tagSet[tag]; !ok {
			return false
		}
	}
	return true
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "-"
	}
	return strings.Join(tags, ",")
}

func builtInTemplates() map[string]noteTemplate {
	return map[string]noteTemplate{
		"daily": {
			Name:         "daily",
			Description:  "Daily log with priorities, notes, and follow-up prompts",
			DefaultTitle: defaultDailyTitle,
			DefaultTags:  []string{"daily"},
			Body: func(title string) string {
				return strings.Join([]string{
					"## Top of Mind",
					"",
					"## Priorities",
					"- [ ]",
					"",
					"## Notes",
					"",
					"## Wins",
					"",
					"## Tomorrow",
				}, "\n")
			},
		},
		"meeting": {
			Name:         "meeting",
			Description:  "Meeting notes with agenda, attendees, notes, and action items",
			DefaultTitle: func() string { return "Meeting " + now().Format("2006-01-02") },
			DefaultTags:  []string{"meeting"},
			Body: func(title string) string {
				return strings.Join([]string{
					"## Details",
					"- Date: " + now().Format("2006-01-02"),
					"- Attendees:",
					"- Agenda:",
					"",
					"## Notes",
					"",
					"## Decisions",
					"",
					"## Action Items",
					"- [ ]",
				}, "\n")
			},
		},
		"project": {
			Name:         "project",
			Description:  "Project brief with goals, milestones, links, and next actions",
			DefaultTitle: func() string { return "Project " + now().Format("2006-01-02") },
			DefaultTags:  []string{"project"},
			Body: func(title string) string {
				return strings.Join([]string{
					"## Summary",
					"",
					"## Goals",
					"- [ ]",
					"",
					"## Milestones",
					"- [ ]",
					"",
					"## Links",
					"- [[ ]]",
					"",
					"## Next Actions",
					"- [ ]",
				}, "\n")
			},
		},
	}
}

func defaultDailyTitle() string {
	return now().Format("2006-01-02")
}

func printTemplates() {
	names := make([]string, 0, len(builtInTemplates()))
	for name := range builtInTemplates() {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("Available templates:")
	for _, name := range names {
		template := builtInTemplates()[name]
		fmt.Printf("  %s\t%s\n", template.Name, template.Description)
	}
}

func validateRenameID(id string) error {
	if id == "" {
		return errors.New("new id cannot be empty")
	}
	if strings.ContainsAny(id, `/\`) {
		return errors.New("new id cannot contain path separators")
	}
	if id == "." || id == ".." {
		return errors.New("new id cannot be . or ..")
	}
	return nil
}

func printUsage() {
	fmt.Println("notes <command> [arguments]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  create <title> [content] [--tag <tag>] [--tags a,b]  Create a Markdown note")
	fmt.Println("  create [<title>] [content] --template <name>         Create a note from a built-in template")
	fmt.Println("  template [list|<name> [<title>] [content]]           List templates or create from one directly")
	fmt.Println("  edit <id> [content] [--tag <tag>] [--tags a,b]       Replace note body/tags or open in $EDITOR")
	fmt.Println("  archive <id>                Mark a note as archived")
	fmt.Println("  unarchive <id>              Remove archived status from a note")
	fmt.Println("  rename <old-id> <new-id>    Rename a note file and rewrite matching note links")
	fmt.Println("  list [--tag <tag>]... [--include-archived|--archived-only]   List saved notes")
	fmt.Println("  search <query> [--tag <tag>]... [--include-archived|--archived-only] Search note titles and bodies")
	fmt.Println("  today                     Create or open today's daily note")
	fmt.Println("  view <id>                 Print a note")
	fmt.Println("  links <id>                List outgoing [[note-id]] links from a note")
	fmt.Println("  backlinks <id>            List notes that link to a note")
	fmt.Println("  graph                     Emit the notebook link graph as Graphviz DOT")
	fmt.Println("  delete <id>               Delete a note")
	fmt.Println("  doctor [--fix] [--report] Check for broken wiki links and orphaned notes")
}
