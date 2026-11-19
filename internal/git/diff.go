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
}

// Changes are the files that differ from commit from to commit to; from ""
// is the empty tree, for a first commit.
func (r *Repo) Changes(from, to string) ([]Change, error) {
	var out string
	var err error
	if from == "" {
		out, err = r.Run("diff-tree", "-r", "-M", "--root", "--no-commit-id", "--name-status", "-z", to)
	} else {
		out, err = r.Run("diff", "--no-ext-diff", "-M", "--name-status", "-z", from, to, "--")
	}
	if err != nil {
		return nil, err
	}
	return parseNameStatus(out), nil
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

// Diff is the patch of one file from commit from to commit to (from "" is the
// empty tree), as git diff prints it.
func (r *Repo) Diff(from, to string, c Change) ([]string, error) {
	paths := []string{c.Path}
	if c.Orig != "" {
		paths = append(paths, c.Orig)
	}
	var out string
	var err error
	if from == "" {
		out, err = r.Run(append([]string{"diff-tree", "-p", "-M", "--root", "--no-commit-id", "--no-color", "--no-ext-diff", to, "--"}, paths...)...)
	} else {
		out, err = r.Run(append([]string{"diff", "--no-color", "--no-ext-diff", "-M", from, to, "--"}, paths...)...)
	}
	if err != nil {
		return nil, err
	}
	return Lines(out), nil
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

// StashDiff is a stash entry's patch.
func (r *Repo) StashDiff(ref string) ([]string, error) {
	out, err := r.Run("stash", "show", "-p", "--no-color", "--no-ext-diff", ref)
	return Lines(out), err
}
