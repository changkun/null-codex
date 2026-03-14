package main

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	notesDir        = "notes"
	noteHistoryDir  = ".history"
	noteTemplateDir = "templates"
	inboxNoteID     = "inbox"
	inboxNoteTitle  = "Inbox"
)

var noteLinkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)
var inlineCodePattern = regexp.MustCompile("`([^`]+)`")
var boldPattern = regexp.MustCompile(`\*\*([^*]+)\*\*`)
var italicPattern = regexp.MustCompile(`\*([^*]+)\*`)

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
	BodyLine int
	Body     string
}

type searchResult struct {
	Note    noteMeta
	Snippet string
}

type noteTask struct {
	Text   string
	Line   int
	Open   bool
	Source string
}

type brokenLink struct {
	Source string
	Target string
}

type noteEdge struct {
	Source string
	Target string
}

type noteVersion struct {
	ID        string
	Timestamp time.Time
	Action    string
	Path      string
}

type serveOptions struct {
	Addr  string
	Watch bool
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

type notebookPageData struct {
	Title           string
	HeaderTitle     string
	HeaderSubtitle  string
	SearchQuery     string
	FilterTagsInput string
	ActiveTags      []string
	TagFilters      []webTagFilter
	ArchivedMode    string
	ClearTagsURL    string
	FilterPage      string
	ShowTasks       bool
	TasksPageURL    string
	JournalPageURL  string
	Notes           []webNoteSummary
	TaskGroups      []webTaskGroup
	CurrentNote     *webNoteDetail
	NoteHistory     *webNoteHistory
	NoteForm        *webNoteForm
	Journal         *webJournalView
	LiveReload      bool
}

type webNoteSummary struct {
	ID           string
	Title        string
	Tags         []string
	Body         string
	Snippet      string
	Archived     bool
	ModTime      string
	LinksCount   int
	Backlinks    int
	BrokenLinks  []string
	DetailURL    string
}

type webTask struct {
	Text string
	Line int
}

type webTaskGroup struct {
	ID        string
	Title     string
	Tags      []string
	Archived  bool
	ModTime   string
	DetailURL string
	Tasks     []webTask
}

type webNoteDetail struct {
	ID                string
	Title             string
	Tags              []string
	Archived          bool
	ModTime           string
	RenderedBody      template.HTML
	OutgoingLinks     []webLink
	Backlinks         []webLink
	BrokenLinks       []string
	OutgoingLinksText string
}

type webNoteVersion struct {
	ID         string
	Timestamp  string
	Action     string
	BrowseURL  string
	IsSelected bool
}

type webNoteHistory struct {
	NoteID          string
	NoteTitle       string
	NoteURL         string
	Versions        []webNoteVersion
	SelectedVersion *webNoteVersionDetail
}

type webNoteVersionDetail struct {
	ID         string
	Timestamp  string
	Action     string
	Diff       string
	RestoreURL string
}

type taskRenderOptions struct {
	NoteID    string
	ReturnURL string
}

type webNoteForm struct {
	Title       string
	Tags        string
	Body        string
	Archived    bool
	ActionURL   string
	CancelURL   string
	SubmitLabel string
	Error       string
	IsEditing   bool
}

type webLink struct {
	ID    string
	Title string
	URL   string
}

type webTagFilter struct {
	Name   string
	URL    string
	Active bool
}

type webFilterOptions struct {
	Query        string
	Tags         []string
	TagsInput    string
	ArchivedMode string
}

type webJournalView struct {
	MonthLabel        string
	SelectedDate      string
	SelectedDateLabel string
	TodayURL          string
	PrevMonthURL      string
	NextMonthURL      string
	Days              []webJournalDay
	HasEntry          bool
	SelectedNoteURL   string
}

type webJournalDay struct {
	Date           string
	DayOfMonth     int
	WeekdayLabel   string
	IsCurrentMonth bool
	IsToday        bool
	IsSelected     bool
	HasEntry       bool
	BrowseURL      string
}

type notebookSnapshot struct {
	Notes         []webNoteSummary
	NotesByID     map[string]webNoteSummary
	RenderedNotes map[string]webNoteDetail
	TaskGroups    []webTaskGroup
	Tags          []string
	JournalNotes  map[string]webNoteSummary
}

type notebookServer struct {
	mu         sync.RWMutex
	snapshot   notebookSnapshot
	watch      bool
	version    int64
	subscribers map[chan int64]struct{}
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
	case "history":
		return historyNote(args[1:])
	case "list", "ls":
		return listNotes(args[1:])
	case "search":
		return searchNotes(args[1:])
	case "tasks":
		return listTasks(args[1:])
	case "archive":
		return archiveNote(args[1:])
	case "unarchive":
		return unarchiveNote(args[1:])
	case "rename", "mv":
		return renameNote(args[1:])
	case "today":
		return openTodayNote()
	case "inbox":
		return captureInbox(args[1:])
	case "view", "show":
		return viewNote(args[1:])
	case "links":
		return listNoteLinks(args[1:])
	case "backlinks":
		return listNoteBacklinks(args[1:])
	case "graph":
		return graphNotes(args[1:])
	case "serve":
		return serveNotes(args[1:])
	case "delete", "rm":
		return deleteNote(args[1:])
	case "restore":
		return restoreNote(args[1:])
	case "doctor":
		return doctorNotes(args[1:])
	case "sync":
		return syncNotes(args[1:])
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
		return printTemplates()
	}
	if len(args) == 1 && args[0] == "list" {
		return printTemplates()
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

func listTasks(args []string) error {
	if len(args) > 0 && args[0] == "toggle" {
		return toggleTask(args[1:])
	}

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

	var groups []webTaskGroup
	for _, note := range notes {
		content, err := readNoteContent(notePath(note.ID))
		if err != nil {
			return err
		}

		tasks := openTasksFromContent(content)
		if len(tasks) == 0 {
			continue
		}

		group := webTaskGroup{
			ID:       note.ID,
			Title:    note.Title,
			Tags:     append([]string(nil), note.Tags...),
			Archived: note.Archived,
			ModTime:  note.ModTime.Format(time.RFC3339),
		}
		for _, task := range tasks {
			group.Tasks = append(group.Tasks, webTask{
				Text: task.Text,
				Line: task.Line,
			})
		}
		groups = append(groups, group)
	}

	if len(groups) == 0 {
		fmt.Println("no open tasks found")
		return nil
	}

	for _, group := range groups {
		fmt.Printf("%s\t%s\t%s\t%s\n", group.ID, group.ModTime, group.Title, formatTags(group.Tags))
		for _, task := range group.Tasks {
			fmt.Printf("\t%d\t[ ] %s\n", task.Line, task.Text)
		}
	}

	return nil
}

func toggleTask(args []string) error {
	if len(args) != 2 {
		return errors.New("tasks toggle requires a note id and line number")
	}

	id := strings.TrimSpace(args[0])
	line, err := strconv.Atoi(strings.TrimSpace(args[1]))
	if err != nil || line < 1 {
		return errors.New("task line must be a positive integer")
	}

	task, err := toggleTaskByLine(id, line)
	if err != nil {
		return err
	}

	state := "open"
	if !task.Open {
		state = "done"
	}
	fmt.Printf("toggled task %d in %s to %s\n", line, id, state)
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
		formatted := formatNote(content)
		if err := os.WriteFile(path, []byte(formatted), 0o644); err != nil {
			return err
		}
		if err := appendHistoryVersion(today, "create", []byte(formatted)); err != nil {
			return err
		}
	}

	return openNoteInEditor(today)
}

func captureInbox(args []string) error {
	entry, asTask, err := parseInboxArgs(args)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return err
	}

	content, existing, err := loadOrCreateInboxNote()
	if err != nil {
		return err
	}

	line := formatInboxEntry(entry, asTask)
	if content.Body == "" {
		content.Body = line
	} else {
		content.Body += "\n" + line
	}

	formatted := formatNote(content)
	if !existing {
		if err := os.WriteFile(notePath(inboxNoteID), []byte(formatted), 0o644); err != nil {
			return err
		}
		if err := appendHistoryVersion(inboxNoteID, "create", []byte(formatted)); err != nil {
			return err
		}
	} else {
		if err := writeNoteVersioned(inboxNoteID, "inbox-capture", []byte(formatted)); err != nil {
			return err
		}
	}

	fmt.Printf("captured to %s\n", inboxNoteID)
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
		return openNoteInEditor(id)
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
		return openNoteInEditor(id)
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

	if err := writeNoteVersioned(id, "edit", []byte(formatNote(content))); err != nil {
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

func serveNotes(args []string) error {
	opts, err := parseServeOptions(args)
	if err != nil {
		return err
	}

	serverState, err := newNotebookServer(opts.Watch)
	if err != nil {
		return err
	}
	if opts.Watch {
		go watchNotebook(serverState, 500*time.Millisecond)
	}

	server := &http.Server{
		Addr:    opts.Addr,
		Handler: newServeMux(serverState),
	}

	fmt.Printf("serving notebook at http://%s\n", normalizeServeAddr(opts.Addr))
	return server.ListenAndServe()
}

func newNotebookServer(watch bool) (*notebookServer, error) {
	snapshot, err := buildNotebookSnapshot()
	if err != nil {
		return nil, err
	}
	return &notebookServer{
		snapshot:    snapshot,
		watch:       watch,
		subscribers: make(map[chan int64]struct{}),
	}, nil
}

func (s *notebookServer) snapshotCopy() notebookSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (s *notebookServer) currentSnapshot() (notebookSnapshot, error) {
	if s.watch {
		return s.snapshotCopy(), nil
	}
	return buildNotebookSnapshot()
}

func (s *notebookServer) refresh() error {
	snapshot, err := buildNotebookSnapshot()
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.snapshot = snapshot
	s.version++
	version := s.version
	subscribers := make([]chan int64, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subscribers = append(subscribers, ch)
	}
	s.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- version:
		default:
		}
	}
	return nil
}

func (s *notebookServer) subscribe() (chan int64, func()) {
	ch := make(chan int64, 1)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subscribers, ch)
		s.mu.Unlock()
	}
}

func newServeMux(server *notebookServer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.serveIndexPage)
	mux.HandleFunc("/events", server.serveEvents)
	mux.HandleFunc("/journal", server.serveJournalPage)
	mux.HandleFunc("/tasks/toggle", server.serveToggleTask)
	mux.HandleFunc("/tasks", server.serveTasksPage)
	mux.HandleFunc("/new", server.serveCreateNotePage)
	mux.HandleFunc("/notes", server.serveCreateNote)
	mux.HandleFunc("/notes/", server.serveNotePage)
	return mux
}

