package tasks

import (
	"strings"
	"testing"
	"time"
)

func TestToggleTaskLinePreservesIndentBulletAndDueDate(t *testing.T) {
	input := "# Project\n\n  * [ ] Ship release due: 2026-03-20\n"

	updated, task, err := ToggleTaskLine(input, 3)
	if err != nil {
		t.Fatal(err)
	}
	if task.Open {
		t.Fatalf("expected closed task after toggle: %+v", task)
	}
	if !strings.Contains(updated, "  * [x] Ship release due: 2026-03-20") {
		t.Fatalf("unexpected updated text: %q", updated)
	}
}

func TestAnnotateDueClassifiesRelativeDates(t *testing.T) {
	today := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)

	overdue := AnnotateDue(Task{DueDate: "2026-03-13", DueTime: time.Date(2026, 3, 13, 0, 0, 0, 0, time.UTC)}, today)
	current := AnnotateDue(Task{DueDate: "2026-03-14", DueTime: today}, today)
	upcoming := AnnotateDue(Task{DueDate: "2026-03-15", DueTime: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)}, today)

	if overdue.DueStatus != "overdue" || current.DueStatus != "today" || upcoming.DueStatus != "upcoming" {
		t.Fatalf("unexpected statuses: overdue=%q today=%q upcoming=%q", overdue.DueStatus, current.DueStatus, upcoming.DueStatus)
	}
}
