package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Hunk is one conflicted block of a file, between git's markers:
//
//	<<<<<<< HEAD
//	mine
//	||||||| base        (with merge.conflictStyle diff3)
//	base
//	=======
//	theirs
//	>>>>>>> feature
type Hunk struct {
	// Start and End are the lines of its first and last marker, from 1.
	Start, End int
	// MineLabel and TheirsLabel are what the markers name ("HEAD",
	// "feature").
	MineLabel, TheirsLabel string
	Mine, Base, Theirs     []string
}

// Choice is what a conflicted block becomes.
type Choice int

const (
	Mine   Choice = iota // the lines of the branch checked out
	Theirs               // the lines of the one merged in
	Both                 // mine, then theirs
)

// Conflicts are the conflicted blocks of a working tree file.
func (r *Repo) Conflicts(path string) ([]Hunk, error) {
	lines, _, err := r.readLines(path)
	if err != nil {
		return nil, err
	}
	return parseHunks(lines), nil
}

// Markers are the lines of a working tree file, from 1, that open or close a
// conflict (<<<<<<< and >>>>>>>), a block half deleted too: what is left of
// its conflicts after the user edited it. ======= alone is not counted: a
// file may have it as a line of its own.
func (r *Repo) Markers(path string) ([]int, error) {
	lines, _, err := r.readLines(path)
	if err != nil {
		return nil, err
	}
	var at []int
	for i, l := range lines {
		_, open := marker(l, '<')
		_, end := marker(l, '>')
		if open || end {
			at = append(at, i+1)
		}
	}
	return at, nil
}

// Editor is the editor git would open, as a shell command: GIT_EDITOR,
// core.editor, VISUAL, EDITOR, then vi (git var GIT_EDITOR).
func (r *Repo) Editor() (string, error) {
	out, err := r.Run("var", "GIT_EDITOR")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// readLines reads a working tree file as lines, each keeping a \r of a CRLF;
// end reports a line break after the last line.
func (r *Repo) readLines(path string) (lines []string, end bool, err error) {
	b, err := os.ReadFile(filepath.Join(r.Dir, path))
	if err != nil {
		return nil, false, err
	}
	text := string(b)
	end = strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" && !end {
		return nil, false, nil
	}
	return strings.Split(text, "\n"), end, nil
}

func marker(line string, c byte) (string, bool) {
	line = strings.TrimSuffix(line, "\r")
	if len(line) < 7 || strings.Count(line[:7], string(c)) != 7 {
		return "", false
	}
	if len(line) > 7 && line[7] != ' ' {
		return "", false
	}
	return strings.TrimSpace(line[7:]), true
}

func parseHunks(lines []string) []Hunk {
	var hs []Hunk
	for i := 0; i < len(lines); i++ {
		label, ok := marker(lines[i], '<')
		if !ok {
			continue
		}
		h := Hunk{Start: i + 1, MineLabel: label}
		part := &h.Mine
		closed := false
		for j := i + 1; j < len(lines); j++ {
			if _, ok := marker(lines[j], '|'); ok && part == &h.Mine {
				part = &h.Base
				continue
			}
			if _, ok := marker(lines[j], '='); ok && part != &h.Theirs {
				part = &h.Theirs
				continue
			}
			if l, ok := marker(lines[j], '>'); ok && part == &h.Theirs {
				h.End, h.TheirsLabel, closed = j+1, l, true
				i = j
				break
			}
			*part = append(*part, lines[j])
		}
		if !closed {
			break
		}
		hs = append(hs, h)
	}
	return hs
}

// Resolve makes the n-th conflicted block of path (from 0) what choice says,
// the rest of the file as it was, and gives how many blocks are left.
func (r *Repo) Resolve(path string, n int, choice Choice) (int, error) {
	lines, end, err := r.readLines(path)
	if err != nil {
		return 0, err
	}
	hs := parseHunks(lines)
	if n < 0 || n >= len(hs) {
		return len(hs), fmt.Errorf("%s has no conflict %d", path, n+1)
	}
	h := hs[n]
	var with []string
	switch choice {
	case Mine:
		with = h.Mine
	case Theirs:
		with = h.Theirs
	case Both:
		with = append(append([]string(nil), h.Mine...), h.Theirs...)
	}
	out := append(append(append([]string(nil), lines[:h.Start-1]...), with...), lines[h.End:]...)
	text := strings.Join(out, "\n")
	if end && len(out) > 0 {
		text += "\n"
	}
	full := filepath.Join(r.Dir, path)
	st, err := os.Stat(full)
	if err != nil {
		return len(hs), err
	}
	if err := os.WriteFile(full, []byte(text), st.Mode().Perm()); err != nil {
		return len(hs), err
	}
	return len(hs) - 1, nil
}