func watchNotebook(server *notebookServer, interval time.Duration) {
	signature, _ := notebookWatchSignature()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		next, err := notebookWatchSignature()
		if err != nil {
			continue
		}
		if next == signature {
			continue
		}
		if err := server.refresh(); err == nil {
			signature = next
		}
	}
}

func notebookWatchSignature() (string, error) {
	entries, err := os.ReadDir(notesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}

	var b strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s|%d|%d\n", entry.Name(), info.Size(), info.ModTime().UnixNano())
	}
	return b.String(), nil
}

func (s *notebookServer) serveIndexPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	snapshot, err := s.currentSnapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filter := parseWebFilterOptions(r.URL.Query())
	notes := filterSnapshotNotes(snapshot.Notes, filter)

	data := notebookPageData{
		Title:           "Notebook",
		HeaderTitle:     "Notebook",
		HeaderSubtitle:  notebookSubtitle(len(notes)),
		SearchQuery:     filter.Query,
		FilterTagsInput: filter.TagsInput,
		ActiveTags:      append([]string(nil), filter.Tags...),
		TagFilters:      buildTagFilters("/", snapshot.Tags, filter),
		ArchivedMode:    filter.ArchivedMode,
		ClearTagsURL:    clearTagFiltersURL("/", filter),
		FilterPage:      "/",
		TasksPageURL:    tasksURL(filter),
		JournalPageURL:  journalURL(defaultJournalDate()),
		LiveReload:      s.watch,
	}

	data.Notes = notes
	renderNotebookPage(w, data)
}

func (s *notebookServer) serveJournalPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/journal" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshot, err := s.currentSnapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filter := parseWebFilterOptions(r.URL.Query())
	selectedDate, err := parseJournalDate(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	journal := buildJournalView(selectedDate, snapshot.JournalNotes)

	data := notebookPageData{
		Title:           "Journal",
		HeaderTitle:     "Journal",
		HeaderSubtitle:  journal.SelectedDateLabel,
		SearchQuery:     filter.Query,
		FilterTagsInput: filter.TagsInput,
		ActiveTags:      append([]string(nil), filter.Tags...),
		TagFilters:      buildTagFilters("/", snapshot.Tags, filter),
		ArchivedMode:    filter.ArchivedMode,
		ClearTagsURL:    clearTagFiltersURL("/", filter),
		FilterPage:      "/",
		TasksPageURL:    tasksURL(filter),
		JournalPageURL:  journalURL(selectedDate),
		Notes:           filterSnapshotNotes(snapshot.Notes, filter),
		Journal:         &journal,
		LiveReload:      s.watch,
	}
	if detail, ok := snapshot.RenderedNotes[journal.SelectedDate]; ok {
		data.CurrentNote = &detail
	}
	renderNotebookPage(w, data)
}

func (s *notebookServer) serveEvents(w http.ResponseWriter, r *http.Request) {
	if !s.watch {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path != "/events" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsubscribe := s.subscribe()
	defer unsubscribe()

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case version := <-ch:
			fmt.Fprintf(w, "event: refresh\ndata: %d\n\n", version)
			flusher.Flush()
		}
	}
}

func (s *notebookServer) serveTasksPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/tasks" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshot, err := s.currentSnapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filter := parseWebFilterOptions(r.URL.Query())
	taskGroups := filterTaskGroups(snapshot.TaskGroups, filter)
	taskCount := 0
	for _, group := range taskGroups {
		taskCount += len(group.Tasks)
	}

	data := notebookPageData{
		Title:           "Open Tasks",
		HeaderTitle:     "Open Tasks",
		HeaderSubtitle:  fmt.Sprintf("%d open %s", taskCount, pluralize(taskCount, "task", "tasks")),
		SearchQuery:     filter.Query,
		FilterTagsInput: filter.TagsInput,
		ActiveTags:      append([]string(nil), filter.Tags...),
		TagFilters:      buildTagFilters("/tasks", snapshot.Tags, filter),
		ArchivedMode:    filter.ArchivedMode,
		ClearTagsURL:    clearTagFiltersURL("/tasks", filter),
		FilterPage:      "/tasks",
		ShowTasks:       true,
		TasksPageURL:    tasksURL(filter),
		JournalPageURL:  journalURL(defaultJournalDate()),
		Notes:           filterSnapshotNotes(snapshot.Notes, filter),
		TaskGroups:      taskGroups,
		LiveReload:      s.watch,
	}
	renderNotebookPage(w, data)
}

func (s *notebookServer) serveToggleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := strings.TrimSpace(r.FormValue("note"))
	line, err := strconv.Atoi(strings.TrimSpace(r.FormValue("line")))
	if err != nil || line < 1 {
		http.Error(w, "invalid task line", http.StatusBadRequest)
		return
	}
	if _, err := toggleTaskByLine(id, line); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "line") || strings.Contains(err.Error(), "task") {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	if err := s.refresh(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, safeReturnURL(r.FormValue("return_to"), noteURL(id)), http.StatusSeeOther)
}

func (s *notebookServer) serveCreateNotePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/new" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshot, err := s.currentSnapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filter := parseWebFilterOptions(r.URL.Query())

	data := notebookPageData{
		Title:           "New Note",
		HeaderTitle:     "New Note",
		HeaderSubtitle:  "Create a notebook entry in the browser",
		SearchQuery:     filter.Query,
		FilterTagsInput: filter.TagsInput,
		ActiveTags:      append([]string(nil), filter.Tags...),
		TagFilters:      buildTagFilters("/", snapshot.Tags, filter),
		ArchivedMode:    filter.ArchivedMode,
		ClearTagsURL:    clearTagFiltersURL("/", filter),
		FilterPage:      "/",
		TasksPageURL:    tasksURL(filter),
		JournalPageURL:  journalURL(defaultJournalDate()),
		Notes:           filterSnapshotNotes(snapshot.Notes, filter),
		NoteForm: &webNoteForm{
			ActionURL:   "/notes",
			CancelURL:   "/",
			SubmitLabel: "Create note",
		},
	}
	renderNotebookPage(w, data)
}

func (s *notebookServer) serveCreateNote(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/notes" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	form := webNoteForm{
		Title:       strings.TrimSpace(r.FormValue("title")),
		Tags:        strings.TrimSpace(r.FormValue("tags")),
		Body:        normalizeWebBody(r.FormValue("body")),
		Archived:    r.FormValue("archived") != "",
		ActionURL:   "/notes",
		CancelURL:   "/",
		SubmitLabel: "Create note",
	}

	id, err := createWebNote(form)
	if err != nil {
		s.renderWebFormError(w, r, http.StatusBadRequest, "New Note", "Create a notebook entry in the browser", form, err)
		return
	}
	if err := s.refresh(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, noteURL(id), http.StatusSeeOther)
}

