// Package git runs the git the user has installed, for gittu: it reads a
// repository (its refs, its history, its working tree) and runs the
// operations the screens offer. Every git runs in a session of its own, with no
// terminal to ask a password on (tdp P5: what outside programs to call is the
// app's logic).
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo is a git repository's working tree.
type Repo struct {
	// Dir is the top of the working tree.
	Dir string
	// GitDir is the repository's git directory (.git, or where a worktree's
	// is).
	GitDir string
}

// ErrNoGit is the error when there is no git to run.
var ErrNoGit = errors.New("git not found in PATH")

// Open finds the repository dir is in, as git does: dir or any directory
// above it.
func Open(dir string) (*Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, ErrNoGit
	}
	st, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", dir)
	}
	r := &Repo{Dir: dir}
	out, err := r.Run("rev-parse", "--show-toplevel", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("%s is not in a git repository", dir)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 || lines[0] == "" {
		// A bare repository has no working tree to show.
		return nil, fmt.Errorf("%s is not in a git working tree", dir)
	}
	r.Dir, r.GitDir = lines[0], lines[1]
	return r, nil
}

// Name is the name of the repository's directory.
func (r *Repo) Name() string { return filepath.Base(r.Dir) }

// Error is a git that did not end well: what it printed, line by line.
type Error struct {
	Args []string
	Code int
	// Lines are what git printed on stderr, or on stdout when stderr was
	// empty: its own words, to show as they are.
	Lines []string
}

func (e *Error) Error() string {
	if len(e.Lines) > 0 {
		return strings.Join(e.Lines, "\n")
	}
	return fmt.Sprintf("git %s exited with %d", strings.Join(e.Args, " "), e.Code)
}

// Run runs git with args in the working tree and gives what it printed.
func (r *Repo) Run(args ...string) (string, error) { return r.RunIn("", args...) }

// RunIn runs git with args, stdin given to it, and gives what it printed on
// stdout. It fails with an *Error when git does.
func (r *Repo) RunIn(stdin string, args ...string) (string, error) {
	out, _, err := r.run(stdin, args...)
	return out, err
}

// run runs git and gives its stdout and stderr.
func (r *Repo) run(stdin string, args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // no asking for a user name or password
		"GIT_MERGE_AUTOEDIT=no", // a merge takes its message as git writes it
		"GIT_OPTIONAL_LOCKS=0",  // reading the status does not take the index lock
	)
	detach(cmd)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		e := &Error{Args: args, Code: -1}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			e.Code = exit.ExitCode()
		}
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = strings.TrimSpace(stdout.String())
		}
		if text == "" && e.Code < 0 {
			text = err.Error()
		}
		e.Lines = Lines(text)
		return stdout.String(), stderr.String(), e
	}
	return stdout.String(), stderr.String(), nil
}

// Lines splits text into its lines, a \r before each \n dropped, with no empty
// line at the end.
func Lines(text string) []string {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// ok runs git for its exit status alone: 0 is true.
func (r *Repo) ok(args ...string) bool {
	_, _, err := r.run("", args...)
	return err == nil
}

// ValidBranch reports whether name can be a branch's name.
func (r *Repo) ValidBranch(name string) bool {
	return name != "" && r.ok("check-ref-format", "--branch", name)
}

// ValidTag reports whether name can be a tag's name.
func (r *Repo) ValidTag(name string) bool {
	return name != "" && !strings.HasPrefix(name, "-") && r.ok("check-ref-format", "refs/tags/"+name)
}

// Short is a commit hash cut to 7 characters, as git writes it.
func Short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
