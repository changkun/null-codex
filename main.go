package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

const notesDir = "notes"

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

type noteOptions struct {
	Tags            []string
	ClearTags       bool
	IncludeArchived bool
	ArchivedOnly    bool
	Body            string
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
	case "today":
		return openTodayNote()
	case "view", "show":
		return viewNote(args[1:])
	case "delete", "rm":
		return deleteNote(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func createNote(args []string) error {
	if len(args) == 0 {
		return errors.New("create requires a title")
	}

	title := strings.TrimSpace(args[0])
	if title == "" {
		return errors.New("title cannot be empty")
	}

	opts, err := parseNoteOptions(args[1:])
	if err != nil {
		return err
	}
	if err := validateMutationOptions(opts); err != nil {
		return err
	}

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return err
	}

	id, err := nextNoteID(title)
	if err != nil {
		return err
	}

	content := formatNote(noteContent{
		Title: title,
		Tags:  opts.Tags,
		Body:  opts.Body,
	})

	path := notePath(id)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}

	fmt.Printf("created %s\n", id)
	return nil
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
	var matches []noteMeta
	for _, note := range notes {
		if query == "" {
			matches = append(matches, note)
			continue
		}

		body, err := os.ReadFile(notePath(note.ID))
		if err != nil {
			return err
		}

		searchText := strings.ToLower(note.Title + "\n" + string(body))
		if strings.Contains(searchText, query) {
			matches = append(matches, note)
		}
	}

	if len(matches) == 0 {
		fmt.Println("no matching notes found")
		return nil
	}

	for _, note := range matches {
		fmt.Printf("%s\t%s\t%s\t%s\n", note.ID, note.ModTime.Format(time.RFC3339), note.Title, formatTags(note.Tags))
	}

	return nil
}

func openTodayNote() error {
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return err
	}

	today := now().Format("2006-01-02")
	path := notePath(today)
	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		content := formatNote(noteContent{Title: today})
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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

	data, err := os.ReadFile(notePath(args[0]))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("note %q not found", args[0])
		}
		return err
	}

	fmt.Print(string(data))
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
	if opts.ArchivedOnly {
		opts.IncludeArchived = true
	}
	return opts, nil
}

func validateMutationOptions(opts noteOptions) error {
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

func printUsage() {
	fmt.Println("notes <command> [arguments]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  create <title> [content] [--tag <tag>] [--tags a,b]  Create a Markdown note")
	fmt.Println("  edit <id> [content] [--tag <tag>] [--tags a,b]       Replace note body/tags or open in $EDITOR")
	fmt.Println("  archive <id>                Mark a note as archived")
	fmt.Println("  unarchive <id>              Remove archived status from a note")
	fmt.Println("  list [--tag <tag>]... [--include-archived|--archived-only]   List saved notes")
	fmt.Println("  search <query> [--tag <tag>]... [--include-archived|--archived-only] Search note titles and bodies")
	fmt.Println("  today                     Create or open today's daily note")
	fmt.Println("  view <id>                 Print a note")
	fmt.Println("  delete <id>               Delete a note")
}