func (s *notebookServer) serveNotePage(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/notes/")
	path = strings.TrimSpace(strings.Trim(path, "/"))
	isHistoryRestoreRoute := strings.HasSuffix(path, "/history/restore")
	if isHistoryRestoreRoute {
		path = strings.TrimSpace(strings.TrimSuffix(path, "/history/restore"))
	}
	isHistoryRoute := !isHistoryRestoreRoute && strings.HasSuffix(path, "/history")
	if isHistoryRoute {
		path = strings.TrimSpace(strings.TrimSuffix(path, "/history"))
	}
	isEditRoute := strings.HasSuffix(path, "/edit")
	if isEditRoute {
		path = strings.TrimSpace(strings.TrimSuffix(path, "/edit"))
	}
	id := path
	unescapedID, err := url.PathUnescape(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	id = unescapedID
	if id == "" {
		http.NotFound(w, r)
		return
	}

	if isEditRoute {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.serveEditNotePage(w, r, id)
		return
	}
	if isHistoryRestoreRoute {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.serveRestoreNote(w, r, id)
		return
	}
	if isHistoryRoute {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.serveNoteHistoryPage(w, r, id)
		return
	}

	if r.Method == http.MethodPost {
		s.serveUpdateNote(w, r, id)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshot, err := s.currentSnapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	detail, ok := snapshot.RenderedNotes[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	filter := parseWebFilterOptions(r.URL.Query())

	data := notebookPageData{
		Title:           detail.Title,
		HeaderTitle:     detail.Title,
		HeaderSubtitle:  id,
		SearchQuery:     filter.Query,
		FilterTagsInput: filter.TagsInput,
		ActiveTags:      append([]string(nil), filter.Tags...),
		TagFilters:      buildTagFilters("/", snapshot.Tags, filter),
		ArchivedMode:    filter.ArchivedMode,
		ClearTagsURL:    clearTagFiltersURL("/", filter),
		FilterPage:      "/",
		TasksPageURL:    tasksURL(filter),
		JournalPageURL:  journalURL(defaultJournalDate()),
		Notes:           filterSnapshotNotes(snapshot.Notes, filter),
		CurrentNote:     &detail,
		LiveReload:      s.watch,
	}
	renderNotebookPage(w, data)
}

func renderNotebookPage(w http.ResponseWriter, data notebookPageData) {
	renderNotebookPageStatus(w, http.StatusOK, data)
}

func renderNotebookPageStatus(w http.ResponseWriter, status int, data notebookPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := notebookPageTemplate().Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *notebookServer) serveNoteHistoryPage(w http.ResponseWriter, r *http.Request, id string) {
	snapshot, err := s.currentSnapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	detail, ok := snapshot.RenderedNotes[id]
	if !ok {
		http.NotFound(w, r)
		return
	}

	history, err := buildWebNoteHistory(id, strings.TrimSpace(r.URL.Query().Get("version")))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "invalid version") {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	history.NoteTitle = detail.Title
	filter := parseWebFilterOptions(r.URL.Query())

	data := notebookPageData{
		Title:           "History for " + detail.Title,
		HeaderTitle:     "Note History",
		HeaderSubtitle:  id,
		SearchQuery:     filter.Query,
		FilterTagsInput: filter.TagsInput,
		ActiveTags:      append([]string(nil), filter.Tags...),
		TagFilters:      buildTagFilters("/", snapshot.Tags, filter),
		ArchivedMode:    filter.ArchivedMode,
		ClearTagsURL:    clearTagFiltersURL("/", filter),
		FilterPage:      "/",
		TasksPageURL:    tasksURL(filter),
		JournalPageURL:  journalURL(defaultJournalDate()),
		Notes:           filterSnapshotNotes(snapshot.Notes, filter),
		CurrentNote:     &detail,
		NoteHistory:     history,
		LiveReload:      s.watch,
	}
	renderNotebookPage(w, data)
}

func (s *notebookServer) serveEditNotePage(w http.ResponseWriter, r *http.Request, id string) {
	snapshot, err := s.currentSnapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	detail, ok := snapshot.RenderedNotes[id]
	if !ok {
		http.NotFound(w, r)
		return
	}

	content, err := readNoteContent(notePath(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filter := parseWebFilterOptions(r.URL.Query())

	data := notebookPageData{
		Title:           "Edit " + detail.Title,
		HeaderTitle:     "Edit Note",
		HeaderSubtitle:  id,
		SearchQuery:     filter.Query,
		FilterTagsInput: filter.TagsInput,
		ActiveTags:      append([]string(nil), filter.Tags...),
		TagFilters:      buildTagFilters("/", snapshot.Tags, filter),
		ArchivedMode:    filter.ArchivedMode,
		ClearTagsURL:    clearTagFiltersURL("/", filter),
		FilterPage:      "/",
		TasksPageURL:    tasksURL(filter),
		JournalPageURL:  journalURL(defaultJournalDate()),
		Notes:           filterSnapshotNotes(snapshot.Notes, filter),
		NoteForm: &webNoteForm{
			Title:       content.Title,
			Tags:        strings.Join(content.Tags, ", "),
			Body:        content.Body,
			Archived:    content.Archived,
			ActionURL:   noteURL(id),
			CancelURL:   noteURL(id),
			SubmitLabel: "Save changes",
			IsEditing:   true,
		},
	}
	renderNotebookPage(w, data)
}

func (s *notebookServer) serveUpdateNote(w http.ResponseWriter, r *http.Request, id string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	form := webNoteForm{
		Title:       strings.TrimSpace(r.FormValue("title")),
		Tags:        strings.TrimSpace(r.FormValue("tags")),
		Body:        normalizeWebBody(r.FormValue("body")),
		Archived:    r.FormValue("archived") != "",
		ActionURL:   noteURL(id),
		CancelURL:   noteURL(id),
		SubmitLabel: "Save changes",
		IsEditing:   true,
	}

	if err := updateWebNote(id, form); err != nil {
		s.renderWebFormError(w, r, http.StatusBadRequest, "Edit Note", id, form, err)
		return
	}
	if err := s.refresh(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, noteURL(id), http.StatusSeeOther)
}

func (s *notebookServer) serveRestoreNote(w http.ResponseWriter, r *http.Request, id string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	versionID := strings.TrimSpace(r.FormValue("version"))
	if versionID == "" {
		http.Error(w, "missing version id", http.StatusBadRequest)
		return
	}
	if err := restoreNoteVersion(id, versionID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "invalid version") {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	if err := s.refresh(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, noteURL(id), http.StatusSeeOther)
}

func (s *notebookServer) renderWebFormError(w http.ResponseWriter, r *http.Request, status int, headerTitle, headerSubtitle string, form webNoteForm, err error) {
	snapshot, err := s.currentSnapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	form.Error = err.Error()
	filter := parseWebFilterOptions(r.URL.Query())
	data := notebookPageData{
		Title:           headerTitle,
		HeaderTitle:     headerTitle,
		HeaderSubtitle:  headerSubtitle,
		SearchQuery:     filter.Query,
		FilterTagsInput: filter.TagsInput,
		ActiveTags:      append([]string(nil), filter.Tags...),
		TagFilters:      buildTagFilters("/", snapshot.Tags, filter),
		ArchivedMode:    filter.ArchivedMode,
		ClearTagsURL:    clearTagFiltersURL("/", filter),
		FilterPage:      "/",
		TasksPageURL:    tasksURL(filter),
		JournalPageURL:  journalURL(defaultJournalDate()),
		Notes:           filterSnapshotNotes(snapshot.Notes, filter),
		NoteForm:        &form,
		LiveReload:      false,
	}
	renderNotebookPageStatus(w, status, data)
}

func deleteNote(args []string) error {
	if len(args) != 1 {
		return errors.New("delete requires a note id")
	}

	id := args[0]
	path := notePath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("note %q not found", id)
		}
		return err
	}
	if err := appendHistoryVersion(id, "delete", data); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}

	fmt.Printf("deleted %s\n", id)
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

func syncNotes(args []string) error {
	if len(args) != 0 {
		return errors.New("sync does not take any arguments")
	}

	if _, err := os.Stat(notesDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s is not a Git repository; initialize it with git and configure a remote first", notesDir)
		}
		return err
	}

	if _, err := runGit("rev-parse", "--is-inside-work-tree"); err != nil {
		return fmt.Errorf("%s is not a Git repository; initialize it with git and configure a remote first", notesDir)
	}

	upstream, err := runGit("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return errors.New("notes Git remote is not configured; set an upstream branch for notes/ before syncing")
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return errors.New("notes Git remote is not configured; set an upstream branch for notes/ before syncing")
	}

	if _, err := runGit("add", "--all", "."); err != nil {
		return err
	}

	committed := false
	if hasChanges, err := gitHasStagedChanges(); err != nil {
		return err
	} else if hasChanges {
		message := fmt.Sprintf("sync notebook %s", now().UTC().Format(time.RFC3339))
		if _, err := runGit("commit", "-m", message); err != nil {
			return err
		}
		committed = true
	}

	if _, err := runGit("pull", "--rebase"); err != nil {
		return err
	}
	if _, err := runGit("push"); err != nil {
		return err
	}

	if committed {
		fmt.Printf("synced notes with %s\n", upstream)
	} else {
		fmt.Printf("synced notes with %s (no local changes)\n", upstream)
	}
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

	rollbackRename := func(cause error) error {
		if rollbackErr := os.Rename(newPath, oldPath); rollbackErr != nil {
			return fmt.Errorf("%v (rollback failed: %v)", cause, rollbackErr)
		}
		if moveErr := moveHistory(newID, oldID); moveErr != nil {
			return fmt.Errorf("%v (history rollback failed: %v)", cause, moveErr)
		}
		return cause
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	if err := moveHistory(oldID, newID); err != nil {
		if rollbackErr := os.Rename(newPath, oldPath); rollbackErr != nil {
			return fmt.Errorf("move history after rename: %v (rollback failed: %v)", err, rollbackErr)
		}
		return err
	}
	oldData, err := os.ReadFile(newPath)
	if err != nil {
		return rollbackRename(fmt.Errorf("read renamed note: %w", err))
	}
	if err := appendHistoryVersion(newID, "rename", oldData); err != nil {
		return rollbackRename(fmt.Errorf("record history after rename: %w", err))
	}
	if err := renameLinkedReferences(oldID, newID); err != nil {
		return rollbackRename(fmt.Errorf("update links after rename: %w", err))
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
	if err := writeNoteVersioned(id, map[bool]string{true: "archive", false: "unarchive"}[archived], []byte(formatNote(content))); err != nil {
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

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", notesDir}, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}

	return stdout.String(), nil
}

func gitHasStagedChanges() (bool, error) {
	cmd := exec.Command("git", "-C", notesDir, "diff", "--cached", "--quiet", "--exit-code")
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("git diff --cached --quiet --exit-code: %v", err)
	}
	return false, nil
}

func openNoteInEditor(id string) error {
	path := notePath(id)
	before, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("note %q not found", id)
		}
		return err
	}

	if err := openInEditor(path); err != nil {
		return err
	}

	after, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(after) == string(before) {
		return nil
	}
	return appendHistoryVersion(id, "edit", before)
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

	formatted := formatNote(content)
	if err := os.WriteFile(notePath(id), []byte(formatted), 0o644); err != nil {
		return err
	}
	if err := appendHistoryVersion(id, "create", []byte(formatted)); err != nil {
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
		template, err := findTemplate(opts.Template)
		if err != nil {
			return "", noteContent{}, err
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

func loadOrCreateInboxNote() (noteContent, bool, error) {
	path := notePath(inboxNoteID)
	content, err := readNoteContent(path)
	if err == nil {
		return content, true, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return noteContent{}, false, err
	}
	return noteContent{
		Title: inboxNoteTitle,
		Tags:  []string{"inbox"},
	}, false, nil
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
	content.BodyLine = bodyStart + 1
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

func customTemplateDir() string {
	return filepath.Join(notesDir, noteTemplateDir)
}

func customTemplatePath(name string) (string, error) {
	if name == "" {
		return "", errors.New("template name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid template name %q", name)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("invalid template name %q", name)
	}
	return filepath.Join(customTemplateDir(), name+".md"), nil
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
		if err := appendHistoryVersion(strings.TrimSuffix(entry.Name(), ".md"), "rename-links", data); err != nil {
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

func buildNotebookSnapshot() (notebookSnapshot, error) {
	notes, err := loadNotes()
	if err != nil {
		return notebookSnapshot{}, err
	}

	contents := make(map[string]noteContent, len(notes))
	existing := make(map[string]struct{}, len(notes))
	tagSet := make(map[string]struct{})
	for _, note := range notes {
		content, err := readNoteContent(notePath(note.ID))
		if err != nil {
			return notebookSnapshot{}, err
		}
		contents[note.ID] = content
		existing[note.ID] = struct{}{}
		for _, tag := range note.Tags {
			tagSet[tag] = struct{}{}
		}
	}

	backlinks := make(map[string][]webLink, len(notes))
	for _, note := range notes {
		for _, target := range extractNoteLinks(contents[note.ID].Body) {
			if _, ok := existing[target]; !ok {
				continue
			}
			backlinks[target] = append(backlinks[target], webLink{
				ID:    note.ID,
				Title: note.Title,
				URL:   noteURL(note.ID),
			})
		}
	}

	var tags []string
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	snapshot := notebookSnapshot{
		NotesByID:     make(map[string]webNoteSummary, len(notes)),
		RenderedNotes: make(map[string]webNoteDetail, len(notes)),
		JournalNotes:  make(map[string]webNoteSummary),
		Tags:          tags,
	}
	for _, note := range notes {
		content := contents[note.ID]
		links := extractNoteLinks(content.Body)
		outgoing := buildOutgoingLinks(links, notes)
		broken := collectBrokenTargets(links, existing)
		currentBacklinks := append([]webLink(nil), backlinks[note.ID]...)
		sort.Slice(currentBacklinks, func(i, j int) bool {
			return currentBacklinks[i].ID < currentBacklinks[j].ID
		})

		summary := webNoteSummary{
			ID:          note.ID,
			Title:       note.Title,
			Tags:        append([]string(nil), note.Tags...),
			Body:        content.Body,
			Archived:    note.Archived,
			ModTime:     note.ModTime.Format(time.RFC3339),
			LinksCount:  len(outgoing),
			Backlinks:   len(currentBacklinks),
			BrokenLinks: broken,
			DetailURL:   noteURL(note.ID),
		}
		detail := webNoteDetail{
			ID:                note.ID,
			Title:             note.Title,
			Tags:              append([]string(nil), note.Tags...),
			Archived:          note.Archived,
			ModTime:           note.ModTime.Format(time.RFC3339),
			RenderedBody:      renderMarkdownHTML(content, existing, &taskRenderOptions{NoteID: note.ID, ReturnURL: noteURL(note.ID)}),
			OutgoingLinks:     outgoing,
			Backlinks:         currentBacklinks,
			BrokenLinks:       broken,
			OutgoingLinksText: formatOutgoingLinks(outgoing),
		}

		snapshot.Notes = append(snapshot.Notes, summary)
		snapshot.NotesByID[note.ID] = summary
		snapshot.RenderedNotes[note.ID] = detail
		if _, ok := parseJournalNoteID(note.ID); ok {
			snapshot.JournalNotes[note.ID] = summary
		}
		if tasks := openTasksFromContent(content); len(tasks) > 0 {
			group := webTaskGroup{
				ID:        note.ID,
				Title:     note.Title,
				Tags:      append([]string(nil), note.Tags...),
				Archived:  note.Archived,
				ModTime:   note.ModTime.Format(time.RFC3339),
				DetailURL: noteURL(note.ID),
			}
			for _, task := range tasks {
				group.Tasks = append(group.Tasks, webTask{
					Text: task.Text,
					Line: task.Line,
				})
			}
			snapshot.TaskGroups = append(snapshot.TaskGroups, group)
		}
	}

	sort.Slice(snapshot.Notes, func(i, j int) bool {
		if snapshot.Notes[i].ModTime == snapshot.Notes[j].ModTime {
			return snapshot.Notes[i].ID < snapshot.Notes[j].ID
		}
		return snapshot.Notes[i].ModTime > snapshot.Notes[j].ModTime
	})
	sort.Slice(snapshot.TaskGroups, func(i, j int) bool {
		if snapshot.TaskGroups[i].ModTime == snapshot.TaskGroups[j].ModTime {
			return snapshot.TaskGroups[i].ID < snapshot.TaskGroups[j].ID
		}
		return snapshot.TaskGroups[i].ModTime > snapshot.TaskGroups[j].ModTime
	})
	return snapshot, nil
}

func buildOutgoingLinks(links []string, notes []noteMeta) []webLink {
	var outgoing []webLink
	for _, target := range links {
		targetMeta := findNoteMeta(notes, target)
		if targetMeta.ID == "" {
			continue
		}
		outgoing = append(outgoing, webLink{
			ID:    targetMeta.ID,
			Title: targetMeta.Title,
			URL:   noteURL(targetMeta.ID),
		})
	}
	return outgoing
}

func collectBrokenTargets(links []string, existing map[string]struct{}) []string {
	var broken []string
	for _, target := range links {
		if _, ok := existing[target]; ok {
			continue
		}
		broken = append(broken, target)
	}
	return broken
}

func findNoteMeta(notes []noteMeta, id string) noteMeta {
	for _, note := range notes {
		if note.ID == id {
			return note
		}
	}
	return noteMeta{}
}

func parseWebFilterOptions(values url.Values) webFilterOptions {
	filter := webFilterOptions{
		Query:        strings.TrimSpace(values.Get("q")),
		ArchivedMode: queryArchivedMode(values),
	}
	filter.Tags = parseWebTags(values.Get("tags"))
	for _, tag := range values["tag"] {
		filter.Tags = normalizeTags(append(filter.Tags, tag))
	}
	filter.TagsInput = strings.Join(filter.Tags, ", ")
	return filter
}

func filterSnapshotNotes(notes []webNoteSummary, filter webFilterOptions) []webNoteSummary {
	var filtered []webNoteSummary
	query := strings.ToLower(filter.Query)
	for _, note := range notes {
		if !hasAllTags(note.Tags, filter.Tags) {
			continue
		}
		if filter.ArchivedMode == "exclude" && note.Archived {
			continue
		}
		if filter.ArchivedMode == "only" && !note.Archived {
			continue
		}
		if query != "" {
			searchText := strings.ToLower(note.Title + "\n" + note.Body)
			if !strings.Contains(searchText, query) {
				continue
			}
		}

		current := note
		current.Snippet = buildSearchSnippet(note.Body, query)
		current.DetailURL = noteFilterURL(note.ID, filter)
		filtered = append(filtered, current)
	}
	return filtered
}

func filterTaskGroups(groups []webTaskGroup, filter webFilterOptions) []webTaskGroup {
	var filtered []webTaskGroup
	for _, group := range groups {
		if !hasAllTags(group.Tags, filter.Tags) {
			continue
		}
		if filter.ArchivedMode == "exclude" && group.Archived {
			continue
		}
		if filter.ArchivedMode == "only" && !group.Archived {
			continue
		}
		filtered = append(filtered, group)
	}
	return filtered
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

func noteURL(id string) string {
	return "/notes/" + url.PathEscape(id)
}

func noteHistoryURL(id string) string {
	return noteURL(id) + "/history"
}

func noteHistoryVersionURL(id, versionID string) string {
	values := url.Values{}
	values.Set("version", versionID)
	return noteHistoryURL(id) + "?" + values.Encode()
}

func noteRestoreURL(id string) string {
	return noteHistoryURL(id) + "/restore"
}

func noteEditURL(id string) string {
	return noteURL(id) + "/edit"
}

func noteFilterURL(id string, filter webFilterOptions) string {
	return noteURL(id) + filterQueryString(filter, true)
}

func tasksURL(filter webFilterOptions) string {
	values := url.Values{}
	for _, tag := range filter.Tags {
		values.Add("tag", tag)
	}
	switch filter.ArchivedMode {
	case "include":
		values.Set("archived", "1")
	case "only":
		values.Set("archived", "only")
	}
	if encoded := values.Encode(); encoded != "" {
		return "/tasks?" + encoded
	}
	return "/tasks"
}

func filterPageURL(page string, filter webFilterOptions) string {
	return page + filterQueryString(filter, true)
}

func journalURL(date time.Time) string {
	values := url.Values{}
	values.Set("date", dateOnly(date).Format("2006-01-02"))
	return "/journal?" + values.Encode()
}

func filterQueryString(filter webFilterOptions, includeQuery bool) string {
	values := url.Values{}
	if includeQuery && filter.Query != "" {
		values.Set("q", filter.Query)
	}
	for _, tag := range filter.Tags {
		values.Add("tag", tag)
	}
	switch filter.ArchivedMode {
	case "include":
		values.Set("archived", "1")
	case "only":
		values.Set("archived", "only")
	}
	if encoded := values.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

func buildTagFilters(page string, availableTags []string, filter webFilterOptions) []webTagFilter {
	filters := make([]webTagFilter, 0, len(availableTags))
	for _, tag := range availableTags {
		next := webFilterOptions{
			Query:        filter.Query,
			Tags:         append([]string(nil), filter.Tags...),
			ArchivedMode: filter.ArchivedMode,
		}
		active := hasAllTags(filter.Tags, []string{tag})
		if active {
			var remaining []string
			for _, existing := range next.Tags {
				if existing != tag {
					remaining = append(remaining, existing)
				}
			}
			next.Tags = remaining
		} else {
			next.Tags = normalizeTags(append(next.Tags, tag))
		}
		filters = append(filters, webTagFilter{
			Name:   tag,
			URL:    filterPageURL(page, next),
			Active: active,
		})
	}
	return filters
}

func clearTagFiltersURL(page string, filter webFilterOptions) string {
	next := webFilterOptions{
		Query:        filter.Query,
		ArchivedMode: filter.ArchivedMode,
	}
	return filterPageURL(page, next)
}

func safeReturnURL(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return fallback
	}
	return raw
}

func notebookSubtitle(count int) string {
	return fmt.Sprintf("%d %s", count, pluralize(count, "note", "notes"))
}

func defaultJournalDate() time.Time {
	return dateOnly(now())
}

func parseJournalDate(values url.Values) (time.Time, error) {
	raw := strings.TrimSpace(values.Get("date"))
	if raw == "" {
		return defaultJournalDate(), nil
	}
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid journal date %q", raw)
	}
	return parsed, nil
}

func parseJournalNoteID(id string) (time.Time, bool) {
	id = strings.TrimSpace(id)
	parsed, err := time.Parse("2006-01-02", id)
	if err != nil || parsed.Format("2006-01-02") != id {
		return time.Time{}, false
	}
	return parsed, true
}

func buildJournalView(selectedDate time.Time, journalNotes map[string]webNoteSummary) webJournalView {
	selectedDate = dateOnly(selectedDate)
	currentMonth := startOfMonth(selectedDate)
	firstGridDay := startOfWeek(currentMonth)
	today := defaultJournalDate()

	days := make([]webJournalDay, 0, 42)
	for i := 0; i < 42; i++ {
		day := firstGridDay.AddDate(0, 0, i)
		id := day.Format("2006-01-02")
		_, hasEntry := journalNotes[id]
		days = append(days, webJournalDay{
			Date:           id,
			DayOfMonth:     day.Day(),
			WeekdayLabel:   day.Format("Mon"),
			IsCurrentMonth: day.Month() == currentMonth.Month(),
			IsToday:        day.Equal(today),
			IsSelected:     day.Equal(selectedDate),
			HasEntry:       hasEntry,
			BrowseURL:      journalURL(day),
		})
	}

	entry, hasEntry := journalNotes[selectedDate.Format("2006-01-02")]
	view := webJournalView{
		MonthLabel:        currentMonth.Format("January 2006"),
		SelectedDate:      selectedDate.Format("2006-01-02"),
		SelectedDateLabel: selectedDate.Format("Monday, January 2, 2006"),
		TodayURL:          journalURL(today),
		PrevMonthURL:      journalURL(shiftMonth(selectedDate, -1)),
		NextMonthURL:      journalURL(shiftMonth(selectedDate, 1)),
		Days:              days,
		HasEntry:          hasEntry,
	}
	if hasEntry {
		view.SelectedNoteURL = entry.DetailURL
	}
	return view
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func startOfMonth(t time.Time) time.Time {
	t = dateOnly(t)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func startOfWeek(t time.Time) time.Time {
	t = dateOnly(t)
	offset := (int(t.Weekday()) + 6) % 7
	return t.AddDate(0, 0, -offset)
}

func shiftMonth(t time.Time, delta int) time.Time {
	t = dateOnly(t)
	targetMonth := startOfMonth(t).AddDate(0, delta, 0)
	lastDay := targetMonth.AddDate(0, 1, -1).Day()
	day := t.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(targetMonth.Year(), targetMonth.Month(), day, 0, 0, 0, 0, time.UTC)
}

func formatOutgoingLinks(links []webLink) string {
	if len(links) == 0 {
		return ""
	}
	parts := make([]string, 0, len(links))
	for _, link := range links {
		parts = append(parts, link.ID)
	}
	return strings.Join(parts, ", ")
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

func parseServeOptions(args []string) (serveOptions, error) {
	opts := serveOptions{Addr: "127.0.0.1:8080"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--watch":
			opts.Watch = true
		case arg == "--addr":
			if i+1 >= len(args) {
				return serveOptions{}, errors.New("--addr requires a value")
			}
			opts.Addr = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--addr="):
			opts.Addr = strings.TrimSpace(strings.TrimPrefix(arg, "--addr="))
		default:
			return serveOptions{}, fmt.Errorf("unknown serve argument %q", arg)
		}
	}
	if opts.Addr == "" {
		return serveOptions{}, errors.New("serve address cannot be empty")
	}
	return opts, nil
}

func normalizeServeAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
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
		return errors.New("--include-archived and --archived-only are only valid for list/search/tasks")
	}
	if opts.Template != "" {
		return errors.New("--template is only valid for create/template")
	}
	return nil
}

func validateCreateOptions(opts noteOptions) error {
	if opts.IncludeArchived || opts.ArchivedOnly {
		return errors.New("--include-archived and --archived-only are only valid for list/search/tasks")
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

func normalizeWebBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	return strings.TrimRight(body, "\n")
}

func parseWebTags(tags string) []string {
	return normalizeTags(strings.FieldsFunc(tags, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}))
}

func createWebNote(form webNoteForm) (string, error) {
	id, content, err := buildNoteFromOptions(createOptions{
		Title: form.Title,
		noteOptions: noteOptions{
			Tags: parseWebTags(form.Tags),
			Body: form.Body,
		},
	})
	if err != nil {
		return "", err
	}
	content.Archived = form.Archived

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return "", err
	}
	formatted := formatNote(content)
	if err := os.WriteFile(notePath(id), []byte(formatted), 0o644); err != nil {
		return "", err
	}
	if err := appendHistoryVersion(id, "create", []byte(formatted)); err != nil {
		return "", err
	}
	return id, nil
}

func updateWebNote(id string, form webNoteForm) error {
	path := notePath(id)
	content, err := readNoteContent(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("note %q not found", id)
		}
		return err
	}

	title := strings.TrimSpace(form.Title)
	if title == "" {
		return errors.New("title cannot be empty")
	}

	content.Title = title
	content.Tags = parseWebTags(form.Tags)
	content.Body = form.Body
	content.Archived = form.Archived

	return writeNoteVersioned(id, "edit", []byte(formatNote(content)))
}

func historyNote(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("history requires a note id and optional version id")
	}

	id := strings.TrimSpace(args[0])
	versions, err := loadNoteVersions(id)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return fmt.Errorf("no history found for %q", id)
	}

	if len(args) == 1 {
		for i := len(versions) - 1; i >= 0; i-- {
			version := versions[i]
			fmt.Printf("%s\t%s\t%s\n", version.ID, version.Timestamp.Format(time.RFC3339Nano), version.Action)
		}
		return nil
	}

	version, err := findNoteVersion(id, args[1])
	if err != nil {
		return err
	}
	diff, err := noteVersionDiff(id, version)
	if err != nil {
		return err
	}
	fmt.Print(diff)
	return nil
}

func restoreNote(args []string) error {
	if len(args) != 2 {
		return errors.New("restore requires a note id and version id")
	}

	id := strings.TrimSpace(args[0])
	versionID := strings.TrimSpace(args[1])
	if err := restoreNoteVersion(id, versionID); err != nil {
		return err
	}
	fmt.Printf("restored %s to %s\n", id, versionID)
	return nil
}

func restoreNoteVersion(id, versionID string) error {
	version, err := findNoteVersion(id, versionID)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(version.Path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		return err
	}
	return writeNoteVersioned(id, "restore", data)
}

func writeNoteVersioned(id, action string, newData []byte) error {
	path := notePath(id)
	oldData, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil && string(oldData) == string(newData) {
		return os.WriteFile(path, newData, 0o644)
	}
	if err == nil {
		if err := appendHistoryVersion(id, action, oldData); err != nil {
			return err
		}
	}
	return os.WriteFile(path, newData, 0o644)
}

func appendHistoryVersion(id, action string, data []byte) error {
	if err := os.MkdirAll(historyPath(id), 0o755); err != nil {
		return err
	}

	versionID, err := nextVersionID(id, action)
	if err != nil {
		return err
	}
	return os.WriteFile(historyVersionPath(id, versionID), data, 0o644)
}

func nextVersionID(id, action string) (string, error) {
	entries, err := os.ReadDir(historyPath(id))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		return "", err
	}

	seq := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		seq++
	}
	return fmt.Sprintf("%s-%04d-%s", now().UTC().Format("20060102T150405.000000000Z"), seq+1, slugify(action)), nil
}

func loadNoteVersions(id string) ([]noteVersion, error) {
	entries, err := os.ReadDir(historyPath(id))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var versions []noteVersion
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		versionID := strings.TrimSuffix(entry.Name(), ".md")
		version, err := parseNoteVersion(id, versionID)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Timestamp.Equal(versions[j].Timestamp) {
			return versions[i].ID < versions[j].ID
		}
		return versions[i].Timestamp.Before(versions[j].Timestamp)
	})
	return versions, nil
}

func findNoteVersion(id, versionID string) (noteVersion, error) {
	versionID = strings.TrimSpace(versionID)
	version, err := parseNoteVersion(id, versionID)
	if err != nil {
		return noteVersion{}, err
	}
	if _, err := os.Stat(version.Path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return noteVersion{}, fmt.Errorf("version %q not found for %q", versionID, id)
		}
		return noteVersion{}, err
	}
	return version, nil
}

