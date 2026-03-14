package tasks

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var duePattern = regexp.MustCompile(`(?i)\s*\[?due:\s*(\d{4}-\d{2}-\d{2})\]?\s*$`)

type Task struct {
	Text      string
	RawText   string
	Line      int
	Open      bool
	Source    string
	DueDate   string
	DueTime   time.Time
	DueStatus string
}

type GroupTask struct {
	Text      string
	Line      int
	DueDate   string
	DueStatus string
}

type Group struct {
	ID      string
	ModTime string
	Tasks   []GroupTask
	NextDue string
}

func OpenFromBody(body string, bodyLine int, source string) []Task {
	lines := strings.Split(body, "\n")
	var tasks []Task
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !(strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) {
			continue
		}
		task, ok := ParseLine(strings.TrimSpace(trimmed[2:]))
		if !ok || !task.Open {
			continue
		}
		task.Line = bodyLine + i
		task.Source = source
		tasks = append(tasks, task)
	}
	return tasks
}

func ToggleTaskLine(data string, line int) (string, Task, error) {
	lines := strings.Split(data, "\n")
	if line < 1 || line > len(lines) {
		return "", Task{}, fmt.Errorf("line %d is out of range", line)
	}

	raw := lines[line-1]
	trimmed := strings.TrimLeft(raw, " \t")
	if !(strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) {
		return "", Task{}, fmt.Errorf("line %d is not a Markdown task", line)
	}
	task, ok := ParseLine(strings.TrimSpace(trimmed[2:]))
	if !ok {
		return "", Task{}, fmt.Errorf("line %d is not a Markdown task", line)
	}

	indent := raw[:len(raw)-len(trimmed)]
	task.Open = !task.Open
	state := "[x]"
	if task.Open {
		state = "[ ]"
	}

	lines[line-1] = indent + trimmed[:2] + state + " " + task.RawText
	task.Line = line
	return strings.Join(lines, "\n"), task, nil
}

func ParseLine(text string) (Task, bool) {
	if len(text) < 4 || text[0] != '[' || text[2] != ']' || text[3] != ' ' {
		return Task{}, false
	}
	state := text[1]
	rawText := strings.TrimSpace(text[4:])
	body, dueDate, dueTime := SplitDue(rawText)
	switch state {
	case ' ':
		return Task{Text: body, RawText: rawText, Open: true, DueDate: dueDate, DueTime: dueTime}, true
	case 'x', 'X':
		return Task{Text: body, RawText: rawText, Open: false, DueDate: dueDate, DueTime: dueTime}, true
	default:
		return Task{}, false
	}
}

func SplitDue(text string) (string, string, time.Time) {
	matches := duePattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		return strings.TrimSpace(text), "", time.Time{}
	}
	dueDate := matches[1]
	dueTime, err := time.ParseInLocation("2006-01-02", dueDate, time.UTC)
	if err != nil {
		return strings.TrimSpace(text), "", time.Time{}
	}
	body := strings.TrimSpace(duePattern.ReplaceAllString(text, ""))
	return body, dueDate, dueTime
}

func AnnotateDue(task Task, today time.Time) Task {
	if task.DueDate == "" || task.DueTime.IsZero() {
		return task
	}
	taskDay := DateOnly(task.DueTime)
	today = DateOnly(today)
	switch {
	case taskDay.Before(today):
		task.DueStatus = "overdue"
	case taskDay.Equal(today):
		task.DueStatus = "today"
	default:
		task.DueStatus = "upcoming"
	}
	return task
}

func MatchesView(task Task, view string) bool {
	switch view {
	case "upcoming":
		return task.DueStatus == "today" || task.DueStatus == "upcoming"
	case "overdue":
		return task.DueStatus == "overdue"
	default:
		return true
	}
}

func NextDueDate(tasks []GroupTask) string {
	var dueDates []string
	for _, task := range tasks {
		if task.DueDate != "" {
			dueDates = append(dueDates, task.DueDate)
		}
	}
	sort.Strings(dueDates)
	if len(dueDates) == 0 {
		return ""
	}
	return dueDates[0]
}

func SortGroups(groups []Group, view string) {
	sort.Slice(groups, func(i, j int) bool {
		leftDue := groups[i].NextDue
		rightDue := groups[j].NextDue
		if view == "overdue" || view == "upcoming" {
			switch {
			case leftDue == "" && rightDue != "":
				return false
			case leftDue != "" && rightDue == "":
				return true
			case leftDue != rightDue:
				return leftDue < rightDue
			}
		}
		if groups[i].ModTime == groups[j].ModTime {
			return groups[i].ID < groups[j].ID
		}
		return groups[i].ModTime > groups[j].ModTime
	})
}

func FormatDueSuffix(task GroupTask) string {
	if task.DueDate == "" {
		return ""
	}
	switch task.DueStatus {
	case "overdue":
		return " (due " + task.DueDate + ", overdue)"
	case "today":
		return " (due " + task.DueDate + ", today)"
	default:
		return " (due " + task.DueDate + ")"
	}
}

func DateOnly(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
