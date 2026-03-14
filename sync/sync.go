package sync

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	Upstream  string
	Committed bool
}

func Notebook(dir string, now func() time.Time) (Result, error) {
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, fmt.Errorf("%s is not a Git repository; initialize it with git and configure a remote first", dir)
		}
		return Result{}, err
	}

	if _, err := runGit(dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return Result{}, fmt.Errorf("%s is not a Git repository; initialize it with git and configure a remote first", dir)
	}

	upstream, err := runGit(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return Result{}, errors.New("notes Git remote is not configured; set an upstream branch for notes/ before syncing")
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return Result{}, errors.New("notes Git remote is not configured; set an upstream branch for notes/ before syncing")
	}

	if _, err := runGit(dir, "add", "--all", "."); err != nil {
		return Result{}, err
	}

	committed := false
	if hasChanges, err := gitHasStagedChanges(dir); err != nil {
		return Result{}, err
	} else if hasChanges {
		message := fmt.Sprintf("sync notebook %s", now().UTC().Format(time.RFC3339))
		if _, err := runGit(dir, "commit", "-m", message); err != nil {
			return Result{}, err
		}
		committed = true
	}

	if _, err := runGit(dir, "pull", "--rebase"); err != nil {
		return Result{}, err
	}
	if _, err := runGit(dir, "push"); err != nil {
		return Result{}, err
	}

	return Result{Upstream: upstream, Committed: committed}, nil
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
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

func gitHasStagedChanges(dir string) (bool, error) {
	cmd := exec.Command("git", "-C", dir, "diff", "--cached", "--quiet", "--exit-code")
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("git diff --cached --quiet --exit-code: %v", err)
	}
	return false, nil
}