func buildWebNoteHistory(id, selectedVersionID string) (*webNoteHistory, error) {
	versions, err := loadNoteVersions(id)
	if err != nil {
		return nil, err
	}

	history := &webNoteHistory{
		NoteID:   id,
		NoteURL:  noteURL(id),
		Versions: make([]webNoteVersion, 0, len(versions)),
	}
	if len(versions) == 0 {
		return history, nil
	}

	selectedVersion := versions[len(versions)-1]
	if selectedVersionID != "" {
		selectedVersion, err = findNoteVersion(id, selectedVersionID)
		if err != nil {
			return nil, err
		}
	}
	for i := len(versions) - 1; i >= 0; i-- {
		version := versions[i]
		history.Versions = append(history.Versions, webNoteVersion{
			ID:         version.ID,
			Timestamp:  version.Timestamp.Format(time.RFC3339Nano),
			Action:     version.Action,
			BrowseURL:  noteHistoryVersionURL(id, version.ID),
			IsSelected: version.ID == selectedVersion.ID,
		})
	}

	diff, err := noteVersionDiff(id, selectedVersion)
	if err != nil {
		return nil, err
	}
	history.SelectedVersion = &webNoteVersionDetail{
		ID:         selectedVersion.ID,
		Timestamp:  selectedVersion.Timestamp.Format(time.RFC3339Nano),
		Action:     selectedVersion.Action,
		Diff:       diff,
		RestoreURL: noteRestoreURL(id),
	}

	return history, nil
}

