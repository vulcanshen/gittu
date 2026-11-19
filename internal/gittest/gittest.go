// Package gittest makes git repositories for gitreu's tests, with git set up
// apart from the user's own (no ~/.gitconfig, a fixed author and dates).
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Setenv makes every git the test runs leave the user's settings out and
// write a fixed author.
func Setenv(t testing.TB) {
	t.Helper()
	for k, v := range map[string]string{
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME":     "Ada",
		"GIT_AUTHOR_EMAIL":    "ada@example.com",
		"GIT_COMMITTER_NAME":  "Ada",
		"GIT_COMMITTER_EMAIL": "ada@example.com",
		"GIT_AUTHOR_DATE":     "2026-11-19T10:00:00+08:00",
		"GIT_COMMITTER_DATE":  "2026-11-19T10:00:00+08:00",
	} {
		t.Setenv(k, v)
	}
}

// Repo is a repository a test makes.
type Repo struct {
	t   testing.TB
	Dir string
}

// New makes an empty repository on main.
func New(t testing.TB) *Repo {
	t.Helper()
	Setenv(t)
	r := &Repo{t: t, Dir: t.TempDir()}
	r.Git("init", "-q", "-b", "main")
	r.Git("config", "commit.gpgsign", "false")
	return r
}

// Git runs git in the repository and gives what it printed; a failure fails
// the test.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	out, err := r.Try(args...)
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// Try runs git and gives what it printed and its error.
func (r *Repo) Try(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Write writes a file of the working tree.
func (r *Repo) Write(path, text string) {
	r.t.Helper()
	full := filepath.Join(r.Dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// Read reads a file of the working tree.
func (r *Repo) Read(path string) string {
	r.t.Helper()
	b, err := os.ReadFile(filepath.Join(r.Dir, path))
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

// Commit writes path with text, stages everything and commits it with
// subject.
func (r *Repo) Commit(path, text, subject string) string {
	r.t.Helper()
	r.Write(path, text)
	r.Git("add", "-A")
	r.Git("commit", "-q", "-m", subject)
	return strings.TrimSpace(r.Git("rev-parse", "HEAD"))
}

// Sample is a repository with some history: main with three commits, a
// feature branch off its first commit with two, merged back into main, a
// topic branch not merged, a tag, a stash, and a remote origin with main
// pushed.
func Sample(t testing.TB) *Repo {
	t.Helper()
	r := New(t)
	r.Commit("README.md", "# demo\n", "Start the demo")
	r.Git("checkout", "-q", "-b", "feature")
	r.Commit("feature.txt", "one\n", "Add the feature")
	r.Commit("feature.txt", "one\ntwo\n", "Grow the feature")
	r.Git("checkout", "-q", "main")
	r.Commit("README.md", "# demo\n\nMore.\n", "Say more")
	r.Git("merge", "-q", "--no-edit", "feature")
	r.Git("tag", "v0.1.0")
	r.Git("checkout", "-q", "-b", "topic")
	r.Commit("topic.txt", "topic\n", "Try a topic")
	r.Git("checkout", "-q", "main")
	r.Commit("README.md", "# demo\n\nMore.\nAnd more.\n", "Say even more")
	remote := t.TempDir()
	cmd := exec.Command("git", "init", "-q", "--bare", remote)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	r.Git("remote", "add", "origin", remote)
	r.Git("push", "-q", "-u", "origin", "main")
	r.Write("README.md", "# demo\n\nMore.\nAnd more.\nStashed.\n")
	r.Git("stash", "push", "-q", "-m", "Half a thought")
	return r
}

// Conflicted is a repository stopped in a merge of feature into main with
// both changing two lines of notes.txt far apart: two conflicts.
func Conflicted(t testing.TB) *Repo {
	t.Helper()
	r := New(t)
	r.Commit("notes.txt", "a\nb\nc\nd\ne\nx1\nx2\nx3\nx4\nf\ng\n", "Start the notes")
	r.Git("checkout", "-q", "-b", "feature")
	r.Commit("notes.txt", "a\nB from feature\nc\nd\ne\nx1\nx2\nx3\nx4\nF from feature\ng\n", "Change b and f on feature")
	r.Git("checkout", "-q", "main")
	r.Commit("notes.txt", "a\nB from main\nc\nd\ne\nx1\nx2\nx3\nx4\nF from main\ng\n", "Change b and f on main")
	if out, err := r.Try("merge", "--no-edit", "feature"); err == nil {
		t.Fatalf("the merge has no conflict:\n%s", out)
	}
	return r
}
