package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Change is a file one commit changed against another.
type Change struct {
	// Status is git's letter: M modified, A added, D deleted, R renamed,
	// C copied, T type changed.
	Status byte
	Path   string
	// Orig is the path it had before, for a rename or a copy.
	Orig string
	// Binary reports a file git sees as binary: it has no lines to compare.
	Binary bool
}

// Changes are the files that differ from commit from to commit to; from ""
// is the empty tree, for a first commit.
func (r *Repo) Changes(from, to string) ([]Change, error) {
	run := func(format string) (string, error) {
		if from == "" {
			return r.Run("diff-tree", "-r", "-M", "--root", "--no-commit-id", format, "-z", to)
		}
		return r.Run("diff", "--no-ext-diff", "-M", format, "-z", from, to, "--")
	}
	out, err := run("--name-status")
	if err != nil {
		return nil, err
	}
	cs := parseNameStatus(out)
	if out, err = run("--numstat"); err != nil {
		return nil, err
	}
	binary := parseBinary(out)
	for i := range cs {
		cs[i].Binary = binary[cs[i].Path]
	}
	return cs, nil
}

// parseBinary reads git's --numstat -z: the paths whose counts are "-", the
// binary files.
func parseBinary(out string) map[string]bool {
	bin := map[string]bool{}
	f := strings.Split(out, "\x00")
	for i := 0; i < len(f); i++ {
		parts := strings.SplitN(f[i], "\t", 3)
		if len(parts) < 3 {
			continue
		}
		path := parts[2]
		if path == "" && i+2 < len(f) { // a rename: the old and the new path follow
			path = f[i+2]
			i += 2
		}
		if parts[0] == "-" && parts[1] == "-" {
			bin[path] = true
		}
	}
	return bin
}

// Show is a file's text at rev: a commit, or "" for the index; "" with no
// error when the file is not there.
func (r *Repo) Show(rev, path string) (string, error) {
	out, err := r.Run("show", rev+":"+path)
	var e *Error
	if errors.As(err, &e) && e.Code == 128 {
		return "", nil // not in that tree
	}
	return out, err
}

// WorkFile is a working tree file's text; "" when it is not there.
func (r *Repo) WorkFile(path string) (string, error) {
	b, err := os.ReadFile(filepath.Join(r.Dir, path))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

func parseNameStatus(out string) []Change {
	var cs []Change
	f := strings.Split(out, "\x00")
	for i := 0; i < len(f); i++ {
		if f[i] == "" {
			continue
		}
		c := Change{Status: f[i][0]}
		if c.Status == 'R' || c.Status == 'C' {
			if i+2 >= len(f) {
				break
			}
			c.Orig, c.Path = f[i+1], f[i+2]
			i += 2
		} else {
			if i+1 >= len(f) {
				break
			}
			c.Path = f[i+1]
			i++
		}
		cs = append(cs, c)
	}
	return cs
}

// WorkDiff is the patch of a working tree file: its staged change when staged,
// otherwise its change in the working tree; an untracked file whole, as added.
func (r *Repo) WorkDiff(f File, staged bool) ([]string, error) {
	switch {
	case f.Untracked():
		if st, err := os.Stat(filepath.Join(r.Dir, f.Path)); err == nil && st.IsDir() {
			return r.untrackedDir(f.Path)
		}
		out, err := r.Run("diff", "--no-color", "--no-ext-diff", "--no-index", "--", os.DevNull, f.Path)
		var e *Error
		if errors.As(err, &e) && e.Code == 1 {
			err = nil // --no-index exits 1 when the two differ, which they do
		}
		if err != nil {
			return nil, err
		}
		return Lines(out), nil
	case staged:
		out, err := r.Run("diff", "--cached", "--no-color", "--no-ext-diff", "-M", "--", f.Path)
		return Lines(out), err
	default:
		out, err := r.Run("diff", "--no-color", "--no-ext-diff", "--", f.Path)
		return Lines(out), err
	}
}

// untrackedDir lists what an untracked directory holds, as a diff would only
// say it is new.
func (r *Repo) untrackedDir(dir string) ([]string, error) {
	out, err := r.Run("ls-files", "--others", "--exclude-standard", "--", dir)
	if err != nil {
		return nil, err
	}
	lines := []string{"Untracked directory " + dir + ":"}
	for _, l := range Lines(out) {
		lines = append(lines, "+ "+l)
	}
	return lines, nil
}