func noteVersionDiff(id string, version noteVersion) (string, error) {
	current, currentErr := os.ReadFile(notePath(id))
	if currentErr != nil && !errors.Is(currentErr, fs.ErrNotExist) {
		return "", currentErr
	}
	target, err := os.ReadFile(version.Path)
	if err != nil {
		return "", err
	}

	currentLabel := notePath(id)
	if errors.Is(currentErr, fs.ErrNotExist) {
		currentLabel = "(deleted)"
		current = nil
	}
	return unifiedDiff("history:"+version.ID, currentLabel, string(target), string(current)), nil
}

func parseNoteVersion(id, versionID string) (noteVersion, error) {
	parts := strings.SplitN(versionID, "-", 3)
	if len(parts) != 3 {
		return noteVersion{}, fmt.Errorf("invalid version %q", versionID)
	}
	timestamp, err := time.Parse("20060102T150405.000000000Z", parts[0])
	if err != nil {
		return noteVersion{}, fmt.Errorf("invalid version %q", versionID)
	}
	return noteVersion{
		ID:        versionID,
		Timestamp: timestamp,
		Action:    parts[2],
		Path:      historyVersionPath(id, versionID),
	}, nil
}

func moveHistory(oldID, newID string) error {
	oldHistory := historyPath(oldID)
	if _, err := os.Stat(oldHistory); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	newHistory := historyPath(newID)
	if _, err := os.Stat(newHistory); err == nil {
		return fmt.Errorf("history for %q already exists", newID)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newHistory), 0o755); err != nil {
		return err
	}
	return os.Rename(oldHistory, newHistory)
}

func historyPath(id string) string {
	return filepath.Join(notesDir, noteHistoryDir, id)
}

func historyVersionPath(id, versionID string) string {
	return filepath.Join(historyPath(id), versionID+".md")
}

func unifiedDiff(fromLabel, toLabel, fromText, toText string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", fromLabel)
	fmt.Fprintf(&b, "+++ %s\n", toLabel)

	fromLines := splitDiffLines(fromText)
	toLines := splitDiffLines(toText)
	ops := diffOperations(fromLines, toLines)
	for _, op := range ops {
		switch op.kind {
		case "equal":
			fmt.Fprintf(&b, " %s\n", op.line)
		case "delete":
			fmt.Fprintf(&b, "-%s\n", op.line)
		case "insert":
			fmt.Fprintf(&b, "+%s\n", op.line)
		}
	}
	return b.String()
}

