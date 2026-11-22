package git

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// IsDir reports whether path in the working tree is a directory: git says an
// untracked folder is new without its files.
func (r *Repo) IsDir(path string) bool {
	st, err := os.Stat(filepath.Join(r.Dir, path))
	return err == nil && st.IsDir()
}

// UntrackedFiles are the files an untracked directory holds.
func (r *Repo) UntrackedFiles(dir string) ([]string, error) {
	out, err := r.Run("ls-files", "--others", "--exclude-standard", "--", dir)
	if err != nil {
		return nil, err
	}
	return Lines(out), nil
}

// FileCommit is a commit that changed a file; Binary reports that git sees
// the file there as binary, with no lines to compare.
type FileCommit struct {
	Commit
	Binary bool
}

// FileLog is the commits from HEAD that changed path, the newest first, Limit
// at the most. Renames are not followed: the history is of the path.
func (r *Repo) FileLog(path string) ([]FileCommit, error) {
	out, err := r.Run("log", "-n", strconv.Itoa(Limit), "--numstat",
		"--format=%x1e%H%x1f%P%x1f%an%x1f%at%x1f%s", "HEAD", "--", path)
	if err != nil {
		return nil, err
	}
	var cs []FileCommit
	for _, rec := range strings.Split(out, "\x1e") {
		head, rest, _ := strings.Cut(rec, "\n")
		f := strings.Split(head, "\x1f")
		if len(f) < 5 {
			continue
		}
		at, _ := strconv.ParseInt(f[3], 10, 64)
		c := FileCommit{Commit: Commit{SHA: f[0], Author: f[2], Date: time.Unix(at, 0), Subject: f[4]}}
		if f[1] != "" {
			c.Parents = strings.Fields(f[1])
		}
		for _, l := range strings.Split(rest, "\n") {
			if strings.HasPrefix(l, "-\t-\t") { // numstat writes - - for a binary file
				c.Binary = true
			}
		}
		cs = append(cs, c)
	}
	return cs, nil
}
