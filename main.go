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

type noteMeta struct {
	ID      string
	Title   string
	ModTime time.Time
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
		return listNotes()
	case "search":
		return searchNotes(args[1:])
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

	body := ""
	if len(args) > 1 {
		body = strings.Join(args[1:], " ")
	}

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return err
	}

	id, err := nextNoteID(title)
	if err != nil {
		return err
	}

	content := formatNote(title, body)

	path := notePath(id)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}

	fmt.Printf("created %s\n", id)
	return nil
}

func listNotes() error {
	notes, err := loadNotes()
	if err != nil {
		return err
	}
	if len(notes) == 0 {
		fmt.Println("no notes found")
		return nil
	}

	for _, note := range notes {
		fmt.Printf("%s\t%s\t%s\n", note.ID, note.ModTime.Format(time.RFC3339), note.Title)
	}

	return nil
}

func searchNotes(args []string) error {
	if len(args) == 0 {
		return errors.New("search requires a query")
	}

	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		return errors.New("search query cannot be empty")
	}

	notes, err := loadNotes()
	if err != nil {
		return err
	}

	query = strings.ToLower(query)
	var matches []noteMeta
	for _, note := range notes {
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
		fmt.Printf("%s\t%s\t%s\n", note.ID, note.ModTime.Format(time.RFC3339), note.Title)
	}

	return nil
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

	title, err := readTitle(path)
	if err != nil {
		return err
	}

	body := strings.Join(args[1:], " ")
	content := formatNote(title, body)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# ")), nil
		}
		if line != "" {
			return line, nil
		}
	}

	return strings.TrimSuffix(filepath.Base(path), ".md"), nil
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
		title, err := readTitle(notePath(id))
		if err != nil {
			return nil, err
		}

		notes = append(notes, noteMeta{
			ID:      id,
			Title:   title,
			ModTime: info.ModTime(),
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

func formatNote(title, body string) string {
	content := fmt.Sprintf("# %s\n\n", title)
	if body != "" {
		content += body + "\n"
	}
	return content
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

func printUsage() {
	fmt.Println("notes <command> [arguments]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  create <title> [content]  Create a Markdown note")
	fmt.Println("  edit <id> [content]       Replace note body or open in $EDITOR")
	fmt.Println("  list                      List saved notes")
	fmt.Println("  search <query>            Search note titles and bodies")
	fmt.Println("  view <id>                 Print a note")
	fmt.Println("  delete <id>               Delete a note")
}