type diffOp struct {
	kind string
	line string
}

func splitDiffLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func diffOperations(a, b []string) []diffOp {
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{kind: "equal", line: a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{kind: "delete", line: a[i]})
			i++
		default:
			ops = append(ops, diffOp{kind: "insert", line: b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		ops = append(ops, diffOp{kind: "delete", line: a[i]})
	}
	for ; j < len(b); j++ {
		ops = append(ops, diffOp{kind: "insert", line: b[j]})
	}
	return ops
}

func renderMarkdownHTML(content noteContent, existing map[string]struct{}, tasks *taskRenderOptions) template.HTML {
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
		lineNo := content.BodyLine + i
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
			b.WriteString("<h3>" + renderInlineMarkdown(strings.TrimSpace(trimmed[4:]), existing) + "</h3>")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			closeList()
			b.WriteString("<h2>" + renderInlineMarkdown(strings.TrimSpace(trimmed[3:]), existing) + "</h2>")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			closeList()
			b.WriteString("<h1>" + renderInlineMarkdown(strings.TrimSpace(trimmed[2:]), existing) + "</h1>")
			continue
		}

		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			b.WriteString("<li>" + renderListItemMarkdown(strings.TrimSpace(trimmed[2:]), existing, tasks, lineNo) + "</li>")
			continue
		}

		closeList()
		b.WriteString("<p>" + renderInlineMarkdown(trimmed, existing) + "</p>")
	}

	closeList()
	if inCode {
		b.WriteString("</code></pre>")
	}
	return template.HTML(b.String())
}

func renderListItemMarkdown(text string, existing map[string]struct{}, tasks *taskRenderOptions, line int) string {
	task, ok := parseTaskLine(text)
	if !ok {
		return renderInlineMarkdown(text, existing)
	}

	checked := ""
	if !task.Open {
		checked = " checked"
	}
	body := renderInlineMarkdown(task.Text, existing)
	if tasks == nil || tasks.NoteID == "" {
		return `<label class="task-item"><input type="checkbox" disabled` + checked + `><span>` + body + `</span></label>`
	}

	return `<form class="task-toggle-form" method="post" action="/tasks/toggle">` +
		`<input type="hidden" name="note" value="` + html.EscapeString(tasks.NoteID) + `">` +
		`<input type="hidden" name="line" value="` + strconv.Itoa(line) + `">` +
		`<input type="hidden" name="return_to" value="` + html.EscapeString(tasks.ReturnURL) + `">` +
		`<label class="task-item task-item-toggle"><input type="checkbox" onchange="this.form.submit()"` + checked + `><span>` + body + `</span></label>` +
		`</form>`
}

func renderInlineMarkdown(text string, existing map[string]struct{}) string {
	text = html.EscapeString(text)
	text = renderWikiLinks(text, existing)
	text = inlineCodePattern.ReplaceAllString(text, "<code>$1</code>")
	text = boldPattern.ReplaceAllString(text, "<strong>$1</strong>")
	text = italicPattern.ReplaceAllString(text, "<em>$1</em>")
	return text
}

func openTasksFromBody(body string) []noteTask {
	lines := strings.Split(body, "\n")
	var tasks []noteTask
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if !(strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) {
			continue
		}
		task, ok := parseTaskLine(strings.TrimSpace(trimmed[2:]))
		if !ok || !task.Open {
			continue
		}
		task.Line = i + 1
		tasks = append(tasks, task)
	}
	return tasks
}

func openTasksFromContent(content noteContent) []noteTask {
	tasks := openTasksFromBody(content.Body)
	for i := range tasks {
		tasks[i].Line += content.BodyLine - 1
	}
	return tasks
}

func toggleTaskByLine(id string, line int) (noteTask, error) {
	path := notePath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return noteTask{}, fmt.Errorf("note %q not found", id)
		}
		return noteTask{}, err
	}

	updated, task, err := toggleTaskLine(string(data), line)
	if err != nil {
		return noteTask{}, err
	}
	if err := writeNoteVersioned(id, "toggle-task", []byte(updated)); err != nil {
		return noteTask{}, err
	}
	return task, nil
}

func toggleTaskLine(data string, line int) (string, noteTask, error) {
	lines := strings.Split(data, "\n")
	if line < 1 || line > len(lines) {
		return "", noteTask{}, fmt.Errorf("line %d is out of range", line)
	}

	raw := lines[line-1]
	trimmed := strings.TrimLeft(raw, " \t")
	if !(strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) {
		return "", noteTask{}, fmt.Errorf("line %d is not a Markdown task", line)
	}

	task, ok := parseTaskLine(strings.TrimSpace(trimmed[2:]))
	if !ok {
		return "", noteTask{}, fmt.Errorf("line %d is not a Markdown task", line)
	}

	indent := raw[:len(raw)-len(trimmed)]
	task.Open = !task.Open
	state := "[x]"
	if task.Open {
		state = "[ ]"
	}

	lines[line-1] = indent + trimmed[:2] + state + " " + task.Text
	task.Line = line
	return strings.Join(lines, "\n"), task, nil
}

func parseTaskLine(text string) (noteTask, bool) {
	if len(text) < 4 || text[0] != '[' || text[2] != ']' {
		return noteTask{}, false
	}
	if text[3] != ' ' {
		return noteTask{}, false
	}

	state := text[1]
	switch state {
	case ' ':
		return noteTask{Text: strings.TrimSpace(text[4:]), Open: true}, true
	case 'x', 'X':
		return noteTask{Text: strings.TrimSpace(text[4:]), Open: false}, true
	default:
		return noteTask{}, false
	}
}

func renderWikiLinks(text string, existing map[string]struct{}) string {
	return noteLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := noteLinkPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		target := strings.TrimSpace(parts[1])
		if target == "" {
			return match
		}
		label := "[[" + html.EscapeString(target) + "]]"
		if _, ok := existing[target]; !ok {
			return `<span class="broken-link">` + label + `</span>`
		}
		return `<a class="wiki-link" href="` + noteURL(target) + `">` + label + `</a>`
	})
}

func notebookPageTemplate() *template.Template {
	return template.Must(template.New("notebook").Funcs(template.FuncMap{
		"noteEditURL":    noteEditURL,
		"noteHistoryURL": noteHistoryURL,
	}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    :root { color-scheme: light; --bg:#f4efe7; --card:#fffdf9; --ink:#1f1a17; --muted:#6c6258; --line:#d7cbbd; --accent:#0f766e; --accent-soft:#d6f0ed; --warn:#b42318; --warn-soft:#fde7e5; }
    * { box-sizing:border-box; }
    body { margin:0; font-family: Georgia, "Times New Roman", serif; color:var(--ink); background:radial-gradient(circle at top, #fff8ef 0, var(--bg) 55%); }
    a { color:inherit; }
    .layout { display:grid; grid-template-columns: minmax(280px, 360px) minmax(0, 1fr); min-height:100vh; }
    .sidebar { border-right:1px solid var(--line); padding:2rem 1.5rem; background:rgba(255,253,249,0.8); backdrop-filter: blur(8px); }
    .content { padding:2rem; }
    .panel { background:var(--card); border:1px solid var(--line); border-radius:18px; box-shadow:0 10px 30px rgba(60,35,15,0.08); }
    .sidebar .panel { padding:1.25rem; }
    .content .panel { padding:1.5rem 1.75rem; max-width:900px; }
    h1, h2, h3 { font-weight:600; line-height:1.2; margin:0 0 0.75rem; }
    p { line-height:1.65; margin:0 0 1rem; }
    ul { margin:0 0 1rem 1.25rem; padding:0; }
    pre { overflow:auto; background:#f8f3ec; padding:0.9rem; border-radius:12px; border:1px solid var(--line); }
    code { font-family: "SFMono-Regular", Consolas, monospace; font-size:0.92em; }
    .eyebrow { text-transform:uppercase; letter-spacing:0.08em; font-size:0.74rem; color:var(--muted); margin-bottom:0.4rem; }
    .subtitle { color:var(--muted); margin-bottom:1.25rem; }
    .filter-form { display:grid; gap:0.75rem; margin-top:1rem; }
    .filter-form .field input[type="search"], .filter-form .field input[type="text"], .filter-form .field select { width:100%; padding:0.7rem 0.8rem; border-radius:12px; border:1px solid var(--line); background:#fff; color:var(--ink); font:inherit; }
    .note-list { display:grid; gap:0.85rem; margin-top:1.25rem; }
    .note-item { display:block; padding:0.9rem 1rem; border-radius:14px; text-decoration:none; border:1px solid var(--line); background:#fff; }
    .note-item:hover { border-color:var(--accent); transform:translateY(-1px); transition:160ms ease; }
    .note-item strong { display:block; margin-bottom:0.25rem; }
    .meta, .filters { display:flex; gap:0.4rem; flex-wrap:wrap; align-items:center; }
    .meta { color:var(--muted); font-size:0.85rem; }
    .tag, .filter-link { display:inline-flex; align-items:center; padding:0.25rem 0.55rem; border-radius:999px; border:1px solid var(--line); background:#fff; text-decoration:none; font-size:0.85rem; }
    .filter-link.active { background:var(--accent); color:#fff; border-color:var(--accent); }
    .warning { margin:1rem 0; padding:0.85rem 1rem; border-radius:14px; border:1px solid #efb3ab; background:var(--warn-soft); color:var(--warn); }
    .warning ul { margin-bottom:0; }
    .wiki-link { color:var(--accent); text-decoration:none; font-weight:600; }
    .broken-link { color:var(--warn); font-weight:600; background:var(--warn-soft); padding:0.05rem 0.25rem; border-radius:6px; }
    .section { margin-top:1.5rem; padding-top:1.25rem; border-top:1px solid var(--line); }
    .empty { color:var(--muted); font-style:italic; }
    .archived { color:var(--warn); font-weight:600; }
    .toolbar { margin:1rem 0 0; display:flex; gap:0.6rem; flex-wrap:wrap; }
    .toggle { text-decoration:none; font-size:0.9rem; color:var(--accent); }
    .button { display:inline-flex; align-items:center; justify-content:center; padding:0.6rem 0.95rem; border-radius:999px; border:1px solid var(--accent); background:var(--accent); color:#fff; text-decoration:none; font-size:0.92rem; cursor:pointer; }
    .button.secondary { background:transparent; color:var(--accent); }
    .actions { margin-top:1rem; display:flex; gap:0.6rem; flex-wrap:wrap; }
    .form-grid { display:grid; gap:1rem; margin-top:1.25rem; }
    .field { display:grid; gap:0.4rem; }
    .field label { font-weight:600; }
    .field input[type="text"], .field textarea { width:100%; padding:0.75rem 0.85rem; border-radius:12px; border:1px solid var(--line); background:#fff; color:var(--ink); font:inherit; }
    .field textarea { min-height:320px; resize:vertical; font-family: "SFMono-Regular", Consolas, monospace; font-size:0.95rem; line-height:1.5; }
    .field .hint { color:var(--muted); font-size:0.85rem; }
    .checkbox { display:flex; align-items:center; gap:0.55rem; }
    .task-groups { display:grid; gap:1rem; margin-top:1rem; }
    .task-group { border:1px solid var(--line); border-radius:16px; padding:1rem 1rem 0.9rem; background:#fff; }
    .task-group h2 { margin:0 0 0.35rem; font-size:1.1rem; }
    .task-list { display:grid; gap:0.65rem; margin-top:0.85rem; }
    .task-toggle-form { margin:0; }
    .task-item { display:flex; align-items:flex-start; gap:0.6rem; }
    .task-item input { margin-top:0.2rem; }
    .task-item-toggle { cursor:pointer; }
    .form-actions { display:flex; gap:0.75rem; flex-wrap:wrap; }
    .history-layout { display:grid; grid-template-columns: minmax(240px, 320px) minmax(0, 1fr); gap:1rem; align-items:start; }
    .history-list { display:grid; gap:0.75rem; }
    .history-item { display:block; border:1px solid var(--line); border-radius:14px; padding:0.85rem 0.95rem; background:#fff; text-decoration:none; }
    .history-item.active { border-color:var(--accent); background:var(--accent-soft); }
    .history-item strong { display:block; margin-bottom:0.25rem; }
    .history-panel { border:1px solid var(--line); border-radius:16px; padding:1rem; background:#fff; }
    .history-actions { display:flex; gap:0.75rem; flex-wrap:wrap; margin-bottom:1rem; }
    .history-restore-form { margin:0; }
    .journal-header { display:flex; justify-content:space-between; gap:1rem; align-items:flex-start; flex-wrap:wrap; }
    .journal-nav { display:flex; gap:0.6rem; flex-wrap:wrap; }
    .calendar-weekdays, .calendar-grid { display:grid; grid-template-columns:repeat(7, minmax(0, 1fr)); gap:0.55rem; }
    .calendar-weekdays { margin-top:1.25rem; color:var(--muted); font-size:0.82rem; text-transform:uppercase; letter-spacing:0.08em; }
    .calendar-grid { margin-top:0.6rem; }
    .calendar-day { display:block; min-height:92px; padding:0.7rem; border:1px solid var(--line); border-radius:16px; background:#fff; text-decoration:none; }
    .calendar-day:hover { border-color:var(--accent); transform:translateY(-1px); transition:160ms ease; }
    .calendar-day.muted { opacity:0.45; }
    .calendar-day.selected { border-color:var(--accent); background:var(--accent-soft); }
    .calendar-day.today { box-shadow: inset 0 0 0 1px var(--accent); }
    .calendar-day .meta { justify-content:space-between; align-items:flex-start; }
    .calendar-dot { width:0.55rem; height:0.55rem; border-radius:999px; background:var(--accent); display:inline-block; }
    .calendar-empty { color:var(--muted); font-size:0.85rem; margin-top:1.4rem; }
    @media (max-width: 880px) {
      .layout { grid-template-columns: 1fr; }
      .sidebar { border-right:none; border-bottom:1px solid var(--line); }
      .content { padding-top:1rem; }
      .history-layout { grid-template-columns: 1fr; }
      .calendar-day { min-height:76px; }
    }
  </style>
</head>
<body>
  <div class="layout">
    <aside class="sidebar">
      <div class="panel">
        <div class="eyebrow">Local notebook</div>
        <h1>{{.HeaderTitle}}</h1>
        <div class="subtitle">{{.HeaderSubtitle}}</div>
        <form class="filter-form" method="get" action="{{.FilterPage}}">
          <div class="field">
            <label for="sidebar-search">Search</label>
            <input id="sidebar-search" name="q" type="search" value="{{.SearchQuery}}" placeholder="Title or body">
          </div>
          <div class="field">
            <label for="sidebar-tags">Tags</label>
            <input id="sidebar-tags" name="tags" type="text" value="{{.FilterTagsInput}}" placeholder="work, review">
            <div class="hint">Matches notes containing every listed tag.</div>
          </div>
          <div class="field">
            <label for="sidebar-archived">Archived</label>
            <select id="sidebar-archived" name="archived">
              <option value="" {{if eq .ArchivedMode "exclude"}}selected{{end}}>Hide archived</option>
              <option value="1" {{if eq .ArchivedMode "include"}}selected{{end}}>Include archived</option>
              <option value="only" {{if eq .ArchivedMode "only"}}selected{{end}}>Archived only</option>
            </select>
          </div>
          <div class="form-actions">
            <button class="button" type="submit">Apply</button>
            <a class="button secondary" href="{{.FilterPage}}">Reset</a>
          </div>
        </form>
        <div class="filters">
          <a class="filter-link {{if eq (len .ActiveTags) 0}}active{{end}}" href="{{.ClearTagsURL}}">All</a>
          {{range .TagFilters}}
            <a class="filter-link {{if .Active}}active{{end}}" href="{{.URL}}">#{{.Name}}</a>
          {{end}}
        </div>
        <div class="toolbar">
          <a class="button secondary" href="/new">New note</a>
          <a class="button secondary" href="{{.TasksPageURL}}">Open tasks</a>
          <a class="button secondary" href="{{.JournalPageURL}}">Journal</a>
        </div>
        <div class="note-list">
          {{range .Notes}}
            <a class="note-item" href="{{.DetailURL}}">
              <strong>{{.Title}}</strong>
              <div class="meta">{{.ID}} · {{.ModTime}}</div>
              <div class="meta">
                {{if .Archived}}<span class="archived">archived</span>{{end}}
                {{if .Tags}}{{range .Tags}}<span class="tag">#{{.}}</span>{{end}}{{end}}
              </div>
              <p>{{.Snippet}}</p>
              <div class="meta">{{.LinksCount}} links · {{.Backlinks}} backlinks{{if .BrokenLinks}} · {{len .BrokenLinks}} broken{{end}}</div>
            </a>
          {{else}}
            <div class="empty">No notes match this filter.</div>
          {{end}}
        </div>
      </div>
    </aside>
    <main class="content">
      <div class="panel">
        {{if .NoteForm}}
          <div class="eyebrow">{{if .NoteForm.IsEditing}}Editing{{else}}Create{{end}}</div>
          <h1>{{.HeaderTitle}}</h1>
          <p>{{.HeaderSubtitle}}</p>
          {{if .NoteForm.Error}}
            <div class="warning">{{.NoteForm.Error}}</div>
          {{end}}
          <form class="form-grid" method="post" action="{{.NoteForm.ActionURL}}">
            <div class="field">
              <label for="title">Title</label>
              <input id="title" name="title" type="text" value="{{.NoteForm.Title}}" required>
            </div>
            <div class="field">
              <label for="tags">Tags</label>
              <input id="tags" name="tags" type="text" value="{{.NoteForm.Tags}}">
              <div class="hint">Comma-separated tags. They will be normalized to notebook tag IDs.</div>
            </div>
            <div class="field">
              <label for="body">Markdown</label>
              <textarea id="body" name="body">{{.NoteForm.Body}}</textarea>
            </div>
            <label class="checkbox" for="archived">
              <input id="archived" name="archived" type="checkbox" value="1" {{if .NoteForm.Archived}}checked{{end}}>
              <span>Archived</span>
            </label>
            <div class="form-actions">
              <button class="button" type="submit">{{.NoteForm.SubmitLabel}}</button>
              <a class="button secondary" href="{{.NoteForm.CancelURL}}">Cancel</a>
            </div>
          </form>
        {{else if .Journal}}
          <div class="eyebrow">Daily Notes</div>
          <div class="journal-header">
            <div>
              <h1>{{.Journal.MonthLabel}}</h1>
              <p>{{.Journal.SelectedDateLabel}}</p>
            </div>
            <div class="journal-nav">
              <a class="button secondary" href="{{.Journal.PrevMonthURL}}">Previous month</a>
              <a class="button secondary" href="{{.Journal.TodayURL}}">Today</a>
              <a class="button secondary" href="{{.Journal.NextMonthURL}}">Next month</a>
            </div>
          </div>
          <div class="calendar-weekdays">
            <div>Mon</div>
            <div>Tue</div>
            <div>Wed</div>
            <div>Thu</div>
            <div>Fri</div>
            <div>Sat</div>
            <div>Sun</div>
          </div>
          <div class="calendar-grid">
            {{range .Journal.Days}}
              <a class="calendar-day {{if not .IsCurrentMonth}}muted{{end}} {{if .IsSelected}}selected{{end}} {{if .IsToday}}today{{end}}" href="{{.BrowseURL}}">
                <div class="meta">
                  <strong>{{.DayOfMonth}}</strong>
                  {{if .HasEntry}}<span class="calendar-dot" title="Entry available"></span>{{end}}
                </div>
                <div class="calendar-empty">{{if .HasEntry}}Entry saved{{else}}No entry{{end}}</div>
              </a>
            {{end}}
          </div>
          <div class="section">
            {{if .Journal.HasEntry}}
              <div class="eyebrow">{{.CurrentNote.ID}}</div>
              <h2>{{.CurrentNote.Title}}</h2>
              <div class="meta">{{.CurrentNote.ModTime}}{{if .CurrentNote.Archived}} · <span class="archived">archived</span>{{end}}</div>
              <div class="actions">
                <a class="button secondary" href="{{.Journal.SelectedNoteURL}}">Open note page</a>
                <a class="button secondary" href="{{noteEditURL .CurrentNote.ID}}">Edit note</a>
              </div>
              {{if .CurrentNote.Tags}}
                <div class="meta" style="margin-top:0.75rem;">
                  {{range .CurrentNote.Tags}}<span class="tag">#{{.}}</span>{{end}}
                </div>
              {{end}}
              <div class="section">{{.CurrentNote.RenderedBody}}</div>
            {{else}}
              <h2>{{.Journal.SelectedDate}}</h2>
              <p>No journal entry exists for this day yet.</p>
              <div class="actions">
                <a class="button secondary" href="/new">Create a note</a>
              </div>
            {{end}}
          </div>
        {{else if .CurrentNote}}
          <div class="eyebrow">{{.CurrentNote.ID}}</div>
          <h1>{{.CurrentNote.Title}}</h1>
          <div class="meta">{{.CurrentNote.ModTime}}{{if .CurrentNote.Archived}} · <span class="archived">archived</span>{{end}}</div>
          <div class="actions">
            <a class="button secondary" href="{{noteEditURL .CurrentNote.ID}}">Edit note</a>
            <a class="button secondary" href="{{noteHistoryURL .CurrentNote.ID}}">History</a>
          </div>
          {{if .CurrentNote.Tags}}
            <div class="meta" style="margin-top:0.75rem;">
              {{range .CurrentNote.Tags}}<span class="tag">#{{.}}</span>{{end}}
            </div>
          {{end}}
          {{if .CurrentNote.BrokenLinks}}
            <div class="warning">
              Broken links in this note:
              <ul>
                {{range .CurrentNote.BrokenLinks}}<li>[[{{.}}]] has no matching note.</li>{{end}}
              </ul>
            </div>
          {{end}}
          <div class="section">{{.CurrentNote.RenderedBody}}</div>
          <div class="section">
            <h2>Outgoing Links</h2>
            {{if .CurrentNote.OutgoingLinks}}
              <div class="filters">
                {{range .CurrentNote.OutgoingLinks}}<a class="tag" href="{{.URL}}">{{.ID}}</a>{{end}}
              </div>
            {{else}}
              <div class="empty">No outgoing wiki links.</div>
            {{end}}
          </div>
          <div class="section">
            <h2>Backlinks</h2>
            {{if .CurrentNote.Backlinks}}
              <div class="filters">
                {{range .CurrentNote.Backlinks}}<a class="tag" href="{{.URL}}">{{.ID}}</a>{{end}}
              </div>
            {{else}}
              <div class="empty">No backlinks yet.</div>
            {{end}}
          </div>
          {{if .NoteHistory}}
            <div class="section">
              <h2>History</h2>
              {{if .NoteHistory.Versions}}
                <div class="history-layout">
                  <div class="history-list">
                    {{range .NoteHistory.Versions}}
                      <a class="history-item {{if .IsSelected}}active{{end}}" href="{{.BrowseURL}}">
                        <strong>{{.Action}}</strong>
                        <div class="meta">{{.Timestamp}}</div>
                        <div class="meta"><code>{{.ID}}</code></div>
                      </a>
                    {{end}}
                  </div>
                  {{with .NoteHistory.SelectedVersion}}
                    <div class="history-panel">
                      <div class="history-actions">
                        <a class="button secondary" href="{{$.NoteHistory.NoteURL}}">Back to note</a>
                        <form class="history-restore-form" method="post" action="{{.RestoreURL}}">
                          <input type="hidden" name="version" value="{{.ID}}">
                          <button class="button" type="submit">Restore this version</button>
                        </form>
                      </div>
                      <h3>{{.Action}}</h3>
                      <div class="meta">{{.Timestamp}}</div>
                      <pre><code>{{.Diff}}</code></pre>
                    </div>
                  {{end}}
                </div>
              {{else}}
                <div class="empty">No saved history for this note yet.</div>
              {{end}}
            </div>
          {{end}}
        {{else if .ShowTasks}}
          <div class="eyebrow">Notebook</div>
          <h1>{{.HeaderTitle}}</h1>
          <p>{{.HeaderSubtitle}}</p>
          {{if .TaskGroups}}
            <div class="task-groups">
              {{range .TaskGroups}}
                <section class="task-group">
                  <h2><a href="{{.DetailURL}}">{{.Title}}</a></h2>
                  <div class="meta">{{.ID}} · {{.ModTime}}{{if .Archived}} · <span class="archived">archived</span>{{end}}</div>
                  {{if .Tags}}
                    <div class="meta" style="margin-top:0.5rem;">
                      {{range .Tags}}<span class="tag">#{{.}}</span>{{end}}
                    </div>
                  {{end}}
                  <div class="task-list">
                    {{$group := .}}
                    {{range .Tasks}}
                      <form class="task-toggle-form" method="post" action="/tasks/toggle">
                        <input type="hidden" name="note" value="{{$group.ID}}">
                        <input type="hidden" name="line" value="{{.Line}}">
                        <input type="hidden" name="return_to" value="{{$.TasksPageURL}}">
                        <label class="task-item task-item-toggle"><input type="checkbox" onchange="this.form.submit()"><span>{{.Text}}</span></label>
                      </form>
                    {{end}}
                  </div>
                </section>
              {{end}}
            </div>
          {{else}}
            <div class="empty">No open tasks match this filter.</div>
          {{end}}
        {{else}}
          <div class="eyebrow">Notebook</div>
          <h1>Rendered notes</h1>
          <p>Select a note from the left to browse rendered Markdown, follow wiki links, inspect backlinks, catch broken references, or open the notebook-wide task list.</p>
          <div class="actions">
            <a class="button" href="/new">Create a note</a>
          </div>
        {{end}}
      </div>
    </main>
  </div>
  {{if .LiveReload}}
  <script>
    if (window.EventSource) {
      const events = new EventSource("/events");
      events.addEventListener("refresh", () => window.location.reload());
    }
  </script>
  {{end}}
</body>
</html>`))
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

func customTemplates() (map[string]noteTemplate, error) {
	entries, err := os.ReadDir(customTemplateDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]noteTemplate{}, nil
		}
		return nil, err
	}

	templates := make(map[string]noteTemplate)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		name := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(entry.Name(), ".md")))
		if name == "" {
			continue
		}
		if _, exists := builtInTemplates()[name]; exists {
			continue
		}

		path := filepath.Join(customTemplateDir(), entry.Name())
		content, err := readNoteContent(path)
		if err != nil {
			return nil, err
		}

		defaultTitle := content.Title
		body := content.Body
		defaultTags := append([]string(nil), content.Tags...)
		templates[name] = noteTemplate{
			Name:         name,
			Description:  fmt.Sprintf("Custom template loaded from %s", path),
			DefaultTitle: func() string { return defaultTitle },
			DefaultTags:  defaultTags,
			Body: func(title string) string {
				return body
			},
		}
	}

	return templates, nil
}

func allTemplates() (map[string]noteTemplate, error) {
	templates := builtInTemplates()
	custom, err := customTemplates()
	if err != nil {
		return nil, err
	}
	for name, tmpl := range custom {
		templates[name] = tmpl
	}
	return templates, nil
}

func findTemplate(name string) (noteTemplate, error) {
	templates, err := allTemplates()
	if err != nil {
		return noteTemplate{}, err
	}

	template, ok := templates[name]
	if !ok {
		return noteTemplate{}, fmt.Errorf("unknown template %q", name)
	}
	return template, nil
}

func defaultDailyTitle() string {
	return now().Format("2006-01-02")
}

func printTemplates() error {
	templates, err := allTemplates()
	if err != nil {
		return err
	}

	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("Available templates:")
	for _, name := range names {
		template := templates[name]
		fmt.Printf("  %s\t%s\n", template.Name, template.Description)
	}
	return nil
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
	fmt.Println("  create [<title>] [content] --template <name>         Create a note from a built-in or disk template")
	fmt.Println("  template [list|<name> [<title>] [content]]           List templates or create from one directly")
	fmt.Println("  edit <id> [content] [--tag <tag>] [--tags a,b]       Replace note body/tags or open in $EDITOR")
	fmt.Println("  history <id> [version]       List saved versions or diff one against the current note")
	fmt.Println("  archive <id>                Mark a note as archived")
	fmt.Println("  unarchive <id>              Remove archived status from a note")
	fmt.Println("  rename <old-id> <new-id>    Rename a note file and rewrite matching note links")
	fmt.Println("  list [--tag <tag>]... [--include-archived|--archived-only]   List saved notes")
	fmt.Println("  search <query> [--tag <tag>]... [--include-archived|--archived-only] Search note titles and bodies")
	fmt.Println("  tasks [--tag <tag>]... [--include-archived|--archived-only]  List open Markdown checkbox tasks with line numbers")
	fmt.Println("  tasks toggle <id> <line>   Toggle a Markdown checkbox task by file line")
	fmt.Println("  today                     Create or open today's daily note")
	fmt.Println("  inbox [--task] <text>     Append a quick capture to the inbox note")
	fmt.Println("  view <id>                 Print a note")
	fmt.Println("  links <id>                List outgoing [[note-id]] links from a note")
	fmt.Println("  backlinks <id>            List notes that link to a note")
	fmt.Println("  graph                     Emit the notebook link graph as Graphviz DOT")
	fmt.Println("  serve [--addr host:port] [--watch]  Serve a local web UI for browsing rendered notes")
	fmt.Println("  delete <id>               Delete a note")
	fmt.Println("  restore <id> <version>    Restore a note from local history")
	fmt.Println("  doctor [--fix] [--report] Check for broken wiki links and orphaned notes")
	fmt.Println("  sync                      Commit note changes and pull/push notes/ to its Git upstream")
}

func parseInboxArgs(args []string) (string, bool, error) {
	var bodyParts []string
	asTask := false
	parseFlags := true

	for _, arg := range args {
		if parseFlags && arg == "--" {
			parseFlags = false
			continue
		}
		if parseFlags && arg == "--task" {
			asTask = true
			continue
		}
		if parseFlags && strings.HasPrefix(arg, "--") {
			return "", false, fmt.Errorf("unknown flag %q", arg)
		}
		bodyParts = append(bodyParts, arg)
	}

	body := strings.TrimSpace(strings.Join(bodyParts, " "))
	if body == "" {
		return "", false, errors.New("inbox requires text to capture")
	}
	return body, asTask, nil
}

func formatInboxEntry(body string, asTask bool) string {
	stamp := now().UTC().Format("2006-01-02 15:04 UTC")
	if asTask {
		return fmt.Sprintf("- [ ] [%s] %s", stamp, body)
	}
	return fmt.Sprintf("- [%s] %s", stamp, body)
}
